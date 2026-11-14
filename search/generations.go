package search

import "sync"

// Generation is the result of one search mode. The results of several
// generations stay (and overlap) until Clear is called.
type Generation struct {
	ID       int
	Options  Options
	Spans    []Span // sorted, without overlaps
	Revision uint64 // revision of the buffer the spans were found in
}

// Generations holds the generations of one buffer view. It is safe to use from
// several goroutines. The last generation is the current one: moving to the
// next result and replacing work on it only.
type Generations struct {
	mu     sync.Mutex
	list   []*Generation
	nextID int
}

// Begin adds an empty generation and returns its ID.
func (g *Generations) Begin(o Options) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.nextID++
	g.list = append(g.list, &Generation{ID: g.nextID, Options: o})
	return g.nextID
}

// Set replaces the spans of generation id. It returns false (and changes
// nothing) if the generation no longer exists, which is how a result of a
// superseded search is dropped.
func (g *Generations) Set(id int, o Options, spans []Span, revision uint64) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, gen := range g.list {
		if gen.ID == id {
			gen.Options, gen.Spans, gen.Revision = o, spans, revision
			return true
		}
	}
	return false
}

// Has reports whether generation id exists.
func (g *Generations) Has(id int) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, gen := range g.list {
		if gen.ID == id {
			return true
		}
	}
	return false
}

// Snapshot returns copies of all generations, oldest first.
func (g *Generations) Snapshot() []Generation {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]Generation, len(g.list))
	for i, gen := range g.list {
		out[i] = *gen
	}
	return out
}

// Drop removes generation id (an empty search text leaves no generation).
func (g *Generations) Drop(id int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for i, gen := range g.list {
		if gen.ID == id {
			g.list = append(g.list[:i], g.list[i+1:]...)
			return
		}
	}
}

// Clear removes every generation.
func (g *Generations) Clear() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.list = nil
}

func (g *Generations) Len() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.list)
}

// Current returns a copy of the last generation.
func (g *Generations) Current() (Generation, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.list) == 0 {
		return Generation{}, false
	}
	return *g.list[len(g.list)-1], true
}

// Union is what the highlight layer shows: the spans of all generations
// merged, and whether any of them overlapped.
func (g *Generations) Union() ([]Span, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	all := make([][]Span, len(g.list))
	for i, gen := range g.list {
		all[i] = gen.Spans
	}
	return Union(all...)
}

// Update applies f to every span of every generation (used to follow an edit
// of the buffer). A span for which f returns false is removed.
func (g *Generations) Update(f func(*Span) bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, gen := range g.list {
		kept := make([]Span, 0, len(gen.Spans)) // new slice: readers may hold the old one
		for _, s := range gen.Spans {
			if f(&s) {
				kept = append(kept, s)
			}
		}
		gen.Spans = kept
	}
}
