package search

import (
	"github.com/ge-editor/editorleaf/highlight"
)

// Implements highlight.SpanTemplate
type SearchResult struct {
	*highlight.Span

	SearchText string
	Matches    [][]string
}

func (s SearchResult) GetSpan() *highlight.Span {
	return s.Span
}

// ----------------------
// SearchResults
// ----------------------

type SearchResults struct {
	// Results []SearchResult // highlight に Store する
	Index int

	CurrentSearchIndex int
}

/*
func (s SearchResults) Highlights() *highlight.Highlights {
	h := highlight.NewHighlights()

	for _, result := range s.Results {
		h.Spans = append(h.Spans, result)
	}

	return h
}

func (s *SearchResults) FromHighlights(h *highlight.Highlights) {
	results := make([]SearchResult, 0, len(h.Spans))

	for _, span := range h.Spans {
		result, ok := span.(SearchResult)
		gelog.Debug("NIL?", "span", span, "result", result, "ok", ok)
		if !ok {
			continue
		}
		results = append(results, result)
	}

	s.Results = results
	s.Index = h.Index
}
*/

// Indexes -> Results

// Returns the first index of the found search position that matches the row index.
// Return -1, not found match position.
/*
func (s *SearchResults) GetFoundPosition(rowIndex int) int {
	for i := 0; i < len(s.Results); i++ {
		if s.Results[i].Start.RowIndex >= rowIndex {
			return i
		}
	}
	return -1
}
*/

/*
func (s *SearchResults) GetFindIndexes() []SearchResult {
	return s.Results
}
*/
