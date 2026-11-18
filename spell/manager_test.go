package spell

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/editorleaf/highlight"
)

// fakeEngine flags the words "teh" and "recieve".
type fakeEngine struct {
	added, ignored []string
	closed         bool
	loadedMode     string // the mode LoadMode was asked for
}

// LoadMode: the Go mode has a dictionary and a broken one.
func (f *fakeEngine) LoadMode(mode string) (loaded, warnings []string) {
	f.loadedMode = mode
	if mode == "Go" {
		return []string{"go"}, []string{"extra: cannot open"}
	}
	return nil, nil
}

func (f *fakeEngine) Misspelled(text []byte) []Range {
	var out []Range
	for _, bad := range []string{"teh", "recieve"} {
		for from := 0; ; {
			i := strings.Index(string(text[from:]), bad)
			if i < 0 {
				break
			}
			out = append(out, Range{from + i, from + i + len(bad)})
			from += i + len(bad)
		}
	}
	slices.SortFunc(out, func(a, b Range) int { return a.Start - b.Start })
	return out
}

// MisspelledCode flags "recieve" only: the code of the tests.
func (f *fakeEngine) MisspelledCode(text []byte) []Range {
	var out []Range
	for from := 0; ; {
		i := strings.Index(string(text[from:]), "recieve")
		if i < 0 {
			return out
		}
		out = append(out, Range{from + i, from + i + len("recieve")})
		from += i + len("recieve")
	}
}

func (f *fakeEngine) Suggest(word string, n int) []string { return []string{"the"} }
func (f *fakeEngine) Add(word string) error { f.added = append(f.added, word); return nil }
func (f *fakeEngine) Ignore(word string) { f.ignored = append(f.ignored, word) }
func (f *fakeEngine) Close() { f.closed = true }

// harness plays the UI goroutine: the functions that Dispatch gets are run by
// pump on the test goroutine.
type harness struct {
	t     *testing.T
	eb    *editbuffer.EditBuffer
	layer *highlight.HighlightsLayer
	eng   *fakeEngine
	ch    chan func()
}

func newHarness(t *testing.T, text string) *harness {
	t.Helper()
	eng := &fakeEngine{}
	Register(func(string) (Engine, error) { return eng, nil })
	t.Cleanup(func() { Register(nil) })

	eb := editbuffer.NewEditBuffer(nil)
	if err := eb.LoadFrom(strings.NewReader(text)); err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, eb: eb, layer: highlight.NewHighlightsLayer(), eng: eng, ch: make(chan func(), 64)}
	t.Cleanup(func() { Release(eb) })
	return h
}

func (h *harness) enable(mode string) { h.enableWith(mode, nil) }

func (h *harness) enableWith(mode string, scope Scope) {
	h.t.Helper()
	unsub := Subscribe(h.eb, h.layer, highlight.LayerSpell)
	h.t.Cleanup(unsub)
	err := Enable(h.eb, mode, "", Options{
		Dispatch: func(f func()) { h.ch <- f },
		Delay:    time.Millisecond,
		Scope:    scope,
	})
	if err != nil {
		h.t.Fatal(err)
	}
}

// pump runs what was dispatched until nothing comes for a while.
func (h *harness) pump() {
	h.t.Helper()
	for {
		select {
		case f := <-h.ch:
			f()
		case <-time.After(100 * time.Millisecond):
			return
		}
	}
}

func (h *harness) words() []wordPos {
	hl := h.layer.Peek(highlight.LayerSpell)
	var out []wordPos
	for i, n := 0, hl.SpansLength(); i < n; i++ {
		sp := hl.GetSpan(i)
		out = append(out, wordPos{sp.Start.RowIndex, sp.Start.ColIndex, sp.End.ColIndex})
	}
	return out
}

func TestEnableMarksMisspelledWords(t *testing.T) {
	h := newHarness(t, "teh cat\nrecieve it, teh\nfine\n")
	h.enable("Fundamental")
	h.pump()

	want := []wordPos{{0, 0, 3}, {1, 0, 7}, {1, 12, 15}}
	if got := h.words(); !slices.Equal(got, want) {
		t.Fatalf("words = %v, want %v", got, want)
	}
}

func TestEditsMoveAndRecheck(t *testing.T) {
	h := newHarness(t, "teh cat\nrecieve it\n")
	h.enable("Fundamental")
	h.pump()

	// A newline at the start: the row that is split is checked again, the
	// words of the rows below move down at once, before any check.
	h.eb.InsertRegion(0, 0, editbuffer.Rows_{editbuffer.Row_(""), editbuffer.Row_("")})
	if got, want := h.words(), []wordPos{{2, 0, 7}}; !slices.Equal(got, want) {
		t.Fatalf("after the newline: %v, want %v", got, want)
	}
	h.pump()
	if got, want := h.words(), []wordPos{{1, 0, 3}, {2, 0, 7}}; !slices.Equal(got, want) {
		t.Fatalf("after the check: %v, want %v", got, want)
	}

	// Typing in a row drops its words at once (the columns are not valid
	// any more); the check finds the new ones.
	h.eb.InsertRegion(1, 0, editbuffer.Rows_{editbuffer.Row_("xx ")})
	if got, want := h.words(), []wordPos{{2, 0, 7}}; !slices.Equal(got, want) {
		t.Fatalf("after typing: %v, want %v", got, want)
	}
	h.pump()
	if got, want := h.words(), []wordPos{{1, 3, 6}, {2, 0, 7}}; !slices.Equal(got, want) {
		t.Fatalf("after the check: %v, want %v", got, want)
	}

	// Removing rows moves the words up.
	h.eb.RemoveRegion(editbuffer.RowsPos{RowIndex: 0, ColIndex: 0}, editbuffer.RowsPos{RowIndex: 1, ColIndex: 0})
	h.pump()
	if got, want := h.words(), []wordPos{{0, 3, 6}, {1, 0, 7}}; !slices.Equal(got, want) {
		t.Fatalf("after removing a row: %v, want %v", got, want)
	}
}

func TestMarkdownCodeBlockIsNotChecked(t *testing.T) {
	h := newHarness(t, "teh text\n```\nteh code\n```\n")
	h.enable("Markdown")
	h.pump()
	if got, want := h.words(), []wordPos{{0, 0, 3}}; !slices.Equal(got, want) {
		t.Fatalf("words = %v, want %v", got, want)
	}

	// Removing the line that opens the block makes the code prose.
	h.eb.RemoveRegion(editbuffer.RowsPos{RowIndex: 1, ColIndex: 0}, editbuffer.RowsPos{RowIndex: 1, ColIndex: 3})
	h.pump()
	// (the closing fence now opens a block of its own, after the code)
	if got, want := h.words(), []wordPos{{0, 0, 3}, {2, 0, 3}}; !slices.Equal(got, want) {
		t.Fatalf("after the fence was removed: %v, want %v", got, want)
	}
}

func TestDisableClearsAndClosesTheEngine(t *testing.T) {
	h := newHarness(t, "teh\n")
	h.enable("Fundamental")
	h.pump()
	if len(h.words()) != 1 || !Enabled(h.eb) {
		t.Fatal("not enabled")
	}
	Disable(h.eb)
	if len(h.words()) != 0 || Enabled(h.eb) || !h.eng.closed {
		t.Fatalf("words %v, enabled %v, closed %v", h.words(), Enabled(h.eb), h.eng.closed)
	}
	// A timer that was still running does nothing.
	h.pump()
}

func TestWordAtAddAndIgnore(t *testing.T) {
	h := newHarness(t, "say teh now\n")
	h.enable("Fundamental")
	h.pump()

	for _, col := range []int{4, 6, 7} { // inside, and right after the word
		w, r, ok := WordAt(h.eb, editbuffer.RowsPos{RowIndex: 0, ColIndex: col})
		if !ok || w != "teh" || r != (Range{4, 7}) {
			t.Errorf("col %d: %q %v %v", col, w, r, ok)
		}
	}
	if _, _, ok := WordAt(h.eb, editbuffer.RowsPos{RowIndex: 0, ColIndex: 1}); ok {
		t.Error("a word was found where there is none")
	}

	if err := Add(h.eb, "teh"); err != nil || !slices.Equal(h.eng.added, []string{"teh"}) {
		t.Fatalf("add: %v %v", err, h.eng.added)
	}
	Ignore(h.eb, "teh")
	if !slices.Equal(h.eng.ignored, []string{"teh"}) {
		t.Fatalf("ignore: %v", h.eng.ignored)
	}
	if got := Suggest(h.eb, "teh", 5); !slices.Equal(got, []string{"the"}) {
		t.Fatalf("suggest: %v", got)
	}
}

func TestEnableErrors(t *testing.T) {
	eb := editbuffer.NewEditBuffer(nil)
	opts := Options{Dispatch: func(func()) {}}

	Register(nil)
	if err := Enable(eb, "Fundamental", "", opts); err != ErrNoEngine {
		t.Errorf("no engine: %v", err)
	}
	Register(func(string) (Engine, error) { return &fakeEngine{}, nil })
	defer Register(nil)
	if err := Enable(eb, "Go", "", opts); err != ErrUnsupportedMode {
		t.Errorf("Go: %v", err)
	}
	if Enabled(eb) {
		t.Error("enabled after an error")
	}
}

// An edit that leaves every misspelled word where it is must not touch the
// layer: its version is part of what the cached row styles are made from.
func TestUnchangedWordsDoNotBumpTheLayerVersion(t *testing.T) {
	h := newHarness(t, "teh cat\nfine\n")
	h.enable("Fundamental")
	h.pump()

	before := h.layer.Peek(highlight.LayerSpell).Version()
	h.eb.InsertRegion(1, 0, editbuffer.Rows_{editbuffer.Row_("x")}) // row 1 has no word
	h.pump()
	if after := h.layer.Peek(highlight.LayerSpell).Version(); after != before {
		t.Fatalf("version %d -> %d", before, after)
	}
}

// In a program only the comments and strings are checked, not the code.
func TestCodeScopeChecksOnlyCommentsAndStrings(t *testing.T) {
	h := newHarness(t, "x := 1 // teh\ny := \"recieve\"\nteh := 2\n")
	h.enableWith("Go", &lexicalScope{d: goLike})
	h.pump()
	if got, want := h.words(), []wordPos{{0, 10, 13}, {1, 6, 13}}; !slices.Equal(got, want) {
		t.Fatalf("words = %v, want %v", got, want)
	}
}

// Opening a block comment makes the rows below it prose, although they were
// not edited.
func TestOpeningACommentChecksTheRowsBelow(t *testing.T) {
	h := newHarness(t, "teh\nteh\n")
	h.enableWith("Go", &lexicalScope{d: goLike})
	h.pump()
	if len(h.words()) != 0 {
		t.Fatalf("code was checked: %v", h.words())
	}

	h.eb.InsertRegion(0, 0, editbuffer.Rows_{editbuffer.Row_("/*")})
	h.pump()
	if got, want := h.words(), []wordPos{{0, 2, 5}, {1, 0, 3}}; !slices.Equal(got, want) {
		t.Fatalf("after the comment was opened: %v, want %v", got, want)
	}
}

// The names of the code are checked when asked for, and the comments and
// strings are checked either way.
func TestSetCodeChecksIdentifiers(t *testing.T) {
	h := newHarness(t, "recieve := 1 // teh\n")
	h.enableWith("Go", &lexicalScope{d: goLike})
	h.pump()
	if got, want := h.words(), []wordPos{{0, 16, 19}}; !slices.Equal(got, want) {
		t.Fatalf("code off: %v, want %v", got, want)
	}
	if CodeOn(h.eb) {
		t.Fatal("the code is on by default")
	}

	if err := SetCode(h.eb, true); err != nil {
		t.Fatal(err)
	}
	h.pump()
	if got, want := h.words(), []wordPos{{0, 0, 7}, {0, 16, 19}}; !slices.Equal(got, want) {
		t.Fatalf("code on: %v, want %v", got, want)
	}
	if !CodeOn(h.eb) {
		t.Fatal("not on")
	}

	if err := SetCode(h.eb, false); err != nil {
		t.Fatal(err)
	}
	h.pump()
	if got, want := h.words(), []wordPos{{0, 16, 19}}; !slices.Equal(got, want) {
		t.Fatalf("code off again: %v, want %v", got, want)
	}
}

func TestSetCodeErrors(t *testing.T) {
	h := newHarness(t, "teh\n")
	if err := SetCode(h.eb, true); err != ErrNotEnabled {
		t.Errorf("not enabled: %v", err)
	}
	h.enable("Fundamental") // text: no code to add
	if err := SetCode(h.eb, true); err != ErrNoCodeScope {
		t.Errorf("text: %v", err)
	}
}

// The dictionaries of the language mode are asked for when the check starts,
// and what came of it can be asked.
func TestModeDictionariesWithScope(t *testing.T) {
	h := newHarness(t, "teh\n")
	h.enableWith("Go", &lexicalScope{d: goLike})
	if h.eng.loadedMode != "Go" {
		t.Fatalf("LoadMode was asked for %q", h.eng.loadedMode)
	}
	if got := Dictionaries(h.eb); !slices.Equal(got, []string{"go"}) {
		t.Errorf("Dictionaries = %v", got)
	}
	if got := Warnings(h.eb); !slices.Equal(got, []string{"extra: cannot open"}) {
		t.Errorf("Warnings = %v", got)
	}

	h2 := newHarness(t, "teh\n")
	h2.enable("Fundamental")
	if Dictionaries(h2.eb) != nil || Warnings(h2.eb) != nil {
		t.Error("a mode without a dictionary has none")
	}
	if Dictionaries(editbuffer.NewEditBuffer(nil)) != nil {
		t.Error("a buffer that is not checked has none")
	}
}

func TestTakeStartIsOnceForABuffer(t *testing.T) {
	eb := editbuffer.NewEditBuffer(nil)
	other := editbuffer.NewEditBuffer(nil)
	t.Cleanup(func() { Release(eb); Release(other) })

	if !TakeStart(eb) {
		t.Fatal("the first time must be the start")
	}
	if TakeStart(eb) {
		t.Fatal("a buffer starts once")
	}
	if !TakeStart(other) {
		t.Fatal("another buffer has its own start")
	}
	Release(eb)
	if !TakeStart(eb) {
		t.Fatal("a buffer that was released starts again")
	}
}
