package editbuffer

import (
	"slices"
	"sort"
)

// Point is a (row, byte column) position. It has the same meaning as
// rows.RowsPos and as tree-sitter's Point, but is a separate type so that
// this file does not depend on any parser library.
type Point struct {
	Row    int
	Column int
}

// Edit describes one modification in the form tree-sitter's InputEdit
// expects: byte offsets and points, all measured in a text where rows are
// joined with a single '\n'.
//
// StartByte/StartPoint and OldEnd* are in the text before the edit,
// NewEnd* is in the text after the edit.
type Edit struct {
	StartByte, OldEndByte, NewEndByte    int
	StartPoint, OldEndPoint, NewEndPoint Point
}

// LineIndex maps (row, byte column) to a byte offset in the text obtained
// by joining all rows with '\n', and back.
//
// It is kept in sync with the buffer by feeding it every Change through
// Apply. Apply costs O(number of rows after the edit), which is a simple
// integer add per row and is fine for ordinary files.
//
// LineIndex is not safe for concurrent use. The intended owner is the
// worker that also owns the parser's copy of the text.
type LineIndex struct {
	starts []int // starts[i] = byte offset of the first byte of row i
	size   int   // total number of bytes of the joined text
}

// NewLineIndex builds an index for rs.
func NewLineIndex(rs Rows_) *LineIndex {
	li := &LineIndex{}
	li.Reset(rs)
	return li
}

// Reset rebuilds the index from rs.
func (li *LineIndex) Reset(rs Rows_) {
	li.starts = li.starts[:0]
	off := 0
	for _, r := range rs {
		li.starts = append(li.starts, off)
		off += len(r) + 1
	}
	if len(rs) > 0 {
		off-- // no newline after the last row
	}
	li.size = off
}

// NumRows returns the number of rows.
func (li *LineIndex) NumRows() int { return len(li.starts) }

// Size returns the byte length of the joined text.
func (li *LineIndex) Size() int { return li.size }

// Offset returns the byte offset of pos. pos must be a valid position.
func (li *LineIndex) Offset(pos RowsPos) int {
	return li.starts[pos.RowIndex] + pos.ColIndex
}

// Point returns the (row, column) of a byte offset in [0, Size()].
func (li *LineIndex) Point(off int) Point {
	row := sort.Search(len(li.starts), func(i int) bool { return li.starts[i] > off }) - 1
	if row < 0 {
		row = 0
	}
	return Point{Row: row, Column: off - li.starts[row]}
}

func (li *LineIndex) clone() *LineIndex {
	return &LineIndex{starts: slices.Clone(li.starts), size: li.size}
}

// rowLen returns the byte length of row (without the newline).
func (li *LineIndex) rowLen(row int) int {
	if row+1 < len(li.starts) {
		return li.starts[row+1] - li.starts[row] - 1
	}
	return li.size - li.starts[row]
}

// validPos reports whether p is a valid position in the indexed text.
// A column equal to the row length (the end of the row) is valid.
func (li *LineIndex) validPos(p RowsPos) bool {
	return p.RowIndex >= 0 &&
		p.RowIndex < len(li.starts) &&
		p.ColIndex >= 0 &&
		p.ColIndex <= li.rowLen(p.RowIndex)
}

// Apply returns the Edit for c (computed in the coordinates before c) and
// then updates the index so that it describes the buffer after c.
//
// ok is false when c cannot be applied: ChangeReset (rebuild with Reset
// instead), or a change that does not fit the index, which means the index
// is out of sync. In that case the index is left unchanged and the caller
// should rebuild it and reparse from scratch.
func (li *LineIndex) Apply(c Change) (e Edit, ok bool) {
	switch c.Kind {
	case ChangeInsert:
		return li.applyInsert(c)
	case ChangeDelete:
		return li.applyDelete(c)
	default:
		return Edit{}, false
	}
}

func (li *LineIndex) applyInsert(c Change) (Edit, bool) {
	if !li.validPos(c.Start) || len(c.Text) == 0 {
		return Edit{}, false
	}

	row := c.Start.RowIndex
	startByte := li.starts[row] + c.Start.ColIndex

	// Byte length of the inserted text, rows joined with '\n'.
	length := len(c.Text) - 1
	for _, r := range c.Text {
		length += len(r)
	}

	e := Edit{
		StartByte:   startByte,
		OldEndByte:  startByte,
		NewEndByte:  startByte + length,
		StartPoint:  Point{Row: row, Column: c.Start.ColIndex},
		OldEndPoint: Point{Row: row, Column: c.Start.ColIndex},
		NewEndPoint: Point{Row: c.NewEnd.RowIndex, Column: c.NewEnd.ColIndex},
	}

	// Rows after the insertion point move by length bytes.
	for j := row + 1; j < len(li.starts); j++ {
		li.starts[j] += length
	}

	// Rows created by the insertion (all text rows except the first).
	if n := len(c.Text); n > 1 {
		created := make([]int, 0, n-1)
		next := li.starts[row] + c.Start.ColIndex + len(c.Text[0]) + 1
		created = append(created, next)
		for i := 1; i < n-1; i++ {
			next += len(c.Text[i]) + 1
			created = append(created, next)
		}
		li.starts = slices.Insert(li.starts, row+1, created...)
	}

	li.size += length
	return e, true
}

func (li *LineIndex) applyDelete(c Change) (Edit, bool) {
	if !li.validPos(c.Start) || !li.validPos(c.OldEnd) {
		return Edit{}, false
	}

	startByte := li.Offset(c.Start)
	oldEndByte := li.Offset(c.OldEnd)
	if oldEndByte <= startByte || oldEndByte > li.size {
		return Edit{}, false
	}
	length := oldEndByte - startByte

	e := Edit{
		StartByte:   startByte,
		OldEndByte:  oldEndByte,
		NewEndByte:  startByte,
		StartPoint:  Point{Row: c.Start.RowIndex, Column: c.Start.ColIndex},
		OldEndPoint: Point{Row: c.OldEnd.RowIndex, Column: c.OldEnd.ColIndex},
		NewEndPoint: Point{Row: c.Start.RowIndex, Column: c.Start.ColIndex},
	}

	// Rows fully swallowed by the deletion disappear.
	r1, r2 := c.Start.RowIndex, c.OldEnd.RowIndex
	if r2 > r1 {
		li.starts = slices.Delete(li.starts, r1+1, r2+1)
	}

	// Rows after the deletion move back by length bytes.
	for j := r1 + 1; j < len(li.starts); j++ {
		li.starts[j] -= length
	}

	li.size -= length
	return e, true
}
