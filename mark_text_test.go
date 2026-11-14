package editorleaf

import "testing"

func TestMarkLineText(t *testing.T) {
	cases := []struct {
		line string
		col  int
		want string
	}{
		{"func main() {", 5, "main() {"},
		{"\t\tfoo  \t bar", 2, "foo bar"},    // blanks are one space
		{"  indented", 99, "indented"},       // col past the end: the whole row
		{"abc   ", 3, "abc"},                 // nothing after col: the whole row
		{"日本語のテキスト", 3, "本語のテキスト"}, // a character boundary
		{"日本語のテキスト", 4, "本語のテキスト"}, // inside a character: moves back
		{"", 0, ""},
	}
	for _, c := range cases {
		if got := markLineText([]byte(c.line), c.col); got != c.want {
			t.Errorf("markLineText(%q, %d) = %q, want %q", c.line, c.col, got, c.want)
		}
	}
}
