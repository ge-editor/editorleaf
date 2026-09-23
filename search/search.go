package search

import (
	"context"
	"sync"

	"github.com/ge-editor/editorleaf/highlight"
	"github.com/ge-editor/utils"
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
	// Results []SearchResult // highlight に Store する為、ここには定義しない
	Index              int
	CurrentSearchIndex int

	SearchMu     sync.Mutex
	SearchCancel context.CancelFunc

	History utils.History[string]
}
