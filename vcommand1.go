package editorleaf

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ge-editor/editorleaf/editbuffer/rows"
	"github.com/ge-editor/gecore"
	"github.com/ge-editor/gecore/define"
	"github.com/ge-editor/gelog"
	"github.com/ge-editor/locale"
	"github.com/ge-editor/theme"
)

// --------------------
// Move cursor
// --------------------

// Move cursor to next word.
func (e *Editorleaf) MoveCursorNextWord() {
	x, y := e.meta.ScreenPos.Col, e.meta.ScreenPos.Row

	lines := e.editBuffer.Rows
	// line, _ := lines.GetRow(e.meta.RowsPos.RowIndex)
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
		return
	}

	var prevCc, cc locale.CharClass
	notUppercaseBit := ^locale.UPPERCASE
	for {
		ch, size, ok := (*lines).Row(e.meta.RowsPos.RowIndex).DecodeRune(e.meta.RowsPos.ColIndex)
		if !ok {
			gelog.Error("error")
			panic("err")
		}
		/* w, ok := e.runeWidth(ch, e.meta.RowsPos.RowIndex, e.meta.RowsPos.ColIndex)
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
		// if e.isEndOfLogicalRow(e.meta.RowsPos.RowIndex, e.meta.RowsPos.ColIndex) {
		if e.bsArray.OnEndOfLogicalRow(e.meta.RowsPos.RowIndex, e.meta.RowsPos.ColIndex) {
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

func (e *Editorleaf) MoveCursorPreviousWord() {
	x, y := e.meta.ScreenPos.Col, e.meta.ScreenPos.Row

	if e.meta.RowsPos.ColIndex == 0 {
		if e.meta.RowsPos.RowIndex == 0 {
			gecore.Echo.AddText("Beginning of buffer")
			return
		}
		y--
		e.meta.RowsPos.RowIndex-- // previous line
		lastBs := e.bsArray.LastBoundary(e.meta.RowsPos.RowIndex)
		ch, _, colIndex, _ := e.editBuffer.Rows.Row(e.meta.RowsPos.RowIndex).DecodeEndRune()
		w := e.locale.RuneWidth(ch)
		x = lastBs.LogicalRowWidth - w     // on newline
		e.meta.RowsPos.ColIndex = colIndex // lastBs.stopIndex - size
	} else {
		var prevCc, cc locale.CharClass
		notUppercaseBit := ^locale.UPPERCASE
		for {
			before := e.bsArray.GetIndexOfLogicalRow(e.meta.RowsPos.RowIndex, e.meta.RowsPos.ColIndex)
			ch, _, colIndex, ok := e.editBuffer.Rows.Row(e.meta.RowsPos.RowIndex).DecodePrevRune(e.meta.RowsPos.ColIndex)
			if !ok {
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
			e.meta.RowsPos.ColIndex = colIndex
			after := e.bsArray.GetIndexOfLogicalRow(e.meta.RowsPos.RowIndex, e.meta.RowsPos.ColIndex)
			if !ok {
				panic("3")
			}
			if after < before {
				y--
				x = e.bsArray.Boundary(e.meta.RowsPos.RowIndex, after).LogicalRowWidth
			} else {
				x -= w
			}
			if prevCc != 0 && prevCc&locale.UPPERCASE == 0 && cc&locale.UPPERCASE > 0 {
				break
			}
		}
	}

	e.meta.PrevScreenPos.Col = x
	e.moveCursor(x, y)
}

// gelog.Debug("u.stack", "u.stack", u.stack, "a", a.Data.String([]byte{'\n'}))

// --------------------
// Utilities
// --------------------

// Return content widthout special charactor
func (e *Editorleaf) getContentWidthoutSpecialCharactor(current rows.RowsPos, maxContentWidth int) (content string) {
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
