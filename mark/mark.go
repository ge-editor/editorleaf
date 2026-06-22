package mark

import (
	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/gecore/screen"
)

func NewMark(ff *editbuffer.EditBuffer, current screen.Cursor, content string) *Mark {
	return &Mark{
		File:    ff,
		Cursor:  current,
		Content: content,
	}
}

type Mark struct {
	File *editbuffer.EditBuffer
	screen.Cursor
	Content string
}

// AdjustForInsertion updates the mark position after text insertion.
// insertStart: insertion start position
// insertEnd: insertion end position after insertion
func (m *Mark) AdjustForInsertion(insertStart, insertEnd screen.Cursor) {
	// If the mark is located before the insertion row,
	// its position is unaffected.
	if m == nil || m.RowIndex < insertStart.RowIndex {
		return
	}

	// Number of rows added by the insertion.
	rowOffset := insertEnd.RowIndex - insertStart.RowIndex

	// Mark is on the same row as insertion start.
	if m.RowIndex == insertStart.RowIndex {
		// Only adjust when insertion occurs before or at the mark position.
		if m.ColIndex >= insertStart.ColIndex {

			// Single-line insertion:
			// shift only the column position.
			if rowOffset == 0 {
				m.ColIndex += insertEnd.ColIndex - insertStart.ColIndex
			} else {
				// Multi-line insertion:
				// move the mark to the corresponding position
				// in the inserted end row.
				m.ColIndex = m.ColIndex - insertStart.ColIndex + insertEnd.ColIndex
				m.RowIndex += rowOffset
			}
		}
		return
	}

	// Mark is below the insertion area:
	// shift downward by the inserted row count.
	m.RowIndex += rowOffset
}

// AdjustForDeletion updates the mark position after text deletion.
// deleteStart: deletion start position
// deleteEnd: deletion end position
func (m *Mark) AdjustForDeletion(deleteStart, deleteEnd screen.Cursor) {
	// If the mark is before the deletion start row,
	// its position is unaffected.
	if m == nil || m.RowIndex < deleteStart.RowIndex {
		return
	}

	// Mark is on the same row as deletion start.
	if m.RowIndex == deleteStart.RowIndex {

		// If the mark is at or before the deletion start,
		// no adjustment is needed.
		if m.ColIndex <= deleteStart.ColIndex {
			return
		}

		// Otherwise, move the mark to the deletion start.
		m.ColIndex = deleteStart.ColIndex
		return
	}

	// Mark is below the deleted range:
	// shift upward by the number of removed rows.
	if m.RowIndex > deleteEnd.RowIndex {
		m.RowIndex -= deleteEnd.RowIndex - deleteStart.RowIndex
		return
	}

	// Mark is inside the deleted range:
	// move it to the deletion start position.
	m.RowIndex = deleteStart.RowIndex
	m.ColIndex = deleteStart.ColIndex
}
