package editorleaf

import (
	"reflect"
	"testing"
)

func TestSplitCaseWords(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []string
	}{
		{"foo_bar", []string{"foo", "bar"}},
		{"foo-bar-baz", []string{"foo", "bar", "baz"}},
		{"fooBar", []string{"foo", "Bar"}},
		{"FooBar", []string{"Foo", "Bar"}},
		{"HTTPServer", []string{"HTTP", "Server"}},
		{"foo2Bar", []string{"foo2", "Bar"}},
		{"FOO_BAR", []string{"FOO", "BAR"}},
		{"foo bar", []string{"foo", "bar"}},
		{"foo", []string{"foo"}},
		{"", nil},
	} {
		if got := splitCaseWords(tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("splitCaseWords(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// caseCycle follows nextCaseText n times from text, the way the mode does.
func caseCycle(text string, n int) []string {
	var out []string

	cur := guessCurrentCase(text)
	for i := 0; i < n; i++ {
		next, c, ok := nextCaseText(text, cur)
		if !ok {
			break
		}

		out = append(out, next)
		text, cur = next, c
	}

	return out
}

func TestNextCaseTextCycle(t *testing.T) {
	// The guess for "foo_bar" is snake_case, so the cycle starts after it.
	want := []string{"FOO_BAR", "foo-bar", "Foo-Bar", "fooBar", "FooBar", "foo_bar", "FOO_BAR"}
	if got := caseCycle("foo_bar", 7); !reflect.DeepEqual(got, want) {
		t.Errorf("cycle = %q, want %q", got, want)
	}
}

func TestNextCaseTextOneWord(t *testing.T) {
	// One word: several cases give the same text. Each text is a step once,
	// so the cycle is foo, FOO, Foo, and not foo, FOO, foo, Foo, foo, ...
	want := []string{"FOO", "Foo", "foo", "FOO"}
	if got := caseCycle("foo", 4); !reflect.DeepEqual(got, want) {
		t.Errorf("cycle = %q, want %q", got, want)
	}

	// The place in the cycle is found from the text when the guess is unknown.
	want = []string{"Foo", "foo", "FOO"}
	if got := caseCycle("FOO", 3); !reflect.DeepEqual(got, want) {
		t.Errorf("caseCycle(FOO) = %q, want %q", got, want)
	}
}

// The case that is kept is what the next change starts from, whatever the
// text would be guessed to be.
func TestNextCaseTextUsesKeptCase(t *testing.T) {
	// "ab_cd" is snake_case for a guess; kept as Train-Case it goes on to
	// lowerCamelCase.
	out, next, ok := nextCaseText("ab_cd", int(CaseTrain))
	if !ok || out != "abCd" || next != int(CaseLowerCamel) {
		t.Errorf("got %q %d %v", out, next, ok)
	}
}

func TestNextCaseTextKeepsAffixes(t *testing.T) {
	out, _, ok := nextCaseText("_fooBar", guessCurrentCase("_fooBar"))
	if !ok || out != "_FooBar" {
		t.Errorf("got %q %v, want %q", out, ok, "_FooBar")
	}

	out, _, ok = nextCaseText("foo_bar_", -1)
	if !ok || out != "FOO_BAR_" {
		t.Errorf("got %q %v, want %q", out, ok, "FOO_BAR_")
	}
}

func TestNextCaseTextPhrase(t *testing.T) {
	// A text with a space cycles through all the cases, the spaced ones too.
	want := []string{"UPPER CASE", "Upper Case", "upper_case"}
	if got := caseCycle("upper case", 3); !reflect.DeepEqual(got, want) {
		t.Errorf("cycle = %q, want %q", got, want)
	}
}

// Text that has characters the conversion would lose is not changed.
func TestNextCaseTextRefuses(t *testing.T) {
	for _, in := range []string{"", "   ", "__", "foo_あ", "foo\tbar", "foo\nbar", "a.b"} {
		if out, _, ok := nextCaseText(in, -1); ok {
			t.Errorf("nextCaseText(%q) = %q, want no change", in, out)
		}
	}
}

// Whatever the start, the cycle ends: no input makes it loop.
func TestNextCaseTextTerminates(t *testing.T) {
	for _, in := range []string{"a1", "a_1", "ab", "AB", "Ab", "a-b", "x2y"} {
		for cur := -1; cur < len(CaseTypes); cur++ {
			nextCaseText(in, cur) // must return
		}
	}
}

func TestGuessTargetWord(t *testing.T) {
	for _, tc := range []struct {
		line      string
		col       int
		wantWord  string
		wantStart int
		wantEnd   int
		wantOK    bool
	}{
		{"x := fooBar + 1", 7, "fooBar", 5, 11, true},
		{"x := fooBar + 1", 5, "fooBar", 5, 11, true},  // at the start of the word
		{"x := fooBar + 1", 11, "fooBar", 5, 11, true}, // right behind it
		{"call(foo_bar)", 8, "foo_bar", 5, 12, true},
		{"a --foo-bar-- b", 6, "foo-bar", 4, 11, true}, // '-' at both ends is not part of it
		{"x := 1", 5, "", 0, 0, false},                  // too short
		{"1foo", 2, "", 0, 0, false},                    // must start with a letter
		{"foo_", 2, "", 0, 0, false},                    // must end with a letter or digit
		{"a  b", 2, "", 0, 0, false},                    // between words
		{"", 0, "", 0, 0, false},
		{"foo", 9, "", 0, 0, false}, // column out of the row
	} {
		start, end, word, ok := guessTargetWord([]byte(tc.line), tc.col)
		if ok != tc.wantOK || word != tc.wantWord || (ok && (start != tc.wantStart || end != tc.wantEnd)) {
			t.Errorf("guessTargetWord(%q, %d) = %d %d %q %v, want %d %d %q %v",
				tc.line, tc.col, start, end, word, ok, tc.wantStart, tc.wantEnd, tc.wantWord, tc.wantOK)
		}
	}
}
