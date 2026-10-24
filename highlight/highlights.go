package highlight

import (
	"sync"
	"sync/atomic"

	"github.com/ge-editor/editorleaf/editbuffer"
)

func NewHighlights() *Highlights {
	return &Highlights{
		Spans: make([]*SpanTemplate, 0),
		Index: 0,
	}
}

// Highlights is the spans of one priority of a HighlightsLayer.
//
// The editor draws on its own goroutine while the event loop replaces spans
// (syntax updates, search), so every method locks. The spans themselves are
// treated as immutable once published. Prefer SetSpans over Clear followed by
// AppendSpan: it swaps the whole slice at once, so a concurrent draw never
// sees a half-filled layer.
type Highlights struct {
	mu      sync.RWMutex
	Spans   []*SpanTemplate // guarded by mu; use the methods
	Index   int
	version atomic.Uint64 // bumped on every change of Spans
}

func (h *Highlights) Version() uint64 { return h.version.Load() }

func (h *Highlights) SpansLength() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.Spans)
}

// GetSpan returns the span at index. An index that is out of range, which
// can happen when the spans were replaced after the caller checked
// SpansLength, gives an empty span that matches no position.
func (h *Highlights) GetSpan(index int) *Span {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return spanAt(h.Spans, index)
}

func (h *Highlights) AppendSpan(span *SpanTemplate) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Spans = append(h.Spans, span)
	h.version.Add(1)
}

// SetSpans replaces all spans at once. The caller must not modify spans
// afterwards.
func (h *Highlights) SetSpans(spans []*SpanTemplate) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Spans = spans
	h.version.Add(1)
}

// SetSpan sets the span at index, padding with nil placeholders if needed.
func (h *Highlights) SetSpan(index int, span *SpanTemplate) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for len(h.Spans) <= index {
		h.Spans = append(h.Spans, nil)
	}
	h.Spans[index] = span
	h.version.Add(1)
}

func (h *Highlights) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	// A new slice, not Spans[:0]: a reader may still hold the old one.
	h.Spans = nil
	h.version.Add(1)
}

// spanTemplate returns the span template at index, or nil.
func (h *Highlights) spanTemplate(index int) *SpanTemplate {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if index < 0 || index >= len(h.Spans) {
		return nil
	}
	return h.Spans[index]
}

// spanAt returns the span at index of spans, or an empty span (matching no
// position) if there is none.
func spanAt(spans []*SpanTemplate, index int) *Span {
	if index < 0 || index >= len(spans) || spans[index] == nil || *spans[index] == nil {
		return &Span{}
	}
	if span := (*spans[index]).GetSpan(); span != nil {
		return span
	}
	return &Span{}
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
/* func (h *Highlights) MatcheFirstRegenSpanIndex(
	pos editbuffer.RowsPos,
	startSpansIndex int,
) int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for i := max(startSpansIndex, 0); i < len(h.Spans); i++ {
		if h.Spans[i] == nil || *h.Spans[i] == nil {
			continue // placeholder from SetSpan
		}
		if PosInSpan(pos, spanAt(h.Spans, i)) {
			return i
		}
		if posBefore(pos, s.Start) {
			return -1
		} // span は昇順なので以降はヒットしない
	}

	// return len(h.Spans)
	return -1 // not found
} */

func (h *Highlights) MatcheFirstRegenSpanIndex(
	pos editbuffer.RowsPos,
	startSpansIndex int,
) int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for i := max(startSpansIndex, 0); i < len(h.Spans); i++ {
		if h.Spans[i] == nil || *h.Spans[i] == nil {
			continue // placeholder from SetSpan
		}
		s := spanAt(h.Spans, i)
		if PosInSpan(pos, s) {
			return i
		}
		// spans are sorted by Start: once a span starts after pos,
		// no later span can contain it.
		if posBefore(pos, s.Start) {
			return -1
		}
	}

	return -1 // not found
}

func PosInSpan(r editbuffer.RowsPos, s *Span) bool {
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

// posBefore reports whether a is strictly before b (row-major order).
func posBefore(a, b editbuffer.RowsPos) bool {
	if a.RowIndex != b.RowIndex {
		return a.RowIndex < b.RowIndex
	}
	return a.ColIndex < b.ColIndex
}
