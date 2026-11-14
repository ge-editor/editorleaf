package editbuffer

type RowsPos struct {
	RowIndex int
	ColIndex int
}

func (c RowsPos) Equals(other RowsPos) bool {
	return c.RowIndex == other.RowIndex && c.ColIndex == other.ColIndex
}

// AdjustForDeletion updates the position to reflect the deleted text within the buffer.
func (c *RowsPos) AdjustForDeletion(deleteStart, deleteEnd RowsPos) {
	// If the cursor is before the deletion row, no adjustment is needed.
	if c.RowIndex < deleteStart.RowIndex {
		return
	}

	// At or before the start of the deletion: unchanged.
	if c.RowIndex == deleteStart.RowIndex && c.ColIndex <= deleteStart.ColIndex {
		return
	}

	// After the deleted rows: only the row index changes.
	if c.RowIndex > deleteEnd.RowIndex {
		c.RowIndex -= deleteEnd.RowIndex - deleteStart.RowIndex
		return
	}

	// On the last row of the deleted range but behind its end: the text from
	// there on is joined to the start of the deletion, so the position keeps
	// its distance from the end of the deleted text.
	if c.RowIndex == deleteEnd.RowIndex && c.ColIndex >= deleteEnd.ColIndex {
		c.ColIndex = deleteStart.ColIndex + (c.ColIndex - deleteEnd.ColIndex)
		c.RowIndex = deleteStart.RowIndex
		return
	}

	// Within the deleted range: move it to the start of the deletion.
	*c = deleteStart
}

// AdjustForInsertion updates the position to reflect the inserted text within the buffer.
func (c *RowsPos) AdjustForInsertion(insertStart, insertEnd RowsPos) {
	// If the cursor is before the insertion row, no adjustment is needed.
	if c.RowIndex < insertStart.RowIndex {
		return
	}

	// Calculate the number of rows added by the insertion.
	rowOffset := insertEnd.RowIndex - insertStart.RowIndex

	// If the insertion is on the same row as the cursor:
	if c.RowIndex == insertStart.RowIndex {
		// If the insertion is before the cursor in the same row, adjust the cursor column.
		if c.ColIndex >= insertStart.ColIndex {
			// If the insertion doesn't span multiple rows, adjust only the column.
			if rowOffset == 0 {
				c.ColIndex += insertEnd.ColIndex - insertStart.ColIndex
			} else {
				// Adjust column to the end of the insertion in the current row, then add row offset.
				c.ColIndex = c.ColIndex - insertStart.ColIndex + insertEnd.ColIndex
				c.RowIndex += rowOffset
			}
		}
		return
	}

	// If the cursor is after the insertion row(s), increment the row index.
	c.RowIndex += rowOffset
}
