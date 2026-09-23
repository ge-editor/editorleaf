package editorleaf

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"
	"unicode/utf8"

	"github.com/atotto/clipboard"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/editorleaf/editbuffer/rows"
	"github.com/ge-editor/editorleaf/mark"
	"github.com/ge-editor/gecore"
	"github.com/ge-editor/gecore/define"
	"github.com/ge-editor/gecore/killbuffer"
	"github.com/ge-editor/gecore/marks"
	"github.com/ge-editor/gecore/tree"
	"github.com/ge-editor/gelog"
	"github.com/ge-editor/locale"
	"github.com/ge-editor/theme"
	"github.com/ge-editor/utils"
)

// --------------------
// File
// --------------------

// If the file has already been read, use that buffer
func (e *Editorleaf) OpenFile(path string) (editbuffer.Result, error) {
	// Save current Editorleaf Meta data before switching editing content.
	e.GetBuffers().BufferSet(e.editBuffer).PushMeta(e.meta)

	// Open new editing content in the editor buffer
	ff, meta, result, err := BufferSets.GetFileAndMeta(path)
	e.editBuffer = ff
	e.meta = meta
	e.bsArray.ClearAll()
	return result, err
}

func (e *Editorleaf) adjustFormattedCursorPosition() {
	rows := e.editBuffer.Rows
	if e.meta.RowsPos.RowIndex >= rows.Length() {
		e.meta.RowsPos.RowIndex = rows.Length() - 1
	}

	row := rows.Row(e.meta.RowsPos.RowIndex)
	i := e.meta.RowsPos.ColIndex
	if i >= row.Length() {
		i = row.Length() - 1
	}
	for i > 0 {
		if _, _, ok := row.DecodeRune(i); ok {
			break
		}
		i--
	}
	e.meta.RowsPos.ColIndex = i
}

// If the file does not exist, a backup error will occur
func (e *Editorleaf) SaveFile() {
	backupMessage := ""
	if err := e.editBuffer.Backup(); err != nil {
		backupMessage = " (" + err.Error() + ")"
	}

	result, err := e.editBuffer.Save()
	if result&editbuffer.ResultSaved == 0 {
		gecore.Echo.AddText(err.Error() + backupMessage)
		return
	}

	if result&editbuffer.ResultFormatted != 0 {
		tree.GetRootTree().ForEachLeaf(func(l tree.Leaf) {
			ed, ok := l.(*Editorleaf)
			if !ok {
				return
			}

			ed.adjustFormattedCursorPosition()
			ed.bsArray.ClearAll()

			// May need adjust cursor position if formatted content
			rowLength := ed.editBuffer.Rows.Length()
			if ed.meta.RowsPos.RowIndex >= rowLength {
				ed.meta.RowsPos.RowIndex = rowLength - 1
			}
			line := (*ed.editBuffer.Rows)[ed.meta.RowsPos.RowIndex]
			colLength := len(line)
			if ed.meta.RowsPos.ColIndex >= colLength {
				// cursor on newline or EOF
				ed.meta.RowsPos.ColIndex = colLength - 1
			}
			for !utf8.RuneStart(line[ed.meta.RowsPos.ColIndex]) && ed.meta.RowsPos.ColIndex > 0 {
				ed.meta.RowsPos.ColIndex--
			}
		})
	}

	gecore.Echo.AddText("Wrote " + e.editBuffer.GetPath() + backupMessage)

	e.editBuffer.UndoAction.MarkSaved()
}

// If an existing file is specified, it will be overwritten
// Backup works so no data is lost, but...
func (e *Editorleaf) ChangeFilePath(path string) {
	e.editBuffer.SetPath(path)
}

func (e *Editorleaf) GetPath() string {
	return e.editBuffer.GetPath()
}

// --------------------
// Move cursor
// --------------------

// Move cursor one character forward.
func (e *Editorleaf) MoveCursorForward() {
	x, y := e.meta.ScreenPos.Col, e.meta.ScreenPos.Row

	lines := e.editBuffer.Rows
	line := lines.Row(e.meta.RowsPos.RowIndex)
	if line.IsColIndexAtRowEnd(e.meta.RowsPos.ColIndex) {
		if lines.IsRowIndexLastRow(e.meta.RowsPos.RowIndex) {
			gecore.Echo.AddText("End of buffer")
			return
		}
		y++
		e.meta.RowsPos.RowIndex++
		x = 0
		e.meta.RowsPos.ColIndex = 0
	} else {
		ch, size, ok := lines.Row(e.meta.RowsPos.RowIndex).DecodeRune(e.meta.RowsPos.ColIndex)
		if !ok {
			gelog.Error("error")
			panic("err")
		}
		w := e.locale.RuneWidth(ch)
		if !ok {
			gelog.Error("error")
		}
		if e.bsArray.IsEndOfLogicalRow(e.meta.RowsPos.RowIndex, e.meta.RowsPos.ColIndex) {
			y++
			x = 0
		} else {
			x += w
		}
		e.meta.RowsPos.ColIndex += size
	}

	e.meta.PrevScreenPos.Col = x
	e.moveCursor(x, y)
}

// Move cursor one character backward.
func (e *Editorleaf) MoveCursorBackward() {
	x, y := e.meta.ScreenPos.Col, e.meta.ScreenPos.Row

	if e.meta.RowsPos.ColIndex == 0 {
		if e.meta.RowsPos.RowIndex == 0 {
			gecore.Echo.AddText("Beginning of buffer")
			return
		}
		// previous line
		y--
		e.meta.RowsPos.RowIndex--

		prevBs := e.bsArray.LastBoundary(e.meta.RowsPos.RowIndex)
		// gelog.Debug("prev Boundary", fmt.Sprintf("StartLogicalRowByteIndex: %d, StopLogicalRowByteIndex: %d, LogicalRowWidth: %d, TotalCellWidth: %d, ", prevBs.StartLogicalRowByteIndex, prevBs.StopLogicalRowByteIndex, prevBs.LogicalRowWidth, prevBs.TotalCellWidth))

		// on newline
		x = prevBs.LogicalRowWidth - 1
		e.meta.RowsPos.ColIndex = prevBs.StopLogicalRowByteIndex - 1
	} else {
		before := e.bsArray.GetIndexOfLogicalRow(e.meta.RowsPos.RowIndex, e.meta.RowsPos.ColIndex)
		ch, _, colIndex, ok := e.editBuffer.Rows.Row(e.meta.RowsPos.RowIndex).DecodePrevRune(e.meta.RowsPos.ColIndex)
		if !ok {
			panic("2")
		}
		w := e.locale.RuneWidth(ch)
		e.meta.RowsPos.ColIndex = colIndex
		after := e.bsArray.GetIndexOfLogicalRow(e.meta.RowsPos.RowIndex, e.meta.RowsPos.ColIndex)
		if !ok {
			panic("3")
		}
		// gelog.Debug("logical row", "before", before, "after", after)
		if after < before {
			y--
			x = e.bsArray.Boundary(e.meta.RowsPos.RowIndex, after).LogicalRowWidth - w
		} else {
			x -= w
		}
	}

	e.meta.PrevScreenPos.Col = x
	e.moveCursor(x, y)
}

// Move cursor to next word.
func (e *Editorleaf) MoveCursorNextWord() {
	x, y := e.meta.ScreenPos.Col, e.meta.ScreenPos.Row

	lines := e.editBuffer.Rows
	line := lines.Row(e.meta.RowsPos.RowIndex)
	if line.IsColIndexAtRowEnd(e.meta.RowsPos.ColIndex) {
		if lines.IsRowIndexLastRow(e.meta.RowsPos.RowIndex) {
			gecore.Echo.AddText("End of buffer")
			return
		}
		y++
		e.meta.RowsPos.RowIndex++
		x = 0
		e.meta.RowsPos.ColIndex = 0
	} else {

		var prevCc, cc locale.CharClass
		notUppercaseBit := ^locale.UPPERCASE
		for {
			ch, size, ok := (*lines).Row(e.meta.RowsPos.RowIndex).DecodeRune(e.meta.RowsPos.ColIndex)
			if !ok {
				gelog.Error("MoveCursorNextWord: error", "RowsPos", e.meta.RowsPos)
				return
			}

			w := e.locale.RuneWidth(ch)
			prevCc = cc
			cc = e.locale.GetCharClass(ch)
			if prevCc != 0 {
				if prevCc&locale.UPPERCASE == 0 && cc&locale.UPPERCASE > 0 {
					break
				}
				prevCc &= notUppercaseBit
				cc &= notUppercaseBit
				if prevCc != cc && cc&locale.TAB == 0 && cc&locale.SPACE == 0 && cc&locale.SYMBOL == 0 {
					break
				}
			}
			if e.bsArray.IsEndOfLogicalRow(e.meta.RowsPos.RowIndex, e.meta.RowsPos.ColIndex) {
				y++
				x = 0
			} else {
				x += w
			}
			e.meta.RowsPos.ColIndex += size
		}
	}

	e.meta.PrevScreenPos.Col = x
	e.moveCursor(x, y)
}

// Move cursor to previous word.
func (e *Editorleaf) MoveCursorPreviousWord() {
	x, y := e.meta.ScreenPos.Col, e.meta.ScreenPos.Row

	if e.meta.RowsPos.ColIndex == 0 {
		if e.meta.RowsPos.RowIndex == 0 {
			gecore.Echo.AddText("Beginning of buffer")
			return
		}

		y--
		e.meta.RowsPos.RowIndex--

		lastBs := e.bsArray.LastBoundary(e.meta.RowsPos.RowIndex)
		ch, _, colIndex, _ := e.editBuffer.Rows.Row(e.meta.RowsPos.RowIndex).DecodeEndRune()
		w := e.locale.RuneWidth(ch)

		// Move to the last character of the previous logical row.
		x = lastBs.LogicalRowWidth - w
		e.meta.RowsPos.ColIndex = colIndex
	} else {
		var prevCc, cc locale.CharClass
		notUppercaseBit := ^locale.UPPERCASE

		for {
			before := e.bsArray.GetIndexOfLogicalRow(
				e.meta.RowsPos.RowIndex,
				e.meta.RowsPos.ColIndex,
			)

			ch, _, colIndex, ok := e.editBuffer.Rows.Row(
				e.meta.RowsPos.RowIndex,
			).DecodePrevRune(e.meta.RowsPos.ColIndex)
			if !ok {
				break
			}

			w := e.locale.RuneWidth(ch)

			// cc is the character we just moved onto.
			// prevCc is the character which was to its right.
			prevCc = cc
			cc = e.locale.GetCharClass(ch)

			if prevCc != 0 {
				// The forward direction stops at:
				//
				//     lowercase -> UPPERCASE
				//
				// Therefore, when moving backwards, stop at the
				// corresponding boundary:
				//
				//     UPPERCASE <- lowercase
				//
				if prevCc&locale.UPPERCASE > 0 &&
					cc&locale.UPPERCASE == 0 {
					break
				}

				savePrevCC, saveCC := prevCc, cc

				prevCc &= notUppercaseBit
				cc &= notUppercaseBit

				// Same class transition rule as MoveCursorNextWord,
				// evaluated in the reverse direction.
				if prevCc != cc &&
					prevCc&locale.TAB == 0 &&
					prevCc&locale.SPACE == 0 &&
					prevCc&locale.SYMBOL == 0 {
					break
				}

				prevCc, cc = savePrevCC, saveCC
			}

			e.meta.RowsPos.ColIndex = colIndex

			after := e.bsArray.GetIndexOfLogicalRow(
				e.meta.RowsPos.RowIndex,
				e.meta.RowsPos.ColIndex,
			)

			if after < before {
				// Crossed a logical-row boundary.
				y--
				x = e.bsArray.Boundary(
					e.meta.RowsPos.RowIndex,
					after,
				).LogicalRowWidth
			} else {
				x -= w
			}
		}
	}

	e.meta.PrevScreenPos.Col = x
	e.moveCursor(x, y)
}

// Move cursor to the next line.
func (e *Editorleaf) MoveCursorNextLine() {
	x, y := e.meta.ScreenPos.Col, e.meta.ScreenPos.Row
	// gelog.Debug("x", x, "y", y)

	indexOfLogicalRow := 0

	if e.bsArray.OnEndOfLogicalRow(e.meta.RowsPos.RowIndex, e.meta.RowsPos.ColIndex) { // last logical line
		if e.editBuffer.Rows.IsRowIndexLastRow(e.meta.RowsPos.RowIndex) {
			gecore.Echo.AddText("End of buffer")
			return
		}
		// move to next line
		e.meta.RowsPos.RowIndex++
	} else {
		// What index number in logic line?
		// and move to next logical line
		indexOfLogicalRow = e.bsArray.GetIndexOfLogicalRow(e.meta.RowsPos.RowIndex, e.meta.RowsPos.ColIndex) + 1
	}

	e.meta.RowsPos.ColIndex, x = e.getColumnIndexClosestToCursorXPosition(e.meta.RowsPos.RowIndex, indexOfLogicalRow, e.meta.PrevScreenPos.Col)
	// e.meta.RowsPos.ColIndex, x = 0, 0
	// e.meta.RowsPos.ColIndex = 0

	e.moveCursor(x, y+1)
	// gelog.Debug("x", x, "y", y)
}

// Move cursor to the previous row.
func (e *Editorleaf) MoveCursorPrevLine() {
	x, y := e.meta.ScreenPos.Col, e.meta.ScreenPos.Row

	// What index number in logical row?
	indexOfLogicalRow := e.bsArray.GetIndexOfLogicalRow(e.meta.RowsPos.RowIndex, e.meta.RowsPos.ColIndex)

	if indexOfLogicalRow == 0 { // first logical row
		if e.meta.RowsPos.RowIndex == 0 {
			gecore.Echo.AddText("Beginning of buffer")
			return
		}
		// move to prev row
		e.meta.RowsPos.RowIndex--
		// last logical row
		indexOfLogicalRow = e.bsArray.GetIndexOfLastLogicalRow(e.meta.RowsPos.RowIndex)
	} else {
		indexOfLogicalRow--
	}

	e.meta.RowsPos.ColIndex, x = e.getColumnIndexClosestToCursorXPosition(e.meta.RowsPos.RowIndex, indexOfLogicalRow, e.meta.PrevScreenPos.Col)

	e.moveCursor(x, y-1)
}

// Move cursor to the end of the line.
func (e *Editorleaf) MoveCursorEndOfLine() {
	// What index number in logic row?
	indexOfLogicalRow := e.bsArray.GetIndexOfLogicalRow(e.meta.RowsPos.RowIndex, e.meta.RowsPos.ColIndex)

	colLength := e.editBuffer.Rows.Row(e.meta.RowsPos.RowIndex).Length()
	e.meta.RowsPos.ColIndex = colLength

	// cursor display position y
	lastIndexOfLogicalRow := e.bsArray.GetIndexOfLastLogicalRow(e.meta.RowsPos.RowIndex)
	e.meta.ScreenPos.Row += lastIndexOfLogicalRow - indexOfLogicalRow

	// cursor display position x
	e.meta.ScreenPos.Col = e.bsArray.Boundary(e.meta.RowsPos.RowIndex, lastIndexOfLogicalRow).LogicalRowWidth
	e.meta.PrevScreenPos.Col = e.meta.ScreenPos.Col
}

// Move cursor to the beginning of the line.
// Consider indentation
func (e *Editorleaf) MoveCursorBeginningOfLine() {
	if e.meta.RowsPos.RowIndex == 0 && e.meta.RowsPos.ColIndex == 0 {
		gecore.Echo.AddText("Beginning of buffer")
		return
	}

	nowIndexOfLogicalRow := e.bsArray.GetIndexOfLogicalRow(e.meta.RowsPos.RowIndex, e.meta.RowsPos.ColIndex)
	rows := e.editBuffer.Rows
	indentedIndex := 0
	indentedWidth := 0
	for indentedIndex < rows.Row(e.meta.RowsPos.RowIndex).Length() {
		ch, size, _ := rows.Row(e.meta.RowsPos.RowIndex).DecodeRune(indentedIndex)
		if ch != ' ' && ch != '\t' {
			break
		}
		w := e.locale.RuneWidth(ch)
		indentedWidth += w
		indentedIndex += size
	}
	newIndexOfLogicalRow := e.bsArray.GetIndexOfLogicalRow(e.meta.RowsPos.RowIndex, indentedIndex)

	if e.meta.RowsPos.ColIndex == 0 || indentedIndex < e.meta.RowsPos.ColIndex {
		e.meta.RowsPos.ColIndex = indentedIndex
		e.meta.ScreenPos.Col = indentedWidth
	} else {
		e.meta.RowsPos.ColIndex = 0
		e.meta.ScreenPos.Col = 0
	}

	if newIndexOfLogicalRow < nowIndexOfLogicalRow {
		e.meta.ScreenPos.Row -= nowIndexOfLogicalRow - newIndexOfLogicalRow
	}
}

// Move cursor to the end of logical the line.
// At the end of a logical line, the cursor should at the beginning of the next logical line. so I see...
func (e *Editorleaf) MoveCursorEndOfLogicalLine() {
	// What index number in logic row?
	indexOfLogicalRow := e.bsArray.GetIndexOfLogicalRow(e.meta.RowsPos.RowIndex, e.meta.RowsPos.ColIndex)
	bo := e.bsArray.Boundary(e.meta.RowsPos.RowIndex, indexOfLogicalRow)

	if indexOfLogicalRow == e.bsArray.BoundariesLen(e.meta.RowsPos.RowIndex)-1 {
		e.meta.RowsPos.ColIndex = bo.StopLogicalRowByteIndex - 1
		e.meta.ScreenPos.Col = bo.LogicalRowWidth - 1
	} else {
		// Move to the first character of the next logical row
		e.meta.RowsPos.ColIndex = bo.StopLogicalRowByteIndex
		e.meta.ScreenPos.Row++
		e.meta.ScreenPos.Col = 0
	}

	e.meta.PrevScreenPos.Col = e.meta.ScreenPos.Col
}

// Move cursor to the beginning of the logical row.
// Consider logical row
// Consider indentation....
func (e *Editorleaf) MoveCursorBeginningOfLogicalLine() {
	if e.meta.RowsPos.RowIndex == 0 && e.meta.RowsPos.ColIndex == 0 {
		gecore.Echo.AddText("Beginning of buffer")
		return
	}

	nowIndexOfLogicalRow := e.bsArray.GetIndexOfLogicalRow(e.meta.RowsPos.RowIndex, e.meta.RowsPos.ColIndex)
	if nowIndexOfLogicalRow == 0 {
		e.MoveCursorBeginningOfLine()
		return
	}

	bo := e.bsArray.Boundary(e.meta.RowsPos.RowIndex, nowIndexOfLogicalRow)
	if e.meta.RowsPos.ColIndex == bo.StartLogicalRowByteIndex {
		if nowIndexOfLogicalRow == 1 {
			e.MoveCursorBeginningOfLine()
			return
		}

		bo := e.bsArray.Boundary(e.meta.RowsPos.RowIndex, nowIndexOfLogicalRow-1)
		e.meta.RowsPos.ColIndex = bo.StartLogicalRowByteIndex
		e.meta.ScreenPos.Row--
		return
	}
	e.meta.RowsPos.ColIndex = bo.StartLogicalRowByteIndex
	e.meta.ScreenPos.Col = 0
	e.meta.PrevScreenPos.Col = 0
}

func (e *Editorleaf) MoveCursorBeginningOfFile() {
	e.meta.RowsPos.RowIndex = 0
	e.meta.RowsPos.ColIndex = 0
	e.meta.ScreenPos.Row = 0
	e.meta.ScreenPos.Col = 0
}

func (e *Editorleaf) MoveCursorEndOfFile() {
	e.meta.RowsPos.RowIndex = e.editBuffer.Rows.Length() - 1
	lastBs := e.bsArray.LastBoundary(e.meta.RowsPos.RowIndex)
	e.meta.RowsPos.ColIndex = lastBs.StopLogicalRowByteIndex - 1 // left of the LF or EOF
	e.meta.ScreenPos.Col = lastBs.LogicalRowWidth - 1            // left of the LF or EOF
	e.meta.ScreenPos.Row = e.editArea.Height - e.verticalThreshold

	e.meta.PrevScreenPos.Col = e.meta.ScreenPos.Col
}

// Move view 'n' lines forward only if it's possible.
func (e *Editorleaf) MoveViewHalfForward() {
	n := e.screen.Height / 2

	logicalRowIndex := e.bsArray.GetIndexOfLogicalRow(
		e.meta.RowsPos.RowIndex,
		e.meta.RowsPos.ColIndex,
	)

	rowIndex := e.meta.RowsPos.RowIndex
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

	e.meta.RowsPos.RowIndex = rowIndex

	e.meta.RowsPos.ColIndex, _ =
		e.getColumnIndexClosestToCursorXPosition(
			e.meta.RowsPos.RowIndex,
			logicalRowIndex,
			e.meta.PrevScreenPos.Col,
		)
}

// Move view 'n' lines backward only if it's possible.
func (e *Editorleaf) MoveViewHalfBackward() {
	n := e.screen.Height / 2

	logicalRowIndex := e.bsArray.GetIndexOfLogicalRow(
		e.meta.RowsPos.RowIndex,
		e.meta.RowsPos.ColIndex,
	)

	rowIndex := e.meta.RowsPos.RowIndex

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

	e.meta.RowsPos.RowIndex = rowIndex

	e.meta.RowsPos.ColIndex, _ =
		e.getColumnIndexClosestToCursorXPosition(
			e.meta.RowsPos.RowIndex,
			logicalRowIndex,
			e.meta.PrevScreenPos.Col,
		)
}

func (e *Editorleaf) MoveCursorGoToLine(lineNumber int) {
	if lineNumber < 1 || lineNumber > e.editBuffer.Rows.Length() {
		return
	}
	e.meta.RowsPos.RowIndex = lineNumber - 1
	e.meta.ScreenPos.Row = (e.screen.Height - 1) / 2
	e.meta.RowsPos.ColIndex = 0
	e.meta.PrevScreenPos.Col = 0
}

// --------------------
// Edit
// --------------------

func (e *Editorleaf) InsertTab() {
	before := e.bsArray.GetIndexOfLastLogicalRow(e.meta.RowsPos.RowIndex)

	w := utils.TabWidth(e.meta.ScreenPos.Col, e.editBuffer.GetTabWidth())
	if (*e.editBuffer.GetLangMode()).GetSoftTab() {
		for range w {
			e.insertBytes([]byte{' '}, true)
		}
	} else {
		e.insertBytes([]byte{'\t'}, true)
	}

	after := e.bsArray.GetIndexOfLastLogicalRow(e.meta.RowsPos.RowIndex)

	increaseY := after - before
	e.meta.ScreenPos.Row += increaseY

	// e.meta.PrevCx を更新できていない
}

func (e *Editorleaf) DeleteRuneBackward() {
	start := e.meta.RowsPos
	stop := e.meta.RowsPos

	if e.meta.RowsPos.ColIndex == 0 {
		if e.meta.RowsPos.RowIndex == 0 {
			gecore.Echo.AddText("Beginning of buffer")
			return
		}
		// join to prev row
		_, size, colIndex, _ := e.editBuffer.Rows.Row(e.meta.RowsPos.RowIndex - 1).DecodeEndRune()
		start.RowIndex--
		start.ColIndex = colIndex + size
	} else {
		_, _, prevRuneColIndex, _ := e.editBuffer.Rows.Row(e.meta.RowsPos.RowIndex).DecodePrevRune(e.meta.RowsPos.ColIndex)
		start.ColIndex = prevRuneColIndex
	}

	removed := e.editBuffer.RemoveRegion(start, stop)
	if removed == nil {
		return
	}

	e.meta.RowsPos = start
	e.meta.PrevScreenPos.Col = start.ColIndex

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
	start := e.meta.RowsPos
	stop := e.meta.RowsPos

	if e.editBuffer.Rows.Row(e.meta.RowsPos.RowIndex).IsColIndexAtRowEnd(e.meta.RowsPos.ColIndex) {
		stop.RowIndex++
		stop.ColIndex = 0
	} else {
		_, size, _ := e.editBuffer.Rows.Row(e.meta.RowsPos.RowIndex).DecodeRune(e.meta.RowsPos.ColIndex)
		stop.ColIndex += size
	}

	removed := e.editBuffer.RemoveRegion(start, stop)
	if removed == nil {
		return
	}

	e.meta.PrevScreenPos.Col = start.ColIndex

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
	line := e.editBuffer.Rows.Row(e.meta.RowsPos.RowIndex)
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
	e.meta.ScreenPos.Row++

	e.insertBytes(indent, true)
}

// unix-line-discard
// backward-kill-line
func (e Editorleaf) BackwardKillLine() {
	if e.meta.RowsPos.ColIndex == 0 {
		return
	}

	start := rows.RowsPos{
		RowIndex: e.meta.RowsPos.RowIndex,
		ColIndex: 0,
	}

	before := e.meta.RowsPos

	removed := e.editBuffer.RemoveRegion(start, e.meta.RowsPos)
	if removed == nil {
		gelog.Debug("BackwardKillLine: RemoveRegion returned nil")
		return
	}

	tree.GetRootTree().ForEachLeaf(func(l tree.Leaf) {
		ed, ok := l.(*Editorleaf)
		if !ok {
			return
		}
		ed.bsArray.ClearRow(e.meta.RowsPos.RowIndex)
	})

	e.syncCursorAndBufferForEdit(DELETE, start, e.meta.RowsPos)

	e.editBuffer.UndoAction.PushAction(&editbuffer.EditAction{
		Class:  editbuffer.DELETE_BACKWARD,
		Before: before,
		After:  start,
		Data:   removed,
	}, true)

	e.meta.RowsPos = start
	e.meta.PrevScreenPos.Col = 0
}

// Kill line:
// If not at the EOL, remove contents of the current line from the cursor to the end.
// Otherwise behave like 'delete'.
// 行を削除します:
// EOL でない場合は、カーソルから末尾までの現在の行の内容を削除します。
// それ以外の場合は、「delete」のように動作します。
func (e *Editorleaf) KillLine() {
	lines := e.editBuffer.Rows
	line := lines.Row(e.meta.RowsPos.RowIndex)
	if line.IsColIndexAtRowEnd(e.meta.RowsPos.ColIndex) {
		if lines.IsRowIndexLastRow(e.meta.RowsPos.RowIndex) {
			gecore.Echo.AddText("End of buffer")
			return
		}
		// delete newline
		e.DeleteRune()
		return
	}

	stop := e.meta.RowsPos
	stop.ColIndex = lines.Row(e.meta.RowsPos.RowIndex).Length()
	removed := e.editBuffer.RemoveRegion(e.meta.RowsPos, stop)
	if removed == nil {
		return
	}

	e.meta.PrevScreenPos.Col = e.meta.RowsPos.ColIndex

	tree.GetRootTree().ForEachLeaf(func(l tree.Leaf) {
		ed, ok := l.(*Editorleaf)
		if !ok {
			return
		}
		ed.bsArray.ClearRow(e.meta.RowsPos.RowIndex)
	})

	e.syncCursorAndBufferForEdit(DELETE, e.meta.RowsPos, stop)
	e.editBuffer.UndoAction.PushAction(&editbuffer.EditAction{Class: editbuffer.DELETE, Before: e.meta.RowsPos, After: e.meta.RowsPos, Data: removed}, true)
}

// --------------------
// Yank
// --------------------

func (e *Editorleaf) YankFromClipboard() {
	s, err := clipboard.ReadAll()
	if err != nil {
		gecore.Echo.AddText(err.Error())
		return
	}
	e.insertBytes([]byte(s), true)
}

func (e *Editorleaf) Yank() {
	b := killbuffer.KillBuffer.GetLast()
	if b == nil {
		return
	}
	// e.insertBytes(e.meta.RowsPos.RowIndex, e.meta.RowsPos.ColIndex, r)
	e.insertBytesArray(b, true)
}

// --------------------
// Mark
// --------------------

func (e *Editorleaf) MoveToMarkPosition(m *mark.Mark) {
	e.meta.RowsPos = m.RowsPos
}

func (e *Editorleaf) SetMarkAtCursor() {
	label := e.getContentWidthoutSpecialCharactor(e.meta.RowsPos, 20)
	newMark := mark.NewMark(e.editBuffer, e.meta.RowsPos, label)

	for _, a := range marks.Marks.Items() {
		m, ok := a.(*mark.Mark)
		if !ok || !m.Equals(newMark) {
			continue
		}

		gecore.Echo.AddText("Unset mark")
		marks.Marks.Remove(m)
		return
	}

	gecore.Echo.AddText("Set mark")
	marks.Marks.Add(newMark)
}

func (e *Editorleaf) findLastByEditBuffer() *mark.Mark {
	for _, a := range marks.Marks.Items() {
		lastMark, ok := a.(*mark.Mark)
		if ok && lastMark.EditBuffer == e.editBuffer {
			return lastMark
		}
	}
	return nil
}

var tmpMarkPos *mark.Mark

func (e *Editorleaf) SwapCursorAndMark() {
	lastMark := e.findLastByEditBuffer()
	if lastMark == nil {
		gecore.Echo.AddText("No mark set")
		return
	}

	if lastMark.RowsPos == e.meta.RowsPos {
		if tmpMarkPos != nil {
			e.meta.RowsPos = tmpMarkPos.RowsPos
			tmpMarkPos = nil
			return
		}
		gecore.Echo.AddText("Already at the last mark")
		return
	} else {
		tmpMarkPos = mark.NewMark(e.editBuffer, e.meta.RowsPos, e.getContentWidthoutSpecialCharactor(e.meta.RowsPos, 20))
		e.meta.RowsPos = lastMark.RowsPos
	}
}

func (e *Editorleaf) FilterByCharacters(chars string) []*mark.Mark {
	items := []*mark.Mark{}

	for _, a := range marks.Marks.Items() {
		mk, ok := a.(*mark.Mark)
		if !ok {
			continue
		}

		text := fmt.Sprintf("%s %s", mk.EditBuffer.GetBase(), utils.RemoveSymbols(mk.Label))
		if chars != "" {
			if utils.ContainsAllCharacters(text, chars) {
				items = append(items, mk)
			}
		} else {
			items = append(items, mk)
		}
	}

	return items
}

// --------------------
// Region
// --------------------

// Copy region to kill buffer
func (e *Editorleaf) copyRegion(a, b rows.RowsPos) error {
	s := e.editBuffer.GetRegion(a, b)
	if s == nil {
		return nil
	}

	err := killbuffer.KillBuffer.PushKillBuffer(s.BytesArray(), e.editBuffer.GetNewLine().Bytes())
	return err
}

// Copy cursor region to Kill Buffer and Clipboard
func (e *Editorleaf) CopyRegion() {
	mark := e.findLastByEditBuffer()
	if mark == nil {
		gecore.Echo.AddText("The mark is not set now, so there is no region")
		return
	}
	if mark.RowIndex == e.meta.RowsPos.RowIndex && mark.ColIndex == e.meta.RowsPos.ColIndex {
		gecore.Echo.AddText("Mark and cursor position are the same, so there is no region")
		return
	}

	var err error
	if mark.RowIndex == e.meta.RowsPos.RowIndex {
		if mark.ColIndex > e.meta.RowsPos.ColIndex {
			err = e.copyRegion(e.meta.RowsPos, mark.RowsPos)
		} else {
			err = e.copyRegion(mark.RowsPos, e.meta.RowsPos)
		}
	} else if mark.RowIndex > e.meta.RowsPos.RowIndex {
		err = e.copyRegion(e.meta.RowsPos, mark.RowsPos)
	} else {
		err = e.copyRegion(mark.RowsPos, e.meta.RowsPos)
	}
	if err != nil {
		gecore.Echo.AddText("Copied, " + err.Error())
	} else {
		gecore.Echo.AddText("Copied")
	}
}

// Delete start to stop bytes and push the bytes to undo-stack and kill-buffer
// 開始から終了までのバイトを削除し、そのバイトを undo スタックと kill バッファにプッシュする
func (e *Editorleaf) killRegion(start, stop rows.RowsPos) {
	removed := e.editBuffer.RemoveRegion(start, stop)
	if removed == nil {
		return
	}
	// gelog.Debug("killRegion", "removed", removed.String([]byte{'\n'}))

	tree.GetRootTree().ForEachLeaf(func(l tree.Leaf) {
		ed, ok := l.(*Editorleaf)
		if !ok {
			return
		}
		ed.bsArray.ClearRow(start.RowIndex)
		ed.bsArray.Delete(start.RowIndex+1, stop.RowIndex+1)
	})

	e.syncCursorAndBufferForEdit(DELETE, start, stop)
	e.editBuffer.UndoAction.PushAction(&editbuffer.EditAction{
		Class:  editbuffer.DELETE_BACKWARD,
		Before: start,
		After:  stop,
		Data:   removed,
	}, true)

	if err := killbuffer.KillBuffer.PushKillBuffer(removed.BytesArray(), e.editBuffer.GetNewLine().Bytes()); err != nil {
		gecore.Echo.AddText(err.Error())
	}

	e.meta.RowsPos = start
}

// Kill region between last mark to cursor
// and push undo and kill buffers
func (e *Editorleaf) KillRegion() {
	mark := e.findLastByEditBuffer()
	if mark == nil {
		gecore.Echo.AddText("The mark is not set now, so there is no region")
		return
	}
	if mark.RowIndex == e.meta.RowsPos.RowIndex && mark.ColIndex == e.meta.RowsPos.ColIndex {
		gecore.Echo.AddText("Mark and cursor position are the same, so there is no region")
		return
	}

	gelog.Debug("KillRegion", "mark", fmt.Sprintf("%d:%d", mark.RowsPos.RowIndex, mark.RowsPos.ColIndex), "current", fmt.Sprintf("%d:%d", e.meta.RowsPos.RowIndex, e.meta.RowsPos.ColIndex))

	if mark.RowIndex == e.meta.RowsPos.RowIndex {
		if mark.ColIndex > e.meta.RowsPos.ColIndex {
			e.killRegion(e.meta.RowsPos, mark.RowsPos)
		} else {
			e.killRegion(mark.RowsPos, e.meta.RowsPos)
		}
	} else if mark.RowIndex > e.meta.RowsPos.RowIndex {
		e.killRegion(e.meta.RowsPos, mark.RowsPos)
	} else {
		e.killRegion(mark.RowsPos, e.meta.RowsPos)
	}
	gecore.Echo.AddText("Copied")
}

// --------------------
// Undo / Redo
// --------------------

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
			e.meta.RowsPos = a.Before

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
			e.meta.RowsPos = a.Before

		case editbuffer.DELETE_BACKWARD:
			gelog.Debug("Undo DELETE_BACKWARD", "after", fmt.Sprintf("%d:%d", a.After.RowIndex, a.After.ColIndex), "before", fmt.Sprintf("%d:%d", a.Before.RowIndex, a.Before.ColIndex), "data", a.Data.String([]byte{'\n'}))

			e.insertRows(a.Data, false)
			e.syncCursorAndBufferForEdit(
				INSERT,
				a.After,
				a.Before,
			)
			e.meta.RowsPos = a.After

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
			e.meta.RowsPos = a.Before
			e.insertRows(a.Data, false)
			e.meta.RowsPos = a.After

		case editbuffer.DELETE_BACKWARD:
			gelog.Debug("Redo DELETE_BACKWARD", "after", fmt.Sprintf("%d:%d", a.After.RowIndex, a.After.ColIndex), "before", fmt.Sprintf("%d:%d", a.Before.RowIndex, a.Before.ColIndex), "data", a.Data.String([]byte{'\n'}))

			e.meta.RowsPos = a.Before
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
			// e.meta.RowsPos = a.After
			e.meta.RowsPos = a.Before

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
			e.meta.RowsPos = a.After

		default:
			return
		}

	}

	e.rebuildBufferState()
}

func (e *Editorleaf) cursorAfterDelete(a *editbuffer.EditAction) rows.RowsPos {
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

// --------------------
// Other
// --------------------

func (e *Editorleaf) SetRowsPos(c rows.RowsPos) {
	e.meta.RowsPos = c
}

func (e Editorleaf) SetRows(r rows.Rows) {
	e.editBuffer.SetRows(r)

	tree.GetRootTree().ForEachLeaf(func(l tree.Leaf) {
		ed, ok := l.(*Editorleaf)
		if !ok {
			return
		}
		ed.bsArray.ClearAll()
	})
}

/*
func (e Editorleaf) SetRows(r [][]byte) {
	e.editBuffer.SetBytesArray(r)

	tree.GetRootTree().ForEachLeaf(func(l tree.Leaf) {
		ed, ok := l.(*Editorleaf)
		if !ok {
			return
		}
		ed.bsArray.ClearAll()
	})
}
*/

func (e *Editorleaf) RowsLength() int {
	return e.editBuffer.Rows.Length()
}

// 編集中のテキストの []byte を返す
// editorleaf.editbuffer.Bytes() のラッパー
func (e *Editorleaf) GetBytes() ([]byte, []int, error) {
	return e.editBuffer.Bytes()
}

func (e Editorleaf) IsEndOfLine() bool {
	line := (*e.editBuffer.Rows)[e.meta.RowsPos.RowIndex]
	return len(line)-1 == e.meta.RowsPos.ColIndex
}

func (e *Editorleaf) CharInfo() {
	ch, str := e.CharInfoOnCursor()

	s := fmt.Sprintf("Char: '%s' (dec: %d, oct: %s, hex: %02X, %s), Cursor index: %d,%d", str, ch, strconv.FormatInt(int64(ch), 8), ch, utils.WidthKindString(ch), e.meta.RowsPos.RowIndex, e.meta.RowsPos.ColIndex)
	gecore.Echo.AddText(s)
}

func (e *Editorleaf) IsDirtyFlag() bool {
	return e.editBuffer.IsDirtyFlag()
}

func (e *Editorleaf) Recenter() {
	e.meta.ScreenPos.Row = int(e.editArea.Height / 2)
}

// --------------------
// Utilities
// --------------------

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
	beforeCursor := e.meta.RowsPos

	e.editBuffer.Rows.InsertRegion(e.meta.RowsPos.RowIndex, e.meta.RowsPos.ColIndex, data)

	// Current cursor
	e.meta.RowsPos.RowIndex += data.Length() - 1
	// increase := len(rs.BytesArray()[rs.Length()-1])
	increase := data.Row(data.Length() - 1).Length()
	if data.Length() == 1 {
		e.meta.RowsPos.ColIndex = beforeCursor.ColIndex + increase
	} else {
		e.meta.RowsPos.ColIndex = increase
	}
	// 正確に更新するには論理行の計算が必要
	// e.meta.Cy += data.Length() - 1

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
		e.editBuffer.UndoAction.PushAction(&editbuffer.EditAction{Class: editbuffer.INSERT, Before: beforeCursor, After: e.meta.RowsPos, Data: data}, true)
	}
	e.syncCursorAndBufferForEdit(INSERT, beforeCursor, e.meta.RowsPos)

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
}

// Wrapper is insertBytes
func (e *Editorleaf) InsertString(s string) {
	e.insertBytes([]byte(s), true)
}

// Wrapper is insertBytes
func (e *Editorleaf) InsertRune(ch rune) {
	e.insertBytes(utils.RuneToBytes(ch), true)
}

func (e *Editorleaf) CharInfoOnCursor() (rune, string) {
	isEOF := false
	var ch rune
	var str string
	if e.editBuffer.Rows.Row(e.meta.RowsPos.RowIndex).IsColIndexAtRowEnd(e.meta.RowsPos.ColIndex) {
		if e.editBuffer.Rows.IsRowIndexLastRow(e.meta.RowsPos.RowIndex) {
			isEOF = true
			ch = define.EOF
		} else {
			ch = '\n'
		}
	} else {
		ch, _, _ = (*e).editBuffer.Rows.Row(e.meta.RowsPos.RowIndex).DecodeRune(e.meta.RowsPos.ColIndex)
	}
	if isEOF {
		str = "EOF"
	} else {
		str = e.RuneStatus(ch)
	}
	return ch, str
}

// The provided Go function getColumnIndexClosestToCursorXPosition calculates the column index (colIndex) and the cursor's horizontal position (cx) in the logical row at a specific horizontal cursor position (cursorXPos).
// It does so by decoding the UTF-8 runes in the row and accumulating their widths until it reaches or surpasses cursorXPos. Here's an explanation of the code
// この関数 getColumnIndexClosestToCursorXPosition は、特定の水平カーソル位置 (cursorXPos) に最も近い論理行のカラムインデックス (colIndex) とカーソル位置 (cx) を計算します。
// この処理は、行内の UTF-8 ルーンをデコードし、それらの幅を累積して cursorXPos に到達または超えるまで続けます。
func (e *Editorleaf) getColumnIndexClosestToCursorXPosition(rowIndex, indexOfLogicalRow, cursorXPos int) (colIndex, cx int) {
	// Get the boundaries of the logical row within the physical row.
	bo := e.bsArray.Boundary(rowIndex, indexOfLogicalRow)
	// Get the line content for the specified rowIndex.
	row := e.editBuffer.Row(rowIndex)
	// Initialize colIndex to the start index of the logical row.
	for colIndex = bo.StartLogicalRowByteIndex; ; {
		// Decode the next rune starting from colIndex.
		ch, size := utf8.DecodeRune((*row)[colIndex:])
		// Get the display width of the rune.
		w := e.locale.RuneWidth(ch)
		// Check if adding the width of the rune exceeds cursorXPos or if colIndex reaches the end of the logical row.
		if cx+w > cursorXPos || colIndex+size >= bo.StopLogicalRowByteIndex {
			return colIndex, cx
		}
		// Update the horizontal cursor position.
		cx += w
		// Advance colIndex by the size of the decoded rune.
		colIndex += size
	}
}

// Return content widthout special charactor
func (e *Editorleaf) getContentWidthoutSpecialCharactor(current rows.RowsPos, maxContentWidth int) (content string) {
	isSpecialChar := func(ch rune) bool {
		return ch < 32 || ch == define.DEL || ch == '　' || ch == define.NO_BREAK_SPACE
	}

	width := 0
	skip := false
	startCol := current.ColIndex
	for y := current.RowIndex; y < e.editBuffer.Rows.Length(); y++ {
		row := e.editBuffer.Rows.Row(y)
		for x := startCol; x < len(*row); {
			ch, size := utf8.DecodeRune((*row)[x:])
			w := e.locale.RuneWidth(ch)
			x += size // Don't use x below
			s := string(ch)
			if isSpecialChar(ch) {
				if skip {
					continue
				}
				skip = true
				switch ch {
				case '\n':
					s = string(theme.MarkNewline)
				case '\t':
					s = string(' ')
				default:
					s = string(theme.MarkContinue)
				}
				w = 1
			} else {
				skip = false
			}
			if width+w > maxContentWidth {
				return content
			}
			content += s
			width += w
		}
		startCol = 0
	}
	return content
}
