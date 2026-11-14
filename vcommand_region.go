package editorleaf

import (
	"fmt"
	"strings"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/gecore"
	"github.com/ge-editor/gelog"
)

// --------------------
// Indent / Outdent region
// --------------------

// regionRows returns the first and the last row of the lines between the
// last mark and the cursor. As in most editors, a region that ends at the
// very start of a row does not include that row.
func (e *Editorleaf) regionRows() (first, last int, ok bool) {
	mark := e.findLastByEditBuffer()
	if mark == nil {
		gecore.Echo.AddText("The mark is not set now, so there is no region")
		return 0, 0, false
	}
	a, b := mark.RowsPos, e.meta.RowsPos
	if a == b {
		gecore.Echo.AddText("Mark and cursor position are the same, so there is no region")
		return 0, 0, false
	}
	if cmpRowsPos(a, b) > 0 {
		a, b = b, a
	}

	first, last = a.RowIndex, b.RowIndex
	if last > first && b.ColIndex == 0 {
		last--
	}
	return first, last, true
}

// indentUnit is what Indent inserts at the start of a line: a tab, or
// spaces as wide as a tab when the mode uses soft tabs.
func (e *Editorleaf) indentUnit() string {
	if (*e.editBuffer.GetLangMode()).GetSoftTab() {
		return strings.Repeat(" ", e.tabWidth())
	}
	return "\t"
}

func (e *Editorleaf) tabWidth() int {
	if w := e.editBuffer.GetTabWidth(); w > 0 {
		return w
	}
	return 4
}

// IndentRegion indents the lines between the cursor and the mark by one
// level (C-x >). Empty lines are left empty. The cursor and the marks stay
// at the same text, so the region can be indented again.
func (e *Editorleaf) IndentRegion() {
	first, last, ok := e.regionRows()
	if !ok {
		return
	}

	unit := e.indentUnit()
	cursor := e.meta.RowsPos

	undo := e.editBuffer.UndoAction
	start := undo.Index()
	undo.Seal() // do not merge into what the user typed before

	changed := 0
	for row := first; row <= last; row++ {
		line := e.editBuffer.Rows().Row(row)
		if line == nil || line.Length() == 0 {
			continue
		}

		e.meta.RowsPos = editbuffer.RowsPos{RowIndex: row, ColIndex: 0}
		e.InsertString(unit)
		if row == cursor.RowIndex {
			cursor.ColIndex += len(unit)
		}
		changed++
	}
	undo.MergeSince(start) // one undo step for the whole region

	e.SetRowsPos(cursor)
	e.meta.PrevScreenPos.Col = cursor.ColIndex
	gecore.Echo.AddText(fmt.Sprintf("Indented %d lines", changed))
}

// outdentBytes returns how many bytes Outdent removes from the start of
// the row: one tab, or as many leading spaces as a tab is wide (at most).
func (e *Editorleaf) outdentBytes(row int) int {
	line := e.editBuffer.Rows().Row(row)
	if line == nil || line.Length() == 0 {
		return 0
	}

	b := []byte(*line)
	if b[0] == '\t' {
		return 1
	}
	n, width := 0, e.tabWidth()
	for n < len(b) && n < width && b[n] == ' ' {
		n++
	}
	return n
}

// OutdentRegion outdents the lines between the cursor and the mark by one
// level (C-x <). Lines that do not start with a tab or a space are left
// as they are.
func (e *Editorleaf) OutdentRegion() {
	first, last, ok := e.regionRows()
	if !ok {
		return
	}

	cursor := e.meta.RowsPos

	undo := e.editBuffer.UndoAction
	start := undo.Index()
	undo.Seal() // do not merge into what the user typed before

	changed := 0
	for row := first; row <= last; row++ {
		n := e.outdentBytes(row)
		if n == 0 {
			continue
		}

		e.deleteRegion(
			editbuffer.RowsPos{RowIndex: row, ColIndex: 0},
			editbuffer.RowsPos{RowIndex: row, ColIndex: n},
		)
		if row == cursor.RowIndex {
			cursor.ColIndex = max(0, cursor.ColIndex-n)
		}
		changed++
	}
	undo.MergeSince(start) // one undo step for the whole region

	e.SetRowsPos(cursor)
	e.meta.PrevScreenPos.Col = cursor.ColIndex
	gecore.Echo.AddText(fmt.Sprintf("Outdented %d lines", changed))
}

// --------------------
// Delete word
// --------------------

// wordMoveTarget returns where Shift+Right (forward) or Shift+Left would
// take the cursor, and leaves the cursor where it was. It runs the very
// same movement (MoveCursorNextWord / MoveCursorPreviousWord) so that what
// is deleted is always what the cursor would have moved over.
func (e *Editorleaf) wordMoveTarget(forward bool) editbuffer.RowsPos {
	pos := e.meta.RowsPos
	screenPos := e.meta.ScreenPos
	prevScreenPos := e.meta.PrevScreenPos

	if forward {
		e.MoveCursorNextWord()
	} else {
		e.MoveCursorPreviousWord()
	}
	target := e.meta.RowsPos

	e.meta.RowsPos = pos
	e.meta.ScreenPos = screenPos
	e.meta.PrevScreenPos = prevScreenPos
	return target
}

// DeleteWordForward deletes from the cursor to where Shift+Right would
// move it (Shift+Delete).
func (e *Editorleaf) DeleteWordForward() {
	start := e.meta.RowsPos
	stop := e.wordMoveTarget(true)
	e.deleteWordRegion(start, stop)
}

// DeleteWordBackward deletes from where Shift+Left would move the cursor
// to the cursor (Shift+Backspace).
func (e *Editorleaf) DeleteWordBackward() {
	stop := e.meta.RowsPos
	start := e.wordMoveTarget(false)
	e.deleteWordRegion(start, stop)
}

func (e *Editorleaf) deleteWordRegion(start, stop editbuffer.RowsPos) {
	gelog.Debug("deleteWordRegion",
		"start", fmt.Sprintf("%d:%d", start.RowIndex, start.ColIndex),
		"stop", fmt.Sprintf("%d:%d", stop.RowIndex, stop.ColIndex))

	if cmpRowsPos(start, stop) >= 0 {
		return // the cursor could not move (the movement said so)
	}
	if e.deleteRegion(start, stop) == nil {
		return
	}
	e.meta.PrevScreenPos.Col = start.ColIndex
}
