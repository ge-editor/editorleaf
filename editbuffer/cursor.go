package editbuffer

type Cursor struct {
	RowIndex int
	ColIndex int
}

func (c Cursor) Equals(other Cursor) bool {
	return c.RowIndex == other.RowIndex && c.ColIndex == other.ColIndex
}

// adjustCursorForDeletion updates the cursor position to reflect the deleted text within the buffer.
func (c *Cursor) AdjustForDeletion(deleteStart, deleteEnd Cursor) {
	// If the cursor is before the deletion row, no adjustment is needed.
	if c.RowIndex < deleteStart.RowIndex {
		return
	}

	// If the cursor is on the same row as the deletion start:
	if c.RowIndex == deleteStart.RowIndex {
		// If the deletion is after the cursor column, leave the cursor unchanged.
		// if cursor.ColIndex < deleteStart.ColIndex {
		if c.ColIndex <= deleteStart.ColIndex {
			return
		}
		// If the deletion affects the cursor's position within the same row, move it to the start of deletion.
		c.ColIndex = deleteStart.ColIndex
		return
	}

	// If the cursor is after the deletion row range:
	if c.RowIndex > deleteEnd.RowIndex {
		// Adjust the row index to account for the rows deleted.
		c.RowIndex -= deleteEnd.RowIndex - deleteStart.RowIndex
		return
	}

	// If the cursor is within the deleted range, move it to the start of the deletion.
	c.RowIndex = deleteStart.RowIndex
	c.ColIndex = deleteStart.ColIndex
	//return
}

// adjustCursorForInsertion updates the cursor position to reflect the inserted text within the buffer.
func (c *Cursor) AdjustForInsertion(insertStart, insertEnd Cursor) {
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
