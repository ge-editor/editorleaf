package highlight

import (
	"github.com/gdamore/tcell/v3"
	"github.com/ge-editor/editorleaf/editbuffer"
)

/*
search
syntax
mark
selection
diagnostic
bracket matching
spell check
*/

const (
	LayerSyntax    = iota // buffer
	LayerSemantic         // buffer
	LayerSearch           // meta
	LayerSelection        // meta
	LayerCursor           // meta, multi cursor
)

type Span struct {
	Start         editbuffer.RowsPos
	End           editbuffer.RowsPos
	Color         tcell.Style
	ColorIfActive tcell.Style // For example, when the cursor is over a span
	Priority      int
}

// GetSpan implements SpanTemplate. It lets a *Span be used directly as a
// SpanTemplate when the caller has no extra per-match data to keep
// (unlike e.g. search, which keeps Matches [][]string alongside its spans).
func (s *Span) GetSpan() *Span { return s }

// 例えば
// SpanTemplate interface を実装した Search struct の場合は
// 独自に Matches [][]string 等を持つ
type SpanTemplate interface {
	GetSpan() *Span
}
