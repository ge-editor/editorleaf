package syntax_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/editorleaf/highlight"
	"github.com/ge-editor/editorleaf/syntax"
	"github.com/ge-editor/gecore/lang"
)

// fakeParser is a trivial Parser used to test Worker without depending on
// any real grammar: it just remembers the current source length and
// reports one span covering the whole buffer, so tests can check that
// Parse/Edit/Spans/Close are called in the right order and that the
// resulting span reaches the HighlightsLayer.
type fakeParser struct {
	mu      sync.Mutex
	src     []byte
	parses  int
	edits   int
	closed  bool
	lastCtx context.Context
}

func (p *fakeParser) Parse(ctx context.Context, src []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.src = append([]byte(nil), src...)
	p.parses++
	p.lastCtx = ctx
}

func (p *fakeParser) Edit(ctx context.Context, _ syntax.Edit, src []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.src = append([]byte(nil), src...)
	p.edits++
	p.lastCtx = ctx
}

func (p *fakeParser) Spans() []highlight.Span {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.src) == 0 {
		return nil
	}
	return []highlight.Span{{
		Start: editbuffer.RowsPos{RowIndex: 0, ColIndex: 0},
		End:   editbuffer.RowsPos{RowIndex: 0, ColIndex: len(p.src)},
	}}
}

func (p *fakeParser) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
}

func (p *fakeParser) snapshot() (parses, edits int, closed bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.parses, p.edits, p.closed
}

func newTestBuffer(t *testing.T, lines ...string) *editbuffer.EditBuffer {
	t.Helper()
	rs := make(editbuffer.Rows_, 0, len(lines))
	for _, l := range lines {
		rs = append(rs, editbuffer.Row_(l))
	}
	return editbuffer.NewEditBuffer(&rs)
}

// waitFor polls cond until it is true or t seconds pass, failing the test
// on timeout. Worker does its work on a background goroutine, so tests
// observe its effects asynchronously.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

type fakeMode struct {
	lang.Mode
	name string
}

func (m fakeMode) Name() string { return m.name }

func setLang(eb *editbuffer.EditBuffer, name string) {
	var m lang.Mode = fakeMode{name: name}
	eb.SetLangMode(&m)
}

func spanEnd(h *highlight.HighlightsLayer) int {
	got := h.GetSpanTemplate(highlight.LayerSyntax, 0)
	if got == nil {
		return -1
	}
	return (*got).GetSpan().End.ColIndex
}

func TestSubscribeUnknownLanguage(t *testing.T) {
	eb := newTestBuffer(t, "x")
	setLang(eb, "no-such-language-xyz")
	syntax.Subscribe(eb, &highlight.HighlightsLayer{}, highlight.LayerSyntax, nil)
	syntax.Release(eb)
}

func TestWorkerParsesEditsAndPublishes(t *testing.T) {
	const lang = "fake-test-lang-1"
	fp := &fakeParser{}
	syntax.Register(lang, func() syntax.Parser { return fp })

	eb := newTestBuffer(t, "abc")
	setLang(eb, lang)
	target := &highlight.HighlightsLayer{}
	redrawn := make(chan struct{}, 16)
	syntax.Subscribe(eb, target, highlight.LayerSyntax,
		func(apply func()) { apply(); redrawn <- struct{}{} })
	defer syntax.Release(eb)

	// Initial priming: one Parse call, one published span covering "abc".
	waitFor(t, func() bool { p, _, _ := fp.snapshot(); return p == 1 })
	waitFor(t, func() bool { return target.SpanLength(highlight.LayerSyntax) == 1 })
	if got := target.GetSpanTemplate(highlight.LayerSyntax, 0); got == nil || (*got).GetSpan().End.ColIndex != 3 {
		t.Fatalf("unexpected initial span: %+v", got)
	}

	// An edit reaches the parser as Edit, not another Parse.
	eb.InsertRegion(0, 3, editbuffer.Rows_{editbuffer.Row_("de")})
	waitFor(t, func() bool { _, e, _ := fp.snapshot(); return e == 1 })
	if p, _, _ := fp.snapshot(); p != 1 {
		t.Fatalf("Parse should not be called again for an ordinary edit, called %d times", p)
	}
	waitFor(t, func() bool {
		got := target.GetSpanTemplate(highlight.LayerSyntax, 0)
		return got != nil && (*got).GetSpan().End.ColIndex == 5 // "abcde"
	})

	// A reset (e.g. reload) triggers a fresh Parse.
	eb.SetBytesArray([][]byte{[]byte("z")})
	waitFor(t, func() bool { p, _, _ := fp.snapshot(); return p == 2 })

	select {
	case <-redrawn:
	default:
		t.Fatal("redraw callback was never invoked")
	}

	syntax.Release(eb)
	if _, _, closed := fp.snapshot(); !closed {
		t.Fatal("Release did not Close the parser")
	}
}

func TestWorkerDesyncClearsLayerAndStopsApplying(t *testing.T) {
	const lang = "fake-test-lang-2"
	fp := &fakeParser{}
	syntax.Register(lang, func() syntax.Parser { return fp })

	eb := newTestBuffer(t, "abc")
	setLang(eb, lang)
	target := &highlight.HighlightsLayer{}
	syntax.Subscribe(eb, target, highlight.LayerSyntax, nil)
	defer syntax.Release(eb)

	waitFor(t, func() bool { p, _, _ := fp.snapshot(); return p == 1 })

	// Simulate code that mutates the buffer without going through
	// EditBuffer's notifying methods -- exactly the hazard documented in
	// editbuffer/change.go ("callers must use these EditBuffer methods,
	// not eb.Rows.InsertRegion(...) directly"). This call bypasses
	// notification, so the worker's LineIndex still thinks row 0 is 3
	// bytes ("abc") once it is done draining the changes above.
	eb.InsertRegion(0, 3, editbuffer.Rows_{editbuffer.Row_("XYZ")}) // real row 0 is now "abcXYZ" (6 bytes)

	// This edit is valid against the real (6-byte) row but not against
	// what the worker's stale LineIndex believes (3 bytes), so
	// LineIndex.Apply must reject it.
	eb.InsertRegion(0, 5, editbuffer.Rows_{editbuffer.Row_("!")})

	waitFor(t, func() bool { return target.SpanLength(highlight.LayerSyntax) == 0 })

	// Further edits are ignored until a Reset arrives.
	eb.InsertRegion(0, 0, editbuffer.Rows_{editbuffer.Row_("q")})
	time.Sleep(20 * time.Millisecond)
	if _, edits, _ := fp.snapshot(); edits != 0 {
		t.Fatalf("parser.Edit should not be called while desynced, called %d times", edits)
	}

	eb.SetBytesArray([][]byte{[]byte("fresh")})
	waitFor(t, func() bool { p, _, _ := fp.snapshot(); return p == 2 })
	waitFor(t, func() bool { return target.SpanLength(highlight.LayerSyntax) == 1 })
}

// Panes only subscribe/unsubscribe: the Worker (parser + tree) belongs to
// the buffer and survives until Release.
func TestSubscribeSharesOneWorkerPerBuffer(t *testing.T) {
	const lang = "fake-test-lang-3"
	var mu sync.Mutex
	var parsers []*fakeParser
	syntax.Register(lang, func() syntax.Parser {
		mu.Lock()
		defer mu.Unlock()
		p := &fakeParser{}
		parsers = append(parsers, p)
		return p
	})
	parser := func(i int) *fakeParser {
		mu.Lock()
		defer mu.Unlock()
		if i >= len(parsers) {
			return nil
		}
		return parsers[i]
	}

	eb := newTestBuffer(t, "abc")
	setLang(eb, lang)
	a, b := &highlight.HighlightsLayer{}, &highlight.HighlightsLayer{}

	unsubA := syntax.Subscribe(eb, a, highlight.LayerSyntax, nil)
	defer syntax.Release(eb)
	waitFor(t, func() bool { return a.SpanLength(highlight.LayerSyntax) == 1 })

	// A second pane on the same buffer gets the current spans right away
	// and does not create another parser.
	unsubB := syntax.Subscribe(eb, b, highlight.LayerSyntax, nil)
	if got := b.SpanLength(highlight.LayerSyntax); got != 1 {
		t.Fatalf("new subscriber should receive the latest spans immediately, got %d spans", got)
	}
	if parser(1) != nil {
		t.Fatal("second Subscribe created a second parser")
	}

	// Closing pane A must neither stop the Worker nor reparse.
	unsubA()
	if _, _, closed := parser(0).snapshot(); closed {
		t.Fatal("unsubscribe stopped the Worker")
	}
	eb.InsertRegion(0, 3, editbuffer.Rows_{editbuffer.Row_("de")})
	waitFor(t, func() bool {
		got := b.GetSpanTemplate(highlight.LayerSyntax, 0)
		return got != nil && (*got).GetSpan().End.ColIndex == 5
	})
	if got := a.GetSpanTemplate(highlight.LayerSyntax, 0); got == nil || (*got).GetSpan().End.ColIndex != 3 {
		t.Fatal("an unsubscribed layer must not be updated any more")
	}
	if p, _, _ := parser(0).snapshot(); p != 1 {
		t.Fatalf("Parse should have run once for the buffer, ran %d times", p)
	}

	// Pane B closes too: still alive (the buffer is still open). A later
	// pane for the same buffer reuses it.
	unsubB()
	c := &highlight.HighlightsLayer{}
	unsubC := syntax.Subscribe(eb, c, highlight.LayerSyntax, nil)
	defer unsubC()
	if parser(1) != nil {
		t.Fatal("re-subscribing after all panes closed created a new parser")
	}
	if got := c.SpanLength(highlight.LayerSyntax); got != 1 {
		t.Fatalf("expected the retained spans, got %d", got)
	}

	// Release (kill-buffer) stops the Worker; a later Subscribe starts a new one.
	syntax.Release(eb)
	if _, _, closed := parser(0).snapshot(); !closed {
		t.Fatal("Release did not Close the parser")
	}
	d := &highlight.HighlightsLayer{}
	setLang(eb, lang)
	syntax.Subscribe(eb, d, highlight.LayerSyntax, nil)
	defer syntax.Release(eb)
	waitFor(t, func() bool { return parser(1) != nil })
}

// Meta.DeepCopy copies the HighlightLayer pointer, so two panes can register
// the very same layer. It must be written once and stay registered until
// both have unsubscribed.
func TestSubscribeSharedLayerIsRefCounted(t *testing.T) {
	const lang = "fake-test-lang-4"
	fp := &fakeParser{}
	syntax.Register(lang, func() syntax.Parser { return fp })

	eb := newTestBuffer(t, "abc")
	setLang(eb, lang)
	layer := &highlight.HighlightsLayer{}
	unsub1 := syntax.Subscribe(eb, layer, highlight.LayerSyntax, nil)
	unsub2 := syntax.Subscribe(eb, layer, highlight.LayerSyntax, nil)
	defer syntax.Release(eb)
	waitFor(t, func() bool { return layer.SpanLength(highlight.LayerSyntax) == 1 })

	unsub1()
	unsub1() // idempotent: must not drop the second pane's reference
	eb.InsertRegion(0, 3, editbuffer.Rows_{editbuffer.Row_("de")})
	waitFor(t, func() bool {
		got := layer.GetSpanTemplate(highlight.LayerSyntax, 0)
		return got != nil && (*got).GetSpan().End.ColIndex == 5
	})
	if n := layer.SpanLength(highlight.LayerSyntax); n != 1 {
		t.Fatalf("spans were written more than once into the shared layer: %d", n)
	}
	unsub2()
}

func TestSetLangModeStartsWorkerForExistingSubscription(t *testing.T) {
	const langName = "fake-test-lang-5"
	fp := &fakeParser{}
	syntax.Register(langName, func() syntax.Parser { return fp })

	eb := newTestBuffer(t, "abc")
	setLang(eb, "no-such-language-xyz")
	layer := &highlight.HighlightsLayer{}
	syntax.Subscribe(eb, layer, highlight.LayerSyntax, nil)
	defer syntax.Release(eb)

	time.Sleep(10 * time.Millisecond)
	if n := layer.SpanLength(highlight.LayerSyntax); n != 0 {
		t.Fatalf("no parser is registered, but %d spans were published", n)
	}

	setLang(eb, langName)
	waitFor(t, func() bool { return spanEnd(layer) == 3 })
}

func TestSetLangModeReplacesWorkerAndKeepsSinks(t *testing.T) {
	const langA, langB = "fake-test-lang-6a", "fake-test-lang-6b"
	pa, pb := &fakeParser{}, &fakeParser{}
	syntax.Register(langA, func() syntax.Parser { return pa })
	syntax.Register(langB, func() syntax.Parser { return pb })

	eb := newTestBuffer(t, "abc")
	setLang(eb, langA)
	layer := &highlight.HighlightsLayer{}
	syntax.Subscribe(eb, layer, highlight.LayerSyntax, nil)
	defer syntax.Release(eb)
	waitFor(t, func() bool { return spanEnd(layer) == 3 })

	setLang(eb, langB)
	if _, _, closed := pa.snapshot(); !closed {
		t.Fatal("SetLangMode did not stop the previous language's parser")
	}
	waitFor(t, func() bool { p, _, _ := pb.snapshot(); return p == 1 })
	waitFor(t, func() bool { return spanEnd(layer) == 3 })

	eb.InsertRegion(0, 3, editbuffer.Rows_{editbuffer.Row_("de")})
	waitFor(t, func() bool { _, e, _ := pb.snapshot(); return e == 1 })
	waitFor(t, func() bool { return spanEnd(layer) == 5 })
	if _, e, _ := pa.snapshot(); e != 0 {
		t.Fatalf("the old parser received %d edits after SetLangMode", e)
	}

	setLang(eb, "no-such-language-xyz")
	if _, _, closed := pb.snapshot(); !closed {
		t.Fatal("SetLangMode did not stop the parser")
	}
	if n := layer.SpanLength(highlight.LayerSyntax); n != 0 {
		t.Fatalf("stale spans remained after switching to a mode without a Parser: %d", n)
	}

	setLang(eb, langB)
	waitFor(t, func() bool { p, _, _ := pb.snapshot(); return p >= 2 })
}
