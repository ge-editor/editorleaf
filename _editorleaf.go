// Editor Struct implements the gecore.tree.Leaf interface

package editorleaf

import (
	"bytes"
	"fmt"

	"github.com/gdamore/tcell/v3"

	"github.com/ge-editor/editorleaf/buffer"
	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/editorleaf/mark"
	"github.com/ge-editor/editorleaf/search"
	"github.com/ge-editor/gecore"
	"github.com/ge-editor/gecore/define"
	"github.com/ge-editor/gecore/screen"
	"github.com/ge-editor/gecore/styleresolver"
	"github.com/ge-editor/gecore/tree"
	"github.com/ge-editor/gelog"
	"github.com/ge-editor/keychord"
	"github.com/ge-editor/locale"
	"github.com/ge-editor/theme"
	"github.com/ge-editor/utils"
)

const (
	verticalThreshold = 5
)

var (
	// Initialization has been moved to the newEditor function
	// BufferSets, _ = buffer.NewBufferSets(gecore.Files)
	BufferSets *buffer.BufferSets
	Marks      = mark.NewMarks()
)

func newEditorLeaf() *Editorleaf {
	if BufferSets == nil {
		var err error
		BufferSets, err = buffer.NewBufferSets(gecore.Files)
		if err != nil {
			gelog.Error(err.Error())
			gecore.Echo.AddText(err.Error()).JustNowActive(gecore.EchoRed)
		}

		// gelog.Info("CloseGuardManager.Register", "BufferSets", BufferSets)
		gecore.QuitGuardManager.Register(NewQuitGuard(BufferSets), gecore.GuardWaitResolved)
	}
	e := &Editorleaf{
		screen:     screen.Get(),
		editBuffer: (*BufferSets)[0].EditBuffer,
		meta:       (*BufferSets)[0].PopMeta(),
		mode:       ModeEditor,
		locale:     locale.New(),

		styleResolver:       styleresolver.New(),
		searchResolver:      &search.SearchResolver{},
		specialCharResolver: &styleresolver.SpecialCharResolver{},
	}
	e.bsArray = NewBoundariesArray(e)

	e.styleResolver.Add(&search.SearchResolver{})
	e.styleResolver.Add(&styleresolver.SpecialCharResolver{})

	return e
}

// ------------------------------------------------------------------
// Editor implement gecore Leaf interface
// ------------------------------------------------------------------

// Editorleaf Struct implements the gecore.tree.Leaf interface
type Editorleaf struct {
	parentLeafType tree.LeafType
	screen         *screen.Screen
	active         bool

	viewArea utils.Rect // include mode line
	editArea utils.Rect

	verticalThreshold int // Changes depending on screen size

	editBuffer *editbuffer.EditBuffer
	meta       *buffer.Meta

	bsArray BoundariesArray // boundaries array of logical row

	mode Mode

	keyDispatcher *keychord.RootNode

	lineNumberWidth int

	locale locale.Locale // Locale interface

	styleResolver       *styleresolver.Manager
	searchResolver      *search.SearchResolver
	specialCharResolver *styleresolver.SpecialCharResolver
}

func (e *Editorleaf) SetKeyDispatcher(km *keychord.RootNode) {
	e.keyDispatcher = km
}

func (e *Editorleaf) DispatchKey(ev tcell.EventKey) (string, keychord.KeyDispatchTransition) {
	// gelog.Info("e.keyDispatcher")
	if e.keyDispatcher == nil {
		// gelog.Info("e.keyDispatcher == nil")
		return "", keychord.DispatchNotFound
	}

	s, res := e.keyDispatcher.Dispatch(ev)
	switch res {
	case keychord.DispatchNotFound:
		e.InsertString(ev.Str())
	case keychord.DispatchPrefix:
	case keychord.DispatchExecuted:
	case keychord.DispatchInvalidAfterPrefix:
	}

	return s, res
}

// ------------------------------------------------------------------
// Methods of gecore Leaf interface
// ------------------------------------------------------------------

func (e *Editorleaf) LeafType() tree.LeafType {
	return e.parentLeafType
}

func (e *Editorleaf) Resize(viewArea utils.Rect) {
	e.viewArea = viewArea
	e.editArea = viewArea
	if e.mode != ModeEditor {
		e.verticalThreshold = 0
	} else {
		if !e.rightmost() {
			e.editArea.Width -= 1 // right bar
		}
		e.editArea.Height -= 1 // status
		e.verticalThreshold = utils.Threshold(verticalThreshold, e.editArea.Height)
	}
	e.bsArray.ClearAll()
	e.Draw() // Need call self drawing
}

func (e *Editorleaf) Draw() bool {
	if e.drawEditorleaf() {
		return true
	}
	e.drawRightBar()
	return false
}

func (e *Editorleaf) Kill(leaf tree.Leaf, isActive bool) tree.Leaf {
	killTargetBuf := e.editBuffer

	var toReplace []*Editorleaf

	tree.GetRootTree().ForEachLeaf(func(l tree.Leaf) {
		e, ok := l.(*Editorleaf)
		if ok {
			if e.editBuffer == killTargetBuf {
				toReplace = append(toReplace, e)
			}
		}
	})

	// 該当するバッファを取り除く
	BufferSets.RemoveByBufferFile(killTargetBuf)

	var bufferSetsIndexForReplace int
	l := len(*BufferSets)
	if l == 0 {
		// Create new bufferSet into BufferSets
		eb, _, _, err := BufferSets.GetFileAndMeta("unnamed")
		if err != nil {
			gecore.Echo.AddText(err.Error())
			return leaf
		}
		bufferSetsIndexForReplace = BufferSets.GetIndexByBufferFile(eb)
	} else {
		bufferSetsIndexForReplace = l - 1
	}

	for _, e := range toReplace {
		e.editBuffer = (*BufferSets)[bufferSetsIndexForReplace].EditBuffer
		e.meta = (*BufferSets)[bufferSetsIndexForReplace].PopMeta()
	}

	return leaf
}

func (e *Editorleaf) Active(a bool) {
	e.active = a
}

func (e *Editorleaf) IsActive() bool {
	return e.active
}

func (e *Editorleaf) Resume() {
}

func (e *Editorleaf) Init() {
}

func (e *Editorleaf) WillClose() {
}

func (e *Editorleaf) MinibufferMode(mode Mode) {
	e.mode = mode
}

// ------------------------------------------------------------------
// SyncEditor
// undo/redo の同期も必要
// ------------------------------------------------------------------

/*
formatter 前後の差分を下記の形式に落とし込みをする必要がある

[]EditOp{
 Insert{},
 Delete{},
 Replace{},
}
*/

type syncType int

const (
	INSERT syncType = iota
	DELETE
)

// syncEdits adjusts cursor positions and buffer boundaries based on the type of edit (insert or delete).
func (e *Editorleaf) syncCursorAndBufferForEdit(sync syncType, start, end screen.Cursor) {
	// Ensure start is before end; swap if necessary.
	if start.RowIndex > end.RowIndex || (start.RowIndex == end.RowIndex && start.ColIndex > end.ColIndex) {
		start, end = end, start
	}

	// Synchronize cursor positions in the buffer sets associated with the edited file.
	for _, buffSet := range *BufferSets {
		// Skip if this buffer set is not linked to the file being edited.
		/*
			if buffSet.EditBuffer != e.editBuffer {
				continue
			}
		*/

		for _, meta := range buffSet.GetMetas() {
			// Adjust cursor based on the type of edit.
			switch sync {
			case INSERT:
				meta.Mark.AdjustForInsertion(start, end)

				// Skip if this buffer set is not linked to the file being edited.
				if buffSet.EditBuffer != e.editBuffer {
					continue
				}
				meta.Cursor.AdjustForInsertion(start, end)
			case DELETE:
				meta.Mark.AdjustForDeletion(start, end)

				// Skip if this buffer set is not linked to the file being edited.
				if buffSet.EditBuffer != e.editBuffer {
					continue
				}
				meta.Cursor.AdjustForDeletion(start, end)
			}
		}
		break
	}

	// Synchronize cursor positions and buffer boundaries in other editors linked to the same file.
	tree.GetRootTree().ForEachLeaf(func(leaf tree.Leaf) {
		editor, ok := leaf.(*Editorleaf)
		if !ok {
			return
		}

		// Skip if the editor is linked to a different file or is the current editor.
		if editor.editBuffer != e.editBuffer {
			return
		}
		// Adjust foundIndex that is results of search and replace
		foundIndexes := editor.meta.Search.GetFindIndexes()
		for i := 0; i < len(foundIndexes); i++ {
			// gelog.Info("fc1 %v", foundIndexes[i])
			switch sync {
			case INSERT:
				foundIndexes[i].Start.AdjustForInsertion(start, end)
				foundIndexes[i].Stop.AdjustForInsertion(start, end)
			case DELETE:
				foundIndexes[i].Start.AdjustForDeletion(start, end)
				foundIndexes[i].Stop.AdjustForDeletion(start, end)
			}
			// gelog.Info("fc2 %v", foundIndexes[i])
		}

		/* if editor == e {
			return
		} */

		for _, mark := range *Marks {
			switch sync {
			case INSERT:
				mark.AdjustForInsertion(start, end)
			case DELETE:
				mark.AdjustForDeletion(start, end)
			}
		}

		switch sync {
		case INSERT:
			editor.meta.Mark.AdjustForInsertion(start, end)

			if editor != e {
				editor.meta.Cursor.AdjustForInsertion(start, end)
				// Update buffer boundary array if rows were inserted.
				if end.RowIndex-start.RowIndex > 0 {
					editor.bsArray.Insert(start.RowIndex+1, end.RowIndex-(start.RowIndex+1))
				}
			}
		case DELETE:
			editor.meta.Mark.AdjustForDeletion(start, end)

			if editor != e {
				editor.meta.Cursor.AdjustForDeletion(start, end)
				// Update buffer boundary array if rows were deleted.
				if count := end.RowIndex - start.RowIndex; count > 0 {
					editor.bsArray.Delete(start.RowIndex+1, count)
				}
			}
		}
	})
}

// ------------------------------------------------------------------
//
// ------------------------------------------------------------------

func (e *Editorleaf) GetBuffers() *buffer.BufferSets {
	return BufferSets
}

func (e *Editorleaf) GetBuffersFilterByCharacters(chars string) *buffer.BufferSets {
	items := buffer.BufferSets{}
	b := BufferSets
	//gelog.Info("****** FilterByCharacters", "len", len(*m))
	for i := len(*b) - 1; i >= 0; i-- {
		mk := (*b)[i]
		text := mk.GetBase()
		/* 		if text == "*minibuffer*" {
		   			continue
		   		}
		*/
		if chars != "" {
			if utils.ContainsAllCharacters(text, chars) {
				items = append(items, mk)
			}
		} else {
			items = append(items, mk)
		}
	}
	//gelog.Info("FilterByCharacters", "chars", chars, "items", items)
	//gelog.Info("FilterByCharacters", "chars", chars, "items", items)
	return &items
}

// Convert rune to displaying string on mode line
// Conversion target:
//   - control code: ^X
//   - newline code
//
// Line feed code is depending the editing buffer newline
func (e *Editorleaf) RuneStatus(ch rune) string {
	switch {
	case ch == define.DEL:
		return "^?"

	case ch == '\t':
		return `\t`

	case ch == '\n':
		switch e.editBuffer.GetNewLine() {
		case editbuffer.NewlineTypeLF:
			return `\n`
		case editbuffer.NewlineTypeCRLF:
			return `\r\n`
		case editbuffer.NewlineTypeCR:
			return `\r`
		default:
			return "UNKNOWN"
		}

	case ch < 32:
		// ^A ～ ^Z
		var b [2]byte
		b[0] = '^'
		b[1] = byte(ch + 64)
		return string(b[:])

	default:
		return string(ch)
	}
}

// Use Editor.editArea as relative coordinates
func (e *Editorleaf) showCursor(x, y int) {
	_, iy := e.bsArray.CursorPositionOnScreenLogicalRow(e.meta.RowIndex, e.meta.ColIndex)

	hangingIndentWidth := 0
	if iy > 0 {
		hangingIndentWidth = e.bsArray.GetHangingIndentWidth(e.meta.RowIndex)
	}
	if e.active {
		e.screen.ShowCursor(e.editArea.X+x+e.lineNumberWidth+hangingIndentWidth, e.editArea.Y+y)
	}
}

// Editor.editArea as relative coordinates
func (e *Editorleaf) setCellInEditArea(x, y int, style tcell.Style, ch rune, chWidth int) {
	if y < 0 || x < 0 || y >= e.editArea.Height || x >= e.editArea.Width {
		return
	}

	columnLimit := 80 // language package で定義する

	if x >= columnLimit || (chWidth > 1 && x == columnLimit-1) {
		style = style.Background(theme.ColorColumnLimitOverflowBackground)
	}

	px := x + e.editArea.X + e.lineNumberWidth

	e.screen.SetContent(px, y+e.editArea.Y, ch, nil, style)
	// Wide char and Tab
	for i := 1; i < chWidth; i++ {
		e.screen.SetContent(px+i, y+e.editArea.Y, 0, nil, style)
	}
}

// Editor.editArea as relative coordinates
func (e *Editorleaf) fillInEditArea(
	rect utils.Rect,
	r rune,
	style tcell.Style,
) {
	if rect.Y < 0 || rect.Y >= e.editArea.Height ||
		rect.X < 0 || rect.X >= e.editArea.Width ||
		rect.Width <= 0 {
		return
	}

	const columnLimit = 80

	// screen absolute position
	screenX := rect.X + e.editArea.X + e.lineNumberWidth
	screenY := rect.Y + e.editArea.Y

	// overflowStyle := theme.ColorColumnLimitOverflow
	overflowStyle := style.Background(theme.ColorColumnLimitOverflowBackground)

	// 80桁以内
	if rect.X+rect.Width <= columnLimit {
		rect.X = screenX
		rect.Y = screenY
		e.screen.FillRect(rect, r, style)
		return
	}

	// 完全に80桁超え
	if rect.X >= columnLimit {
		rect.X = screenX
		rect.Y = screenY
		e.screen.FillRect(rect, r, overflowStyle)
		return
	}

	// 80桁をまたぐ
	leftWidth := columnLimit - rect.X
	rightWidth := rect.Width - leftWidth

	left := rect
	left.X = screenX
	left.Y = screenY
	left.Width = leftWidth
	e.screen.FillRect(left, r, style)

	right := rect
	right.X = screenX + leftWidth
	right.Y = screenY
	right.Width = rightWidth
	e.screen.FillRect(right, r, overflowStyle)
}

// Editor.editArea as relative coordinates
/* func (e *Editorleaf) fillInEditArea_1(rect utils.Rect, r rune, style tcell.Style) {
	if rect.Y < 0 || rect.Y >= e.editArea.Height ||
		rect.X < 0 || rect.X >= e.editArea.Width {
		return
	}

	rect.X += e.editArea.X + e.lineNumberWidth
	rect.Y += e.editArea.Y
	e.screen.FillRect(rect, r, style)
}
*/

// Returns bool whether it is the rightmost view
func (e *Editorleaf) rightmost() bool {
	// return e.viewArea.X+e.viewArea.Width >= e.screen.Width
	// line number を表示するため右端の境界線不要
	return true
}

func (e *Editorleaf) drawRightBar() {
	if e.rightmost() || e.mode != ModeEditor {
		return
	}

	x := e.viewArea.X + e.viewArea.Width - 1
	for y := e.viewArea.Y; y < e.viewArea.Y+e.viewArea.Height-1; y++ {
		e.screen.SetContent(x, y, ' ', nil, theme.ColorRightbar)
	}
}

// Draw the screen based on Editor.currentRowIndex, logical row position logicalCY, and cursor position Editor.Cy
func (e *Editorleaf) drawEditorleaf() bool {

	if e.mode == ModeEditor {
		lineNumberWidth := digitsScreenWidth(e.RowsLength()) + 1
		// The display width for the number of rows has been changed.
		if lineNumberWidth != e.lineNumberWidth {
			e.lineNumberWidth = lineNumberWidth
			e.bsArray.ClearAll()
		}
	} else {
		e.lineNumberWidth = 0
	}

	foundPositionIndexes := e.meta.Search.Indexes
	foundPositionIndex := -1

	Width, Height := e.editArea.Width, e.editArea.Height

	// Cursor position in draw view
	_, Cy := e.meta.Cx, e.meta.Cy // ...

	// Cursor position in logical row
	if e.bsArray.NeedsReparse(e.meta.RowIndex) {
		_, canceled := e.drawLineWithCompute(0, e.meta.RowIndex, -1, false, foundPositionIndex, foundPositionIndexes)
		if canceled {
			return true
		}
	} else {
		gelog.Debug("not call compute")
	}
	Lcx, Lcy := e.bsArray.CursorPositionOnScreenLogicalRow(e.meta.RowIndex, e.meta.ColIndex)
	// gecore.Echo.AddText(fmt.Sprintf("(Lcy,Lcx:%d,%d)", Lcy, Lcx))

	totalLogicalRowIfInHeight := 0
	totalRowAboveCursor := -1
	isAll := false
	if e.meta.RowIndex <= Height || e.editBuffer.Length() <= Height {
		isAll = true
		for rowIndex := 0; rowIndex < e.editBuffer.Length(); rowIndex++ {
			if rowIndex == e.meta.RowIndex {
				totalRowAboveCursor = totalLogicalRowIfInHeight + Lcy
			}

			if e.bsArray.NeedsReparse(rowIndex) {
				_, canceled := e.drawLineWithCompute(0, rowIndex, -1, false, foundPositionIndex, foundPositionIndexes)
				if canceled {
					return true
				}
			}
			totalLogicalRowIfInHeight += e.bsArray.BoundariesLen(rowIndex)

			if totalLogicalRowIfInHeight > Height {
				totalLogicalRowIfInHeight = -1
				isAll = false
				break
			}
		}
	}

	if isAll {
		// 1行目よりも上に隙間ができないように補正
		Cy = totalRowAboveCursor
		e.meta.StartDrawRowIndex = 0
		e.meta.StartDrawLogicalIndex = 0
	} else {
		// cursor is below verticalThreshold
		// gecore.Echo.AddText(fmt.Sprintf("(totalRowAboveCursor:%d, Height:%d, Threshold:%d)", totalRowAboveCursor, Height, e.verticalThreshold))
		if Cy >= Height-e.verticalThreshold {
			Cy = Height - e.verticalThreshold - 1
			// gecore.Echo.AddText(fmt.Sprintf("(Cy:%d)", Cy))
		}

		// cursor is above verticalThreshold
		if Cy < e.verticalThreshold {
			Cy = e.verticalThreshold
		}

		if totalRowAboveCursor >= 0 && Cy > totalRowAboveCursor {
			Cy = totalRowAboveCursor
		}

	}
	// gecore.Echo.AddText(fmt.Sprintf("(Cy,Cx:%d,%d)", Cy, Cx))

	// Cursor row
	rowIndex := e.meta.RowIndex
	y := Cy - Lcy
	// gecore.Echo.AddText(fmt.Sprintf("(rowIndex:%d, y:%d)", rowIndex, y))
	if e.bsArray.NeedsReparse(rowIndex) {
		_, canceled := e.drawLineWithCompute(y, rowIndex, Lcy, true, -1, foundPositionIndexes)
		if canceled {
			return true
		}
	} else {
		_, canceled := e.drawLine(y, rowIndex, Lcy, -1, foundPositionIndexes)
		if canceled {
			return true
		}
	}

	// From the cursor position to up
	rowIndex--
	for ; rowIndex >= 0 && y >= 0; rowIndex-- {
		if e.bsArray.NeedsReparse(rowIndex) {
			_, canceled := e.drawLineWithCompute(y, rowIndex, -1, false, -1, foundPositionIndexes)
			if canceled {
				return true
			}
		}
		y -= e.bsArray.BoundariesLen(rowIndex)
		_, canceled := e.drawLine(y, rowIndex, -1, -1, foundPositionIndexes)
		if canceled {
			return true
		}
	}

	// From the cursor position to down
	rowIndex = e.meta.RowIndex
	y = Cy + (e.bsArray.BoundariesLen(rowIndex) - Lcy)
	rowIndex++
	for ; rowIndex < e.RowsLength() && y < Height; rowIndex++ {
		if e.bsArray.NeedsReparse(rowIndex) {
			_, canceled := e.drawLineWithCompute(y, rowIndex, -1, true, -1, foundPositionIndexes)
			if canceled {
				return true
			}
		} else {
			_, canceled := e.drawLine(y, rowIndex, -1, -1, foundPositionIndexes)
			if canceled {
				return true
			}
		}
		y += e.bsArray.BoundariesLen(rowIndex)
	}

	///////////////////////////////////////

	// clear remaining area
	h := Height - y
	if h > 0 {
		e.fillInEditArea(utils.Rect{
			X:      0,
			Y:      y,
			Width:  Width,
			Height: h,
		}, 0, theme.ColorDefault)

		// line number area
		e.screen.FillRect(utils.Rect{
			X:      e.editArea.X,
			Y:      y,
			Width:  e.lineNumberWidth,
			Height: h,
		}, 0, theme.ColorLineNumber)
	}

	///////////////////////////////////////

	e.meta.Cx, e.meta.Cy = Lcx, Cy
	// hangingIndentWidth := e.bsArray.GetHangingIndentWidth(rowIndex)
	e.showCursor(e.meta.Cx, e.meta.Cy)

	// Calculate the number of cursor digits to display on the mode line
	e.meta.ModelineCx = Lcx + 1
	for i := 0; i < Lcy; i++ {
		e.meta.ModelineCx += e.bsArray.Boundary(e.meta.RowIndex, i).LogicalRowWidth
	}
	e.drawModeline()
	// e.screen.Echo(fmt.Sprintf("line: %d:%d-%d", e.StartDrawRowIndex, e.StartDrawLogicalIndex, e.EndDrawRowIndex))

	return false
}

// isCursorInRange checks if the cursor position (row, col) is within the range
// defined by the top-left (row1, col1) and bottom-right (row2, col2) corners.
// It returns:
//
//	-1 if the cursor is before the range,
//	 1 if the cursor is after the range,
//	 0 if the cursor is within the range.
func isCursorInRange(row, col, row1, col1, row2, col2 int) int {
	// Handle cases where the range is reversed (either vertically or horizontally)
	if row1 > row2 {
		row1, row2 = row2, row1
		col1, col2 = col2, col1
	} else if row1 == row2 && col1 > col2 {
		col1, col2 = col2, col1
	}

	// If the row is outside the range, return false
	if row < row1 {
		return -1
	}
	if row > row2 {
		return 1
	}

	// If the range is within a single row (row1 == row2)
	if row1 == row2 {
		if col < col1 {
			return -1
		}
		if col >= col2 {
			return 1
		}
		return 0
	}

	// If the cursor is on the starting row (row == row1), check the column range
	if row == row1 {
		if col < col1 {
			return -1
		}
		return 0
	}

	// If the cursor is on the ending row (row == row2), check the column range
	if row == row2 {
		if col >= col2 {
			return 1
		}
		return 0
	}

	// If the cursor is on a row between the starting and ending rows,
	// it is always within the range
	return 0
}

// 10進数で何桁か
func digitsScreenWidth(n int) int {
	if n == 0 {
		return 1
	}
	if n < 0 {
		n = -n
	}
	count := 0
	for n > 0 {
		n /= 10
		count++
	}
	return count
}

// indent and bullet
// return indentWidth, utils.RuneWidth(r) + 1, false
func (e *Editorleaf) detectHangingIndent(rowIndex int) (int, int, bool) {
	indentWidth := 0

	lines := e.editBuffer.Rows
	rowBytes := lines.Row(rowIndex).Length()

	colIndex := 0
	totalCellWidthForTab := 0
	bytePosOfRow := 0
	for bytePosOfRow < rowBytes {
		r, size, ok := lines.Row(rowIndex).DecodeRune(bytePosOfRow)
		if !ok {
			return 0, 0, false
		}

		width := 0
		if r == '\t' {
			width = utils.TabWidth(totalCellWidthForTab, e.editBuffer.GetTabWidth())
		} else if r == ' ' {
			width = utils.RuneWidth(r)
		} else {
			break
		}

		totalCellWidthForTab += width
		bytePosOfRow += size
		colIndex++
		indentWidth += width
	}

	for _, b := range e.locale.Bullets() {
		if bytes.HasPrefix(lines.Row(rowIndex).Bytes()[bytePosOfRow:], b.Marker) {
			return indentWidth, b.Width, true
		}
	}

	return indentWidth, 0, false
}

// compute boundary of rowIndex
// draw one row
//   - n: y position within the Leaf to draw the row
//   - cursorLogicalCY: Logical row number where the cursor is located,
//     If the row to draw is not the cursor row, set -1 and call
func (e *Editorleaf) drawLineWithCompute(
	startScreenY, rowIndex, cursorLogicalCY int,
	isDraw bool,
	foundPositionIndex int, foundIndexes []search.FoundPosition,
) (int, bool) {
	// 右端から 折り返し候補を探す探索マージン
	const rightEdgeWrapMargin = 8 // Search margin from the right edge for wrap candidates.
	const PageLineCount = 60      // will language に移動する

	contentWidth := e.editArea.Width - e.lineNumberWidth

	sy, sx := startScreenY, 0
	var prevPrevCell, prevCell, currentCell locale.Cell // ★★
	var prevCellCh2 rune                                // 表示する文字 (currentCell.Ch, currentCellCh2) の1個前の文字, Controlcode を表示する為に 2個目の rune を用意 "^", "X" // ★

	var breakpoint Boundary
	lines := e.editBuffer.Rows
	isEndOfRow := rowIndex == (*lines).Length()-1
	rowBytes := lines.Row(rowIndex).Length()
	totalCellWidthForTab := 0 // for compute tab stop

	bo := []Boundary{}
	startLogicalRowByteIndex := 0 // 論理行の開始 byte index

	runeWidth := make([]locale.Cell, rowBytes)

	// Hanging indentation
	indentWidth, bulletWidth, _ := e.detectHangingIndent(rowIndex)
	hangingIndentWidth := indentWidth + bulletWidth

	//
	cursorLineY := -1
	if rowIndex == e.meta.RowIndex {
		cursorLineY = cursorLogicalCY + startScreenY
	}
	isUnderline := func() bool {
		return sy == cursorLineY
	}

	ctx := e.parentLeafType.CancelManager().Get("draw")
	if ctx == nil {
		return -1, true
	}

	for bytePosOfRow := 0; bytePosOfRow < rowBytes; {
		select {
		case <-ctx.Done():
			// if isDraw {
			gelog.Debug("Cancel drawLineWithCompute")
			gecore.Echo.AddText("Cancel drawLineWithCompute")
			e.bsArray.Clear(rowIndex)
			return -1, true // canceled
			//}
		default:
		}

		wrapped := false
		currentCell.Style = theme.ColorDefault

		isLastCh := bytePosOfRow == rowBytes-1
		var currentCellCh2 rune // 表示する文字, Controlcode の場合は "^", "X"
		var ok bool
		currentCell.Ch, currentCell.Size, ok = lines.Row(rowIndex).DecodeRune(bytePosOfRow)
		if !ok {
			panic(fmt.Sprintf("%d '%s'", rowIndex, string((*lines)[rowIndex])))
		}
		currentCell.Width = utils.RuneWidth(currentCell.Ch)
		currentCell.Class = e.locale.GetCharClass(currentCell.Ch)

		// Style
		e.styleResolver.Resolve(&styleresolver.Context{
			RowIndex:        rowIndex,
			ColIndex:        bytePosOfRow,
			Cursor:          e.meta.Cursor,
			IsCursorLine:    false,
			IsCursorEnabled: false,
			IsLastAtRow:     false,
			IsEndRow:        false,
			Cell:            currentCell,
		}, theme.ColorDefault)

		// Special char width
		if currentCell.Ch == define.EOF && isLastCh && isEndOfRow {
			currentCell.Ch = theme.MarkEOF
			currentCell.Width = 1 // End of file
			currentCell.Style = theme.ColorMarkEOF
		} else if currentCell.Ch == '\t' {
			currentCell.Ch = theme.MarkTab
			currentCell.Width = utils.TabWidth(totalCellWidthForTab, e.editBuffer.GetTabWidth())
			currentCell.Style = theme.ColorTab
		} else if currentCell.Ch == '\n' {
			currentCell.Ch = theme.MarkNewline
			currentCell.Style = theme.ColorMarkNewline
		} else if locale.Is(currentCell, locale.CONTROLCODE) {
			currentCell.Ch = '^'
			currentCellCh2 = currentCell.Ch + 64
			currentCell.Width = 2 // ^X
			currentCell.Style = theme.ColorControlCode
		}

		// Is index in the found word
		if foundPositionIndex >= 0 && foundPositionIndex < len(foundIndexes) {
			u := isCursorInRange(rowIndex, bytePosOfRow,
				foundIndexes[foundPositionIndex].Start.RowIndex, foundIndexes[foundPositionIndex].Start.ColIndex,
				foundIndexes[foundPositionIndex].Stop.RowIndex, foundIndexes[foundPositionIndex].Stop.ColIndex)

			if u == 0 {
				if isCursorInRange(e.meta.RowIndex, e.meta.ColIndex,
					foundIndexes[foundPositionIndex].Start.RowIndex, foundIndexes[foundPositionIndex].Start.ColIndex,
					foundIndexes[foundPositionIndex].Stop.RowIndex, foundIndexes[foundPositionIndex].Stop.ColIndex) == 0 {
					currentCell.Style = theme.ColorSearchFoundOnCursor
				} else {
					currentCell.Style = theme.ColorFind
				}
			} else if u == 1 {
				foundPositionIndex++
			}
		}
		currentCell.Style = currentCell.Style.Underline(isUnderline())

		if sx+currentCell.Width >= contentWidth-rightEdgeWrapMargin && locale.IsBreakpoint(prevPrevCell, prevCell, currentCell) { // ★★
			breakpoint = Boundary{
				StartLogicalRowByteIndex: startLogicalRowByteIndex,
				StopLogicalRowByteIndex:  bytePosOfRow,
				LogicalRowWidth:          sx,
				TotalCellWidth:           totalCellWidthForTab,
			}
		}

		// 論理行末には必ず記号が追加される: -, LF, EOF
		if sx+currentCell.Width >= contentWidth {
			if isLastCh {
				// rune is LF or EOF
				bo = append(bo, Boundary{
					StartLogicalRowByteIndex: startLogicalRowByteIndex,
					StopLogicalRowByteIndex:  bytePosOfRow + currentCell.Size,
					LogicalRowWidth:          sx + currentCell.Width,
					TotalCellWidth:           totalCellWidthForTab + currentCell.Width,
				})
				cacheCellInfo(&runeWidth, bytePosOfRow,
					currentCell.Ch, currentCell.Style, currentCell.Size, currentCell.Width, currentCell.Class,
					sy-startScreenY, sx, hangingIndentWidth)
				if isDraw {
					e.setCellInEditArea(sx, sy, currentCell.Style, currentCell.Ch, currentCell.Width)
					/*
						e.fillInEditArea(utils.Rect{ // 実行されていない、もしくは意味ない？
							X:      sx + currentCell.Width,
							Y:      sy,
							Width:  contentWidth - (sx + currentCell.Width),
							Height: 1,
						}, '*', theme.ColorDefault.Underline(isUnderline()))
					*/
				}
			} else if breakpoint.IsEmpty() {
				if locale.Is(currentCell, locale.PROHIBITED) {
					// 折り返した直後が禁則文字だった場合の処理
					// currentCell は次の論理行頭だが、禁則文字だった場合
					bo = append(bo, Boundary{
						StartLogicalRowByteIndex: startLogicalRowByteIndex,
						StopLogicalRowByteIndex:  bytePosOfRow - prevCell.Size, // 直前の文字の前
						LogicalRowWidth:          sx - prevCell.Width,
						TotalCellWidth:           totalCellWidthForTab - prevCell.Width,
					})
					startLogicalRowByteIndex = bytePosOfRow - prevCell.Size // 次の論理行開始位置 byte index
					if isDraw {
						e.setCellInEditArea(sx-prevCell.Width, sy, theme.ColorMarkContinue.Underline(isUnderline()), theme.MarkContinue, 1)
						e.fillInEditArea(utils.Rect{
							X:      sx - prevCell.Width + 1,
							Y:      sy,
							Width:  contentWidth - (sx - prevCell.Width - 1),
							Height: 1,
						}, 0, theme.ColorDefault.Underline(isUnderline()))
					}
					// Next logical row
					sy++
					sx = 0 + hangingIndentWidth
					wrapped = true
					s := prevCell.Style.Underline(isUnderline())
					if locale.Is(prevCell, locale.CONTROLCODE) {
						cacheCellInfo(&runeWidth, bytePosOfRow-prevCell.Size,
							prevCell.Ch, s, 1, 1, prevCell.Class,
							sy-startScreenY, sx, hangingIndentWidth)
						if isDraw {
							e.setCellInEditArea(sx, sy, s, prevCell.Ch, 1)   // ★
							e.setCellInEditArea(sx+1, sy, s, prevCellCh2, 1) // ★
						}
					} else {
						// 折り返した後の情報で再設定
						cacheCellInfo(&runeWidth, bytePosOfRow-prevCell.Size,
							prevCell.Ch, s, prevCell.Size, prevCell.Width, prevCell.Class,
							sy-startScreenY, sx, hangingIndentWidth)
						if isDraw {
							e.setCellInEditArea(sx, sy, s, prevCell.Ch, prevCell.Width) // ★
						}
					}
					sx += prevCell.Width
					s = currentCell.Style.Underline(isUnderline())  // ★★
					if locale.Is(currentCell, locale.CONTROLCODE) { // ★★
						// これを確認する必要があるか？
						cacheCellInfo(&runeWidth, bytePosOfRow,
							currentCell.Ch, s, 1, 1, currentCell.Class,
							sy-startScreenY, sx, hangingIndentWidth)
						if isDraw {
							e.setCellInEditArea(sx, sy, s, currentCell.Ch, 1)
							e.setCellInEditArea(sx+1, sy, s, currentCellCh2, 1)
						}
					} else {
						// これを確認する必要があるか？
						cacheCellInfo(&runeWidth, bytePosOfRow,
							currentCell.Ch, s, currentCell.Size, currentCell.Width, currentCell.Class,
							sy-startScreenY, sx, hangingIndentWidth)
						if isDraw {
							e.setCellInEditArea(sx, sy, s, currentCell.Ch, currentCell.Width) // ★★
						}
					}
				} else {
					bo = append(bo, Boundary{
						StartLogicalRowByteIndex: startLogicalRowByteIndex,
						StopLogicalRowByteIndex:  bytePosOfRow,
						LogicalRowWidth:          sx,
						TotalCellWidth:           totalCellWidthForTab,
					})
					startLogicalRowByteIndex = bytePosOfRow
					if isDraw {
						e.setCellInEditArea(sx, sy, theme.ColorMarkContinue.Underline(isUnderline()), theme.MarkContinue, 1)
						/*
							e.fillInEditArea(utils.Rect{ // 実行されていない、もしくは意味ない？
								X:      sx + 1,
								Y:      sy,
								Width:  contentWidth - (sx + 1),
								Height: 1,
							}, '@', theme.ColorDefault.Underline(isUnderline()))
						*/
					}
					sy++
					sx = 0 + hangingIndentWidth
					wrapped = true
					s := currentCell.Style.Underline(isUnderline()) // ★★
					if locale.Is(currentCell, locale.CONTROLCODE) { // ★★
						cacheCellInfo(&runeWidth, bytePosOfRow,
							currentCell.Ch, currentCell.Style, 1, 1, currentCell.Class,
							sy-startScreenY, sx, hangingIndentWidth)
						if isDraw {
							e.setCellInEditArea(sx, sy, s, currentCell.Ch, 1)   // ★★
							e.setCellInEditArea(sx+1, sy, s, currentCellCh2, 1) // ★★
						}
					} else {
						cacheCellInfo(&runeWidth, bytePosOfRow,
							currentCell.Ch, currentCell.Style, currentCell.Size, currentCell.Width, currentCell.Class,
							sy-startScreenY, sx, hangingIndentWidth)
						if isDraw {
							e.setCellInEditArea(sx, sy, s, currentCell.Ch, currentCell.Width) // ★★
						}
					}
				}
			} else { // breakpoint is exists
				bo = append(bo, breakpoint)
				startLogicalRowByteIndex = breakpoint.StopLogicalRowByteIndex

				bytePosOfRow = breakpoint.StopLogicalRowByteIndex
				sx = breakpoint.LogicalRowWidth
				if isDraw {
					e.setCellInEditArea(sx, sy, theme.ColorMarkContinue.Underline(isUnderline()), theme.MarkContinue, 1)
					e.fillInEditArea(utils.Rect{X: sx + 1, Y: sy, // 必要
						Width: contentWidth - (sx + 1), Height: 1},
						0, theme.ColorDefault.Underline(isUnderline()))
				}
				sy++
				sx = 0 + hangingIndentWidth
				wrapped = true
				//
				breakpoint.Clear()
				prevPrevCell.Clear() // ★★
				prevCell.Clear()     // ★★
				currentCell.Clear()  // ★★

				// Fill Hanging Indent Width
				if isDraw && wrapped && hangingIndentWidth > 0 {
					e.fillInEditArea(utils.Rect{X: 0, Y: sy, // 必要
						Width:  hangingIndentWidth,
						Height: 1},
						0, theme.ColorDefault.Underline(isUnderline()))
					wrapped = false
				}

				continue // ! --------------------
			}
		} else { // if sx+currentCell.Width < contentWidth
			cacheCellInfo(&runeWidth, bytePosOfRow,
				currentCell.Ch, currentCell.Style, currentCell.Size, currentCell.Width, currentCell.Class,
				sy-startScreenY, sx, hangingIndentWidth)

			if isDraw {
				e.setCellInEditArea(sx, sy, currentCell.Style, currentCell.Ch, currentCell.Width) // ★★
			}
			if isLastCh {
				bo = append(bo, Boundary{
					StartLogicalRowByteIndex: startLogicalRowByteIndex,
					StopLogicalRowByteIndex:  bytePosOfRow + currentCell.Size,          // ★★
					LogicalRowWidth:          sx + currentCell.Width,                   // ★★
					TotalCellWidth:           totalCellWidthForTab + currentCell.Width, // ★★
				})
				if isDraw {
					e.fillInEditArea(utils.Rect{ // ★★ ここで,ほぼ全ての行のコンテンツ以降を塗りつぶしている
						X:      sx + currentCell.Width,
						Y:      sy,
						Width:  contentWidth - (sx + currentCell.Width),
						Height: 1,
					}, 0, theme.ColorDefault.Underline(isUnderline()))
				}
				sy++
			}
		}

		// -- tail of loop --

		// Fill Hanging Indent Width
		if isDraw && wrapped && hangingIndentWidth > 0 {
			e.fillInEditArea(utils.Rect{X: 0, Y: sy,
				Width:  hangingIndentWidth,
				Height: 1},
				0, theme.ColorDefault.Underline(isUnderline()))
			wrapped = false
		}

		prevPrevCell = prevCell // ★★
		prevCell = currentCell  // ★★

		prevCellCh2 = currentCellCh2 // ★ ControlCode の 2文字目の rune

		sx += currentCell.Width
		totalCellWidthForTab += currentCell.Width
		bytePosOfRow += currentCell.Size

	} // for

	// Line number
	if isDraw && e.mode != ModeMinibuffer {
		e.drawLineNumber(rowIndex, startScreenY, cursorLineY, len(bo), PageLineCount)
	}

	//
	e.bsArray.Set(rowIndex, bo, hangingIndentWidth, runeWidth)
	return sy - startScreenY, false
}

func (e *Editorleaf) drawLine(
	startScreenY, rowIndex, cursorLogicalCY int,
	foundPositionIndex int, foundIndexes []search.FoundPosition,
) (int, bool) {
	const PageLineCount = 60 // will language に移動する

	contentWidth := e.editArea.Width - e.lineNumberWidth

	sy, sx := startScreenY, 0
	//var prevCellCh2 rune        // 表示する文字 (currentCell.Ch, currentCellCh2) の1個前の文字, Controlcode を表示する為に 2個目の rune を用意 "^", "X" // ★

	logicalRowLen := len(e.bsArray.rows[rowIndex].Boundaries)

	startLogicalRowIndex := 0

	if startScreenY < 0 {
		startLogicalRowIndex = -startScreenY
		sy = 0
	}

	if startLogicalRowIndex >= logicalRowLen {
		return 0, false
	}

	hangingIndentWidth := e.bsArray.rows[rowIndex].HangingIndentWidth
	if startLogicalRowIndex > 0 {
		sx = hangingIndentWidth
	}

	//
	cursorLineY := -1
	if rowIndex == e.meta.RowIndex {
		cursorLineY = cursorLogicalCY + startScreenY
	}

	runeWidthCache := e.bsArray.rows[rowIndex].RuneWidthCache

	ctx := e.parentLeafType.CancelManager().Get("draw")
	if ctx == nil {
		return -1, true
	}

	for logicalRowIndex := startLogicalRowIndex; logicalRowIndex < logicalRowLen; logicalRowIndex++ {

		select {
		case <-ctx.Done():
			gelog.Debug("Cancel drawLine")
			gecore.Echo.AddText("Cancel drawLine")
			return -1, true
		default:
		}
		// <-time.After(500 * time.Microsecond)

		underline := sy == cursorLineY

		// Fill Hanging Indent Width
		if logicalRowIndex > 0 && hangingIndentWidth > 0 {
			style := theme.ColorDefault.Underline(underline)
			if underline {
				style = style.Underline(underline)
			}
			e.fillInEditArea(utils.Rect{
				X:      0,
				Y:      sy,
				Width:  hangingIndentWidth,
				Height: 1,
			}, 0, style)
		}

		bo := e.bsArray.Boundary(rowIndex, logicalRowIndex)

		for bytePosOfRow := bo.StartLogicalRowByteIndex; bytePosOfRow < bo.StopLogicalRowByteIndex; {

			cell := runeWidthCache[bytePosOfRow]
			style := cell.Style
			if underline {
				style = style.Underline(underline)
			}

			e.setCellInEditArea(sx, sy, style, cell.Ch, cell.Width)
			sx += cell.Width
			bytePosOfRow += cell.Size
		}

		if logicalRowIndex < logicalRowLen-1 {
			style := theme.ColorMarkContinue
			if underline {
				style = style.Underline(underline)
			}

			e.setCellInEditArea(sx, sy, style, theme.MarkContinue, 1)
			sx += 1
		}

		style := theme.ColorDefault
		if underline {
			style = style.Underline(underline)
		}
		e.fillInEditArea(utils.Rect{
			X:      sx,
			Y:      sy,
			Width:  contentWidth - sx,
			Height: 1,
		}, 0, style)

		sy++
		sx = 0 + hangingIndentWidth

		if sy >= e.editArea.Y+e.editArea.Height {
			break
		}

	} // for

	// Line number
	if e.mode != ModeMinibuffer {
		e.drawLineNumber(rowIndex, startScreenY, cursorLineY, logicalRowLen, PageLineCount)
	}

	return sy - startScreenY, false
}

func (e *Editorleaf) drawLineNumber(rowIndex, startScreenY, cursorLineY int, h int /* bo []Boundary */, PageLineCount int) {
	if e.lineNumberWidth > 0 {
		y := e.editArea.Y + startScreenY
		// h := len(bo)
		if startScreenY < 0 {
			y = e.editArea.Y
			h += startScreenY
		}
		e.screen.FillRect(utils.Rect{X: e.editArea.X, Y: y,
			Width:  e.lineNumberWidth,
			Height: h},
			0, theme.ColorLineNumber)

		// Underline on line number area
		if cursorLineY != -1 {
			e.screen.FillRect(utils.Rect{X: e.editArea.X, Y: e.editArea.Y + cursorLineY,
				Width:  e.lineNumberWidth,
				Height: 1},
				0, theme.ColorLineNumber.Underline(true))
		}
	}

	// Number
	if startScreenY >= 0 && startScreenY < e.editArea.Height {
		style := theme.ColorLineNumber

		// 60行単位で色変更
		pageIndex := rowIndex / PageLineCount
		if pageIndex%2 != 0 {
			style = theme.ColorLineNumberOnEvenPage
		}

		if startScreenY == cursorLineY {
			style = style.Underline(true)
		}

		e.drawLineNumberNumber(
			rowIndex+1,
			e.lineNumberWidth-2+e.editArea.X,
			startScreenY,
			style,
		)
	}
}

func (e *Editorleaf) drawLineNumberNumber(n int, x, y int, style tcell.Style) {
	for n > 0 {
		d := n % 10
		e.screen.SetContent(x, y+e.editArea.Y, rune('0'+d), nil, style)
		n /= 10
		x--
	}
}

func cacheCellInfo(cache *[]locale.Cell, bytePosOfRow int,
	ch rune, // int32
	style tcell.Style,
	size int,
	width int, // screen cell width
	class locale.CharClass,
	logicalRowIndex int,
	totalWidthLogicalRow int, // screen cell total width of logical row
	hangingIndentWidth int,
) {
	// Exclude hanging indent width
	if logicalRowIndex > 0 {
		totalWidthLogicalRow -= hangingIndentWidth
	}

	(*cache)[bytePosOfRow] = locale.Cell{
		Ch:                   ch,
		Style:                style,
		Size:                 size,
		Width:                width,
		Class:                class,
		LogicalRowIndex:      logicalRowIndex,
		TotalWidthLogicalRow: totalWidthLogicalRow,
	}

	/* 	utils.EnsureSet(cache, bytePosOfRow, locale.Cell{
	   		Ch:                   ch,
	   		Style:                style,
	   		Size:                 size,
	   		Width:                width,
	   		Class:                class,
	   		LogicalRowIndex:      logicalRowIndex,
	   		TotalWidthLogicalRow: totalWidthLogicalRow,
	   	})
	*/
}

// Returns the screen position of the cursor corresponding from cached array to the specified column index in logical rows.
/* func (e *Editorleaf) cursorPositionOnScreenLogicalRow(rowIndex, colIndex int) (lx, ly int) {
	if e.bsArray.NeedsReparse(rowIndex) {
		e.drawLineWithCompute(0, rowIndex, -1, false, 0, nil)
	}
	gelog.Debug("rows", e.bsArray.rows[rowIndex])
	cell := e.bsArray.rows[rowIndex].RuneWidthCache[colIndex]
	return cell.TotalWidthLogicalRow, cell.LogicalRowIndex
}
*/

// Return the index of the logical line that contains the specified column.
/* func (e *Editorleaf) getIndexOfLogicalRow(rowIndex, colIndex int) (int, bool) {
	if e.bsArray.NeedsReparse(rowIndex) {
		e.drawLineWithCompute(0, rowIndex, -1, false, 0, nil)
	}
	cell := e.bsArray.rows[rowIndex].RuneWidthCache[colIndex]
	return cell.LogicalRowIndex, true
}
*/

// Check if the column index is within the last boundary of the specified row
// Return false: out of index or not initialized.
/* func (e *Editorleaf) inEndOfLogicalRow(rowIndex, colIndex int) bool {
	lastBoundary := e.bsArray.LastBoundary(rowIndex)
	return colIndex >= lastBoundary.StartLogicalRowByteIndex && colIndex < lastBoundary.StopLogicalRowByteIndex
}
*/
