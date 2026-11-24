package editorleaf

import (
	"github.com/gdamore/tcell/v3"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/theme"
)

// Gutter marks: four small marks for each row, in the line number area.
//
// The number has a space on each side. A cell can show two marks, one above
// the other, with the upper half block "▀": its foreground colour is the
// upper mark and its background colour is the lower mark. Two cells, so four
// marks:
//
//	left cell      right cell
//	[LeftTop]      [RightTop]
//	[LeftBottom]   [RightBottom]
//
// What is marked is up to the providers (RegisterGutterProvider): the git
// changes use the left cell, and the right cell is free for diagnostics,
// bookmarks, and so on. A provider sets only the slots it owns, so that
// providers do not hide each other.

// GutterSlot is one of the four marks of a row.
type GutterSlot int

const (
	GutterLeftTop GutterSlot = iota
	GutterLeftBottom
	GutterRightTop
	GutterRightBottom

	gutterSlotCount
)

// GutterMarks are the colours of the four slots of a row. The zero value
// (tcell.ColorDefault) is no mark.
type GutterMarks [gutterSlotCount]tcell.Color

// GutterProvider sets the slots it owns of marks for the row rowIndex of eb.
// It is called for every row that is drawn, on the UI goroutine: it must be
// quick (cache what is costly, per buffer version).
type GutterProvider func(eb *editbuffer.EditBuffer, rowIndex int, marks *GutterMarks)

var gutterProviders []GutterProvider

// RegisterGutterProvider adds a provider of gutter marks. Call it at start up,
// before the first draw.
func RegisterGutterProvider(p GutterProvider) {
	gutterProviders = append(gutterProviders, p)
}

// gutterCell is the rune and the style of a cell of the gutter that shows
// the marks top and bottom (zero: none) on the background bg.
func gutterCell(top, bottom, bg tcell.Color) (rune, tcell.Style) {
	if top == tcell.ColorDefault && bottom == tcell.ColorDefault {
		return ' ', theme.ColorLineNumber.Background(bg)
	}
	if top == tcell.ColorDefault {
		top = bg
	}
	if bottom == tcell.ColorDefault {
		bottom = bg
	}
	return '▀', theme.ColorDefault.Foreground(top).Background(bottom)
}

// drawGutterMarks draws the marks of the row rowIndex, on each of the h
// screen rows it takes from startScreenY, over the spaces beside the number.
func (e *Editorleaf) drawGutterMarks(rowIndex, startScreenY, cursorLineY, h int) {
	if e.lineNumberWidth < 2 || len(gutterProviders) == 0 {
		return
	}
	var marks GutterMarks
	for _, p := range gutterProviders {
		p(e.editBuffer, rowIndex, &marks)
	}
	if marks == (GutterMarks{}) {
		return
	}

	for i := 0; i < h; i++ {
		sy := startScreenY + i
		if sy < 0 || sy >= e.editArea.Height {
			continue
		}
		bg := theme.ColorLineNumberBackground
		if sy == cursorLineY {
			bg = theme.ColorCursorLineNumberBackground
		}
		y := e.editArea.Y + sy

		r, st := gutterCell(marks[GutterLeftTop], marks[GutterLeftBottom], bg)
		e.screen.SetContent(e.editArea.X, y, r, nil, st)
		r, st = gutterCell(marks[GutterRightTop], marks[GutterRightBottom], bg)
		e.screen.SetContent(e.editArea.X+e.lineNumberWidth-1, y, r, nil, st)
	}
}
