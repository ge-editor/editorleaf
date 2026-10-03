package syntax_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/editorleaf/highlight"
	"github.com/ge-editor/editorleaf/syntax"
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

func TestAttachUnknownLanguage(t *testing.T) {
	eb := newTestBuffer(t, "x")
	detach, ok := syntax.Attach(eb, "no-such-language-xyz", &highlight.HighlightsLayer{}, highlight.LayerSyntax, nil)
	if ok || detach != nil {
		t.Fatalf("Attach for an unregistered language should fail cleanly, got ok=%v detach=%v", ok, detach)
	}
}

func TestWorkerParsesEditsAndPublishes(t *testing.T) {
	const lang = "fake-test-lang-1"
	fp := &fakeParser{}
	syntax.Register(lang, func() syntax.Parser { return fp })

	eb := newTestBuffer(t, "abc")
	target := &highlight.HighlightsLayer{}
	redrawn := make(chan struct{}, 16)
	detach, ok := syntax.Attach(eb, lang, target, highlight.LayerSyntax,
		func(apply func()) { apply(); redrawn <- struct{}{} })
	if !ok {
		t.Fatal("Attach failed for a registered language")
	}
	defer detach()

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

	detach()
	if _, _, closed := fp.snapshot(); !closed {
		t.Fatal("detach did not Close the parser")
	}
}

func TestWorkerDesyncClearsLayerAndStopsApplying(t *testing.T) {
	const lang = "fake-test-lang-2"
	fp := &fakeParser{}
	syntax.Register(lang, func() syntax.Parser { return fp })

	eb := newTestBuffer(t, "abc")
	target := &highlight.HighlightsLayer{}
	detach, ok := syntax.Attach(eb, lang, target, highlight.LayerSyntax, nil)
	if !ok {
		t.Fatal("Attach failed")
	}
	defer detach()

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
