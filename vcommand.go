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

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/editorleaf/editbuffer/rows"
	"github.com/ge-editor/editorleaf/search"
	"github.com/ge-editor/gecore"
	"github.com/ge-editor/gecore/define"
	"github.com/ge-editor/gecore/killbuffer"
	"github.com/ge-editor/gecore/screen"
	"github.com/ge-editor/gecore/tree"
	"github.com/ge-editor/gelog"
	"github.com/ge-editor/locale"
	"github.com/ge-editor/theme"
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
	rows := e.editBuffer.Rows
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
			if ed.meta.RowIndex >= rowLength {
				ed.meta.RowIndex = rowLength - 1
			}
			line := (*ed.editBuffer.Rows)[ed.meta.RowIndex]
			colLength := len(line)
			if ed.meta.ColIndex >= colLength {
				// cursor on newline or EOF
				ed.meta.ColIndex = colLength - 1
			}
			for !utf8.RuneStart(line[ed.meta.ColIndex]) && ed.meta.ColIndex > 0 {
				ed.meta.ColIndex--
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

func (e *Editorleaf) RowsLength() int {
	return e.editBuffer.Rows.Length()
}

// ------------------------------------------------------------------
// Move cursor
// ------------------------------------------------------------------

// Move cursor to next word.
func (e *Editorleaf) MoveCursorNextWord() {
	x, y := e.meta.Cx, e.meta.Cy

	lines := e.editBuffer.Rows
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
		// if e.isEndOfLogicalRow(e.meta.RowIndex, e.meta.ColIndex) {
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
		ch, _, colIndex, _ := e.editBuffer.Rows.Row(e.meta.RowIndex).DecodeEndRune()
		w := e.locale.RuneWidth(ch)    // , e.meta.RowIndex, colIndex)
		x = lastBs.LogicalRowWidth - w // on newline
		e.meta.ColIndex = colIndex     // lastBs.stopIndex - size
	} else {
		var prevCc, cc locale.CharClass
		notUppercaseBit := ^locale.UPPERCASE
		for {
			//e.makeAvailableBoundariesArray(e.meta.RowIndex) // -------- !
			before := e.bsArray.GetIndexOfLogicalRow(e.meta.RowIndex, e.meta.ColIndex)
			ch, _, colIndex, ok := e.editBuffer.Rows.Row(e.meta.RowIndex).DecodePrevRune(e.meta.ColIndex)
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
			after := e.bsArray.GetIndexOfLogicalRow(e.meta.RowIndex, e.meta.ColIndex)
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

// ------------------------------------------------------------------
// Region
// ------------------------------------------------------------------

// Copy region to kill buffer
func (e *Editorleaf) copyRegion(a, b screen.Cursor) error {
	s := e.editBuffer.GetRegion(a, b)
	if s == nil {
		return nil
	}

	err := killbuffer.KillBuffer.PushKillBuffer(s.BytesArray(), e.editBuffer.GetNewLine().Bytes())
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
func (e *Editorleaf) killRegion(start, stop screen.Cursor) {
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

	e.meta.Cursor = start
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

	gelog.Debug("KillRegion", "mark", fmt.Sprintf("%d:%d", mark.Cursor.RowIndex, mark.Cursor.ColIndex), "current", fmt.Sprintf("%d:%d", e.meta.RowIndex, e.meta.ColIndex))

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

// gelog.Debug("u.stack", "u.stack", u.stack, "a", a.Data.String([]byte{'\n'}))

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
	rows := e.editBuffer.Rows
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

	lines := e.editBuffer.Rows
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
	e.killRegion(screen.Cursor{RowIndex: foundPosition.Start.RowIndex, ColIndex: foundPosition.Start.ColIndex},
		screen.Cursor{RowIndex: foundPosition.Start.RowIndex, ColIndex: foundPosition.Stop.ColIndex})
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
// Utilities
// ------------------------------------------------------------------

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
func (e *Editorleaf) getContentWidthoutSpecialCharactor(current screen.Cursor, maxContentWidth int) (content string) {
	isSpecialChar := func(ch rune) bool {
		return ch < 32 || ch == define.DEL || ch == '　' || ch == define.NO_BREAK_SPACE
	}

	width := 0
	skip := false
	startCol := current.ColIndex
	for y := current.RowIndex; y < e.editBuffer.Rows.Length(); y++ {
		// row, _ := e.Rows().GetRow(y)
		row := e.editBuffer.Rows.Row(y)
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

func (e *Editorleaf) SetCursor(c screen.Cursor) {
	e.meta.Cursor = c
}

// 編集中のテキストの []byte を返す
// editorleaf.editbuffer.Bytes() のラッパー
func (e *Editorleaf) GetBytes() ([]byte, []int, error) {
	return e.editBuffer.Bytes()
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
		e.MoveCursorGoToLine(i)
	}
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

func (e Editorleaf) IsEndOfLine() bool {
	line := (*e.editBuffer.Rows)[e.meta.RowIndex]
	return len(line)-1 == e.meta.ColIndex
}
