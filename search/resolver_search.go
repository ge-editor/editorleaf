package search

import (
	"github.com/gdamore/tcell/v3"

	"github.com/ge-editor/gecore/styleresolver"
)

type SearchResolver struct {
	Found []FoundPosition

	CursorRow int
	CursorCol int
}

func (r *SearchResolver) Resolve(
	ctx *styleresolver.Context,
	current tcell.Style,
) (tcell.Style, styleresolver.ResolveFlag) {

	// for _, fp := range r.Found {

	/* u := isCursorInRange(
		ctx.RowIndex,
		ctx.ByteIndex,

		fp.Start.RowIndex,
		fp.Start.ColIndex,

		fp.Stop.RowIndex,
		fp.Stop.ColIndex,
	)

	if u != 0 {
		continue
	}

	cursorOnHit :=
		isCursorInRange(
			r.CursorRow,
			r.CursorCol,

			fp.Start.RowIndex,
			fp.Start.ColIndex,

			fp.Stop.RowIndex,
			fp.Stop.ColIndex,
		) == 0

	if cursorOnHit {
		return current.Merge(
			theme.ColorSearchFoundOnCursor,
		)
	}

	return current.Merge(theme.ColorFind) */
	// }

	return current, styleresolver.Continue
}

// isCursorInRange checks if the cursor position (row, col) is within the range
// defined by the top-left (row1, col1) and bottom-right (row2, col2) corners.
// It returns:
//
//	-1 if the cursor is before the range,
//	 1 if the cursor is after the range,
//	 0 if the cursor is within the range.
func isCursorInRange(row, col, row1, col1, row2, col2 int) int {
	// Handle cases where the range is reversed (either vertically or horizontally)
	if row1 > row2 {
		row1, row2 = row2, row1
		col1, col2 = col2, col1
	} else if row1 == row2 && col1 > col2 {
		col1, col2 = col2, col1
	}

	// If the row is outside the range, return false
	if row < row1 {
		return -1
	}
	if row > row2 {
		return 1
	}

	// If the range is within a single row (row1 == row2)
	if row1 == row2 {
		if col < col1 {
			return -1
		}
		if col >= col2 {
			return 1
		}
		return 0
	}

	// If the cursor is on the starting row (row == row1), check the column range
	if row == row1 {
		if col < col1 {
			return -1
		}
		return 0
	}

	// If the cursor is on the ending row (row == row2), check the column range
	if row == row2 {
		if col >= col2 {
			return 1
		}
		return 0
	}

	// If the cursor is on a row between the starting and ending rows,
	// it is always within the range
	return 0
}
