package highlight

import "github.com/ge-editor/editorleaf/editbuffer/rows"

func NewHighlights() *Highlights {
	return &Highlights{
		Spans: make([]*SpanTemplate, 0),
		Index: 0,
	}
}

type Highlights struct {
	Spans []*SpanTemplate
	Index int
}

func (h *Highlights) SpansLength() int {
	return len(h.Spans)
}

func (h *Highlights) GetSpan(index int) *Span {
	return (*h.Spans[index]).GetSpan()
}

/*
func (h *Highlights) First() {
	h.Index = 0
}

func (h *Highlights) Current() *Span {
	return (*h.Spans[h.Index]).GetSpan()
}

func (h *Highlights) Next() bool {
	if h.Index+1 >= len(h.Spans) {
		return false
	}
	h.Index++
	return true
}
*/

// return SpanIndex
func (h *Highlights) MatcheFirstRegenSpanIndex(
	pos rows.RowsPos,
	startSpansIndex int,
) int {
	for i := startSpansIndex; i < len(h.Spans); i++ {
		span := (*h.Spans[i]).GetSpan()

		if PosInSpan(pos, span) {
			return i
		}
	}

	// return len(h.Spans)
	return -1 // not found
}

func PosInSpan(r rows.RowsPos, s *Span) bool {
	if r.RowIndex < s.Start.RowIndex ||
		r.RowIndex > s.End.RowIndex {
		return false
	}

	if r.RowIndex == s.Start.RowIndex &&
		r.ColIndex < s.Start.ColIndex {
		return false
	}

	if r.RowIndex == s.End.RowIndex &&
		r.ColIndex >= s.End.ColIndex {
		return false
	}

	return true
}
