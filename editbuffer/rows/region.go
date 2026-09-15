package rows

func (rs *Rows) InsertRegion(rowIndex, colIndex int, rows Rows) {
	if len(rows) == 0 {
		return
	}

	// Insert position
	row := (*rs)[rowIndex]

	// Keep the left side of the current row.
	left := row[:colIndex]

	// Keep the right side of the current row.
	right := row[colIndex:]

	// Single row insertion: simply insert the bytes.
	if len(rows) == 1 {
		(*rs)[rowIndex] = append(
			append(make(Row, 0, len(left)+len(rows[0])+len(right)),
				left...,
			),
			rows[0]...,
		)
		(*rs)[rowIndex] = append((*rs)[rowIndex], right...)
		return
	}

	// First row: left + inserted first row.
	first := make(Row, 0, len(left)+len(rows[0]))
	first = append(first, left...)
	first = append(first, rows[0]...)

	// Last row: inserted last row + right.
	last := make(Row, 0, len(rows[len(rows)-1])+len(right))
	last = append(last, rows[len(rows)-1]...)
	last = append(last, right...)

	// Replace the current row with the first inserted row.
	(*rs)[rowIndex] = first

	// Insert the remaining rows.
	insertIndex := rowIndex + 1

	for i := 1; i < len(rows)-1; i++ {
		(*rs).InsertRow(insertIndex, rows[i])
		insertIndex++
	}

	// Insert the last row.
	(*rs).InsertRow(insertIndex, last)
}

// RemoveRegion removes the specified region and returns the removed bytes.
// The region is half-open: [start, end).
// No undo/redo functionality is provided by this package.
func (rs *Rows) RemoveRegion(start, end RowsPos) Rows {
	return rs.region(start, end, true)
}

// GetRegion returns a copy of the specified region without modifying the rows.
// The region is half-open: [start, end).
// No undo/redo functionality is provided by this package.
func (rs *Rows) GetRegion(start, end RowsPos) Rows {
	return rs.region(start, end, false)
}

// region returns the rows in the specified region.
// When doRemove is true, the region is also removed from rs.
//
// colIndex is a byte offset within a row.
// len(row) is a valid row-end position.
func (rs *Rows) region(start, end RowsPos, doRemove bool) Rows {
	// Validate row indices.
	if start.RowIndex < 0 ||
		end.RowIndex < 0 ||
		start.RowIndex > end.RowIndex ||
		end.RowIndex >= rs.Length() {
		return nil
	}

	// A region must contain at least one byte.
	if start.RowIndex == end.RowIndex && start.ColIndex >= end.ColIndex {
		return nil
	}

	topRow := rs.Row(start.RowIndex)
	bottomRow := rs.Row(end.RowIndex)

	if topRow == nil || bottomRow == nil {
		return nil
	}

	// colIndex is a byte offset.
	// len(row) is a valid position at the end of the row.
	if start.ColIndex < 0 || start.ColIndex > topRow.Length() {
		return nil
	}

	if end.ColIndex < 0 || end.ColIndex > bottomRow.Length() {
		return nil
	}

	// Same row.
	if start.RowIndex == end.RowIndex {
		removed := Rows{
			topRow.Copy(start.ColIndex, end.ColIndex),
		}

		if doRemove {
			topRow.Delete(start.ColIndex, end.ColIndex)
		}

		return removed
	}

	removed := make(Rows, 0, end.RowIndex-start.RowIndex+1)

	// Top row: remove everything from start.ColIndex to the end.
	removed = append(removed,
		Row(topRow.Bytes()[start.ColIndex:]).Clone(),
		// topRow.Copy(start.ColIndex, topRow.Length()),
	)

	if doRemove {
		*topRow = topRow.Bytes()[:start.ColIndex]
	}

	// Middle rows.
	for i := start.RowIndex + 1; i < end.RowIndex; i++ {
		removed = append(removed, rs.Row(i).Clone())
	}

	// Bottom row: remove everything from the beginning to end.ColIndex.
	removed = append(removed,
		Row(bottomRow.Bytes()[:end.ColIndex]).Clone(),
		// bottomRow.Copy(0, end.ColIndex),
	)

	if doRemove {
		// Join the remaining part of the bottom row to the top row.
		*topRow = append(*topRow, bottomRow.Bytes()[end.ColIndex:]...)

		// Remove the middle rows and the bottom row.
		rs.Delete(start.RowIndex+1, end.RowIndex+1)
	}

	return removed
}
