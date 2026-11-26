package editorleaf

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/editorleaf/highlight"
	"github.com/ge-editor/editorleaf/search"
	"github.com/ge-editor/gecore"
	"github.com/ge-editor/theme"
)

// Search and replace.
//
// A search mode makes one generation of results (search.Generations). The
// generations stay after the mode ends and overlap until ClearSearch; the
// highlight layer shows their union. Moving to the next result and replacing
// work on the current (last) generation.
//
// Threads: everything here runs on the UI goroutine, except the search itself
// (search.Find), which works on a copy of the text. SearchWith may be called
// from any goroutine.

const echoSearchOverlapped = "検索結果が重なっています"

func toRowsPos(p search.Pos) editbuffer.RowsPos {
	return editbuffer.RowsPos{RowIndex: p.Row, ColIndex: p.Col}
}

func toSearchPos(p editbuffer.RowsPos) search.Pos {
	return search.Pos{Row: p.RowIndex, Col: p.ColIndex}
}

func matchSpans(ms []search.Match) []search.Span {
	spans := make([]search.Span, len(ms))
	for i, m := range ms {
		spans[i] = m.Span
	}
	return spans
}

// searchDoc copies the text of the buffer. UI goroutine only.
func (e *Editorleaf) searchDoc() *search.Doc {
	rows := e.editBuffer.Rows()
	b := make([][]byte, rows.Length())
	for i := range b {
		b[i] = (*rows)[i]
	}
	return search.NewDoc(b)
}

func (e *Editorleaf) SearchHistory() []string {
	return e.meta.Search.History.Items
}

// BeginSearch starts a new search mode: the next SearchText makes a new
// generation of results, which overlaps the earlier ones. Call it when the
// mode starts (after asking whether to clear, see ClearSearch).
func (e *Editorleaf) BeginSearch() {
	sr := e.meta.Search
	if sr.PendingHistory != "" {
		sr.History.Add(sr.PendingHistory)
		sr.PendingHistory = ""
	}
	sr.SessionGen = 0
	sr.CurrentSearchIndex = -1
	if sr.Overlapped {
		gecore.Echo.AddText(echoSearchOverlapped)
	}
}

// HasSearchResults reports whether any generation of results stays.
func (e *Editorleaf) HasSearchResults() bool {
	return e.meta.Search.Gens.Len() > 0
}

// ClearSearch removes all search results.
func (e *Editorleaf) ClearSearch() {
	sr := e.meta.Search
	sr.Gens.Clear()
	sr.SessionGen = 0
	sr.CurrentSearchIndex = -1
	e.publishSearch(false)
}

// SearchText is kept for the callers that pass the conditions one by one.
func (e *Editorleaf) SearchText(text string, caseSensitive, isRegexp bool) {
	e.SearchWith(search.Options{Text: text, MatchCase: caseSensitive, Regex: isRegexp})
}

// SearchWith searches the buffer for o and makes the result the current
// generation (replacing the result of the same search mode). It can be called
// from any goroutine, for example from the debounce timer.
func (e *Editorleaf) SearchWith(o search.Options) {
	dispatchUI(func() { e.startSearch(o) })
}

// publishSearch shows the union of the generations in the highlight layer.
func (e *Editorleaf) publishSearch(announce bool) {
	sr := e.meta.Search
	spans, overlapped := sr.Gens.Union()

	tpl := make([]*highlight.SpanTemplate, len(spans))
	for i, s := range spans {
		a := NewFoundPosition(s.Start.Row, s.Start.Col, s.End.Row, s.End.Col,
			theme.ColorSearchFound, theme.ColorSearchFoundOnCursor)
		b := highlight.SpanTemplate(a)
		tpl[i] = &b
	}
	e.meta.HighlightLayer.Highlights(highlight.LayerSearch).SetSpans(tpl)

	sr.Overlapped = overlapped
	if announce && overlapped {
		gecore.Echo.AddText(echoSearchOverlapped)
	}
}

// prepareSearch gets ready to search for o: it stops the search that still
// runs, makes (or empties) the generation of the running search mode and
// copies the text. ok is false when there is nothing to search for. UI
// goroutine only.
func (e *Editorleaf) prepareSearch(o search.Options) (id int, doc *search.Doc, ok bool) {
	sr := e.meta.Search
	sr.CurrentSearchIndex = -1

	// A search still running is obsolete.
	sr.SearchMu.Lock()
	if sr.SearchCancel != nil {
		sr.SearchCancel()
		sr.SearchCancel = nil
	}
	sr.SearchMu.Unlock()

	if o.Text == "" {
		// An empty search leaves no generation.
		if sr.SessionGen != 0 {
			sr.Gens.Drop(sr.SessionGen)
			sr.SessionGen = 0
		}
		e.publishSearch(false)
		return 0, nil, false
	}

	if sr.SessionGen == 0 || !sr.Gens.Has(sr.SessionGen) {
		sr.SessionGen = sr.Gens.Begin(o)
	}
	sr.PendingHistory = o.Text
	return sr.SessionGen, e.searchDoc(), true
}

// SearchNow is SearchWith for a caller on the UI goroutine that needs the
// result at once, such as a replace right after typing. It searches in the
// caller's goroutine.
func (e *Editorleaf) SearchNow(o search.Options) {
	id, doc, ok := e.prepareSearch(o)
	if !ok {
		return
	}
	_, ms, err := search.Find(context.Background(), doc, o)
	if err != nil {
		gecore.Echo.AddText("Search: " + err.Error())
		return
	}
	e.meta.Search.Gens.Set(id, o, matchSpans(ms), e.editBuffer.Version())
	e.publishSearch(true)
}

// startSearch runs on the UI goroutine.
func (e *Editorleaf) startSearch(o search.Options) {
	sr := e.meta.Search
	eb := e.editBuffer

	id, doc, ok := e.prepareSearch(o)
	if !ok {
		return
	}
	rev := eb.Version()

	ctx, cancel := context.WithCancel(context.Background())
	sr.SearchMu.Lock()
	sr.SearchCancel = cancel
	sr.SearchMu.Unlock()

	go func() {
		_, ms, err := search.Find(ctx, doc, o)
		if ctx.Err() != nil {
			return
		}

		dispatchUI(func() {
			if ctx.Err() != nil || e.editBuffer != eb || e.meta.Search != sr {
				return // superseded, or the pane shows another buffer now
			}
			if err != nil {
				// Typing a regexp is mostly an invalid regexp. Keep the
				// results that are shown.
				gecore.Echo.AddText("Search: " + err.Error())
				return
			}
			if eb.Version() != rev {
				// The buffer changed while searching: search again.
				e.startSearch(o)
				return
			}
			if !sr.Gens.Set(id, o, matchSpans(ms), rev) {
				return // the generation was removed meanwhile
			}
			sr.CurrentSearchIndex = -1
			e.publishSearch(true)
			if len(ms) == 0 {
				gecore.Echo.AddText("No search results")
			}
		})
	}()
}

// onBufferChangeForSearch makes the results follow an edit of the buffer.
// It is a change listener, so it runs on the UI goroutine for every change,
// also for undo and redo.
func (e *Editorleaf) onBufferChangeForSearch(c editbuffer.Change) {
	sr := e.meta.Search
	if sr == nil || c.Version <= sr.AdjustedVersion {
		return // panes of one buffer share the results: apply a change once
	}
	sr.AdjustedVersion = c.Version
	if sr.Batch {
		return
	}
	if !sr.InReplace {
		sr.CurrentSearchIndex = -1
	}
	if sr.Gens.Len() == 0 {
		return
	}

	switch c.Kind {
	case editbuffer.ChangeReset:
		sr.Gens.Clear()
		sr.SessionGen = 0
	case editbuffer.ChangeInsert:
		sr.Gens.Update(func(s *search.Span) bool {
			// Text typed right after a result is not a part of it.
			atEnd := s.Start != s.End && s.End == toSearchPos(c.Start)
			start := toRowsPos(s.Start)
			start.AdjustForInsertion(c.Start, c.NewEnd)
			s.Start = toSearchPos(start)
			if !atEnd {
				end := toRowsPos(s.End)
				end.AdjustForInsertion(c.Start, c.NewEnd)
				s.End = toSearchPos(end)
			}
			return true
		})
	case editbuffer.ChangeDelete:
		sr.Gens.Update(func(s *search.Span) bool {
			start, end := toRowsPos(s.Start), toRowsPos(s.End)
			start.AdjustForDeletion(c.Start, c.OldEnd)
			end.AdjustForDeletion(c.Start, c.OldEnd)
			s.Start, s.End = toSearchPos(start), toSearchPos(end)
			return s.Start.Before(s.End) // a result that was deleted is gone
		})
	}
	e.publishSearch(false)
}

// ----------------------
// Move
// ----------------------

func (e *Editorleaf) currentSpans() []search.Span {
	g, ok := e.meta.Search.Gens.Current()
	if !ok {
		return nil
	}
	return g.Spans
}

func (e *Editorleaf) jumpToSpan(spans []search.Span, i int) {
	e.meta.Search.CurrentSearchIndex = i
	e.meta.RowsPos = toRowsPos(spans[i].Start)
}

// firstSpanAtOrAfter returns the index of the first span that starts at or
// after pos (len(spans) if there is none).
func firstSpanAtOrAfter(spans []search.Span, pos search.Pos) int {
	return sort.Search(len(spans), func(i int) bool { return !spans[i].Start.Before(pos) })
}

func (e *Editorleaf) MoveNextFoundWord() {
	spans := e.currentSpans()
	n := len(spans)
	if n == 0 {
		gecore.Echo.AddText("No search results")
		return
	}

	cur := toSearchPos(e.meta.RowsPos)
	i := firstSpanAtOrAfter(spans, cur)
	if i < n && spans[i].Start == cur {
		i++ // already on a result: the next one
	}
	if i >= n {
		i = n - 1
	}
	gecore.Echo.AddText(fmt.Sprintf("Search result %d/%d", i+1, n))
	e.jumpToSpan(spans, i)
}

func (e *Editorleaf) MovePrevFoundWord() {
	spans := e.currentSpans()
	n := len(spans)
	if n == 0 {
		gecore.Echo.AddText("No search results")
		return
	}

	cur := toSearchPos(e.meta.RowsPos)
	i := max(firstSpanAtOrAfter(spans, cur)-1, 0)
	gecore.Echo.AddText(fmt.Sprintf("Search result %d/%d", i+1, n))
	e.jumpToSpan(spans, i)
}

// ----------------------
// Replace
// ----------------------

// ReplaceCurrent replaces the result at the cursor, or the first one after it,
// of the current generation and moves to the next result. template is the
// replacement; with regex, $1 and ${name} are expanded. It is one undo step.
// It returns false if nothing was replaced.
func (e *Editorleaf) ReplaceCurrent(template string, regex bool) bool {
	if e.editBuffer.IsReadonly() {
		gecore.Echo.AddText("Buffer is read-only")
		return false
	}
	sr := e.meta.Search
	g, ok := sr.Gens.Current()
	if !ok || len(g.Spans) == 0 {
		gecore.Echo.AddText("No search results")
		return false
	}
	i := firstSpanAtOrAfter(g.Spans, toSearchPos(e.meta.RowsPos))
	if i >= len(g.Spans) {
		gecore.Echo.AddText("No more search results after the cursor")
		return false
	}
	span := g.Spans[i]

	text := template
	if regex && strings.Contains(template, "$") {
		// The groups are needed: find the match again in the current text.
		doc := e.searchDoc()
		re, ms, err := search.Find(context.Background(), doc, g.Options)
		if err == nil {
			for _, m := range ms {
				if m.Start == span.Start {
					text = string(m.Expand(re, doc, template, true))
					break
				}
			}
		}
	}

	undo := e.editBuffer.UndoAction
	first := undo.Index()
	undo.Seal() // do not merge into what the user typed before

	sr.InReplace = true
	e.replaceRange(toRowsPos(span.Start), toRowsPos(span.End), text)
	sr.InReplace = false

	undo.MergeSince(first) // delete + insert is one undo step

	// The replaced result is gone, so index i is the next result now.
	spans := e.currentSpans()
	if i < len(spans) {
		e.jumpToSpan(spans, i)
		gecore.Echo.AddText(fmt.Sprintf("Replaced (%d left)", len(spans)-i))
	} else {
		sr.CurrentSearchIndex = -1
		gecore.Echo.AddText("Replaced (no more search results)")
	}
	return true
}

// ReplaceAll replaces every result of the current generation, as one undo
// step. The generation is used up by it, the other generations are searched
// again. It returns the number of replacements.
func (e *Editorleaf) ReplaceAll(template string, regex bool) int {
	if e.editBuffer.IsReadonly() {
		gecore.Echo.AddText("Buffer is read-only")
		return 0
	}
	sr := e.meta.Search
	g, ok := sr.Gens.Current()
	if !ok || len(g.Spans) == 0 {
		gecore.Echo.AddText("No search results")
		return 0
	}

	// Search again: what is replaced is what the text matches now.
	doc := e.searchDoc()
	re, ms, err := search.Find(context.Background(), doc, g.Options)
	if err != nil {
		gecore.Echo.AddText("Replace: " + err.Error())
		return 0
	}
	if len(ms) == 0 {
		gecore.Echo.AddText("No search results")
		return 0
	}
	texts := make([]string, len(ms))
	for i, m := range ms {
		texts[i] = string(m.Expand(re, doc, template, regex))
	}

	undo := e.editBuffer.UndoAction
	func() {
		first := undo.Index()
		undo.Seal() // do not merge into what the user typed before

		sr.Batch = true
		defer func() { sr.Batch = false }()
		for j := len(ms) - 1; j >= 0; j-- { // bottom to top: the positions stay valid
			e.replaceRange(toRowsPos(ms[j].Start), toRowsPos(ms[j].End), texts[j])
		}

		undo.MergeSince(first) // one undo step for all
	}()

	sr.Gens.Drop(g.ID)
	if sr.SessionGen == g.ID {
		sr.SessionGen = 0
	}
	sr.CurrentSearchIndex = -1
	e.refreshGenerations()

	gecore.Echo.AddText(fmt.Sprintf("Replaced %d", len(ms)))
	return len(ms)
}

// refreshGenerations searches the buffer again for every generation.
func (e *Editorleaf) refreshGenerations() {
	sr := e.meta.Search
	gens := sr.Gens.Snapshot()
	if len(gens) == 0 {
		e.publishSearch(false)
		return
	}

	doc := e.searchDoc()
	rev := e.editBuffer.Version()
	for _, g := range gens {
		_, ms, err := search.Find(context.Background(), doc, g.Options)
		if err != nil {
			ms = nil
		}
		sr.Gens.Set(g.ID, g.Options, matchSpans(ms), rev)
	}
	e.publishSearch(true)
}
