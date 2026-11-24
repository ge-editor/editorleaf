package editorleaf

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/ge-editor/editorleaf/editbuffer"
)

// CursorRowIndex is the row the cursor is on (the first row is 0).
func (e *Editorleaf) CursorRowIndex() int { return e.meta.RowsPos.RowIndex }

// EditBuffer is the buffer this editor shows.
func (e *Editorleaf) EditBuffer() *editbuffer.EditBuffer { return e.editBuffer }

// ReplaceRows replaces the rows [start, end) with rows, with their line ends:
// start == end inserts rows before the row start (start may be the number of
// rows: after the last row), and no rows deletes the rows. The rows to put in
// do not contain line ends. It is one undo step; the cursor is left behind
// the text.
func (e *Editorleaf) ReplaceRows(start, end int, rows [][]byte) error {
	if e.editBuffer.IsReadonly() {
		return errors.New("buffer is read-only")
	}
	n := e.RowsLength()
	if start < 0 || start > end || end > n {
		return fmt.Errorf("rows %d..%d are out of range (%d rows)", start, end, n)
	}
	if start == end && len(rows) == 0 {
		return nil
	}

	rowLen := func(i int) int { return len((*e.editBuffer.Rows())[i]) }
	text := string(bytes.Join(rows, []byte("\n")))
	var from, to editbuffer.RowsPos
	switch {
	case end < n: // whole rows, with the line ends
		from = editbuffer.RowsPos{RowIndex: start}
		to = editbuffer.RowsPos{RowIndex: end}
		if len(rows) > 0 {
			text += "\n"
		}
	case start > 0: // up to the end of the buffer: take the line end before
		from = editbuffer.RowsPos{RowIndex: start - 1, ColIndex: rowLen(start - 1)}
		to = editbuffer.RowsPos{RowIndex: n - 1, ColIndex: rowLen(n - 1)}
		if len(rows) > 0 {
			text = "\n" + text
		}
	default: // the whole buffer
		to = editbuffer.RowsPos{RowIndex: n - 1, ColIndex: rowLen(n - 1)}
	}

	undo := e.editBuffer.UndoAction
	first := undo.Index()
	undo.Seal() // do not merge into what the user typed before
	e.replaceRange(from, to, text)
	undo.MergeSince(first) // delete + insert is one undo step
	return nil
}
