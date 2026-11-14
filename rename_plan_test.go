package editorleaf

import (
	"strings"
	"testing"

	"github.com/ge-editor/editorleaf/editbuffer"
)

func rp(r, c int) editbuffer.RowsPos { return editbuffer.RowsPos{RowIndex: r, ColIndex: c} }

func edit(sr, sc, er, ec int, text string) TextEdit {
	return TextEdit{Start: rp(sr, sc), End: rp(er, ec), NewText: text}
}

func rowsFrom(s string) [][]byte { return splitLines([]byte(s)) }

func textOf(rows [][]byte) string {
	parts := make([]string, len(rows))
	for i, r := range rows {
		parts[i] = string(r)
	}
	return strings.Join(parts, "\n")
}

func TestSplitLinesAndSameRows(t *testing.T) {
	rows := splitLines([]byte("a\r\nb\n"))
	if len(rows) != 3 || string(rows[0]) != "a" || string(rows[1]) != "b" || len(rows[2]) != 0 {
		t.Fatalf("rows = %q", rows)
	}
	if len(splitLines(nil)) != 1 {
		t.Fatal("an empty file is one empty row")
	}

	// A final newline may or may not show as an empty last row.
	if !sameRows(rowsFrom("a\nb\n"), rowsFrom("a\nb")) {
		t.Error("trailing empty rows must not matter")
	}
	if sameRows(rowsFrom("a\nb"), rowsFrom("a\nc")) {
		t.Error("different text compared equal")
	}
	if sameRows(rowsFrom("a"), rowsFrom("a\nb")) {
		t.Error("different row counts compared equal")
	}
}

func TestValidateEdits(t *testing.T) {
	rows := rowsFrom("hello world\nsecond")

	// Out of order input comes back sorted; touching ranges are fine.
	sorted, err := validateEdits(rows, []TextEdit{edit(0, 6, 0, 11, "x"), edit(0, 0, 0, 5, "y"), edit(0, 5, 0, 6, "-")})
	if err != nil || sorted[0].Start.ColIndex != 0 || sorted[1].Start.ColIndex != 5 || sorted[2].Start.ColIndex != 6 {
		t.Fatalf("sorted = %+v, %v", sorted, err)
	}

	for name, edits := range map[string][]TextEdit{
		"row outside":    {edit(5, 0, 5, 1, "x")},
		"column outside": {edit(0, 0, 0, 99, "x")},
		"negative":       {edit(0, -1, 0, 1, "x")},
		"reversed":       {edit(0, 5, 0, 2, "x")},
		"overlap":        {edit(0, 0, 0, 6, "x"), edit(0, 5, 0, 8, "y")},
	} {
		if _, err := validateEdits(rows, edits); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestApplyToRows(t *testing.T) {
	rows := rowsFrom("foo bar foo\nfoo")

	got := applyToRows(rows, []TextEdit{edit(0, 0, 0, 3, "x"), edit(0, 8, 0, 11, "yy"), edit(1, 0, 1, 3, "z")})
	if want := "x bar yy\nz"; textOf(got) != want {
		t.Fatalf("got %q, want %q", textOf(got), want)
	}
	if textOf(rows) != "foo bar foo\nfoo" {
		t.Fatal("the input rows were modified")
	}

	// An edit that adds lines, and one that removes them.
	got = applyToRows(rowsFrom("a b c"), []TextEdit{edit(0, 1, 0, 2, "\n1\n")})
	if want := "a\n1\nb c"; textOf(got) != want {
		t.Fatalf("got %q, want %q", textOf(got), want)
	}
	got = applyToRows(rowsFrom("one\ntwo\nthree"), []TextEdit{edit(0, 2, 2, 2, "-")})
	if want := "on-ree"; textOf(got) != want {
		t.Fatalf("got %q, want %q", textOf(got), want)
	}
}

func TestPreviewLines(t *testing.T) {
	rows := rowsFrom("  foo(foo)\nother\n  foo()")

	got := previewLines("a.go", rows, []TextEdit{
		edit(0, 2, 0, 5, "bar"), edit(0, 6, 0, 9, "bar"), // two edits in one row: one line
		edit(2, 2, 2, 5, "bar"),
	})
	want := []string{
		"a.go:1: foo(foo) -> bar(bar)",
		"a.go:3: foo() -> bar()",
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %q, want %q", got, want)
	}

	// An edit that spans lines cannot be shown as old -> new.
	got = previewLines("a.go", rows, []TextEdit{edit(0, 2, 1, 2, "x")})
	if len(got) != 1 || !strings.Contains(got[0], "spanning lines") {
		t.Fatalf("got %q", got)
	}
}

func TestCursorAfterEdits(t *testing.T) {
	// The cursor inside the renamed identifier goes to its start; an earlier
	// edit on the same line moves it by the change in length.
	edits := []TextEdit{edit(3, 2, 3, 5, "longer"), edit(3, 10, 3, 13, "x")}
	if got := cursorAfterEdits(rp(3, 11), edits); got != rp(3, 10+3) {
		t.Errorf("same line: got %v, want 3:13", got)
	}
	if got := cursorAfterEdits(rp(3, 3), edits); got != rp(3, 2) {
		t.Errorf("inside the first edit: got %v, want 3:2", got)
	}

	// Edits on other lines: only a change in the number of rows above matters.
	if got := cursorAfterEdits(rp(5, 4), []TextEdit{edit(1, 0, 1, 3, "a\nb")}); got != rp(6, 4) {
		t.Errorf("a line added above: got %v, want 6:4", got)
	}
	if got := cursorAfterEdits(rp(5, 4), []TextEdit{edit(7, 0, 7, 3, "a\nb")}); got != rp(5, 4) {
		t.Errorf("an edit below: got %v, want 5:4", got)
	}
	if got := cursorAfterEdits(rp(2, 2), nil); got != rp(2, 2) {
		t.Errorf("no edits: got %v", got)
	}
}

func TestCursorAfterFormat(t *testing.T) {
	// Reindenting: the edit replaces the leading whitespace of the line.
	edits := []TextEdit{edit(4, 0, 4, 2, "\t")}

	// The cursor in the text behind it moves by the change in length ...
	if got := cursorAfterFormat(rp(4, 10), edits); got != rp(4, 9) {
		t.Errorf("behind the edit: got %v, want 4:9", got)
	}
	// ... and one inside the replaced text keeps its place, as far as it goes.
	if got := cursorAfterFormat(rp(4, 1), []TextEdit{edit(4, 0, 4, 8, "    ")}); got != rp(4, 1) {
		t.Errorf("inside: got %v, want 4:1", got)
	}
	if got := cursorAfterFormat(rp(4, 7), []TextEdit{edit(4, 0, 4, 8, "  ")}); got != rp(4, 2) {
		t.Errorf("inside, past the new text: got %v, want 4:2", got)
	}

	// A rename would have put the cursor at the start of the edit instead.
	if got := cursorAfterEdits(rp(4, 7), []TextEdit{edit(4, 0, 4, 8, "  ")}); got != rp(4, 0) {
		t.Errorf("rename mapping: got %v, want 4:0", got)
	}

	// Lines added above push the cursor down; edits below leave it alone.
	if got := cursorAfterFormat(rp(9, 3), []TextEdit{edit(2, 0, 2, 0, "x\ny\n")}); got != rp(11, 3) {
		t.Errorf("lines added above: got %v, want 11:3", got)
	}
	if got := cursorAfterFormat(rp(3, 3), []TextEdit{edit(8, 0, 8, 4, "")}); got != rp(3, 3) {
		t.Errorf("edit below: got %v, want 3:3", got)
	}
}

