package diagnostic

import (
	"strings"
	"testing"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/editorleaf/highlight"
)

func pos(r, c int) editbuffer.RowsPos { return editbuffer.RowsPos{RowIndex: r, ColIndex: c} }

func item(sr, sc, er, ec int, sev Severity, msg string) Item {
	return Item{Start: pos(sr, sc), End: pos(er, ec), Severity: sev, Message: msg}
}

type seg struct{ sr, sc, er, ec int }

func segs(spans []highlight.Span) []seg {
	out := make([]seg, len(spans))
	for i, s := range spans {
		out[i] = seg{s.Start.RowIndex, s.Start.ColIndex, s.End.RowIndex, s.End.ColIndex}
	}
	return out
}

func equalSegs(a, b []seg) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSpansOfDisjoint(t *testing.T) {
	got := spansOf([]Item{
		item(0, 0, 0, 3, Error, "a"),
		item(1, 2, 1, 5, Warning, "b"),
	})
	want := []seg{{0, 0, 0, 3}, {1, 2, 1, 5}}
	if !equalSegs(segs(got), want) {
		t.Fatalf("got %v, want %v", segs(got), want)
	}
	if got[0].Color != StyleError || got[1].Color != StyleWarning {
		t.Fatal("styles do not follow the severity")
	}
}

func TestSpansOfOverlapMostSevereWins(t *testing.T) {
	// A warning over cols 0..10 with an error inside it at 3..6.
	got := spansOf([]Item{
		item(0, 0, 0, 10, Warning, "w"),
		item(0, 3, 0, 6, Error, "e"),
	})
	want := []seg{{0, 0, 0, 3}, {0, 3, 0, 6}, {0, 6, 0, 10}}
	if !equalSegs(segs(got), want) {
		t.Fatalf("got %v, want %v", segs(got), want)
	}
	if got[0].Color != StyleWarning || got[1].Color != StyleError || got[2].Color != StyleWarning {
		t.Fatal("the error must win only where it overlaps")
	}
}

func TestSpansOfMergesAndGaps(t *testing.T) {
	// The same item is not split by a boundary of another item that does not
	// outrank it; a gap between items yields no span.
	got := spansOf([]Item{
		item(0, 0, 0, 8, Error, "e"),
		item(0, 2, 0, 4, Hint, "h"),
		item(0, 10, 0, 12, Hint, "g"),
	})
	want := []seg{{0, 0, 0, 8}, {0, 10, 0, 12}}
	if !equalSegs(segs(got), want) {
		t.Fatalf("got %v, want %v", segs(got), want)
	}
}

func TestSpansOfPointAndMultiRow(t *testing.T) {
	got := spansOf([]Item{
		item(2, 4, 2, 4, Error, "point"),
		item(0, 5, 1, 2, Warning, "multi"),
	})
	want := []seg{{0, 5, 1, 2}, {2, 4, 2, 5}}
	if !equalSegs(segs(got), want) {
		t.Fatalf("got %v, want %v", segs(got), want)
	}

	// Every span must be findable the way the editor looks them up.
	for _, s := range got {
		sp := s
		if !highlight.PosInSpan(sp.Start, &sp) {
			t.Errorf("span %v does not contain its own start", segs([]highlight.Span{s}))
		}
	}
}

func TestSubscribeSetAndRelease(t *testing.T) {
	eb := editbuffer.NewEditBuffer(nil)
	layer := highlight.NewHighlightsLayer()
	const prio = highlight.LayerDiagnostic

	unsub := Subscribe(eb, layer, prio)

	Set(eb, []Item{item(0, 0, 0, 3, Error, "e")})
	if n := layer.SpanLength(prio); n != 1 {
		t.Fatalf("SpanLength = %d, want 1", n)
	}
	if got := layer.GetSpanTemplate(prio, 0); got == nil || (*got).GetSpan().Priority != prio {
		t.Fatal("span missing or without the layer's priority")
	}

	// A pane that subscribes later sees the current diagnostics at once.
	layer2 := highlight.NewHighlightsLayer()
	unsub2 := Subscribe(eb, layer2, prio)
	if layer2.SpanLength(prio) != 1 {
		t.Fatal("a new subscriber must receive the current spans")
	}

	// Set(nil) clears every subscriber.
	Set(eb, nil)
	if layer.SpanLength(prio) != 0 || layer2.SpanLength(prio) != 0 {
		t.Fatal("Set(nil) must clear the layers")
	}

	// After unsubscribe a layer is no longer updated.
	unsub()
	unsub() // twice is fine
	Set(eb, []Item{item(0, 0, 0, 1, Warning, "w")})
	if layer.SpanLength(prio) != 0 {
		t.Fatal("an unsubscribed layer must not change")
	}
	if layer2.SpanLength(prio) != 1 {
		t.Fatal("the other subscriber must still be updated")
	}

	// The diagnostics stay with the buffer until Release.
	if len(Items(eb)) != 1 {
		t.Fatal("Items lost")
	}
	Release(eb)
	if layer2.SpanLength(prio) != 0 || len(Items(eb)) != 0 {
		t.Fatal("Release must clear layers and forget the buffer")
	}
	unsub2()
}

func TestAt(t *testing.T) {
	eb := editbuffer.NewEditBuffer(nil)
	defer Release(eb)

	Set(eb, []Item{
		item(0, 0, 0, 10, Warning, "w"),
		item(0, 4, 0, 6, Error, "e"),
		item(1, 3, 1, 3, Hint, "point"),
	})

	got := At(eb, pos(0, 5))
	if len(got) != 2 || got[0].Message != "e" || got[1].Message != "w" {
		t.Fatalf("At(0,5) = %+v, want the error first, then the warning", got)
	}
	if got := At(eb, pos(0, 12)); len(got) != 0 {
		t.Fatalf("At(0,12) = %+v, want none", got)
	}
	if got := At(eb, pos(1, 3)); len(got) != 1 || got[0].Message != "point" {
		t.Fatalf("At(point) = %+v", got)
	}
}

func TestSummary(t *testing.T) {
	if _, _, ok := Summary(nil); ok {
		t.Fatal("no items must give ok == false")
	}

	text, sev, ok := Summary([]Item{
		{Severity: Error, Message: "undefined: x\nmore detail", Source: "compiler"},
		{Severity: Warning, Message: "other"},
		{Severity: Hint, Message: "third"},
	})
	if !ok || sev != Error || text != "error: undefined: x [compiler] (+2)" {
		t.Fatalf("got %q, %v, %v", text, sev, ok)
	}

	text, _, _ = Summary([]Item{{Severity: Warning, Message: "  unused  "}})
	if text != "warning: unused" {
		t.Fatalf("got %q", text)
	}
	text, _, _ = Summary([]Item{{Severity: Information, Message: "i"}})
	if text != "info: i" {
		t.Fatalf("got %q", text)
	}
}

// ---- following edits ----

func loaded(t *testing.T, text string) *editbuffer.EditBuffer {
	t.Helper()
	eb := editbuffer.NewEditBuffer(nil)
	if err := eb.LoadFrom(strings.NewReader(text)); err != nil {
		t.Fatal(err)
	}
	return eb
}

func startsOf(items []Item) []seg {
	out := make([]seg, len(items))
	for i, it := range items {
		out[i] = seg{it.Start.RowIndex, it.Start.ColIndex, it.End.RowIndex, it.End.ColIndex}
	}
	return out
}

func TestDiagnosticsFollowEdits(t *testing.T) {
	eb := loaded(t, "hello world\nsecond")
	defer Release(eb)

	layer := highlight.NewHighlightsLayer()
	unsub := Subscribe(eb, layer, highlight.LayerDiagnostic)
	defer unsub()

	// "world" on row 0 and "second" on row 1.
	Set(eb, []Item{
		item(0, 6, 0, 11, Error, "world"),
		item(1, 0, 1, 6, Warning, "second"),
	})

	check := func(step string, want ...seg) {
		t.Helper()
		if got := startsOf(Items(eb)); !equalSegs(got, want) {
			t.Fatalf("%s: got %v, want %v", step, got, want)
		}
	}

	// Typing before the first item on its line pushes it right.
	eb.InsertRegion(0, 0, editbuffer.Rows_{editbuffer.Row_("XX")})
	check("insert before", seg{0, 8, 0, 13}, seg{1, 0, 1, 6})

	// Typing right behind an item does not extend it.
	eb.InsertRegion(0, 13, editbuffer.Rows_{editbuffer.Row_("!")})
	check("insert at end", seg{0, 8, 0, 13}, seg{1, 0, 1, 6})

	// Typing inside an item makes it longer.
	eb.InsertRegion(0, 10, editbuffer.Rows_{editbuffer.Row_("--")})
	check("insert inside", seg{0, 8, 0, 15}, seg{1, 0, 1, 6})

	// A new line at the start of row 0 moves everything down.
	eb.InsertRegion(0, 0, editbuffer.Rows_{editbuffer.Row_(""), editbuffer.Row_("")})
	check("insert newline", seg{1, 8, 1, 15}, seg{2, 0, 2, 6})

	// And deleting it brings everything back.
	eb.RemoveRegion(editbuffer.RowsPos{RowIndex: 0, ColIndex: 0}, editbuffer.RowsPos{RowIndex: 1, ColIndex: 0})
	check("delete newline", seg{0, 8, 0, 15}, seg{1, 0, 1, 6})

	// The layer follows too.
	if layer.SpanLength(highlight.LayerDiagnostic) != 2 {
		t.Fatalf("layer has %d spans", layer.SpanLength(highlight.LayerDiagnostic))
	}
	first := (*layer.GetSpanTemplate(highlight.LayerDiagnostic, 0)).GetSpan()
	if first.Start.ColIndex != 8 || first.End.ColIndex != 15 {
		t.Fatalf("layer span = %v..%v", first.Start, first.End)
	}

	// Deleting all of an item's text removes the item.
	eb.RemoveRegion(editbuffer.RowsPos{RowIndex: 1, ColIndex: 0}, editbuffer.RowsPos{RowIndex: 1, ColIndex: 6})
	check("delete whole item", seg{0, 8, 0, 15})
}

func TestShiftPosDeleteCases(t *testing.T) {
	// Delete (0,2)..(1,3): a position inside collapses to the start; one on
	// the last deleted row keeps its distance from the end of the region;
	// one below moves up.
	c := editbuffer.Change{Kind: editbuffer.ChangeDelete, Start: pos(0, 2), OldEnd: pos(1, 3)}

	for _, tc := range []struct{ in, want editbuffer.RowsPos }{
		{pos(0, 1), pos(0, 1)},
		{pos(0, 2), pos(0, 2)},
		{pos(0, 5), pos(0, 2)},
		{pos(1, 3), pos(0, 2)},
		{pos(1, 7), pos(0, 6)},
		{pos(3, 4), pos(2, 4)},
	} {
		if got := shiftPos(tc.in, c, false); got != tc.want {
			t.Errorf("shiftPos(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestResetClearsAndStopsListening(t *testing.T) {
	eb := loaded(t, "abc")
	defer Release(eb)

	layer := highlight.NewHighlightsLayer()
	defer Subscribe(eb, layer, highlight.LayerDiagnostic)()

	Set(eb, []Item{item(0, 0, 0, 3, Error, "e")})
	if layer.SpanLength(highlight.LayerDiagnostic) != 1 {
		t.Fatal("setup failed")
	}

	eb.SetBytesArray([][]byte{[]byte("new text")})
	if len(Items(eb)) != 0 || layer.SpanLength(highlight.LayerDiagnostic) != 0 {
		t.Fatal("a reset of the text must clear the diagnostics")
	}

	// With no items there is no listener left; Set works again afterwards.
	Set(eb, []Item{item(0, 0, 0, 3, Warning, "w")})
	if layer.SpanLength(highlight.LayerDiagnostic) != 1 {
		t.Fatal("Set after reset failed")
	}
}
