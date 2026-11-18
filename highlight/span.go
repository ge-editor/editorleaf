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

// The layers are of two kinds, and only one of them is ordered by this list.
//
//   - LayerSyntax to LayerSelection are text styles. FindHighlightSpan returns
//     the span of the highest layer that covers a position (one span; styles
//     are not merged), so for them the order of the constants is the priority.
//   - LayerCursor, LayerMark and LayerSpell are not text styles: they are
//     painted over the drawn cells (paintRowOverlays) and FindHighlightSpan
//     skips them. Their place in this list only gives each one its slot (see
//     NewHighlightsLayer). What is on top of what is the order of the paints in
//     paintRowOverlays, where the last paint wins: misspelled words, marks,
//     cursors.
const (
	LayerSyntax     = iota // buffer
	LayerSemantic          // buffer
	LayerDiagnostic        // buffer, LSP diagnostics (above syntax, below search)
	LayerSearch            // meta
	LayerSelection         // meta
	LayerCursor            // meta, multi cursor
	LayerMark              // meta, marks of the file. Painted over the syntax color by the editor; FindHighlightSpan skips it
	LayerSpell             // buffer, misspelled words. An underline painted over the cells (paintRowOverlays), like the marks; FindHighlightSpan skips it
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
