package editorleaf

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v3"
	"github.com/ge-editor/gecore/tree"
	"github.com/ge-editor/gecore/workspace"
	"github.com/ge-editor/theme"
	"github.com/ge-editor/utils"
)

func (e *Editorleaf) drawModeline() {
	if e.mode != ModeEditor {
		return
	}

	y := e.editArea.Y + e.editArea.Height
	x := e.editArea.X
	w := e.editArea.Width

	draw := func(s string, style tcell.Style) {
		if w <= 0 {
			return
		}
		sw := utils.WidthOnScreen([]byte(s))
		if sw > w {
			// 描画領域を超える文字列は切り詰める。
			// DrawString 自体も w を上限として描画する。
			e.screen.DrawString(x, y, w, s, style)
			x += w
			w = 0
			return
		}
		e.screen.DrawString(x, y, w, s, style)
		x += sw
		w -= sw
	}

	// Workspace / RootTree
	wsName := ""
	ws := workspace.Current()
	if ws != nil {
		wsName = "[" + ws.Name
	}
	if e.viewArea.Width >= 80 {
		tr := tree.CurrentRootTree()
		if tr != nil {
			wsName += "." + tr.Name
		}
	}
	wsName += "] "
	style := theme.ColorModelineInactive
	if e.active {
		style = theme.ColorModeLineActive1
	}
	draw(wsName, style)

	// Read-only / Modified / File path
	readonlyModified := "--"
	if e.editBuffer.IsReadonly() && e.editBuffer.IsDirtyFlag() {
		readonlyModified = "%*"
	} else {
		if e.editBuffer.IsReadonly() {
			readonlyModified = "%%"
		}
		if e.editBuffer.IsDirtyFlag() {
			readonlyModified = "**"
		}
	}

	s := fmt.Sprintf(" %s %s ",
		readonlyModified, e.editBuffer.GetDispPath())
	style = theme.ColorModelineInactive
	if e.active {
		style = theme.ColorModeLineActive2
	}
	draw(s, style)

	// Cursor position
	percent := 100
	b := e.RowsLength() - 1
	if b > 0 {
		percent = e.meta.RowsPos.RowIndex * 100 / b
	}
	s = fmt.Sprintf(" L%d%% (%d,%d) ",
		percent,
		e.meta.RowsPos.RowIndex+1, e.meta.ModelineCx)
	style = theme.ColorModelineInactive
	if e.active {
		style = theme.ColorModeLineActive3
	}
	draw(s, style)

	// Encoding / Newline / Language
	s = fmt.Sprintf(` %s %s "%s"`,
		e.editBuffer.GetEncoding(),
		e.editBuffer.GetNewLine().String(),
		(*e.editBuffer.GetLangMode()).Name())
	style = theme.ColorModelineInactive
	if e.active {
		style = theme.ColorModeLineActive4
	}
	draw(s, style)

	// Character code
	ch, str := e.CharInfoOnCursor()
	s = fmt.Sprintf(" ('%s', %d, 0x%02X)", str, ch, ch)
	/* 	style = theme.ColorModelineInactive
	   	if e.active {
	   		style = theme.ColorModelineInactive
	   	}
	*/
	draw(s, style)

	// Fill the remaining area
	// a := theme.ColorModelineInactive
	style = theme.ColorModelineInactive
	if e.active {
		style = theme.ColorModeLineActive4
	}
	if w > 0 {
		e.screen.DrawString(x, y, w, strings.Repeat(" ", w), style)
	}
}
