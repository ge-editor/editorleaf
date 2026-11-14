package search

import (
	"context"
	"testing"
)

func rows(lines ...string) [][]byte {
	out := make([][]byte, len(lines))
	for i, l := range lines {
		out[i] = []byte(l)
	}
	return out
}

func find(t *testing.T, d *Doc, o Options) []Span {
	t.Helper()
	_, ms, err := Find(context.Background(), d, o)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]Span, len(ms))
	for i, m := range ms {
		out[i] = m.Span
	}
	return out
}

func sp(r1, c1, r2, c2 int) Span {
	return Span{Pos{r1, c1}, Pos{r2, c2}}
}

func TestDocPosOffset(t *testing.T) {
	d := NewDoc(rows("ab", "", "cde"))
	// "ab\n\ncde"
	cases := []struct {
		off int
		pos Pos
	}{{0, Pos{0, 0}}, {2, Pos{0, 2}}, {3, Pos{1, 0}}, {4, Pos{2, 0}}, {7, Pos{2, 3}}}
	for _, c := range cases {
		if got := d.Pos(c.off); got != c.pos {
			t.Errorf("Pos(%d) = %v, want %v", c.off, got, c.pos)
		}
		if got := d.Offset(c.pos); got != c.off {
			t.Errorf("Offset(%v) = %d, want %d", c.pos, got, c.off)
		}
	}
}

func TestFindMultiline(t *testing.T) {
	d := NewDoc(rows("xx line1", "line2 yy", "line1", "line2"))
	got := find(t, d, Options{Text: "line1\nline2", MatchCase: true})
	want := []Span{sp(0, 3, 1, 5), sp(2, 0, 3, 5)}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestFindCase(t *testing.T) {
	d := NewDoc(rows("Foo foo FOO"))
	if n := len(find(t, d, Options{Text: "foo", MatchCase: true})); n != 1 {
		t.Errorf("match case: %d", n)
	}
	if n := len(find(t, d, Options{Text: "foo"})); n != 3 {
		t.Errorf("ignore case: %d", n)
	}
	// A character whose lower case has another byte length must not move
	// the following offsets.
	d = NewDoc(rows("İx x"))
	got := find(t, d, Options{Text: "x"})
	if len(got) != 2 || got[0] != sp(0, 2, 0, 3) || got[1] != sp(0, 4, 0, 5) {
		t.Errorf("got %v", got)
	}
}

func TestFindWholeWord(t *testing.T) {
	d := NewDoc(rows("cat concat cat_ cat"))
	got := find(t, d, Options{Text: "cat", WholeWord: true})
	if len(got) != 2 || got[0] != sp(0, 0, 0, 3) || got[1] != sp(0, 16, 0, 19) {
		t.Errorf("got %v", got)
	}
	d = NewDoc(rows("日本語です 日本語"))
	got = find(t, d, Options{Text: "日本語", WholeWord: true})
	if len(got) != 1 || got[0].Start != (Pos{0, 16}) {
		t.Errorf("japanese: %v", got)
	}
}

func TestFindRegex(t *testing.T) {
	d := NewDoc(rows("abc", "abd", "xab"))
	// ^ matches at every row; empty matches are left out
	got := find(t, d, Options{Text: "^ab", Regex: true})
	if len(got) != 2 || got[0] != sp(0, 0, 0, 2) || got[1] != sp(1, 0, 1, 2) {
		t.Errorf("got %v", got)
	}
	// . does not match a newline
	if n := len(find(t, d, Options{Text: "c.a", Regex: true})); n != 0 {
		t.Errorf(". matched a newline")
	}
	// A regexp can match across rows with an explicit \n
	if n := len(find(t, d, Options{Text: `c\nab`, Regex: true})); n != 1 {
		t.Errorf(`\n: %d`, n)
	}
	// Literal mode does not treat . specially
	if n := len(find(t, NewDoc(rows("a.c abc")), Options{Text: "a.c"})); n != 1 {
		t.Errorf("literal: %d", n)
	}
	if _, _, err := Find(context.Background(), d, Options{Text: "(", Regex: true}); err == nil {
		t.Errorf("a bad regexp must be an error")
	}
}

func TestExpand(t *testing.T) {
	d := NewDoc(rows("key=value"))
	re, ms, _ := Find(context.Background(), d, Options{Text: `(\w+)=(\w+)`, Regex: true, MatchCase: true})
	if len(ms) != 1 {
		t.Fatal(len(ms))
	}
	if got := string(ms[0].Expand(re, d, "$2=$1", true)); got != "value=key" {
		t.Errorf("got %q", got)
	}
	if got := string(ms[0].Expand(re, d, "$2=$1", false)); got != "$2=$1" {
		t.Errorf("literal got %q", got)
	}
}

func TestUnion(t *testing.T) {
	a := []Span{sp(0, 0, 0, 3), sp(1, 0, 1, 2)}
	b := []Span{sp(0, 2, 0, 5), sp(2, 0, 2, 1)}
	m, over := Union(a, b)
	want := []Span{sp(0, 0, 0, 5), sp(1, 0, 1, 2), sp(2, 0, 2, 1)}
	if !over || len(m) != len(want) {
		t.Fatalf("over=%v merged=%v", over, m)
	}
	for i := range want {
		if m[i] != want[i] {
			t.Errorf("m[%d] = %v, want %v", i, m[i], want[i])
		}
	}

	// Touching spans are not an overlap
	m, over = Union([]Span{sp(0, 0, 0, 3)}, []Span{sp(0, 3, 0, 5)})
	if over || len(m) != 2 {
		t.Errorf("touching: over=%v merged=%v", over, m)
	}

	// One span inside another
	m, over = Union([]Span{sp(0, 0, 3, 0)}, []Span{sp(1, 1, 1, 2)})
	if !over || len(m) != 1 || m[0] != sp(0, 0, 3, 0) {
		t.Errorf("inside: over=%v merged=%v", over, m)
	}
}
