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
)

type Span struct {
	Start    rows.RowsPos
	End      rows.RowsPos
	Color    tcell.Style
	Priority int
}

// 例えば
// SpanTemplate interface を実装した Search struct の場合は
// 独自に Matches [][]string 等を持つ
type SpanTemplate interface {
	GetSpan() *Span
}
