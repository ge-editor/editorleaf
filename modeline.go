package editorleaf

import (
	"fmt"

	"github.com/ge-editor/theme"
)

func (e *Editorleaf) drawModeline() {
	if e.mode != ModeEditor {
		return
	}

	// cursor position
	readonly := "-"
	if e.editBuffer.IsReadonly() {
		readonly = "R"
	}
	modified := "-"
	if e.editBuffer.IsDirtyFlag() {
		modified = "*"
	}

	s := fmt.Sprintf("%s%s-- %s (%d,%d) ", modified, readonly, e.editBuffer.GetDispPath(), e.meta.RowIndex+1, e.meta.ModelineCx)
	s += fmt.Sprintf(`%s %s "%s"`, e.editBuffer.GetEncoding(), e.editBuffer.GetNewLine().String(), (*e.editBuffer.GetLangMode()).Name())

	// char code
	ch, _, _ := (*e).editBuffer.Rows().Row(e.meta.RowIndex).DecodeRune(e.meta.ColIndex)
	str := e.RuneStatus(ch)
	s += fmt.Sprintf(" ('%s', %d, 0x%02X)", str, ch, ch)

	a := theme.ColorModelineInactive
	if e.active {
		a = theme.ColorModeLineActive
	}
	e.screen.DrawString(e.editArea.X, e.editArea.Y+e.editArea.Height, e.editArea.Width, s, a)
}
