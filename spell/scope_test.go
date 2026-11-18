package spell

import (
	"slices"
	"strings"
	"testing"

	"github.com/ge-editor/gecore/lang"
)

func TestScopeFor(t *testing.T) {
	for name, want := range map[string]bool{
		"Markdown": true, "markdown": true, "Fundamental": true, "Org": true,
		"Go": false, "": false,
	} {
		if _, ok := ScopeFor(name); ok != want {
			t.Errorf("ScopeFor(%q) = %v", name, ok)
		}
	}
}

func rowsOf(lines ...string) [][]byte {
	out := make([][]byte, len(lines))
	for i, l := range lines {
		out[i] = []byte(l)
	}
	return out
}

func TestMarkdownScopeSkipsCodeBlocks(t *testing.T) {
	s, _ := ScopeFor("markdown")
	s.Prepare(rowsOf(
		"text",
		"```go",
		"teh code",
		"```",
		"text again",
		"~~~",
		"tilde",
		"~~~",
		"```", // opens a block that is never closed
		"rest",
	))
	var got []bool
	for i := 0; i < 10; i++ {
		got = append(got, s.Skipped(i))
	}
	want := []bool{false, true, true, true, false, true, true, true, true, true}
	if !slices.Equal(got, want) {
		t.Fatalf("skipped = %v", got)
	}
	if s.Skipped(-1) || s.Skipped(99) {
		t.Fatal("rows that do not exist are not skipped")
	}

	// A fence of another kind does not close the block.
	s.Prepare(rowsOf("```", "~~~", "x", "```", "y"))
	if got := []bool{s.Skipped(0), s.Skipped(1), s.Skipped(2), s.Skipped(3), s.Skipped(4)}; !slices.Equal(got, []bool{true, true, true, true, false}) {
		t.Fatalf("skipped = %v", got)
	}
}

func TestMarkdownScopeMasksInlineCodeAndLinks(t *testing.T) {
	s, _ := ScopeFor("markdown")
	for in, want := range map[string]string{
		"plain text": "plain text",
		"call `teh()` now": "call         now",
		"an `unclosed tick": "an `unclosed tick",
		"see [docs](http://x.y/z)": "see [docs]              ",
		"![alt](img/teh.png) after": "![alt]              after",
		"a <b>bold</b> <http://x>": "a    bold               ",
		"日本語 `コード` 日本語": "日本語             日本語",
	} {
		got := string(s.Mask(0, []byte(in)))
		if len(got) != len(in) {
			t.Errorf("%q: the length changed (%d, was %d)", in, len(got), len(in))
		}
		if got != want {
			t.Errorf("Mask(%q) = %q, want %q", in, got, want)
		}
	}
}

// goLike is the delimiters of Go.
var goLike = &lang.Delimiters{
	LineComments:  []string{"//"},
	BlockComments: []lang.BlockDelimiter{{Open: "/*", Close: "*/"}},
	Strings: []lang.StringDelimiter{
		{Open: `"`, Close: `"`, Escape: '\\'},
		{Open: "`", Close: "`", Multiline: true},
		{Open: `'`, Close: `'`, Escape: '\\', Code: true},
	},
}

// proseOf is what a scope leaves of each row: the words, in order.
func proseOf(s Scope, rows [][]byte) [][]string {
	s.Prepare(rows)
	out := make([][]string, len(rows))
	for i, r := range rows {
		out[i] = strings.Fields(string(s.Mask(i, r)))
	}
	return out
}

func TestLexicalScope(t *testing.T) {
	rows := rowsOf(
		`x := "teh\n value" // a comment`, // 0: a string with an escape, a line comment
		`/* block teh`,                    // 1: a block comment starts
		`still block */ y := 'a'`,         // 2: ... ends; a character literal is code
		"raw := `multi",                   // 3: a raw string starts
		"line teh` + z",                   // 4: ... ends
		`plain_code(1, 2)`,                // 5: nothing
		`r := '"' // after quote`,         // 6: a quote in a character literal
		`s := "unclosed`,                  // 7: a string ends with the row
		`// last`,                         // 8
	)
	got := proseOf(&lexicalScope{d: goLike}, rows)
	want := [][]string{
		{"teh", "value", "a", "comment"},
		{"block", "teh"},
		{"still", "block"},
		{"multi"},
		{"line", "teh"},
		nil,
		{"after", "quote"},
		{"unclosed"},
		{"last"},
	}
	for i := range want {
		if !slices.Equal(got[i], want[i]) {
			t.Errorf("row %d %q: %q, want %q", i, rows[i], got[i], want[i])
		}
	}

	s := &lexicalScope{d: goLike}
	s.Prepare(rows)
	for i, skipped := range []bool{false, false, false, false, false, true, false, false, false} {
		if s.Skipped(i) != skipped {
			t.Errorf("Skipped(%d) = %v", i, !skipped)
		}
	}
}

func TestLexicalScopeKeyChangesWhenACommentOpensAbove(t *testing.T) {
	s := &lexicalScope{d: goLike}

	s.Prepare(rowsOf("code", "x := 1"))
	before := s.Key(1)

	s.Prepare(rowsOf("/*", "x := 1"))
	if s.Key(1) == before || s.Skipped(1) {
		t.Fatal("a row inside a block comment must be looked at again")
	}
	// The same rows give the same key.
	k := s.Key(1)
	s.Prepare(rowsOf("/*", "x := 1"))
	if s.Key(1) != k {
		t.Fatal("the key is not stable")
	}
}

func TestLexicalScopeMaskKeepsTheLength(t *testing.T) {
	s := &lexicalScope{d: goLike}
	row := "日本語 := \"héllo\" // wörld"
	s.Prepare(rowsOf(row))
	got := string(s.Mask(0, []byte(row)))
	if len(got) != len(row) {
		t.Fatalf("length %d, was %d", len(got), len(row))
	}
	if f := strings.Fields(got); !slices.Equal(f, []string{"héllo", "wörld"}) {
		t.Fatalf("%q", f)
	}
}

type plainMode struct{ lang.Mode }

func (plainMode) Name() string { return "Plain" }

type codeMode struct{ plainMode }

func (codeMode) SpellDelimiters() *lang.Delimiters { return goLike }

type mdMode struct{ plainMode }

func (mdMode) Name() string { return "Markdown" }

func TestForMode(t *testing.T) {
	if _, ok := ForMode(codeMode{}).(*lexicalScope); !ok {
		t.Error("a mode with delimiters has a lexical scope")
	}
	if _, ok := ForMode(mdMode{}).(*markdownScope); !ok {
		t.Error("Markdown has its own scope")
	}
	if _, ok := ForMode(plainMode{}).(textScope); !ok {
		t.Error("a mode without delimiters is text")
	}
	if _, ok := ForMode(nil).(textScope); !ok {
		t.Error("no mode is text")
	}
}

func TestLexicalScopeCode(t *testing.T) {
	row := `recieve := "teh\n" + 'x' // fine`
	s := &lexicalScope{d: goLike}
	s.Prepare(rowsOf(row))

	// Off: a row has to have prose to be checked.
	if s.Skipped(0) {
		t.Fatal("a row with prose is skipped")
	}
	s.Prepare(rowsOf("code only"))
	if !s.Skipped(0) {
		t.Fatal("a row without prose is checked")
	}
	keyOff := s.Key(0)

	// On: the code is checked, so no row is skipped.
	s.SetCode(true)
	s.Prepare(rowsOf("code only"))
	if s.Skipped(0) || !s.CodeOn() {
		t.Fatal("a row of code is skipped with the code on")
	}
	if s.Key(0) == keyOff {
		t.Fatal("the key must change when the code is turned on")
	}

	s.Prepare(rowsOf(row))
	code := string(s.MaskCode(0, []byte(row)))
	if len(code) != len(row) {
		t.Fatalf("length %d, was %d", len(code), len(row))
	}
	f := strings.Fields(code)
	if !slices.Contains(f, "recieve") {
		t.Errorf("the identifier is not in the code: %q", f)
	}
	for _, gone := range []string{"teh", "fine", "x", `\n`} {
		for _, w := range f {
			if strings.Contains(w, gone) {
				t.Errorf("%q is still in the code: %q", gone, f)
			}
		}
	}
	// The prose is the same with the code on.
	if got := strings.Fields(string(s.Mask(0, []byte(row)))); !slices.Equal(got, []string{"teh", "fine"}) {
		t.Errorf("prose %q", got)
	}
}
