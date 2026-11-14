package search

import "testing"

func TestGenerations(t *testing.T) {
	var g Generations
	if _, ok := g.Current(); ok {
		t.Fatal("empty has a current generation")
	}

	a := g.Begin(Options{Text: "a"})
	b := g.Begin(Options{Text: "b"})
	g.Set(a, Options{Text: "a"}, []Span{sp(0, 0, 0, 3)}, 1)
	g.Set(b, Options{Text: "b"}, []Span{sp(0, 2, 0, 5)}, 1)

	cur, _ := g.Current()
	if cur.ID != b {
		t.Errorf("current = %d, want the last (%d)", cur.ID, b)
	}
	if _, over := g.Union(); !over {
		t.Error("the results of two generations overlap")
	}

	// Update drops the span for which f returns false.
	g.Update(func(s *Span) bool { return s.Start.Col != 0 })
	if gens := g.Snapshot(); len(gens[0].Spans) != 0 || len(gens[1].Spans) != 1 {
		t.Errorf("after Update: %v", gens)
	}

	g.Drop(b)
	if g.Has(b) || !g.Has(a) || g.Len() != 1 {
		t.Error("Drop")
	}
	if g.Set(b, Options{}, nil, 2) {
		t.Error("Set of a dropped generation must fail")
	}
	g.Clear()
	if g.Len() != 0 {
		t.Error("Clear")
	}
}
