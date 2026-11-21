package editorleaf

import "testing"

func rowsOf(lines ...string) [][]byte {
	out := make([][]byte, len(lines))
	for i, l := range lines {
		out[i] = []byte(l)
	}

	return out
}

func TestMatchBracketAt(t *testing.T) {
	text := rowsOf(
		"func f(a [2]int) {", // 0
		"\tg(h(1), {x})",     // 1
		"}",                  // 2
	)

	for _, tc := range []struct {
		row, col, wantRow, wantCol int
	}{
		{0, 6, 0, 15}, // "(" of "f(" -> its ")"
		{0, 15, 0, 6}, // and back
		{0, 17, 2, 0}, // "{" -> "}" on another row, the inner pair skipped
		{2, 0, 0, 17}, // and back
		{1, 2, 1, 12}, // g( ... ) skips the inner h(1)
		{1, 4, 1, 6},  // h(1)
		{0, 9, 0, 11}, // [2]
	} {
		r, c, ok := matchBracketAt(text, tc.row, tc.col)
		if !ok || r != tc.wantRow || c != tc.wantCol {
			t.Errorf("matchBracketAt(%d,%d) = %d,%d,%v; want %d,%d", tc.row, tc.col, r, c, ok, tc.wantRow, tc.wantCol)
		}
	}

	// No partner; and other kinds of brackets are not counted.
	if _, _, ok := matchBracketAt(rowsOf("(a]"), 0, 0); ok {
		t.Error("(a] should not match")
	}

	if _, _, ok := matchBracketAt(rowsOf("(a"), 0, 0); ok {
		t.Error("( without ) should not match")
	}
}

func TestMatchBracketFullWidth(t *testing.T) {
	text := rowsOf("彼は「そう（たぶん）だ」と言った")

	open := len("彼は")
	r, c, ok := matchBracketAt(text, 0, open)
	if !ok || r != 0 || c != len("彼は「そう（たぶん）だ") {
		t.Errorf("got %d,%d,%v", r, c, ok)
	}

	// From the closing one back.
	r, c, ok = matchBracketAt(text, 0, len("彼は「そう（たぶん）だ"))
	if !ok || c != open {
		t.Errorf("back: got %d,%d,%v", r, c, ok)
	}
}

func TestBracketAtCursor(t *testing.T) {
	rows := rowsOf("x(a)", "（b）")

	for _, tc := range []struct {
		row, col int
		wantCol  int
		ok       bool
	}{
		{0, 1, 1, true},          // on "("
		{0, 4, 3, true},          // behind ")" at the end of the row
		{0, 2, 1, true},          // on "a", right behind "("
		{0, 0, 0, false},         // on "x", nothing before
		{1, len("（"), 0, true},    // behind a full-width bracket
		{5, 0, 0, false},         // no such row
	} {
		_, c, ok := bracketAtCursor(rows, tc.row, tc.col)
		if ok != tc.ok || (ok && c != tc.wantCol) {
			t.Errorf("bracketAtCursor(%d,%d) = %d,%v; want %d,%v", tc.row, tc.col, c, ok, tc.wantCol, tc.ok)
		}
	}
}
