package highlight

import (
	"github.com/gdamore/tcell/v3"

	"github.com/ge-editor/editorleaf/editbuffer/rows"
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
	Start         rows.RowsPos
	End           rows.RowsPos
	Color         tcell.Style
	ColorIfActive tcell.Style // For example, when the cursor is over a span
	Priority      int
}

// 例えば
// SpanTemplate interface を実装した Search struct の場合は
// 独自に Matches [][]string 等を持つ
type SpanTemplate interface {
	GetSpan() *Span
}
