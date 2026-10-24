package highlight

import (
	"sync"
	"testing"

	"github.com/ge-editor/editorleaf/editbuffer"
)

func span(row, start, end int) *SpanTemplate {
	var st SpanTemplate = &Span{
		Start: editbuffer.RowsPos{RowIndex: row, ColIndex: start},
		End:   editbuffer.RowsPos{RowIndex: row, ColIndex: end},
	}
	return &st
}

func TestGetSpanOutOfRangeIsAnEmptySpan(t *testing.T) {
	h := NewHighlights()
	h.SetSpans([]*SpanTemplate{span(0, 0, 3)})

	// The caller checked SpansLength, then the spans were replaced.
	h.SetSpans(nil)
	got := h.GetSpan(0)
	if got == nil {
		t.Fatal("GetSpan must not return nil")
	}
	if PosInSpan(editbuffer.RowsPos{}, got) {
		t.Fatal("an out-of-range span must match no position")
	}
}

func TestMatchSkipsPlaceholders(t *testing.T) {
	h := NewHighlights()
	h.SetSpan(2, span(0, 0, 3)) // indexes 0 and 1 are nil placeholders

	if got := h.MatcheFirstRegenSpanIndex(editbuffer.RowsPos{RowIndex: 0, ColIndex: 1}, 0); got != 2 {
		t.Fatalf("MatcheFirstRegenSpanIndex = %d, want 2", got)
	}
	if got := h.MatcheFirstRegenSpanIndex(editbuffer.RowsPos{RowIndex: 5}, -3); got != -1 {
		t.Fatalf("MatcheFirstRegenSpanIndex = %d, want -1", got)
	}
}

func TestLayerAccessorsOutOfRange(t *testing.T) {
	l := NewHighlightsLayer()
	if l.SpanLength(-1) != 0 || l.SpanLength(100) != 0 {
		t.Fatal("SpanLength out of range must be 0")
	}
	if l.GetSpanTemplate(-1, 0) != nil || l.GetSpanTemplate(LayerSyntax, 5) != nil {
		t.Fatal("GetSpanTemplate out of range must be nil")
	}
	l.SetSpan(LayerSearch, 1, span(0, 0, 1))
	if l.SpanLength(LayerSearch) != 2 || l.GetSpanTemplate(LayerSearch, 0) != nil {
		t.Fatal("SetSpan should pad with a nil placeholder")
	}
}

// The draw goroutine reads while the event loop replaces the spans. Run with
// -race for the full check; without it this still must not panic.
func TestConcurrentReadAndReplace(t *testing.T) {
	l := NewHighlightsLayer()
	h := l.Highlights(LayerSyntax)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() { // event loop
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			spans := make([]*SpanTemplate, 0, 50)
			for j := 0; j < 50; j++ {
				spans = append(spans, span(j, 0, 10))
			}
			h.SetSpans(spans)
			if i%7 == 0 {
				h.Clear()
			}
		}
	}()
	go func() { // draw goroutine
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			n := h.SpansLength()
			for j := 0; j < n; j++ {
				_ = h.GetSpan(j)
			}
			_ = h.MatcheFirstRegenSpanIndex(editbuffer.RowsPos{RowIndex: 25, ColIndex: 3}, 0)
			_ = l.GetSpanTemplate(LayerSyntax, 10)
		}
	}()
	wg.Wait()
}
