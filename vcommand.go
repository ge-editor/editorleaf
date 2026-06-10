package editorleaf

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/atotto/clipboard"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/editorleaf/mark"
	"github.com/ge-editor/editorleaf/search"
	"github.com/ge-editor/gecore"
	"github.com/ge-editor/gecore/define"
	"github.com/ge-editor/gecore/killbuffer"
	"github.com/ge-editor/gelog"
	"github.com/ge-editor/locale"
	"github.com/ge-editor/theme"
	"github.com/ge-editor/utils"
)

// ------------------------------------------------------------------
// File
// ------------------------------------------------------------------

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
	rows := e.editBuffer.Rows()
	if e.meta.RowIndex >= rows.Length() {
		e.meta.RowIndex = rows.Length() - 1
	}

	row := rows.Row(e.meta.RowIndex)
	i := e.meta.ColIndex
	if i >= row.Length() {
		i = row.Length() - 1
	}
	for i > 0 {
		if _, _, ok := row.DecodeRune(i); ok {
			break
		}
		i--
	}
	e.meta.ColIndex = i
}

func (e *Editorleaf) IsDirtyFlag() bool {
	return e.editBuffer.IsDirtyFlag()
}

// If the file does not exist, a backup error will occur
func (e *Editorleaf) SaveFile() {
	backupMessage := ""
	if err := e.editBuffer.Backup(); err != nil {
		backupMessage = " (" + err.Error() + ")"
	}

	result, err := e.editBuffer.Save()
	// if IsErrorCode(err, editbuffer.ErrSaved) {
	if result&editbuffer.ResultSaved != 0 {
		// if IsErrorCode(err, editbuffer.ErrFormatted) {
		if result&editbuffer.ResultFormatted != 0 {
			e.adjustFormattedCursorPosition()
			e.bsArray.ClearAll()
		}
		// May need adjust cursor position if formatted content
		rowLength := e.editBuffer.Rows().Length()
		if e.meta.RowIndex >= rowLength {
			e.meta.RowIndex = rowLength - 1
		}
		line := (*e.editBuffer.Rows())[e.meta.RowIndex]
		colLength := len(line)
		if e.meta.ColIndex >= colLength {
			// cursor on newline or EOF
			e.meta.ColIndex = colLength - 1
		}
		for !utf8.RuneStart(line[e.meta.ColIndex]) && e.meta.ColIndex > 0 {
			e.meta.ColIndex--
		}
		gecore.Echo.AddText("Wrote " + e.editBuffer.GetPath() + backupMessage)
	} else {
		gecore.Echo.AddText(err.Error() + backupMessage)
	}

	// e.UndoAction.MoveTo(e.RedoAction)
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

func (e *Editorleaf) RowsLength() int {
	return e.editBuffer.Rows().Length()
}

// ------------------------------------------------------------------
// Move cursor
// ------------------------------------------------------------------

// Move cursor to next word.
func (e *Editorleaf) MoveCursorNextWord() {
	x, y := e.meta.Cx, e.meta.Cy

	lines := e.editBuffer.Rows()
	// line, _ := lines.GetRow(e.meta.RowIndex)
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
		return
	}

	var prevCc, cc locale.CharClass
	notUppercaseBit := ^locale.UPPERCASE
	for {
		ch, size, ok := (*lines).Row(e.meta.RowIndex).DecodeRune(e.meta.ColIndex)
		if !ok {
			gelog.Error("error")
			panic("err")
		}
		/* w, ok := e.runeWidth(ch, e.meta.RowIndex, e.meta.ColIndex)
		if !ok {
			gelog.Error("error")
		}
		*/
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
		//e.makeAvailableBoundariesArray(e.meta.RowIndex) // -------- !
		if e.isEndOfLogicalRow(e.meta.RowIndex, e.meta.ColIndex) {
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

func (e *Editorleaf) isEndOfLogicalRow(rowIndex, colIndex int) bool {
	_, size := utf8.DecodeRune((*e.editBuffer.Rows())[rowIndex][colIndex:])
	for i := 0; i < e.bsArray.BoundariesLen(rowIndex); i++ {
		if colIndex+size == e.bsArray.Boundary(rowIndex, i).StopLogicalRowByteIndex {
			return true
		}
	}
	return false
}

func (e *Editorleaf) MoveCursorPreviousWord() {
	x, y := e.meta.Cx, e.meta.Cy

	if e.meta.ColIndex == 0 {
		if e.meta.RowIndex == 0 {
			gecore.Echo.AddText("Beginning of buffer")
			return
		}
		y--
		e.meta.RowIndex-- // previous line
		//e.makeAvailableBoundariesArray(e.meta.RowIndex) // -------- !
		//bs := e.bsay.Boundaries(e.meta.RowIndex)
		//lastBs := bs.LastBoundary()
		lastBs := e.bsArray.LastBoundary(e.meta.RowIndex)
		// lastBs := e.bsay[e.meta.RowIndex].boundaries[e.bsay[e.meta.RowIndex].Len()-1]
		ch, _, colIndex, _ := e.editBuffer.Rows().Row(e.meta.RowIndex).DecodeEndRune()
		w := e.locale.RuneWidth(ch)    // , e.meta.RowIndex, colIndex)
		x = lastBs.LogicalRowWidth - w // on newline
		e.meta.ColIndex = colIndex     // lastBs.stopIndex - size
	} else {
		var prevCc, cc locale.CharClass
		notUppercaseBit := ^locale.UPPERCASE
		for {
			//e.makeAvailableBoundariesArray(e.meta.RowIndex) // -------- !
			before, ok := e.getIndexOfLogicalRow(e.meta.RowIndex, e.meta.ColIndex)
			if !ok {
				panic("1")
			}
			ch, _, colIndex, ok := e.editBuffer.Rows().Row(e.meta.RowIndex).DecodePrevRune(e.meta.ColIndex)
			if !ok {
				// panic("2")
				break
			}
			w := e.locale.RuneWidth(ch)
			if !ok {
				gelog.Error("error")
			}
			prevCc = cc
			cc = e.locale.GetCharClass(ch)
			if prevCc != 0 {
				savePrevCC, saveCC := prevCc, cc
				prevCc &= notUppercaseBit
				cc &= notUppercaseBit
				if prevCc != cc && (cc&locale.TAB > 0 || cc&locale.SPACE > 0 || cc&locale.SYMBOL > 0) {
					break
				}
				prevCc, cc = savePrevCC, saveCC
			}
			e.meta.ColIndex = colIndex
			after, ok := e.getIndexOfLogicalRow(e.meta.RowIndex, e.meta.ColIndex)
			if !ok {
				panic("3")
			}
			if after < before {
				y--
				//bs := e.bsay.Boundaries(e.meta.RowIndex)
				//x = bs[after].Width - w
				x = e.bsArray.Boundary(e.meta.RowIndex, after).LogicalRowWidth
				// x = e.bsay[e.meta.RowIndex].boundaries[after].Width - w
			} else {
				x -= w
			}
			if prevCc != 0 && prevCc&locale.UPPERCASE == 0 && cc&locale.UPPERCASE > 0 {
				break
			}
		}
	}

	e.meta.PrevCx = x
	e.moveCursor(x, y)
}

// Move cursor one character forward.
func (e *Editorleaf) MoveCursorForward() {
	// gelog.Info("MoveCursorForward")
	x, y := e.meta.Cx, e.meta.Cy

	lines := e.editBuffer.Rows()
	// line, _ := lines.GetRow(e.meta.RowIndex)
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
		// gelog.Info("MoveCursorForward 1")
	} else {
		// gelog.Info("MoveCursorForward 2")
		// bo := e.boundariesArray.GetBoundaries(e.meta.RowIndex)
		// ch, size, ok := lines.DecodeRune(e.meta.RowIndex, e.meta.ColIndex)
		ch, size, ok := (*lines).Row(e.meta.RowIndex).DecodeRune(e.meta.ColIndex)
		if !ok {
			gelog.Error("error")
			panic("err")
		}
		w := e.locale.RuneWidth(ch) //, e.meta.RowIndex, e.meta.ColIndex)
		if !ok {
			gelog.Error("error")
		}
		// gelog.Info("MoveCursorForward %q, %d %d %v", ch, size, w, ok)
		//e.makeAvailableBoundariesArray(e.meta.RowIndex) // -------- !
		if e.isEndOfLogicalRow(e.meta.RowIndex, e.meta.ColIndex) {
			// gelog.Info("MoveCursorForward 3 %q %d,%d", ch, e.meta.RowIndex, e.meta.ColIndex)
			y++
			x = 0
		} else {
			// gelog.Info("MoveCursorForward 4")
			x += w
		}
		e.meta.ColIndex += size
	}

	// gelog.Info("MoveCursorForward x,y %d,%d col %d", x, y, e.meta.ColIndex)
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
		y--
		e.meta.RowIndex-- // previous line
		//e.makeAvailableBoundariesArray(e.meta.RowIndex) // -------- !
		//bs := e.bsay.Boundaries(e.meta.RowIndex)
		//lastBs := bs.LastBoundary()
		lastBs := e.bsArray.LastBoundary(e.meta.RowIndex)
		// lastBs := e.bsay[e.meta.RowIndex].boundaries[e.bsay[e.meta.RowIndex].Len()-1]
		ch, _, colIndex, _ := e.editBuffer.Rows().Row(e.meta.RowIndex).DecodeEndRune()
		w := e.locale.RuneWidth(ch)    //, e.meta.RowIndex, colIndex)
		x = lastBs.LogicalRowWidth - w // on newline
		e.meta.ColIndex = colIndex     // lastBs.stopIndex - size
	} else {
		//e.makeAvailableBoundariesArray(e.meta.RowIndex) // -------- !
		before, ok := e.getIndexOfLogicalRow(e.meta.RowIndex, e.meta.ColIndex)
		if !ok {
			panic("1")
		}
		// ch, _, colIndex, ok := e.Rows().DecodePrevRune(e.meta.RowIndex, e.meta.ColIndex)
		ch, _, colIndex, ok := e.editBuffer.Rows().Row(e.meta.RowIndex).DecodePrevRune(e.meta.ColIndex)
		if !ok {
			panic("2")
		}
		w := e.locale.RuneWidth(ch) // , e.meta.RowIndex, e.meta.ColIndex)
		e.meta.ColIndex = colIndex
		after, ok := e.getIndexOfLogicalRow(e.meta.RowIndex, e.meta.ColIndex)
		if !ok {
			panic("3")
		}
		if after < before {
			y--
			//bs := e.bsay.Boundaries(e.meta.RowIndex)
			//x = bs[after].Width - w
			x = e.bsArray.Boundary(e.meta.RowIndex, after).LogicalRowWidth - w
			// x = e.bsay[e.meta.RowIndex].boundaries[after].Width - w
		} else {
			// x -= vl.GetCell__(e.meta.ColIndex).Width
			x -= w
		}
	}

	e.meta.PrevCx = x
	e.moveCursor(x, y)
}

// Move cursor to the next line.
func (e *Editorleaf) MoveCursorNextLine() {
	var bo *Boundary

	//e.makeAvailableBoundariesArray(e.meta.RowIndex)        // -------- !
	if e.inEndOfLogicalRow(e.meta.RowIndex, e.meta.ColIndex) { // last logical line
		if e.editBuffer.Rows().IsRowIndexLastRow(e.meta.RowIndex) {
			gecore.Echo.AddText("End of buffer")
			return
		}
		// move to next line
		e.meta.RowIndex++
		//e.makeAvailableBoundariesArray(e.meta.RowIndex) // -------- !
		// bo = e.bsay.Boundaries(e.meta.RowIndex)[0]
		bo = e.bsArray.Boundary(e.meta.RowIndex, 0)
		// bo = e.bsay[e.meta.RowIndex].boundaries[0]      // first logical line
	} else {
		// move to next logical line
		//e.makeAvailableBoundariesArray(e.meta.RowIndex) // -------- !
		// What index number in logic line?
		i, _ := e.getIndexOfLogicalRow(e.meta.RowIndex, e.meta.ColIndex)
		i++ // next logical line
		// bo = e.bsay.Boundaries(e.meta.RowIndex)[i]
		bo = e.bsArray.Boundary(e.meta.RowIndex, i)
		// bo = e.bsay[e.meta.RowIndex].boundaries[i]
	}

	x := 0
	i := bo.StartLogicalRowByteIndex
	for {
		ch, size, ok := e.editBuffer.Rows().Row(e.meta.RowIndex).DecodeRune(i)
		if !ok {
			panic("MoveCursorNextLine")
		}
		w := e.locale.RuneWidth(ch) //, e.meta.RowIndex, i)
		if x+w >= e.meta.PrevCx || i+size >= bo.StopLogicalRowByteIndex {
			break
		}
		x += w
		i += size
	}
	e.meta.ColIndex = i

	e.meta.Cx = x
	e.meta.Cy++
}

// Move cursor to the previous line.
func (e *Editorleaf) MoveCursorPrevLine() {
	// What index number in logic row?
	//e.makeAvailableBoundariesArray(e.meta.RowIndex) // -------- !
	indexOfLogicalRow, _ := e.getIndexOfLogicalRow(e.meta.RowIndex, e.meta.ColIndex)

	var bo *Boundary
	if indexOfLogicalRow == 0 { // first logical line
		if e.meta.RowIndex == 0 {
			gecore.Echo.AddText("Beginning of buffer")
			return
		}
		// move to prev row
		e.meta.RowIndex--
		//e.makeAvailableBoundariesArray(e.meta.RowIndex) // -------- !
		//bs := e.bsay.Boundaries(e.meta.RowIndex)
		//bo = bs.LastBoundary() // last logical row
		bo = e.bsArray.LastBoundary(e.meta.RowIndex) // last logical row
		// bo = e.bsay[e.meta.RowIndex].boundaries[e.bsay[e.meta.RowIndex].Len()-1] // last logical row
	} else {
		//e.makeAvailableBoundariesArray(e.meta.RowIndex) // -------- !
		//bs := e.bsay.Boundaries(e.meta.RowIndex)
		//bo = bs[indexOfLogicalRow-1]                          //
		bo = e.bsArray.Boundary(e.meta.RowIndex, indexOfLogicalRow-1) //
		// bo = e.bsay[e.meta.RowIndex].boundaries[indexOfLogicalRow-1] //
	}

	x := 0
	i := bo.StartLogicalRowByteIndex
	for {
		ch, size, _ := e.editBuffer.Rows().Row(e.meta.RowIndex).DecodeRune(i)
		w := e.locale.RuneWidth(ch) //, e.meta.RowIndex, i)
		if x+w > e.meta.PrevCx || i+size >= bo.StopLogicalRowByteIndex {
			break
		}
		x += w
		i += size
	}
	e.meta.ColIndex = i

	e.moveCursor(x, e.meta.Cy-1)
}

// Move cursor to the end of the line.
func (e *Editorleaf) MoveCursorEndOfLine() {
	//e.makeAvailableBoundariesArray(e.meta.RowIndex) // -------- !
	// What index number in logic row?
	indexOfLogicalRow, _ := e.getIndexOfLogicalRow(e.meta.RowIndex, e.meta.ColIndex)

	colLength := e.editBuffer.Rows().Row(e.meta.RowIndex).Length()
	e.meta.ColIndex = colLength - 1

	// cursor display position
	// e.meta.Cy += len(e.boundariesArray[e.meta.RowIndex]) - 1 - indexOfLogicalRow
	// bs := e.bsay.Boundaries(e.meta.RowIndex)
	// e.meta.Cy += bs.Len() - 1 - indexOfLogicalRow
	e.meta.Cy += e.bsArray.BoundariesLen(e.meta.RowIndex) - 1 - indexOfLogicalRow
	// e.meta.Cy += e.bsay[e.meta.RowIndex].Len() - 1 - indexOfLogicalRow
}

// Move cursor to the end of logical the line.
// At the end of a logical line, the cursor should at the beginning of the next logical line. so I see...
func (e *Editorleaf) MoveCursorEndOfLogicalLine() {
	//e.makeAvailableBoundariesArray(e.meta.RowIndex) // -------- !
	// What index number in logic row?
	indexOfLogicalRow, _ := e.getIndexOfLogicalRow(e.meta.RowIndex, e.meta.ColIndex)
	//bs := e.bsay.Boundaries(e.meta.RowIndex)
	// bs := &e.bsay[e.meta.RowIndex].boundaries
	// bo := bs[indexOfLogicalRow]
	bo := e.bsArray.Boundary(e.meta.RowIndex, indexOfLogicalRow)
	// if indexOfLogicalRow == len(*bs)-1 {
	// if indexOfLogicalRow == bs.Len()-1 {
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

// Move cursor to the beginning of the line.
// Consider indentation
func (e *Editorleaf) MoveCursorBeginningOfLine() {
	if e.meta.RowIndex == 0 && e.meta.ColIndex == 0 {
		gecore.Echo.AddText("Beginning of buffer")
		return
	}

	nowIndexOfLogicalRow, _ := e.getIndexOfLogicalRow(e.meta.RowIndex, e.meta.ColIndex)
	rows := e.editBuffer.Rows()
	indentedIndex := 0
	indentedWidth := 0
	for indentedIndex < rows.Row(e.meta.RowIndex).Length() {
		ch, size, _ := rows.Row(e.meta.RowIndex).DecodeRune(indentedIndex)
		if ch != ' ' && ch != '\t' {
			break
		}
		w := e.locale.RuneWidth(ch) //, e.meta.RowIndex, indentedIndex)
		indentedWidth += w
		indentedIndex += size
	}
	newIndexOfLogicalRow, _ := e.getIndexOfLogicalRow(e.meta.RowIndex, indentedIndex)

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

// Move cursor to the beginning of the logical row.
// Consider logical row
// Consider indentation....
func (e *Editorleaf) MoveCursorBeginningOfLogicalLine() {
	if e.meta.RowIndex == 0 && e.meta.ColIndex == 0 {
		gecore.Echo.AddText("Beginning of buffer")
		return
	}

	//e.makeAvailableBoundariesArray(e.meta.RowIndex) // -------- !
	nowIndexOfLogicalRow, _ := e.getIndexOfLogicalRow(e.meta.RowIndex, e.meta.ColIndex)
	if nowIndexOfLogicalRow == 0 {
		e.MoveCursorBeginningOfLine()
		return
	}
	// bo := e.boundariesArray[e.meta.RowIndex][nowIndexOfLogicalRow]
	//bs := e.bsay.Boundaries(e.meta.RowIndex)
	//bo := bs[nowIndexOfLogicalRow]
	bo := e.bsArray.Boundary(e.meta.RowIndex, nowIndexOfLogicalRow)

	if e.meta.ColIndex == bo.StartLogicalRowByteIndex {
		if nowIndexOfLogicalRow == 1 {
			e.MoveCursorBeginningOfLine()
			return
		}

		bo := e.bsArray.Boundary(e.meta.RowIndex, nowIndexOfLogicalRow-1)
		e.meta.ColIndex = bo.StartLogicalRowByteIndex
		return
	}
	e.meta.ColIndex = bo.StartLogicalRowByteIndex
	e.meta.Cx = 0
}

func (e *Editorleaf) MoveCursorBeginningOfFile() {
	e.meta.RowIndex = 0
	e.meta.ColIndex = 0
	e.meta.Cy = 0
	e.meta.Cx = 0
	// e.vlines.AllocateVlines__(e.meta.RowIndex)
}

func (e *Editorleaf) MoveCursorEndOfFile() {
	// e.meta.RowIndex = e.Rows().RowLength() - 1
	e.meta.RowIndex = e.editBuffer.Rows().Length() - 1
	// e.drawLine(0, e.meta.RowIndex, 0, false)
	// bo := e.boundariesArray[e.meta.RowIndex][len(e.boundariesArray[e.meta.RowIndex])-1]
	//e.makeAvailableBoundariesArray(e.meta.RowIndex) // -------- !
	//bs := e.bsay.Boundaries(e.meta.RowIndex)
	//lastBs := bs.LastBoundary()
	lastBs := e.bsArray.LastBoundary(e.meta.RowIndex)
	// bo := e.bsay[e.meta.RowIndex].boundaries[e.bsay[e.meta.RowIndex].Len()-1]
	e.meta.ColIndex = lastBs.StopLogicalRowByteIndex - 1 // left of the LF or EOF
	e.meta.Cx = lastBs.LogicalRowWidth - 1               // left of the LF or EOF
	e.meta.Cy = e.editArea.Height - e.verticalThreshold
}

// Move view 'n' lines forward or backward only if it's possible.
func (e *Editorleaf) MoveViewHalfForward() {
	// n := e.Height / 2
	n := e.screen.Height / 2
	gecore.Echo.AddText("")

	// What index is e.meta.ColIndex in the logical line?
	indexOfLogicalRow, _ := e.getIndexOfLogicalRow(e.meta.RowIndex, e.meta.ColIndex)

	// Compute total, logicalRowLength, rowIndex and vl
	total := -indexOfLogicalRow
	logicalRowLength := 0
	rowIndex := e.meta.RowIndex
	for ; ; rowIndex++ {
		// e.drawLine(0, rowIndex, 0, false)
		//e.makeAvailableBoundariesArray(rowIndex) // -------- !
		// logicalRowLength = len(e.boundariesArray[rowIndex])
		// logicalRowLength = e.bsay.Boundaries(rowIndex).Len()
		logicalRowLength = e.bsArray.BoundariesLen(rowIndex)
		// Add the number of logical row. first subtract the number of logical row before the cursor
		total += logicalRowLength
		// indexOfLogicalRow = 0 // No need to subtract after the second time, so 0
		// if total >= n || rowIndex == e.Rows().RowLength()-1 {
		if total >= n || rowIndex == e.editBuffer.Rows().Length()-1 {
			break
		}
	}
	e.meta.RowIndex = rowIndex

	// Compute logicalRowIndex
	indexOfLogicalRow = logicalRowLength - 1
	// If the number of lines to scroll is exceeded,
	// subtract the number of logical lines exceeded
	if total > n {
		indexOfLogicalRow -= total - n
		/*
			if logicalRowIndex < 0 {
				panic("logicalRowIndex")
			}
		*/
	}

	e.meta.ColIndex, _ = e.getColumnIndexClosestToCursorXPosition(e.meta.RowIndex, indexOfLogicalRow, e.meta.PrevCx)
}

// Move view 'n' lines forward or backward.
func (e *Editorleaf) MoveViewHalfBackward( /* n int */ ) {
	// n := e.Height / 2
	n := e.screen.Height / 2
	gecore.Echo.AddText("")

	// What index number in logic row?
	//e.makeAvailableBoundariesArray(e.meta.RowIndex) // -------- !
	indexOfLogicalRow, _ := e.getIndexOfLogicalRow(e.meta.RowIndex, e.meta.ColIndex)

	total := n
	logicalRowLength := indexOfLogicalRow
	rowIndex := e.meta.RowIndex
	for {
		total -= logicalRowLength // Subtract the number of logical rows
		if total <= 0 || rowIndex == 0 {
			break
		}
		rowIndex--
		//e.makeAvailableBoundariesArray(rowIndex) // -------- !
		// logicalRowLength = vl.LenLogicalRow__()
		// logicalRowLength = len(e.boundariesArray[rowIndex])
		// logicalRowLength = e.bsay.Boundaries(rowIndex).Len()
		logicalRowLength = e.bsArray.BoundariesLen(rowIndex)
	}
	e.meta.RowIndex = rowIndex

	// Compute logicalRowIndex
	indexOfLogicalRow = 0 // logicalRowLength - 1
	// If the number of lines to scroll is exceeded,
	// subtract the number of logical lines exceeded
	if total > n {
		indexOfLogicalRow -= n
	}

	// e.drawLine(0, e.meta.RowIndex, e.meta.PrevCx, false)
	// e.makeAvailableBoundaries(e.meta.RowIndex) // -------- !
	e.meta.ColIndex, _ = e.getColumnIndexClosestToCursorXPosition(e.meta.RowIndex, indexOfLogicalRow, e.meta.PrevCx)
}

func (e *Editorleaf) MoveCursorToLine(lineNumber int) {
	if lineNumber < 1 || lineNumber > e.editBuffer.Rows().Length() {
		return
	}
	e.meta.RowIndex = lineNumber - 1
	e.meta.ColIndex = 0
	// e.meta.Cy = (e.Height - 1) / 2
	e.meta.Cy = (e.screen.Height - 1) / 2
}

func (e *Editorleaf) InsertTab() {
	if (*e.editBuffer.GetLangMode()).GetSoftTab() {
		w := utils.TabWidth(e.meta.Cx, e.editBuffer.GetTabWidth())
		for range w { // for i := 0; i < w; i++ {
			// e.InsertRune__(' ')
			e.insertBytes([]byte{' '}, true)
		}
	} else {
		// e.InsertRune__('\t')
		e.insertBytes([]byte{'\t'}, true)
	}
}

// ------------------------------------------------------------------
// Edit
// ------------------------------------------------------------------

// Wrapper is insertBytes
func (e *Editorleaf) InsertString(s string) {
	e.insertBytes([]byte(s), true)
}

// Wrapper is insertBytes
func (e *Editorleaf) InsertRune(ch rune) {
	e.insertBytes(utils.RuneToBytes(ch), true)
}

func (e *Editorleaf) DeleteRuneBackward() {
	start := e.meta.Cursor
	stop := e.meta.Cursor
	_, _, colIndex, _ := e.editBuffer.Rows().Row(e.meta.RowIndex).DecodePrevRune(e.meta.ColIndex)

	if e.meta.ColIndex == 0 {
		if e.meta.RowIndex == 0 {
			gecore.Echo.AddText("Beginning of buffer")
			return
		}
		// join to prev row
		e.bsArray.rows = slices.Delete(e.bsArray.rows, start.RowIndex, start.RowIndex+1)
		e.bsArray.Clear(start.RowIndex - 1)
		lines := e.editBuffer.Rows()
		start.RowIndex--
		start.ColIndex = len((*lines)[start.RowIndex]) - 1
	} else {
		e.bsArray.Clear(start.RowIndex)
		start.ColIndex = colIndex
	}
	removed := e.editBuffer.RemoveRegion(start, stop)
	if removed == nil {
		return
	}
	e.meta.Cursor = start

	/*
		if count := stop.RowIndex - start.RowIndex; count > 0 {
			e.bsArray.Delete(start.RowIndex+1, count)
		}
	*/

	e.editBuffer.UndoAction.PushAction(&editbuffer.EditAction{Class: editbuffer.DELETE_BACKWARD, Before: stop, After: start, Data: *removed})
	e.syncCursorAndBufferForEdit(DELETE, start, stop)

	e.meta.PrevCx = -1
}

// If at the EOL, move contents of the next line to the end of the current line,
// erasing the next line after that. Otherwise, delete one character under the
// cursor.
func (e *Editorleaf) DeleteRune() {
	start := e.meta.Cursor
	stop := e.meta.Cursor

	beRemovedCh, size, _ := e.editBuffer.Rows().Row(e.meta.RowIndex).DecodeRune(e.meta.ColIndex)
	if beRemovedCh == '\n' {
		e.bsArray.Clear(stop.RowIndex)
		stop.RowIndex++
		stop.ColIndex = 0
		e.bsArray.rows = slices.Delete(e.bsArray.rows, stop.RowIndex, stop.RowIndex+1)
	} else {
		e.bsArray.Clear(stop.RowIndex)
		stop.ColIndex += size
	}
	removed := e.editBuffer.RemoveRegion(start, stop)
	if removed == nil {
		return
	}

	/*
		if count := stop.RowIndex - start.RowIndex; count > 0 {
			e.bsArray.Delete(start.RowIndex+1, count)
		}
	*/

	e.editBuffer.UndoAction.PushAction(&editbuffer.EditAction{Class: editbuffer.DELETE, Before: start, After: start, Data: *removed})
	e.syncCursorAndBufferForEdit(DELETE, stop, start)

	e.meta.PrevCx = -1
}

func (e *Editorleaf) Autoindent() {
	lines := e.editBuffer.Rows()
	line := (*lines)[e.meta.RowIndex]
	indent := make([]byte, 0, len(line))
	indent = append(indent, '\n')
	for i := 0; i < len(line); {
		ch, size := utf8.DecodeRune(line[i:])
		if ch == ' ' || ch == '\t' {
			indent = append(indent, utils.RuneToBytes(ch)...)
			i += size
			continue
		}
		break
	}
	e.insertBytes(indent, true)
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

func (e *Editorleaf) SwapCursorAndMark() {
	m := Marks.FindLastByFile(e.editBuffer)
	if m == nil {
		gecore.Echo.AddText("The mark is not set now, so there is no region")
		return
	}

	Marks.UnsetMarkByValue(m)
	Marks.AddMark(mark.NewMark(e.editBuffer, e.meta.Cursor, e.getContentWidthoutSpecialCharactor(e.meta.Cursor, 20)))
	e.meta.Cursor = m.Cursor
}

func (e *Editorleaf) FilterByCharacters(chars string) []*mark.Mark {
	return Marks.FilterByCharacters(chars)
}

// ------------------------------------------------------------------
// Region
// ------------------------------------------------------------------

// Copy region to kill buffer
func (e *Editorleaf) copyRegion(a, b editbuffer.Cursor) error {
	s := e.editBuffer.GetRegion(a, b)
	if s == nil {
		return nil
	}
	err := killbuffer.KillBuffer.PushKillBuffer([]byte(string(*s)))
	return err
}

// Copy cursor region to Kill Buffer and Clipboard
func (e *Editorleaf) CopyRegion() {
	mark := Marks.FindLastByFile(e.editBuffer)
	if mark == nil {
		gecore.Echo.AddText("The mark is not set now, so there is no region")
		return
	}
	if mark.RowIndex == e.meta.RowIndex && mark.ColIndex == e.meta.ColIndex {
		gecore.Echo.AddText("Mark and cursor position are the same, so there is no region")
		return
	}

	var err error
	if mark.RowIndex == e.meta.RowIndex {
		if mark.ColIndex > e.meta.ColIndex {
			err = e.copyRegion(e.meta.Cursor, mark.Cursor)
		} else {
			err = e.copyRegion(mark.Cursor, e.meta.Cursor)
		}
	} else if mark.RowIndex > e.meta.RowIndex {
		err = e.copyRegion(e.meta.Cursor, mark.Cursor)
	} else {
		err = e.copyRegion(mark.Cursor, e.meta.Cursor)
	}
	if err != nil {
		gecore.Echo.AddText("Copied, " + err.Error())
	} else {
		gecore.Echo.AddText("Copied")
	}
}

// Delete start to stop bytes and push the bytes to undo-stack and kill-buffer
// 開始から終了までのバイトを削除し、そのバイトを undo スタックと kill バッファにプッシュする
func (e *Editorleaf) killRegion(start, stop editbuffer.Cursor) {
	e.meta.Cursor = start
	removed := e.editBuffer.RemoveRegion(start, stop)
	if removed == nil {
		return
	}

	e.bsArray.rows = slices.Delete(e.bsArray.rows, start.RowIndex+1, stop.RowIndex+1)
	e.bsArray.Clear(start.RowIndex)

	//e.makeAvailableBoundariesArray(start.RowIndex) // -------- !
	/*
		if count := stop.RowIndex - start.RowIndex; count > 0 {
			e.bsArray.Delete(start.RowIndex+1, count)
		}
	*/

	e.syncCursorAndBufferForEdit(DELETE, start, stop)
	e.editBuffer.UndoAction.PushAction(&editbuffer.EditAction{Class: editbuffer.DELETE_BACKWARD, Before: start, After: start, Data: *removed})
	if err := killbuffer.KillBuffer.PushKillBuffer([]byte(string(*removed))); err != nil {
		gecore.Echo.AddText(err.Error())
	}
}

// Kill region between last mark to cursor
// and push undo and kill buffers
func (e *Editorleaf) KillRegion() {
	mark := Marks.FindLastByFile(e.editBuffer)
	if mark == nil {
		gecore.Echo.AddText("The mark is not set now, so there is no region")
		return
	}
	if mark.RowIndex == e.meta.RowIndex && mark.ColIndex == e.meta.ColIndex {
		gecore.Echo.AddText("Mark and cursor position are the same, so there is no region")
		return
	}

	if mark.RowIndex == e.meta.RowIndex {
		if mark.ColIndex > e.meta.ColIndex {
			e.killRegion(e.meta.Cursor, mark.Cursor)
		} else {
			e.killRegion(mark.Cursor, e.meta.Cursor)
		}
	} else if mark.RowIndex > e.meta.RowIndex {
		e.killRegion(e.meta.Cursor, mark.Cursor)
	} else {
		e.killRegion(mark.Cursor, e.meta.Cursor)
	}
	gecore.Echo.AddText("Copied")
}

// unix-line-discard
// backward-kill-line
func (e Editorleaf) BackwardKillLine() {
	if e.meta.ColIndex == 0 {
		return
	}

	start := editbuffer.Cursor{
		RowIndex: e.meta.Cursor.RowIndex,
		ColIndex: 0,
	}

	before := e.meta.Cursor

	removed := e.editBuffer.RemoveRegion(start, e.meta.Cursor)
	if removed == nil {
		gelog.Debug("BackwardKillLine: RemoveRegion returned nil")
		return
	}

	// gelog.Info("removed", "'"+string(*removed)+"'")
	e.bsArray.Clear(e.meta.Cursor.RowIndex)

	e.syncCursorAndBufferForEdit(DELETE, start, e.meta.Cursor)

	e.editBuffer.UndoAction.PushAction(&editbuffer.EditAction{
		Class:  editbuffer.DELETE_BACKWARD,
		Before: before,
		After:  start,
		Data:   *removed,
	})

	e.meta.Cursor = start
	e.meta.PrevCx = -1
}

// Kill line:
// If not at the EOL, remove contents of the current line from the cursor to the end.
// Otherwise behave like 'delete'.
// 行を削除します:
// EOL でない場合は、カーソルから末尾までの現在の行の内容を削除します。
// それ以外の場合は、「delete」のように動作します。
func (e *Editorleaf) KillLine() {
	var removed *[]byte
	stop := e.meta.Cursor

	lines := e.editBuffer.Rows()
	// line, _ := lines.GetRow(e.meta.RowIndex)
	line := lines.Row(e.meta.RowIndex)
	//e.makeAvailableBoundariesArray(e.meta.RowIndex) // -------- !
	if line.IsColIndexAtRowEnd(e.meta.ColIndex) {
		if lines.IsRowIndexLastRow(e.meta.RowIndex) {
			gecore.Echo.AddText("End of buffer")
			return
		}
		// delete newline
		e.DeleteRune()
		return
	}

	l := lines.Row(e.meta.RowIndex).Length()
	stop.ColIndex = l - 1
	removed = e.editBuffer.RemoveRegion(e.meta.Cursor, stop)
	if removed == nil {
		return
	}

	e.bsArray.Clear(e.meta.RowIndex)
	// gecore.Echo.AddText(fmt.Sprintf("KillLine %d:%d", start.RowIndex+1, stop.RowIndex+1))
	//e.makeAvailableBoundariesArray(e.meta.RowIndex) // -------- !
	/*
		if count := stop.RowIndex - e.meta.RowIndex; count > 0 {
			e.bsArray.Delete(e.meta.RowIndex+1, count)
		}
	*/
	// e.bsay.bsayDelete(e.meta.RowIndex+1, stop.RowIndex+1)

	e.syncCursorAndBufferForEdit(DELETE, e.meta.Cursor, stop)
	e.editBuffer.UndoAction.PushAction(&editbuffer.EditAction{Class: editbuffer.DELETE, Before: e.meta.Cursor, After: e.meta.Cursor, Data: *removed})

	e.meta.PrevCx = -1
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
	e.insertBytes(r, true)
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

	actions := e.editBuffer.UndoAction.Undo() //.Pop()
	for _, a := range actions {
		// gelog.Info("e.UndoAction.Pop() %v %v '%s'", a.Before, a.After, string(a.Data))
		if a.Class == editbuffer.INSERT {
			data := e.editBuffer.RemoveRegion(a.Before, a.After)

			//if count := a.After.RowIndex - a.Before.RowIndex; count > 0 {
			if data != nil {
				count := a.After.RowIndex - a.Before.RowIndex
				e.bsArray.Delete(a.Before.RowIndex+1, count)
				e.bsArray.Clear(a.Before.RowIndex)
			}

			e.syncCursorAndBufferForEdit(DELETE, a.Before, a.After)
			e.meta.Cursor = a.Before
		} else if a.Class == editbuffer.DELETE {
			e.meta.Cursor = a.Before
			e.insertBytes(a.Data, false)
			e.syncCursorAndBufferForEdit(INSERT, a.Before, a.After)
			e.meta.Cursor = a.Before
		} else if a.Class == editbuffer.DELETE_BACKWARD {
			e.meta.Cursor = a.After
			e.insertBytes(a.Data, false)
			e.syncCursorAndBufferForEdit(INSERT, a.After, a.Before)
		} else {
			return
		}
	}

	gecore.Echo.AddText("Undo!")
	// e.RedoAction.Push(a)
}

func (e *Editorleaf) Redo() {
	if e.editBuffer.UndoAction.IsRedoEmpty() {
		gecore.Echo.AddText("No further redo information")
		return
	}

	actions := e.editBuffer.UndoAction.Redo()
	for _, a := range actions {
		if a.Class == editbuffer.INSERT {
			e.meta.Cursor = a.Before
			e.insertBytes(a.Data, false)
		} else if a.Class == editbuffer.DELETE_BACKWARD {
			data := e.editBuffer.RemoveRegion(a.After, a.Before)

			// if count := a.Before.RowIndex - a.After.RowIndex; count > 0 {
			if data != nil {
				count := a.Before.RowIndex - a.After.RowIndex
				e.bsArray.Delete(a.After.RowIndex+1, count)
				e.bsArray.Clear(a.After.RowIndex)
			}

			e.syncCursorAndBufferForEdit(DELETE, a.After, a.Before)
		} else if a.Class == editbuffer.DELETE {
			lines := editbuffer.SplitByLF(a.Data)
			last := lines[len(lines)-1]
			cursor := a.After
			cursor.RowIndex += len(lines) - 1
			cursor.ColIndex += len(last)
			if last[len(last)-1] == '\n' {
				cursor.RowIndex++
				cursor.ColIndex = 0
			}
			data := e.editBuffer.RemoveRegion(a.Before, cursor)

			// if count := cursor.RowIndex - a.Before.RowIndex; count > 0 {
			if data != nil {
				count := cursor.RowIndex - a.Before.RowIndex
				e.bsArray.Delete(a.Before.RowIndex+1, count)
				e.bsArray.Clear(a.Before.RowIndex)
			}

			e.syncCursorAndBufferForEdit(DELETE, a.Before, cursor)
		} else {
			return
		}
		e.meta.Cursor = a.After
	}

	// gecore.Echo.AddText("Redo!")
	// e.UndoAction.Push(a)
}

// ------------------------------------------------------------------
// Search and replace
// ------------------------------------------------------------------

func (e *Editorleaf) MoveNextFoundWord() {
	search := e.meta.Search
	foundIndexes := search.GetFindIndexes()

	if len(foundIndexes) == 0 {
		return
	}

	if search.CurrentSearchIndex == -1 {
		for i := 0; i < len(search.Indexes); i++ {
			if foundIndexes[i].Start.RowIndex >= e.meta.RowIndex {
				search.CurrentSearchIndex = i
				break
			}
		}
	} else if search.CurrentSearchIndex == len(foundIndexes)-1 {
		search.CurrentSearchIndex = 0
	} else {
		search.CurrentSearchIndex++
	}

	if search.CurrentSearchIndex < 0 {
		search.CurrentSearchIndex = 0
	} else if search.CurrentSearchIndex >= len(foundIndexes) {
		search.CurrentSearchIndex = len(foundIndexes) - 1
	}

	f := foundIndexes[search.CurrentSearchIndex]
	e.meta.RowIndex = f.Start.RowIndex
	e.meta.ColIndex = f.Start.ColIndex
}

func (e *Editorleaf) MovePrevFoundWord() {
	search := e.meta.Search
	foundIndexes := search.GetFindIndexes()

	if len(foundIndexes) == 0 {
		return
	}

	if search.CurrentSearchIndex == -1 {
		for i := len(foundIndexes) - 1; i >= 0; i-- {
			if foundIndexes[i].Start.RowIndex <= e.meta.RowIndex {
				search.CurrentSearchIndex = i
				break
			}
		}
	} else if search.CurrentSearchIndex == 0 {
		search.CurrentSearchIndex = len(foundIndexes) - 1
	} else {
		search.CurrentSearchIndex--
	}

	if search.CurrentSearchIndex < 0 {
		search.CurrentSearchIndex = 0
	} else if search.CurrentSearchIndex >= len(foundIndexes) {
		search.CurrentSearchIndex = len(foundIndexes) - 1
	}

	e.meta.RowIndex = foundIndexes[search.CurrentSearchIndex].Start.RowIndex
	e.meta.ColIndex = foundIndexes[search.CurrentSearchIndex].Start.ColIndex
}

// When not using regular expressions
func (e *Editorleaf) SearchText(text string, caseSensitive, isRegexp bool, ctx context.Context /* , wg *sync.WaitGroup */) {
	// defer wg.Done()

	search := e.meta.Search
	// foundIndexes := search.GetFindIndexes()

	search.CurrentSearchIndex = -1
	//foundIndexes = []FoundPosition{}

	textLen := len(text)
	if textLen == 0 {
		return
	}

	if isRegexp {
		e.SearchRegexp(text, caseSensitive, ctx)
	} else {
		e.searchText(text, caseSensitive, ctx)
	}
}

func (e *Editorleaf) SearchRegexp(searchTerm string, caseSensitive bool, ctx context.Context) {
	// search := e.Meta.Search
	// foundIndexes := search.GetFindIndexes()

	// rows := e.Rows__()
	rows := e.editBuffer.Rows()
	re, err := regexp.Compile(searchTerm)
	if err != nil {
		return
	}
	for i := 0; i < rows.Length(); i++ {
		//s := rows.Row__(i).String__()
		matches := re.FindAllSubmatchIndex((*rows)[i], -1)
		// s := string((*rows)[i])
		// matches := re.FindAllStringSubmatchIndex(s, -1)
		if matches == nil {
			continue
		}
		for _, match := range matches {
			select {
			case <-ctx.Done():
				return
			default:
				e.meta.Search.Indexes = append(e.meta.Search.Indexes, search.NewFoundPosition(i, match[0], i, match[1]))
			}
		}
	}
}

func (e *Editorleaf) searchText(text string, caseSensitive bool, ctx context.Context) {
	// search := e.Meta.Search
	// foundIndexes := search.GetFindIndexes()

	if !caseSensitive {
		text = strings.ToLower(text)
	}
	textBytes := []byte(text)
	textBytesLen := len(textBytes)

	lines := e.editBuffer.Rows()
	for i := 0; i < lines.Length(); i++ {
		line := (*lines)[i]
		index := 0
	loop:
		for limit := 0; ; limit++ {
			select {
			case <-ctx.Done():
				return
			default:
				substring := line[index:]
				if !caseSensitive {
					substring = bytes.ToLower(substring)
				}
				findIndex := bytes.Index(substring, textBytes)
				if findIndex == -1 {
					break loop
				}
				startIndex := len(line[:index+findIndex])
				stopIndex := startIndex + textBytesLen
				e.meta.Search.Indexes = append(e.meta.Search.Indexes, search.NewFoundPosition(i, startIndex, i, stopIndex))
				index += findIndex + textBytesLen
			}

			if limit > 100_000 {
				gecore.Echo.AddText("Search text over 100,000")
				return
			}
		}
	}
}

func (e *Editorleaf) ReplaceCurrentSearchString(str string) {
	// search := e.Meta.Search
	// foundIndexes := search.GetFindIndexes()

	if e.meta.Search.CurrentSearchIndex == -1 {
		return
	}
	foundPosition := e.meta.Search.Indexes[e.meta.Search.CurrentSearchIndex]
	e.killRegion(editbuffer.Cursor{RowIndex: foundPosition.Start.RowIndex, ColIndex: foundPosition.Start.ColIndex},
		editbuffer.Cursor{RowIndex: foundPosition.Start.RowIndex, ColIndex: foundPosition.Stop.ColIndex})
	e.InsertString(str)

	// Correct the changed indexes within the same line where replacement is made
	// How many rune characters change due to replacement?
	l := len([]byte(str)) - (foundPosition.Stop.ColIndex - foundPosition.Start.ColIndex)
	// RowIndex where replacement is made
	rowIndex := e.meta.Search.Indexes[e.meta.Search.CurrentSearchIndex].Start.RowIndex
	// Correct the changed indexes due to replacement within the same line
	for i := e.meta.Search.CurrentSearchIndex + 1; i < len(e.meta.Search.Indexes) && e.meta.Search.Indexes[i].Start.RowIndex == rowIndex; i++ {
		e.meta.Search.Indexes[i].Start.ColIndex += l
		e.meta.Search.Indexes[i].Stop.ColIndex += l
	}
	// Exclude the replaced search result
	// What if the replacement still matches the search after replacement? No consideration for now
	e.meta.Search.Indexes = slices.Delete(e.meta.Search.Indexes, e.meta.Search.CurrentSearchIndex, e.meta.Search.CurrentSearchIndex+1)
}

// ------------------------------------------------------------------
// Other
// ------------------------------------------------------------------

func (e *Editorleaf) CharInfo() {
	ch, _ := utf8.DecodeRune((*e.editBuffer.Rows())[e.meta.RowIndex][e.meta.ColIndex:])
	str := e.RuneStatus(ch)
	s := fmt.Sprintf("Char: '%s' (dec: %d, oct: %s, hex: %02X, %s), Cursor index: %d,%d", str, ch, strconv.FormatInt(int64(ch), 8), ch, utils.WidthKindString(ch), e.meta.RowIndex, e.meta.ColIndex)
	gecore.Echo.AddText(s)
}

// ------------------------------------------------------------------
// Functions
// ------------------------------------------------------------------

// insertBytes は、バイトスライスを現在のカーソル位置に挿入し、カーソルを前進させます。
func (e *Editorleaf) insertBytes(bytes []byte, enableUndo bool) {
	e.bsArray.Clear(e.meta.RowIndex)

	beforeCursor := e.meta.Cursor
	bytesArray := editbuffer.SplitByLF(bytes)
	lines := e.editBuffer.Rows()
	// gelog.Info("InsertBytes %s", string(bytes))

	for _, b := range bytesArray {
		bLen := len(b)
		if b[bLen-1] == '\n' {
			// gelog.Info("1 b '%s' %d+%d", string(b), e.meta.ColIndex, bLen)
			// 新しい行を追加
			lines.InsertRow(e.meta.RowIndex+1, (*lines)[e.meta.RowIndex][e.meta.ColIndex:])
			// 現在の行にバイトスライスを追加
			lines.SetRow(e.meta.RowIndex,
				append(
					append(
						make([]byte, 0, e.meta.ColIndex+bLen),
						(*lines)[e.meta.RowIndex][:e.meta.ColIndex]...),
					b...))

			//e.makeAvailableBoundariesArray(e.meta.RowIndex) // -------- !
			e.meta.RowIndex++
			e.bsArray.Insert(e.meta.RowIndex, 1)
			//e.makeAvailableBoundariesArray(e.meta.RowIndex) // -------- !
			e.meta.ColIndex = 0
			e.meta.Cy += 1
			e.meta.Cx = 0
		} else {
			lines.SetRow(e.meta.RowIndex, slices.Insert((*lines)[e.meta.RowIndex], e.meta.ColIndex, b...))
			e.meta.ColIndex += bLen
		}
	}

	if enableUndo {
		e.editBuffer.UndoAction.PushAction(&editbuffer.EditAction{Class: editbuffer.INSERT, Before: beforeCursor, After: e.meta.Cursor, Data: bytes})
	}
	e.syncCursorAndBufferForEdit(INSERT, beforeCursor, e.meta.Cursor)

	// コメントアウトされた部分は、将来的に必要な場合に対応
	// e.meta.PrevCx = -1
	// e.dirtyFlag = true
}

// The provided Go function getColumnIndexClosestToCursorXPosition calculates the column index (colIndex) and the cursor's horizontal position (cx) in the logical row at a specific horizontal cursor position (cursorXPos).
// It does so by decoding the UTF-8 runes in the row and accumulating their widths until it reaches or surpasses cursorXPos. Here's an explanation of the code
// この関数 getColumnIndexClosestToCursorXPosition は、特定の水平カーソル位置 (cursorXPos) に最も近い論理行のカラムインデックス (colIndex) とカーソル位置 (cx) を計算します。
// この処理は、行内の UTF-8 ルーンをデコードし、それらの幅を累積して cursorXPos に到達または超えるまで続けます。
func (e *Editorleaf) getColumnIndexClosestToCursorXPosition(rowIndex, indexOfLogicalRow, cursorXPos int) (colIndex, cx int) {
	// Get the boundaries of the logical row within the physical row.
	// bo := e.boundariesArray[rowIndex][indexOfLogicalRow]
	//bs := e.bsay.Boundaries(rowIndex)
	//bo := bs[indexOfLogicalRow]
	bo := e.bsArray.Boundary(rowIndex, indexOfLogicalRow)
	// bo := e.bsay[rowIndex].boundaries[indexOfLogicalRow]
	// Get the line content for the specified rowIndex.
	row := &(*e.editBuffer.Rows())[rowIndex]
	// Initialize colIndex to the start index of the logical row.
	for colIndex = bo.StartLogicalRowByteIndex; ; {
		// Decode the next rune starting from colIndex.
		ch, size := utf8.DecodeRune((*row)[colIndex:])
		// Get the display width of the rune.
		// w, _ := e.runeWidth(ch, rowIndex, colIndex)
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
func (e *Editorleaf) getContentWidthoutSpecialCharactor(current editbuffer.Cursor, maxContentWidth int) (content string) {
	isSpecialChar := func(ch rune) bool {
		return ch < 32 || ch == define.DEL || ch == '　' || ch == define.NO_BREAK_SPACE
	}

	width := 0
	skip := false
	startCol := current.ColIndex
	for y := current.RowIndex; y < e.editBuffer.Rows().Length(); y++ {
		// row, _ := e.Rows().GetRow(y)
		row := e.editBuffer.Rows().Row(y)
		for x := startCol; x < len(*row); {
			ch, size := utf8.DecodeRune((*row)[x:])
			// w, _ := e.runeWidth(ch, y, x)
			w := e.locale.RuneWidth(ch)
			x += size // Don't use x below
			s := string(ch)
			if isSpecialChar(ch) {
				if skip {
					continue
				}
				skip = true
				if ch == '\n' {
					s = string(theme.MarkNewline)
				} else if ch == '\t' {
					s = string(' ')
				} else {
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

func (e *Editorleaf) Recenter() {
	e.meta.Cy = int(e.editArea.Height / 2)
}

func (e *Editorleaf) SetCursor(c editbuffer.Cursor) {
	e.meta.Cursor = c
}

func (e *Editorleaf) moveCursor(x, y int) {
	e.meta.Cx = x
	e.meta.Cy = y
	gecore.Echo.AddText("")
	e.showCursor(x, y)
}

// 編集中のテキストの []byte を返す
// editorleaf.editbuffer.RowsStruct.Bytes() のラッパー
func (e *Editorleaf) GetBytes() []byte {
	bytes, _, err := e.editBuffer.RowsStruct.Bytes()
	if err != nil {
		gelog.Error(err.Error())
		return nil
	}
	return bytes
}

func (e *Editorleaf) GetString() string {
	return string(e.GetBytes())
}

func (e *Editorleaf) CommandPalette(s string) {
	input := strings.TrimSpace(s)
	if input == "" {
		return
	}

	switch {
	case strings.HasPrefix(input, "!"):
		cmd := strings.TrimSpace(input[1:])
		// 実行（例として標準出力）
		fmt.Printf("Run command: %s\n", cmd)
	case strings.HasPrefix(input, "|"):
		cmd := strings.TrimSpace(input[1:])
		// カーソル位置に挿入
		fmt.Printf("Insert at cursor: %s\n", cmd)
	default:
		i, err := strconv.Atoi(input)
		if err != nil {
			gelog.Error(err.Error())
			return
		}
		e.MoveCursorToLine(i)
	}
}

func (e Editorleaf) SetRows(r [][]byte) {
	e.editBuffer.SetRows(r)
	e.bsArray.ClearAll()
}

func (e Editorleaf) IsEndOfLine() bool {
	line := (*e.editBuffer.Rows())[e.meta.RowIndex]
	return len(line)-1 == e.meta.ColIndex
}

/* func (e *Editorleaf) isEndOfLogicalRow(rowIndex, colIndex int) bool {
	_, size := utf8.DecodeRune((*e.editBuffer.Rows())[rowIndex][colIndex:])
	for i := 0; i < e.bsArray.BoundariesLen(rowIndex); i++ {
		if colIndex+size == e.bsArray.Boundary(rowIndex, i).StopIndex {
			return true
		}
	}
	return false
}
*/

// ------------------------------------------------------------------
//
// ------------------------------------------------------------------

func removeRegion(rows *[][]byte, start, end editbuffer.Cursor) {
	// Checked row index. The start position of the region is after the end position, or the end position is beyond the last line
	if start.RowIndex > end.RowIndex || end.RowIndex >= len(*rows) {
		return
	}
	if start.RowIndex == end.RowIndex && start.ColIndex >= end.ColIndex {
		return
	}

	topRow := &(*rows)[start.RowIndex]
	// Checked col index. The start position of the region is the right of newline or EOF
	if start.ColIndex >= len(*topRow) {
		return
	}

	bottomRow := &(*rows)[end.RowIndex]
	// Checked col index. The end position of the region is the right of newline or EOF
	if end.ColIndex >= len(*bottomRow) { // EOF
		return
	}

	if start.RowIndex == end.RowIndex {
		// *topRow = delete(topRow, start.ColIndex, end.ColIndex)
		*topRow = slices.Delete(
			*topRow,
			start.ColIndex,
			end.ColIndex, // これでいいか？
		)
		return
	}

	// top row
	*topRow = (*topRow)[:start.ColIndex]
	// bottom row
	// Remove middle and bottom rows
	*topRow = append(*topRow, (*bottomRow)[end.ColIndex:]...)
	if end.RowIndex-start.RowIndex > 0 { // 常に true?
		*rows = slices.Delete(*rows, start.RowIndex+1, end.RowIndex+1)
	}
}
