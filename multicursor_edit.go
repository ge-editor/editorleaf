package editorleaf

import (
	"unicode/utf8"

	"github.com/ge-editor/editorleaf/editbuffer"
)

// Editing at the cursors of the multicursor mode (when the definition is
// finished). The real cursor is not shown; it only follows one of the
// cursors so that the view scrolls to it (followMultiCursor). See
// multicursor.go.

// MultiCursorDirection is where MultiCursorMove moves the cursors.
type MultiCursorDirection int

const (
	MultiCursorLeft MultiCursorDirection = iota
	MultiCursorRight
	MultiCursorUp
	MultiCursorDown
	MultiCursorHome // start of the row
	MultiCursorEnd  // end of the row
)

// EditAtMultiCursors runs edit once for every cursor, with the real cursor
// (e.meta.RowsPos) placed at the cursor. edit is an ordinary editing command
// such as DeleteRune, KillLine, Autoindent or Yank, so there is no second
// implementation of them for the multicursor mode. It works for the commands
// that only look at the text and e.meta.RowsPos; the commands that look at
// the layout of the screen (the cursor movement commands, InsertTab, ...) do
// not work for cursors that may be out of the view.
//
// The cursors are done from the last one to the first so that an edit does
// not move the cursors that are still to be done. The real cursor is put
// back afterwards, and then follows the last cursor (followMultiCursor). The
// edits are one step for undo.
//
// The cursors follow the edits by themselves: adjustMultiCursors is called
// by syncCursorAndBufferForEdit, so after typing a character every cursor is
// behind the new character, and cursors that meet are merged.
func (e *Editorleaf) EditAtMultiCursors(edit func()) {
	if !e.IsMultiCursorFinished() || len(e.multi.positions) == 0 {
		return
	}
	cursors := e.MultiCursorPositions() // the edits change e.multi.positions

	screenPos := e.meta.ScreenPos
	prevScreenPos := e.meta.PrevScreenPos
	e.multi.realPos = e.meta.RowsPos
	e.multi.trackReal = true

	undo := e.editBuffer.UndoAction
	first := undo.Index()
	undo.Seal() // do not merge into what the user did before

	for i := len(cursors) - 1; i >= 0; i-- {
		e.meta.RowsPos = cursors[i]
		edit()
	}
	undo.MergeSince(first) // one undo step for all the cursors

	e.multi.trackReal = false
	e.meta.RowsPos = e.multi.realPos
	e.meta.ClampRowsPos(e.editBuffer)
	e.meta.ScreenPos = screenPos
	e.meta.PrevScreenPos = prevScreenPos

	// Show the last cursor (the others may be out of the view as well).
	if n := len(e.multi.positions); n > 0 {
		e.followMultiCursor(e.multi.positions[n-1])
	}
}

// followMultiCursor puts the real cursor at target so that the view shows
// it. The layout looks at the real cursor only, so this is how the view is
// scrolled. The screen row is estimated from the distance in rows (wrapped
// rows are not counted); the cursor is centered when it would be out of the
// view.
func (e *Editorleaf) followMultiCursor(target editbuffer.RowsPos) {
	cur := e.meta.RowsPos
	if target == cur {
		return
	}

	y := e.meta.ScreenPos.Row + (target.RowIndex - cur.RowIndex)
	if y < 0 || y >= e.editArea.Height {
		y = (e.editArea.Height - 1) / 2
	}

	e.meta.RowsPos = target
	e.meta.ClampRowsPos(e.editBuffer)
	e.meta.ScreenPos.Row = max(y, 0)
	e.meta.PrevScreenPos.Col = target.ColIndex
}

// MultiCursorInsertTab inserts what the Tab key inserts: a tab, or spaces as
// wide as a tab when the mode uses soft tabs. (InsertTab computes the width
// from the screen column, which a cursor out of the view does not have.)
func (e *Editorleaf) MultiCursorInsertTab() {
	unit := e.indentUnit()
	e.EditAtMultiCursors(func() { e.InsertString(unit) })
}

// MultiCursorMove moves every cursor. Cursors that meet are merged.
func (e *Editorleaf) MultiCursorMove(dir MultiCursorDirection) {
	if !e.IsMultiCursorFinished() {
		return
	}

	moved := make([]editbuffer.RowsPos, 0, len(e.multi.positions))
	for _, p := range e.multi.positions {
		moved = append(moved, e.movedPos(p, dir))
	}
	e.multi.positions = normalizeCursorPositions(moved)
	e.applyMultiCursor()

	// Show the cursor that goes ahead.
	if n := len(e.multi.positions); n > 0 {
		switch dir {
		case MultiCursorLeft, MultiCursorUp, MultiCursorHome:
			e.followMultiCursor(e.multi.positions[0])
		default:
			e.followMultiCursor(e.multi.positions[n-1])
		}
	}
}

// --------------------

// rowBytes returns the text of a row (without the line break).
func (e *Editorleaf) rowBytes(rowIndex int) ([]byte, bool) {
	rows := e.editBuffer.Rows()
	if rowIndex < 0 || rowIndex >= rows.Length() {
		return nil, false
	}
	row := rows.Row(rowIndex)
	if row == nil {
		return nil, false
	}
	return []byte(*row), true
}

// previousPos is the position before pos: one character back, or the end of
// the previous row.
func (e *Editorleaf) previousPos(pos editbuffer.RowsPos) (editbuffer.RowsPos, bool) {
	row, ok := e.rowBytes(pos.RowIndex)
	if !ok {
		return pos, false
	}
	col := min(pos.ColIndex, len(row))
	if col > 0 {
		_, size := utf8.DecodeLastRune(row[:col])
		return editbuffer.RowsPos{RowIndex: pos.RowIndex, ColIndex: col - size}, true
	}
	if prev, ok := e.rowBytes(pos.RowIndex - 1); ok {
		return editbuffer.RowsPos{RowIndex: pos.RowIndex - 1, ColIndex: len(prev)}, true
	}
	return pos, false
}

// nextPos is the position after pos: one character on, or the start of the
// next row.
func (e *Editorleaf) nextPos(pos editbuffer.RowsPos) (editbuffer.RowsPos, bool) {
	row, ok := e.rowBytes(pos.RowIndex)
	if !ok {
		return pos, false
	}
	col := min(pos.ColIndex, len(row))
	if col < len(row) {
		_, size := utf8.DecodeRune(row[col:])
		return editbuffer.RowsPos{RowIndex: pos.RowIndex, ColIndex: col + size}, true
	}
	if _, ok := e.rowBytes(pos.RowIndex + 1); ok {
		return editbuffer.RowsPos{RowIndex: pos.RowIndex + 1, ColIndex: 0}, true
	}
	return pos, false
}

// movedPos is where a cursor at pos goes. Up and Down keep the number of
// characters from the start of the row (not the screen column).
func (e *Editorleaf) movedPos(pos editbuffer.RowsPos, dir MultiCursorDirection) editbuffer.RowsPos {
	switch dir {
	case MultiCursorLeft:
		if p, ok := e.previousPos(pos); ok {
			return p
		}
	case MultiCursorRight:
		if p, ok := e.nextPos(pos); ok {
			return p
		}
	case MultiCursorHome:
		return editbuffer.RowsPos{RowIndex: pos.RowIndex, ColIndex: 0}
	case MultiCursorEnd:
		if row, ok := e.rowBytes(pos.RowIndex); ok {
			return editbuffer.RowsPos{RowIndex: pos.RowIndex, ColIndex: len(row)}
		}
	case MultiCursorUp, MultiCursorDown:
		row, ok := e.rowBytes(pos.RowIndex)
		if !ok {
			break
		}
		target := pos.RowIndex - 1
		if dir == MultiCursorDown {
			target = pos.RowIndex + 1
		}
		targetRow, ok := e.rowBytes(target)
		if !ok {
			break
		}

		runes := utf8.RuneCount(row[:min(pos.ColIndex, len(row))])
		col := 0
		for ; runes > 0 && col < len(targetRow); runes-- {
			_, size := utf8.DecodeRune(targetRow[col:])
			col += size
		}
		return editbuffer.RowsPos{RowIndex: target, ColIndex: col}
	}
	return pos
}
