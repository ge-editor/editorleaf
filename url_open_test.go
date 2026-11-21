package editorleaf

import "testing"

func TestFindURL(t *testing.T) {
	for _, tc := range []struct {
		name string
		row  string
		col  int
		want string
		ok   bool
	}{
		{"on it", "see https://example.com/a?b=1 now", 10, "https://example.com/a?b=1", true},
		{"at its start", "see https://example.com/a now", 4, "https://example.com/a", true},
		{"right behind it", "see https://example.com/a now", 25, "https://example.com/a", true},
		{"sentence end", "go to https://example.com/a.", 8, "https://example.com/a", true},
		{"comma", "https://a.example/x, and more", 3, "https://a.example/x", true},
		{"in parentheses", "(see https://example.com/a)", 8, "https://example.com/a", true},
		{"markdown link", "[x](https://example.com/a_(b))", 10, "https://example.com/a_(b)", true},
		{"in angle brackets", "<http://example.com/x>", 5, "http://example.com/x", true},
		{"in quotes", `href="https://example.com/x"`, 9, "https://example.com/x", true},
		{"upper case scheme", "HTTP://EXAMPLE.COM/", 2, "HTTP://EXAMPLE.COM/", true},
		{"file", "open file:///tmp/a.txt please", 8, "file:///tmp/a.txt", true},
		{"japanese text after", "https://example.com/a は便利", 3, "https://example.com/a", true},
		{"japanese text right after", "https://example.com/aです。", 3, "https://example.com/a", true},
		{"full-width bracket after", "（https://example.com/a）", 5, "https://example.com/a", true},
		{"only url of the row, cursor elsewhere", "a https://example.com/a b", 0, "https://example.com/a", true},
		{"two urls, cursor on neither", "a https://x.example/1 b https://y.example/2", 0, "", false},
		{"two urls, cursor on the second", "a https://x.example/1 b https://y.example/2", 30, "https://y.example/2", true},
		{"no url", "just text", 3, "", false},
		{"not a safe scheme", "javascript://x and mailto:a@b", 3, "", false},
	} {
		got, ok := findURL([]byte(tc.row), tc.col)
		if ok != tc.ok || got != tc.want {
			t.Errorf("%s: findURL(%q, %d) = %q, %v; want %q, %v", tc.name, tc.row, tc.col, got, ok, tc.want, tc.ok)
		}
	}
}

func TestTrimURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://a.example/x.":     "https://a.example/x",
		"https://a.example/x?!":    "https://a.example/x",
		"https://a.example/(x)":    "https://a.example/(x)",
		"https://a.example/(x))":   "https://a.example/(x)",
		"https://a.example/x]":     "https://a.example/x",
		"https://a.example/x*":     "https://a.example/x",
		"https://a.example/x/":     "https://a.example/x/",
	} {
		if got := trimURL(in); got != want {
			t.Errorf("trimURL(%q) = %q, want %q", in, got, want)
		}
	}
}
