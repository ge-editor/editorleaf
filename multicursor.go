package editorleaf

import (
	"fmt"
	"sort"
	"sync/atomic"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/editorleaf/highlight"
	"github.com/ge-editor/gecore"
	"github.com/ge-editor/theme"
)

// Multicursor mode: places cursors in the text.
//
// The cursors are independent of the real cursor (the terminal cursor and
// Editorleaf.meta.RowsPos). Nothing in the layout or in the scrolling looks
// at them: the editor is drawn around the real cursor as always, and the
// multicursors are painted over the cells afterwards from the spans of
// highlight.LayerCursor (see paintRowOverlays).
//
// The mode has two phases:
//
//   - defining:  both the real cursor and the multicursors are shown, and the
//     real cursor can be moved to set or unset a cursor.
//   - finished:  only the multicursors are shown; the real cursor is hidden.
//     The text is edited at all of them. There is no way back to defining.
//
// The key handling is in the ge package (ge/multicursor_mode.go).

// multiCursor is the state of the mode in one Editorleaf. It is only touched
// by the UI goroutine; the draw goroutine reads the spans in the highlight
// layer (copied there by applyMultiCursor) and multiHideCursor.
type multiCursor struct {
	active    bool
	finished  bool
	positions []editbuffer.RowsPos // ascending, without duplicates

	// While an edit is made at the cursors (EditAtMultiCursors) the real
	// cursor is moved around; realPos keeps the text position it has to
	// return to, and is adjusted by every edit like the cursors are.
	trackReal bool
	realPos   editbuffer.RowsPos
}

// IsMultiCursor reports whether the multicursor mode is on in this editor.
func (e *Editorleaf) IsMultiCursor() bool { return e.multi.active }

// IsMultiCursorFinished reports whether the definition of the cursors is
// finished (only the multicursors are shown).
func (e *Editorleaf) IsMultiCursorFinished() bool { return e.multi.active && e.multi.finished }

// MultiCursorPositions returns a copy of the positions of the cursors.
func (e *Editorleaf) MultiCursorPositions() []editbuffer.RowsPos {
	return append([]editbuffer.RowsPos(nil), e.multi.positions...)
}

// BeginMultiCursor starts the mode in the defining phase, without cursors.
func (e *Editorleaf) BeginMultiCursor() {
	if e.multi.active {
		return
	}
	e.multi = multiCursor{active: true}
	e.applyMultiCursor()
	// The keys are shown by the mode as its usage (ge/multicursor_mode.go).
}

// EndMultiCursor leaves the mode and discards the cursors.
func (e *Editorleaf) EndMultiCursor() {
	if !e.multi.active && len(e.multi.positions) == 0 {
		return
	}
	e.multi = multiCursor{}
	e.applyMultiCursor()
}

// ToggleMultiCursor sets a cursor at the real cursor, or unsets the one that
// is there.
func (e *Editorleaf) ToggleMultiCursor() {
	if !e.multi.active || e.multi.finished {
		return
	}

	pos := e.meta.RowsPos
	for i, p := range e.multi.positions {
		if p == pos {
			e.multi.positions = append(e.multi.positions[:i:i], e.multi.positions[i+1:]...)
			e.applyMultiCursor()
			gecore.Echo.AddText(fmt.Sprintf("Cursor unset (%d)", len(e.multi.positions)))
			return
		}
	}

	e.multi.positions = normalizeCursorPositions(append(e.multi.positions[:len(e.multi.positions):len(e.multi.positions)], pos))
	e.applyMultiCursor()
	gecore.Echo.AddText(fmt.Sprintf("Cursor set (%d)", len(e.multi.positions)))
}

// FinishMultiCursor is Enter in the defining phase: the definition is
// finished, so only the multicursors are shown and the real cursor is hidden.
// The cursors cannot be edited after that (C-g and start again).
func (e *Editorleaf) FinishMultiCursor() {
	if !e.multi.active || e.multi.finished {
		return
	}

	if len(e.multi.positions) == 0 {
		gecore.Echo.AddText("Multicursor: no cursor is set yet (Space sets one)")
		return
	}
	e.multi.finished = true
	e.applyMultiCursor()
	gecore.Echo.AddText(fmt.Sprintf("%d cursors", len(e.multi.positions)))
}

// adjustMultiCursors keeps the cursors at the same text after an edit of the
// buffer (it is called for every editor of the edited buffer).
func (e *Editorleaf) adjustMultiCursors(sync syncType, start, end editbuffer.RowsPos) {
	if len(e.multi.positions) == 0 && !e.multi.trackReal {
		return
	}
	if cmpRowsPos(start, end) > 0 {
		start, end = end, start
	}

	if e.multi.trackReal {
		switch sync {
		case INSERT:
			e.multi.realPos.AdjustForInsertion(start, end)
		case DELETE:
			e.multi.realPos.AdjustForDeletion(start, end)
		}
	}
	if len(e.multi.positions) == 0 {
		return
	}

	positions := make([]editbuffer.RowsPos, len(e.multi.positions))
	copy(positions, e.multi.positions)
	for i := range positions {
		switch sync {
		case INSERT:
			positions[i].AdjustForInsertion(start, end)
		case DELETE:
			positions[i].AdjustForDeletion(start, end)
		}
	}
	e.multi.positions = normalizeCursorPositions(positions)
	e.applyMultiCursor()
}

// normalizeCursorPositions sorts the positions and removes duplicates.
func normalizeCursorPositions(positions []editbuffer.RowsPos) []editbuffer.RowsPos {
	sort.Slice(positions, func(i, j int) bool { return cmpRowsPos(positions[i], positions[j]) < 0 })

	out := positions[:0]
	for i, p := range positions {
		if i > 0 && p == positions[i-1] {
			continue
		}
		out = append(out, p)
	}
	return out
}

// applyMultiCursor hands the state to the draw goroutine: the cursors as the
// spans of highlight.LayerCursor, and whether the real cursor is shown.
func (e *Editorleaf) applyMultiCursor() {
	var hide int32
	if e.multi.active && e.multi.finished {
		hide = 1
	}
	atomic.StoreInt32(&e.multiHideCursor, hide)

	if e.meta == nil || e.meta.HighlightLayer == nil {
		return
	}
	hl := e.meta.HighlightLayer.Peek(highlight.LayerCursor)
	if hl == nil {
		return
	}

	spans := make([]*highlight.SpanTemplate, 0, len(e.multi.positions))
	for _, p := range e.multi.positions {
		var t highlight.SpanTemplate = &highlight.Span{
			Start:         p,
			End:           editbuffer.RowsPos{RowIndex: p.RowIndex, ColIndex: p.ColIndex + 1},
			Color:         theme.ColorDefault.Background(theme.ColorMultiCursorBackground),
			ColorIfActive: theme.ColorDefault.Background(theme.ColorMultiCursorBackground),
			Priority:      highlight.LayerCursor,
		}
		spans = append(spans, &t)
	}
	hl.SetSpans(spans)
}
