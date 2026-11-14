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

	// Gens are the results of the search modes, one generation per mode.
	// They stay after the mode ends and overlap until cleared.
	Gens Generations

	// SessionGen is the generation of the search mode that is running; 0 when
	// the next SearchText has to begin a new one. UI goroutine only.
	SessionGen int
	// Overlapped is true while results of different generations overlap.
	Overlapped bool
	// PendingHistory is the text of the running search; it enters History
	// when the next search mode begins (not at every key stroke).
	PendingHistory string

	// AdjustedVersion is the buffer version the generations follow. Panes that
	// show the same buffer share one SearchResults, so a change must be
	// applied once only.
	AdjustedVersion uint64
	// Batch is set while many edits are made at once (replace all); the
	// generations are searched again afterwards instead of following each edit.
	Batch bool
	// InReplace is set while one replacement is made, so that the position
	// in the results is kept.
	InReplace bool

	SearchMu     sync.Mutex
	SearchCancel context.CancelFunc

	History utils.History[string]
}
