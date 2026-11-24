package editorleaf

import "testing"

func TestIdentifierAt(t *testing.T) {
	for _, tc := range []struct {
		row  string
		col  int
		want string
	}{
		{"x := fooBar + 1", 7, "fooBar"},
		{"x := fooBar + 1", 5, "fooBar"},  // at the start
		{"x := fooBar + 1", 11, "fooBar"}, // right behind it
		{"call(foo_bar)", 6, "foo_bar"},
		{"user.Name()", 5, "Name"}, // not the receiver
		{"変数名 := 1", len("変"), "変数名"},
		{"a b", 1, "a"},  // right behind it, before a space
		{"a  b", 2, ""},  // between two spaces
		{"a +  b", 3, ""}, // on an operator, after a space
		{"", 0, ""},
		{"abc", 9, ""},
	} {
		if got := identifierAt([]byte(tc.row), tc.col); got != tc.want {
			t.Errorf("identifierAt(%q, %d) = %q, want %q", tc.row, tc.col, got, tc.want)
		}
	}
}
