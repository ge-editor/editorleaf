package editorleaf

import (
	"fmt"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/gecore"
	"github.com/ge-editor/gecore/lang"
)

// ToggleCommentRegion comments the lines of the region, or uncomments them if
// they all are comments: one step of Undo.
//
// The region is the lines from the last mark of the file to the cursor. When
// the region ends at the start of a line (column 0) that line is not part of
// it, as in Emacs. With no mark (or the mark at the cursor) it is the line of
// the cursor. How a comment is written comes from the mode of the buffer (see
// lang.CommentStyleOf); a mode that has none gets a message.
//
// The cursor stays at the same place in the text. Marks follow the edits.
func (e *Editorleaf) ToggleCommentRegion() {
	style, ok := lang.CommentStyleOf(*e.editBuffer.GetLangMode())
	if !ok {
		gecore.Echo.AddText(fmt.Sprintf("No comment syntax in %s mode", (*e.editBuffer.GetLangMode()).Name()))
		return
	}

	first, last := e.commentRows()

	rows := e.editBuffer.Rows()
	lines := make([][]byte, 0, last-first+1)

	for r := first; r <= last; r++ {
		row := rows.Row(r)
		if row == nil {
			return
		}

		lines = append(lines, row.Bytes())
	}

	edits, commented, ok := lang.CommentEdits(lines, style)
	if !ok {
		gecore.Echo.AddText("Nothing to comment")
		return
	}

	cursor := e.meta.RowsPos
	newCol := cursor.ColIndex

	if cursor.RowIndex >= first && cursor.RowIndex <= last {
		newCol = lang.AdjustCol(edits, cursor.RowIndex-first, cursor.ColIndex)
	}

	// One undo step. The edits are in descending order, so each is made where
	// the ones before it have not moved anything.
	undo := e.editBuffer.UndoAction
	firstUndo := undo.Index()
	undo.Seal()

	for _, ed := range edits {
		start := editbuffer.RowsPos{RowIndex: first + ed.Line, ColIndex: ed.Col}
		end := editbuffer.RowsPos{RowIndex: first + ed.Line, ColIndex: ed.Col + ed.Del}
		e.replaceRange(start, end, ed.Ins)
	}

	undo.MergeSince(firstUndo)

	e.SetRowsPos(editbuffer.RowsPos{RowIndex: cursor.RowIndex, ColIndex: max(newCol, 0)})

	n := len(lines)
	if commented {
		gecore.Echo.AddText(fmt.Sprintf("Commented %d line(s)", n))
	} else {
		gecore.Echo.AddText(fmt.Sprintf("Uncommented %d line(s)", n))
	}
}

// commentRows returns the first and the last row to toggle.
func (e *Editorleaf) commentRows() (first, last int) {
	pos := e.meta.RowsPos
	first, last = pos.RowIndex, pos.RowIndex

	mk := e.findLastByEditBuffer()
	if mk == nil || mk.RowsPos == pos {
		return first, last
	}

	a, b := mk.RowsPos, pos
	if cmpRowsPos(a, b) > 0 {
		a, b = b, a
	}

	first, last = a.RowIndex, b.RowIndex

	// A region that ends at the start of a line does not take that line.
	if last > first && b.ColIndex == 0 {
		last--
	}

	if n := e.editBuffer.Rows().Length(); last >= n {
		last = n - 1
	}

	return first, last
}
