package editorleaf

import (
	"fmt"

	"github.com/ge-editor/gecore/tree"
	"github.com/ge-editor/gecore/workspace"
	"github.com/ge-editor/theme"
)

func (e *Editorleaf) drawModeline() {
	if e.mode != ModeEditor {
		return
	}

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
	wsName += "]"

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

	// cursor position
	percent := 100 // current line / total line
	b := e.RowsLength() - 1
	if b > 0 {
		percent = e.meta.RowsPos.RowIndex * 100 / b
	}

	s := fmt.Sprintf("%s %s %s L%d%% (%d,%d) ",
		wsName, readonlyModified, e.editBuffer.GetDispPath(),
		percent,
		e.meta.RowsPos.RowIndex+1, e.meta.ModelineCx)
	s += fmt.Sprintf(`%s %s "%s"`, e.editBuffer.GetEncoding(), e.editBuffer.GetNewLine().String(), (*e.editBuffer.GetLangMode()).Name())

	// char code
	ch, str := e.CharInfoOnCursor()
	s += fmt.Sprintf(" ('%s', %d, 0x%02X)", str, ch, ch)

	a := theme.ColorModelineInactive
	if e.active {
		a = theme.ColorModeLineActive
	}
	e.screen.DrawString(e.editArea.X, e.editArea.Y+e.editArea.Height, e.editArea.Width, s, a)
}
