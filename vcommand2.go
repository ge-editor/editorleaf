package editorleaf

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"

	"github.com/atotto/clipboard"
	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/editorleaf/editbuffer/rows"
	"github.com/ge-editor/editorleaf/mark"
	"github.com/ge-editor/gecore"
	"github.com/ge-editor/gecore/define"
	"github.com/ge-editor/gecore/killbuffer"
	"github.com/ge-editor/gecore/screen"
	"github.com/ge-editor/gecore/tree"
	"github.com/ge-editor/gelog"
	"github.com/ge-editor/utils"
)

// ------------------------------------------------------------------
// Move cursor
// ------------------------------------------------------------------

// Move cursor one character forward.
func (e *Editorleaf) MoveCursorForward() {
	x, y := e.meta.Cx, e.meta.Cy

	lines := e.editBuffer.Rows
	line := lines.Row(e.meta.RowIndex)
	if line.IsColIndexAtRowEnd(e.meta.ColIndex) {
		if lines.IsRowIndexLastRow(e.meta.RowIndex) {
			gecore.Echo.AddText("End of buffer")
			return
		}
		y++
		e.meta.RowIndex++
		x = 0
		e.meta.ColIndex = 0
	} else {
		ch, size, ok := lines.Row(e.meta.RowIndex).DecodeRune(e.meta.ColIndex)
		if !ok {
			gelog.Error("error")
			panic("err")
		}
		w := e.locale.RuneWidth(ch)
		if !ok {
			gelog.Error("error")
		}
		if e.bsArray.OnEndOfLogicalRow(e.meta.RowIndex, e.meta.ColIndex) {
			y++
			x = 0
		} else {
			x += w
		}
		e.meta.ColIndex += size
	}

	e.meta.PrevCx = x
	e.moveCursor(x, y)
}

// Move cursor one character backward.
func (e *Editorleaf) MoveCursorBackward() {
	x, y := e.meta.Cx, e.meta.Cy

	if e.meta.ColIndex == 0 {
		if e.meta.RowIndex == 0 {
			gecore.Echo.AddText("Beginning of buffer")
			return
		}
		// previous line
		y--
		e.meta.RowIndex--

		prevBs := e.bsArray.LastBoundary(e.meta.RowIndex)
		// gelog.Debug("prev Boundary", fmt.Sprintf("StartLogicalRowByteIndex: %d, StopLogicalRowByteIndex: %d, LogicalRowWidth: %d, TotalCellWidth: %d, ", prevBs.StartLogicalRowByteIndex, prevBs.StopLogicalRowByteIndex, prevBs.LogicalRowWidth, prevBs.TotalCellWidth))

		// on newline
		x = prevBs.LogicalRowWidth - 1
		e.meta.ColIndex = prevBs.StopLogicalRowByteIndex - 1
	} else {
		before := e.bsArray.GetIndexOfLogicalRow(e.meta.RowIndex, e.meta.ColIndex)
		ch, _, colIndex, ok := e.editBuffer.Rows.Row(e.meta.RowIndex).DecodePrevRune(e.meta.ColIndex)
		if !ok {
			panic("2")
		}
		w := e.locale.RuneWidth(ch)
		e.meta.ColIndex = colIndex
		after := e.bsArray.GetIndexOfLogicalRow(e.meta.RowIndex, e.meta.ColIndex)
		if !ok {
			panic("3")
		}
		if after < before {
			y--
			x = e.bsArray.Boundary(e.meta.RowIndex, after).LogicalRowWidth - w
		} else {
			x -= w
		}
	}

	e.meta.PrevCx = x
	e.moveCursor(x, y)
}

// Move cursor to the next line.
func (e *Editorleaf) MoveCursorNextLine() {
	x, y := e.meta.Cx, e.meta.Cy
	// gelog.Debug("x", x, "y", y)

	indexOfLogicalRow := 0

	if e.bsArray.OnEndOfLogicalRow(e.meta.RowIndex, e.meta.ColIndex) { // last logical line
		if e.editBuffer.Rows.IsRowIndexLastRow(e.meta.RowIndex) {
			gecore.Echo.AddText("End of buffer")
			return
		}
		// move to next line
		e.meta.RowIndex++
	} else {
		// What index number in logic line?
		// and move to next logical line
		indexOfLogicalRow = e.bsArray.GetIndexOfLogicalRow(e.meta.RowIndex, e.meta.ColIndex) + 1
	}

	e.meta.ColIndex, x = e.getColumnIndexClosestToCursorXPosition(e.meta.RowIndex, indexOfLogicalRow, e.meta.PrevCx)
	// e.meta.ColIndex, x = 0, 0
	// e.meta.ColIndex = 0

	e.moveCursor(x, y+1)
	// gelog.Debug("x", x, "y", y)
}

// Move cursor to the previous row.
func (e *Editorleaf) MoveCursorPrevLine() {
	x, y := e.meta.Cx, e.meta.Cy

	// What index number in logical row?
	indexOfLogicalRow := e.bsArray.GetIndexOfLogicalRow(e.meta.RowIndex, e.meta.ColIndex)

	if indexOfLogicalRow == 0 { // first logical row
		if e.meta.RowIndex == 0 {
			gecore.Echo.AddText("Beginning of buffer")
			return
		}
		// move to prev row
		e.meta.RowIndex--
		// last logical row
		indexOfLogicalRow = e.bsArray.GetIndexOfLastLogicalRow(e.meta.RowIndex)
	} else {
		indexOfLogicalRow--
	}

	e.meta.ColIndex, x = e.getColumnIndexClosestToCursorXPosition(e.meta.RowIndex, indexOfLogicalRow, e.meta.PrevCx)

	e.moveCursor(x, y-1)
}

// Move cursor to the end of the line.
func (e *Editorleaf) MoveCursorEndOfLine() {
	// What index number in logic row?
	indexOfLogicalRow := e.bsArray.GetIndexOfLogicalRow(e.meta.RowIndex, e.meta.ColIndex)

	colLength := e.editBuffer.Rows.Row(e.meta.RowIndex).Length()
	e.meta.ColIndex = colLength

	// cursor display position y
	lastIndexOfLogicalRow := e.bsArray.GetIndexOfLastLogicalRow(e.meta.RowIndex)
	e.meta.Cy += lastIndexOfLogicalRow - indexOfLogicalRow

	// cursor display position x
	e.meta.Cx = e.bsArray.Boundary(e.meta.RowIndex, lastIndexOfLogicalRow).LogicalRowWidth
	e.meta.PrevCx = e.meta.Cx
}

// Move cursor to the beginning of the line.
// Consider indentation
func (e *Editorleaf) MoveCursorBeginningOfLine() {
	if e.meta.RowIndex == 0 && e.meta.ColIndex == 0 {
		gecore.Echo.AddText("Beginning of buffer")
		return
	}

	nowIndexOfLogicalRow := e.bsArray.GetIndexOfLogicalRow(e.meta.RowIndex, e.meta.ColIndex)
	rows := e.editBuffer.Rows
	indentedIndex := 0
	indentedWidth := 0
	for indentedIndex < rows.Row(e.meta.RowIndex).Length() {
		ch, size, _ := rows.Row(e.meta.RowIndex).DecodeRune(indentedIndex)
		if ch != ' ' && ch != '\t' {
			break
		}
		w := e.locale.RuneWidth(ch)
		indentedWidth += w
		indentedIndex += size
	}
	newIndexOfLogicalRow := e.bsArray.GetIndexOfLogicalRow(e.meta.RowIndex, indentedIndex)

	if e.meta.ColIndex == 0 || indentedIndex < e.meta.ColIndex {
		e.meta.ColIndex = indentedIndex
		e.meta.Cx = indentedWidth
	} else {
		e.meta.ColIndex = 0
		e.meta.Cx = 0
	}

	if newIndexOfLogicalRow < nowIndexOfLogicalRow {
		e.meta.Cy -= nowIndexOfLogicalRow - newIndexOfLogicalRow
	}
}

// Move cursor to the end of logical the line.
// At the end of a logical line, the cursor should at the beginning of the next logical line. so I see...
func (e *Editorleaf) MoveCursorEndOfLogicalLine() {
	// What index number in logic row?
	indexOfLogicalRow := e.bsArray.GetIndexOfLogicalRow(e.meta.RowIndex, e.meta.ColIndex)
	bo := e.bsArray.Boundary(e.meta.RowIndex, indexOfLogicalRow)

	if indexOfLogicalRow == e.bsArray.BoundariesLen(e.meta.RowIndex)-1 {
		e.meta.ColIndex = bo.StopLogicalRowByteIndex - 1
		e.meta.Cx = bo.LogicalRowWidth - 1
	} else {
		// Move to the first character of the next logical row
		e.meta.ColIndex = bo.StopLogicalRowByteIndex
		e.meta.Cy++
		e.meta.Cx = 0
	}

	e.meta.PrevCx = e.meta.Cx
}

// Move cursor to the beginning of the logical row.
// Consider logical row
// Consider indentation....
func (e *Editorleaf) MoveCursorBeginningOfLogicalLine() {
	if e.meta.RowIndex == 0 && e.meta.ColIndex == 0 {
		gecore.Echo.AddText("Beginning of buffer")
		return
	}

	nowIndexOfLogicalRow := e.bsArray.GetIndexOfLogicalRow(e.meta.RowIndex, e.meta.ColIndex)
	if nowIndexOfLogicalRow == 0 {
		e.MoveCursorBeginningOfLine()
		return
	}

	bo := e.bsArray.Boundary(e.meta.RowIndex, nowIndexOfLogicalRow)
	if e.meta.ColIndex == bo.StartLogicalRowByteIndex {
		if nowIndexOfLogicalRow == 1 {
			e.MoveCursorBeginningOfLine()
			return
		}

		bo := e.bsArray.Boundary(e.meta.RowIndex, nowIndexOfLogicalRow-1)
		e.meta.ColIndex = bo.StartLogicalRowByteIndex
		e.meta.Cy--
		return
	}
	e.meta.ColIndex = bo.StartLogicalRowByteIndex
	e.meta.Cx = 0
	e.meta.PrevCx = 0
}

func (e *Editorleaf) MoveCursorBeginningOfFile() {
	e.meta.RowIndex = 0
	e.meta.ColIndex = 0
	e.meta.Cy = 0
	e.meta.Cx = 0
}

func (e *Editorleaf) MoveCursorEndOfFile() {
	e.meta.RowIndex = e.editBuffer.Rows.Length() - 1
	lastBs := e.bsArray.LastBoundary(e.meta.RowIndex)
	e.meta.ColIndex = lastBs.StopLogicalRowByteIndex - 1 // left of the LF or EOF
	e.meta.Cx = lastBs.LogicalRowWidth - 1               // left of the LF or EOF
	e.meta.Cy = e.editArea.Height - e.verticalThreshold

	e.meta.PrevCx = e.meta.Cx
}

// Move view 'n' lines forward only if it's possible.
func (e *Editorleaf) MoveViewHalfForward() {
	n := e.screen.Height / 2

	logicalRowIndex := e.bsArray.GetIndexOfLogicalRow(
		e.meta.RowIndex,
		e.meta.ColIndex,
	)

	rowIndex := e.meta.RowIndex
	rowLength := e.bsArray.BoundariesLen(rowIndex)

	// Current position itself is not a distance to move.
	remaining := rowLength - logicalRowIndex - 1

	// The target is still in the current physical row.
	if n <= remaining {
		logicalRowIndex += n
	} else {
		// Move to the next physical row.
		n -= remaining + 1

		if rowIndex == e.editBuffer.Rows.Length()-1 {
			logicalRowIndex = rowLength - 1
		} else {
			rowIndex++

			for {
				rowLength = e.bsArray.BoundariesLen(rowIndex)

				if n < rowLength {
					logicalRowIndex = n
					break
				}

				n -= rowLength

				if rowIndex == e.editBuffer.Rows.Length()-1 {
					logicalRowIndex = rowLength - 1
					break
				}

				rowIndex++
			}
		}
	}

	e.meta.RowIndex = rowIndex

	e.meta.ColIndex, _ =
		e.getColumnIndexClosestToCursorXPosition(
			e.meta.RowIndex,
			logicalRowIndex,
			e.meta.PrevCx,
		)
}

// Move view 'n' lines backward only if it's possible.
func (e *Editorleaf) MoveViewHalfBackward() {
	n := e.screen.Height / 2

	logicalRowIndex := e.bsArray.GetIndexOfLogicalRow(
		e.meta.RowIndex,
		e.meta.ColIndex,
	)

	rowIndex := e.meta.RowIndex

	// The target is still in the current physical row.
	if n <= logicalRowIndex {
		logicalRowIndex -= n
	} else {
		// Move to the previous physical row.
		n -= logicalRowIndex + 1

		if rowIndex == 0 {
			logicalRowIndex = 0
		} else {
			rowIndex--

			for {
				rowLength := e.bsArray.BoundariesLen(rowIndex)

				if n < rowLength {
					logicalRowIndex = rowLength - n - 1
					break
				}

				n -= rowLength

				if rowIndex == 0 {
					logicalRowIndex = 0
					break
				}

				rowIndex--
			}
		}
	}

	e.meta.RowIndex = rowIndex

	e.meta.ColIndex, _ =
		e.getColumnIndexClosestToCursorXPosition(
			e.meta.RowIndex,
			logicalRowIndex,
			e.meta.PrevCx,
		)
}

func (e *Editorleaf) MoveCursorGoToLine(lineNumber int) {
	if lineNumber < 1 || lineNumber > e.editBuffer.Rows.Length() {
		return
	}
	e.meta.RowIndex = lineNumber - 1
	e.meta.Cy = (e.screen.Height - 1) / 2
	e.meta.ColIndex = 0
	e.meta.PrevCx = 0
}

// ------------------------------------------------------------------
// Edit
// ------------------------------------------------------------------

func (e *Editorleaf) InsertTab() {
	before := e.bsArray.GetIndexOfLastLogicalRow(e.meta.RowIndex)

	w := utils.TabWidth(e.meta.Cx, e.editBuffer.GetTabWidth())
	if (*e.editBuffer.GetLangMode()).GetSoftTab() {
		for range w {
			e.insertBytes([]byte{' '}, true)
		}
	} else {
		e.insertBytes([]byte{'\t'}, true)
	}

	after := e.bsArray.GetIndexOfLastLogicalRow(e.meta.RowIndex)

	increaseY := after - before
	e.meta.Cy += increaseY

	// e.meta.PrevCx を更新できていない
}

func (e *Editorleaf) DeleteRuneBackward() {
	start := e.meta.Cursor
	stop := e.meta.Cursor

	if e.meta.ColIndex == 0 {
		if e.meta.RowIndex == 0 {
			gecore.Echo.AddText("Beginning of buffer")
			return
		}
		// join to prev row
		_, _, colIndex, _ := e.editBuffer.Rows.Row(e.meta.RowIndex - 1).DecodeEndRune()
		start.RowIndex--
		start.ColIndex = colIndex
	} else {
		_, _, prevRuneColIndex, _ := e.editBuffer.Rows.Row(e.meta.RowIndex).DecodePrevRune(e.meta.ColIndex)
		start.ColIndex = prevRuneColIndex
	}

	removed := e.editBuffer.RemoveRegion(start, stop)
	if removed == nil {
		return
	}

	e.meta.Cursor = start
	e.meta.PrevCx = start.ColIndex

	count := stop.RowIndex - start.RowIndex
	tree.GetRootTree().ForEachLeaf(func(l tree.Leaf) {
		ed, ok := l.(*Editorleaf)
		if !ok {
			return
		}
		ed.bsArray.ClearRow(start.RowIndex)
		if count > 0 {
			ed.bsArray.Delete(start.RowIndex+1, count)
		}
	})

	e.editBuffer.UndoAction.PushAction(&editbuffer.EditAction{Class: editbuffer.DELETE_BACKWARD, Before: stop, After: start, Data: removed}, true)
	e.syncCursorAndBufferForEdit(DELETE, start, stop)
}

// If at the EOL, move contents of the next line to the end of the current line,
// erasing the next line after that. Otherwise, delete one character under the
// cursor.
func (e *Editorleaf) DeleteRune() {
	start := e.meta.Cursor
	stop := e.meta.Cursor

	if e.editBuffer.Rows.Row(e.meta.RowIndex).IsColIndexAtRowEnd(e.meta.ColIndex) {
		stop.RowIndex++
		stop.ColIndex = 0
	} else {
		_, size, _ := e.editBuffer.Rows.Row(e.meta.RowIndex).DecodeRune(e.meta.ColIndex)
		stop.ColIndex += size
	}

	removed := e.editBuffer.RemoveRegion(start, stop)
	if removed == nil {
		return
	}

	e.meta.PrevCx = start.ColIndex

	count := stop.RowIndex - start.RowIndex
	tree.GetRootTree().ForEachLeaf(func(l tree.Leaf) {
		ed, ok := l.(*Editorleaf)
		if !ok {
			return
		}
		ed.bsArray.ClearRow(start.RowIndex)
		if count > 0 {
			ed.bsArray.Delete(start.RowIndex+1, count)
		}
	})

	e.editBuffer.UndoAction.PushAction(&editbuffer.EditAction{Class: editbuffer.DELETE, Before: start, After: start, Data: removed}, true)
	e.syncCursorAndBufferForEdit(DELETE, stop, start)
}

func (e *Editorleaf) Autoindent() {
	line := e.editBuffer.Rows.Row(e.meta.RowIndex)
	indent := make([]byte, 0, line.Length())
	indent = append(indent, '\n')
	for i := 0; i < line.Length(); {
		ch, size, ok := line.DecodeRune(i)
		if !ok {
			panic("Autoindent")
		}
		if ch == ' ' || ch == '\t' {
			indent = append(indent, utils.RuneToBytes(ch)...)
			i += size
			continue
		}
		break
	}
	e.insertBytes(indent, true)
}

// unix-line-discard
// backward-kill-line
func (e Editorleaf) BackwardKillLine() {
	if e.meta.ColIndex == 0 {
		return
	}

	start := screen.Cursor{
		RowIndex: e.meta.Cursor.RowIndex,
		ColIndex: 0,
	}

	before := e.meta.Cursor

	removed := e.editBuffer.RemoveRegion(start, e.meta.Cursor)
	if removed == nil {
		gelog.Debug("BackwardKillLine: RemoveRegion returned nil")
		return
	}

	tree.GetRootTree().ForEachLeaf(func(l tree.Leaf) {
		ed, ok := l.(*Editorleaf)
		if !ok {
			return
		}
		ed.bsArray.ClearRow(e.meta.Cursor.RowIndex)
	})

	e.syncCursorAndBufferForEdit(DELETE, start, e.meta.Cursor)

	e.editBuffer.UndoAction.PushAction(&editbuffer.EditAction{
		Class:  editbuffer.DELETE_BACKWARD,
		Before: before,
		After:  start,
		Data:   removed,
	}, true)

	e.meta.Cursor = start
	e.meta.PrevCx = 0
}

// Kill line:
// If not at the EOL, remove contents of the current line from the cursor to the end.
// Otherwise behave like 'delete'.
// 行を削除します:
// EOL でない場合は、カーソルから末尾までの現在の行の内容を削除します。
// それ以外の場合は、「delete」のように動作します。
func (e *Editorleaf) KillLine() {
	lines := e.editBuffer.Rows
	line := lines.Row(e.meta.RowIndex)
	if line.IsColIndexAtRowEnd(e.meta.ColIndex) {
		if lines.IsRowIndexLastRow(e.meta.RowIndex) {
			gecore.Echo.AddText("End of buffer")
			return
		}
		// delete newline
		e.DeleteRune()
		return
	}

	stop := e.meta.Cursor
	stop.ColIndex = lines.Row(e.meta.RowIndex).Length()
	removed := e.editBuffer.RemoveRegion(e.meta.Cursor, stop)
	if removed == nil {
		return
	}

	e.meta.PrevCx = e.meta.ColIndex

	tree.GetRootTree().ForEachLeaf(func(l tree.Leaf) {
		ed, ok := l.(*Editorleaf)
		if !ok {
			return
		}
		ed.bsArray.ClearRow(e.meta.RowIndex)
	})

	e.syncCursorAndBufferForEdit(DELETE, e.meta.Cursor, stop)
	e.editBuffer.UndoAction.PushAction(&editbuffer.EditAction{Class: editbuffer.DELETE, Before: e.meta.Cursor, After: e.meta.Cursor, Data: removed}, true)
}

// ------------------------------------------------------------------
// Yank
// ------------------------------------------------------------------

func (e *Editorleaf) YankFromClipboard() {
	s, err := clipboard.ReadAll()
	if err != nil {
		gecore.Echo.AddText(err.Error())
		return
	}
	e.insertBytes([]byte(s), true)
}

func (e *Editorleaf) Yank() {
	r := killbuffer.KillBuffer.GetLast()
	if r == nil {
		return
	}
	// e.insertBytes(e.meta.RowIndex, e.meta.ColIndex, r)
	e.insertBytesArray(r, true)
}

// ------------------------------------------------------------------
// Mark
// ------------------------------------------------------------------

func (e *Editorleaf) SetCurrentMark(m *mark.Mark) {
	e.meta.Mark = m
}

func (e *Editorleaf) SetMarkAtCursor() {
	content := e.getContentWidthoutSpecialCharactor(e.meta.Cursor, 20)
	newMark := mark.NewMark(e.editBuffer, e.meta.Cursor, content)

	if Marks.UnsetMarkByValue(newMark) {
		gecore.Echo.AddText("Unset mark")
		return
	}

	Marks.AddMark(newMark)
	gecore.Echo.AddText("Set mark")
}

var tmpMarkPos *mark.Mark

func (e *Editorleaf) SwapCursorAndMark() {
	lastMark := Marks.FindLastByFile(e.editBuffer)
	if lastMark == nil {
		gecore.Echo.AddText("No mark set")
		return
	}

	if lastMark.Cursor == e.meta.Cursor {
		if tmpMarkPos != nil {
			e.meta.Cursor = tmpMarkPos.Cursor
			tmpMarkPos = nil
			return
		}
		gecore.Echo.AddText("Already at the last mark")
		return
	} else {
		tmpMarkPos = mark.NewMark(e.editBuffer, e.meta.Cursor, e.getContentWidthoutSpecialCharactor(e.meta.Cursor, 20))
		e.meta.Cursor = lastMark.Cursor
	}
}

func (e *Editorleaf) FilterByCharacters(chars string) []*mark.Mark {
	return Marks.FilterByCharacters(chars)
}

// ------------------------------------------------------------------
// Undo / Redo
// ------------------------------------------------------------------

func (e *Editorleaf) IsRedoEmpty() bool {
	return e.editBuffer.UndoAction.IsRedoEmpty()
}

func (e *Editorleaf) Undo() {
	if e.editBuffer.UndoAction.IsUndoEmpty() {
		gecore.Echo.AddText("No further undo information")
		return
	}

	ag := e.editBuffer.UndoAction.Undo()
	if ag == nil {
		return
	}

	// Undo must be applied in reverse order.
	actions := ag.Actions()
	for i := len(actions) - 1; i >= 0; i-- {
		a := actions[i]

		switch a.Class {
		case editbuffer.INSERT:
			gelog.Debug("Undo INSERT", "after", fmt.Sprintf("%d:%d", a.After.RowIndex, a.After.ColIndex), "before", fmt.Sprintf("%d:%d", a.Before.RowIndex, a.Before.ColIndex), "data", a.Data.String([]byte{'\n'}))

			e.editBuffer.RemoveRegion(a.Before, a.After)
			e.syncCursorAndBufferForEdit(
				DELETE,
				a.Before,
				a.After,
			)
			e.meta.Cursor = a.Before

		case editbuffer.DELETE:
			gelog.Debug("Undo DELETE", "after", fmt.Sprintf("%d:%d", a.After.RowIndex, a.After.ColIndex), "before", fmt.Sprintf("%d:%d", a.Before.RowIndex, a.Before.ColIndex), "data", a.Data.String([]byte{'\n'}))

			e.editBuffer.Rows.InsertRegion(
				a.Before.RowIndex,
				a.Before.ColIndex,
				a.Data,
			)
			e.syncCursorAndBufferForEdit(
				INSERT,
				a.Before,
				a.After,
			)
			e.meta.Cursor = a.Before

		case editbuffer.DELETE_BACKWARD:
			gelog.Debug("Undo DELETE_BACKWARD", "after", fmt.Sprintf("%d:%d", a.After.RowIndex, a.After.ColIndex), "before", fmt.Sprintf("%d:%d", a.Before.RowIndex, a.Before.ColIndex), "data", a.Data.String([]byte{'\n'}))

			e.insertRows(a.Data, false)
			e.syncCursorAndBufferForEdit(
				INSERT,
				a.After,
				a.Before,
			)
			e.meta.Cursor = a.After

		default:
			return
		}

	}

	e.rebuildBufferState()

	gecore.Echo.AddText("Undo!")
}

func (e *Editorleaf) Redo() {
	if e.editBuffer.UndoAction.IsRedoEmpty() {
		gecore.Echo.AddText("No further redo information")
		return
	}

	ag := e.editBuffer.UndoAction.Redo()
	if ag == nil {
		return
	}

	// Redo is applied in the original order.
	actions := ag.Actions()
	for _, a := range actions {
		switch a.Class {
		case editbuffer.INSERT:
			gelog.Debug("Redo INSERT", "a", a.Data.String([]byte{'\n'}))
			e.meta.Cursor = a.Before
			e.insertRows(a.Data, false)
			e.meta.Cursor = a.After

		case editbuffer.DELETE_BACKWARD:
			gelog.Debug("Redo DELETE_BACKWARD", "after", fmt.Sprintf("%d:%d", a.After.RowIndex, a.After.ColIndex), "before", fmt.Sprintf("%d:%d", a.Before.RowIndex, a.Before.ColIndex), "data", a.Data.String([]byte{'\n'}))

			e.meta.Cursor = a.Before
			e.editBuffer.RemoveRegion(
				a.Before,
				a.After,
			)
			e.syncCursorAndBufferForEdit(
				DELETE,
				a.Before,
				a.After,
			)
			// これでいいかな。
			// e.meta.Cursor = a.After
			e.meta.Cursor = a.Before

		case editbuffer.DELETE:
			gelog.Debug("Redo DELETE", "after", fmt.Sprintf("%d:%d", a.After.RowIndex, a.After.ColIndex), "before", fmt.Sprintf("%d:%d", a.Before.RowIndex, a.Before.ColIndex), "data", a.Data.String([]byte{'\n'}))

			cursor := e.cursorAfterDelete(a)

			e.editBuffer.RemoveRegion(
				a.Before,
				cursor,
			)
			e.syncCursorAndBufferForEdit(
				DELETE,
				a.Before,
				cursor,
			)
			e.meta.Cursor = a.After

		default:
			return
		}

	}

	e.rebuildBufferState()
}

func (e *Editorleaf) cursorAfterDelete(a *editbuffer.EditAction) screen.Cursor {
	cursor := a.Before

	if len(a.Data) == 0 {
		return cursor
	}

	last := a.Data[len(a.Data)-1]

	cursor.RowIndex += len(a.Data) - 1
	cursor.ColIndex += len(last)

	/*
		if len(last) > 0 && last[len(last)-1] == '\n' {
			cursor.RowIndex++
			cursor.ColIndex = 0
		}
	*/

	return cursor
}

/* func (e *Editorleaf) cursorAfterDelete(a *editbuffer.EditAction) screen.Cursor {
	cursor := a.Before

	if len(a.Data) == 0 {
		return cursor
	}

	last := a.Data[len(a.Data)-1]

	cursor.RowIndex += len(a.Data) - 1
	cursor.ColIndex += len(last)

	if len(last) > 0 && last[len(last)-1] == '\n' {
		cursor.RowIndex++
		cursor.ColIndex = 0
	}

	return cursor
}
*/

func (e *Editorleaf) rebuildBufferState() {
	tree.GetRootTree().ForEachLeaf(func(l tree.Leaf) {
		ed, ok := l.(*Editorleaf)
		if !ok {
			return
		}

		// TODO:
		// editBuffer の変更結果から bsArray を同期する。
		// ed.bsArray.Rebuild(ed.editBuffer.Rows)
		ed.bsArray.ClearAll() // ひとまず このまま
	})
}

// ------------------------------------------------------------------
// Other
// ------------------------------------------------------------------

func (e *Editorleaf) CharInfo() {
	ch, str := e.CharInfoOnCursor(e.meta.Cursor)

	s := fmt.Sprintf("Char: '%s' (dec: %d, oct: %s, hex: %02X, %s), Cursor index: %d,%d", str, ch, strconv.FormatInt(int64(ch), 8), ch, utils.WidthKindString(ch), e.meta.RowIndex, e.meta.ColIndex)
	gecore.Echo.AddText(s)
}

func (e *Editorleaf) IsDirtyFlag() bool {
	return e.editBuffer.IsDirtyFlag()
}

func (e *Editorleaf) Recenter() {
	e.meta.Cy = int(e.editArea.Height / 2)
}

// ------------------------------------------------------------------
// Utilities
// ------------------------------------------------------------------

func SplitRows(data []byte) (rows.Rows, error) {
	reader := bufio.NewReader(bytes.NewReader(data))

	rows := make([]rows.Row, 0)

	for {
		line, _, err := editbuffer.ReadLine(reader)

		// If only a newline, the size is zero.
		// if len(line) > 0 {
		b := make([]byte, len(line), len(line)+16)
		copy(b, line)
		rows = append(rows, b)
		// }

		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, err
		}
	}

	if len(rows) == 0 {
		rows = append(rows, []byte{})
	}

	return rows, nil
}

func (e *Editorleaf) insertRows(data rows.Rows, enableUndo bool) {
	beforeCursor := e.meta.Cursor

	e.editBuffer.Rows.InsertRegion(e.meta.RowIndex, e.meta.ColIndex, data)

	// Current cursor
	// 正確に更新するには論理行の計算が必要
	e.meta.RowIndex += data.Length() - 1
	// increase := len(rs.BytesArray()[rs.Length()-1])
	increase := data.Row(data.Length() - 1).Length()
	if data.Length() == 1 {
		e.meta.ColIndex = beforeCursor.ColIndex + increase
	} else {
		e.meta.ColIndex = increase
	}

	// Release Boundaries
	tree.GetRootTree().ForEachLeaf(func(l tree.Leaf) {
		ed, ok := l.(*Editorleaf)
		if !ok {
			return // continue
		}
		ed.bsArray.ClearRow(beforeCursor.RowIndex)
		ed.bsArray.Insert(beforeCursor.RowIndex+1, data.Length()-1)
	})

	if enableUndo {
		e.editBuffer.UndoAction.PushAction(&editbuffer.EditAction{Class: editbuffer.INSERT, Before: beforeCursor, After: e.meta.Cursor, Data: data}, true)
	}
	e.syncCursorAndBufferForEdit(INSERT, beforeCursor, e.meta.Cursor)

	// コメントアウトされた部分は、将来的に必要な場合に対応
	// -1 insert
	// とか定義する必要があるかもしれない
	// e.meta.PrevCx = -1
	// e.dirtyFlag = true
}

func (e *Editorleaf) insertBytesArray(data [][]byte, enableUndo bool) {
	r := make(rows.Rows, len(data))
	for i := 0; i < len(data); i++ {
		r[i] = data[i]
	}
	e.insertRows(r, enableUndo)
}

// insertBytes は、バイトスライスを現在のカーソル位置に挿入し、カーソルを前進させます。
func (e *Editorleaf) insertBytes(data []byte, enableUndo bool) {
	rs, err := SplitRows(data)
	if err != nil {
		gelog.Error(err.Error())
		gecore.Echo.AddText(err.Error())
		return
	}

	e.insertRows(rs, enableUndo)

	/*
		 	e.editBuffer.Rows.InsertRegion(e.meta.RowIndex, e.meta.ColIndex, rs)

			// Current cursor
			// 正確に更新するには論理行の計算が必要
			e.meta.RowIndex += rs.Length() - 1
			// increase := len(rs.BytesArray()[rs.Length()-1])
			increase := rs.Row(rs.Length() - 1).Length()
			if rs.Length() == 1 {
				e.meta.ColIndex = beforeCursor.ColIndex + increase
			} else {
				e.meta.ColIndex = increase
			}

			// Release Boundaries
			tree.GetRootTree().ForEachLeaf(func(l tree.Leaf) {
				ed, ok := l.(*Editorleaf)
				if !ok {
					return // continue
				}
				ed.bsArray.ClearRow(beforeCursor.RowIndex)
				ed.bsArray.Insert(beforeCursor.RowIndex+1, rs.Length()-1)
			})

			if enableUndo {
				e.editBuffer.UndoAction.PushAction(&editbuffer.EditAction{Class: editbuffer.INSERT, Before: beforeCursor, After: e.meta.Cursor, Data: rs})
			}
			e.syncCursorAndBufferForEdit(INSERT, beforeCursor, e.meta.Cursor)

			// コメントアウトされた部分は、将来的に必要な場合に対応
			// -1 insert
			// とか定義する必要があるかもしれない
			// e.meta.PrevCx = -1
			// e.dirtyFlag = true
	*/
}

// Wrapper is insertBytes
func (e *Editorleaf) InsertString(s string) {
	e.insertBytes([]byte(s), true)
}

// Wrapper is insertBytes
func (e *Editorleaf) InsertRune(ch rune) {
	e.insertBytes(utils.RuneToBytes(ch), true)
}

func (e *Editorleaf) CharInfoOnCursor(cursor screen.Cursor) (rune, string) {
	isEOF := false
	var ch rune
	var str string
	if e.editBuffer.Rows.Row(e.meta.RowIndex).IsColIndexAtRowEnd(e.meta.ColIndex) {
		if e.editBuffer.Rows.IsRowIndexLastRow(e.meta.RowIndex) {
			isEOF = true
			ch = define.EOF
		} else {
			ch = '\n'
		}
	} else {
		ch, _, _ = (*e).editBuffer.Rows.Row(e.meta.RowIndex).DecodeRune(e.meta.ColIndex)
	}
	if isEOF {
		str = "EOF"
	} else {
		str = e.RuneStatus(ch)
	}
	return ch, str
}
