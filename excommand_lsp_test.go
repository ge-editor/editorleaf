package editorleaf

import (
	"testing"

	"github.com/ge-editor/editorleaf/editbuffer"
)

func TestFirstLines(t *testing.T) {
	for _, tc := range []struct {
		in   string
		n    int
		want string
	}{
		{"", 3, ""},
		{"one", 3, "one"},
		{"func f()\n\n  Doc text  \nthird\nfourth", 3, "func f() | Doc text | third"},
		{"a\nb", 1, "a"},
	} {
		if got := firstLines(tc.in, tc.n); got != tc.want {
			t.Errorf("firstLines(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
		}
	}
}

func TestIdentifierStart(t *testing.T) {
	for _, tc := range []struct {
		line string
		col  int
		want int
	}{
		{"fmt.Pri", 7, 4},
		{"fmt.", 4, 4},
		{"x := foo_bar2", 13, 5},
		{"変数名abc", len("変数名abc"), 0},
		{"fmt.Pri", 5, 4},
		{"", 0, 0},
	} {
		if got := identifierStart([]byte(tc.line), tc.col); got != tc.want {
			t.Errorf("identifierStart(%q, %d) = %d, want %d", tc.line, tc.col, got, tc.want)
		}
	}
}

func TestCmpRowsPos(t *testing.T) {
	at := func(r, c int) editbuffer.RowsPos { return editbuffer.RowsPos{RowIndex: r, ColIndex: c} }

	if cmpRowsPos(at(1, 5), at(2, 0)) >= 0 || cmpRowsPos(at(2, 0), at(1, 5)) <= 0 {
		t.Error("the row decides first")
	}
	if cmpRowsPos(at(1, 2), at(1, 3)) >= 0 || cmpRowsPos(at(1, 3), at(1, 2)) <= 0 {
		t.Error("the column decides within a row")
	}
	if cmpRowsPos(at(1, 2), at(1, 2)) != 0 {
		t.Error("equal positions compare equal")
	}
}

func TestSymbolLines(t *testing.T) {
	at := func(row int) Location { return Location{Path: "x.go", Pos: editbuffer.RowsPos{RowIndex: row}} }

	if got, want := outlineLine(Symbol{Name: "Outer", Kind: "struct", Location: at(0)}), "Outer  struct  :1"; got != want {
		t.Errorf("outline: got %q, want %q", got, want)
	}
	if got, want := outlineLine(Symbol{Name: "inner", Kind: "field", Depth: 2, Location: at(4)}), "    inner  field  :5"; got != want {
		t.Errorf("nested outline: got %q, want %q", got, want)
	}
	if got, want := outlineLine(Symbol{Name: "x", Location: at(9)}), "x  :10"; got != want {
		t.Errorf("no kind: got %q, want %q", got, want)
	}

	got := workspaceSymbolLine(Symbol{Name: "Foo", Kind: "func", Container: "pkg", Location: at(41)})
	if want := "Foo  func  pkg  x.go:42"; got != want {
		t.Errorf("workspace: got %q, want %q", got, want)
	}
	if got, want := workspaceSymbolLine(Symbol{Name: "Bar", Location: at(0)}), "Bar  x.go:1"; got != want {
		t.Errorf("workspace, bare: got %q, want %q", got, want)
	}
}

func TestCodeActionLine(t *testing.T) {
	for _, tc := range []struct {
		in   CodeAction
		want string
	}{
		{CodeAction{Title: "Add import", Kind: "quickfix", Preferred: true}, "* Add import  [quickfix]"},
		{CodeAction{Title: "Extract function", Kind: "refactor.extract"}, "Extract function  [refactor.extract]"},
		{CodeAction{Title: "Organize imports"}, "Organize imports"},
	} {
		if got := codeActionLine(tc.in); got != tc.want {
			t.Errorf("codeActionLine(%+v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestHoverDisplayText(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{
			"code fence and text",
			"```go\nfunc Foo(a int) string\n```\n\nFoo returns a string.\n",
			"func Foo(a int) string\n\nFoo returns a string.",
		},
		{"blank lines are not doubled", "a\n\n\n\nb", "a\n\nb"},
		{"blanks at the ends, the indentation stays", "\n\n  a  \n\n", "  a"},
		{"CRLF", "a\r\n\r\nb\r\n", "a\n\nb"},
		{"fence with spaces and a language", "  ```go  \nx\n  ```", "x"},
		{"empty", "", ""},
		{"only a fence", "```\n```", ""},
		{"no change", "one line", "one line"},
	} {
		if got := hoverDisplayText(tc.in); got != tc.want {
			t.Errorf("%s: hoverDisplayText(%q) = %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
}
