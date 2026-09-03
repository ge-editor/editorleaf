package rows

import (
	"slices"
	"strings"

	"github.com/ge-editor/utils"
)

type Rows []Row

func New() *Rows {
	r := make(Rows, 0, 64)
	return &r
}

// SetRows replaces the current rows.
// The supplied rows are normalized so that row data does not contain
// line separators.
func (rs *Rows) SetRows(newRows Rows) {
	*rs = newRows
}

// Clone returns a completely independent copy of the rows.
//
// Both the Rows slice and the byte data of each Row are copied,
// so modifications to the returned rows do not affect the original.
func (rs Rows) Clone() Rows {
	if rs == nil {
		return nil
	}

	cloned := make(Rows, len(rs))

	for i, row := range rs {
		if row == nil {
			continue
		}

		cloned[i] = make(Row, len(row))
		copy(cloned[i], row)
	}

	return cloned
}

func (rs Rows) Reversed() Rows {
	result := make(Rows, len(rs))

	for i := range rs {
		result[len(rs)-1-i] = utils.ReverseUTF8Bytes(rs[i])
	}

	return result
}

// SetBytesArray replaces the current rows from [][]byte.
// Line separators are removed from each row.
func (rs *Rows) SetBytesArray(source [][]byte) {
	rows := make(Rows, 0, len(source))

	for _, b := range source {
		b = trimNewline(b)
		rows = append(rows, Row(b))
	}

	*rs = rows
}

func trimNewline(b []byte) []byte {
	if len(b) >= 2 &&
		b[len(b)-2] == 0x0d &&
		b[len(b)-1] == 0x0a {
		return b[:len(b)-2]
	}

	if len(b) > 0 {
		switch b[len(b)-1] {
		case 0x0a, 0x0d:
			return b[:len(b)-1]
		}
	}

	return b
}

// SetRow sets the content of a specific line by index
func (rs *Rows) SetRow(rowIndex int, row []byte) bool {
	if rowIndex < 0 || rowIndex >= len(*rs) {
		return false
	}
	(*rs)[rowIndex] = row
	return true
}

func (rs Rows) BytesArray() [][]byte {
	b := make([][]byte, len(rs))
	for i, r := range rs {
		b[i] = []byte(r)
	}
	return b
}

func (rs *Rows) Row(rowIndex int) *Row {
	if rowIndex < 0 || rowIndex >= len(*rs) {
		return nil
	}
	return &(*rs)[rowIndex]
}

// AddRow adds a new []byte to the lines **slices**
func (rs *Rows) AddRow(data []byte) {
	*rs = append(*rs, data)
}

// Join joins two Rows by concatenating the last row of rs
// with the first row of r, then appending the remaining rows of r.
//
// For example:
//
//	rs: aa bb cc
//	r:  dd ee
//
//	result: aa bb ccdd ee
func (rs *Rows) Join(r Rows) {
	if len(r) == 0 {
		return
	}
	if len(*rs) == 0 {
		*rs = append(*rs, r...)
		return
	}

	last := len(*rs) - 1

	// Join the last row of rs with the first row of r.
	(*rs)[last] = append((*rs)[last], r[0]...)

	// Append the remaining rows of r.
	*rs = append(*rs, r[1:]...)
}

// Joined joins two Rows without modifying either of them.
// It returns a new Rows containing the joined result.
//
// For example:
//
//	rs: aa bb cc
//	r:  dd ee
//
//	result: aa bb ccdd ee
func (rs Rows) Joined(r Rows) Rows {
	if len(r) == 0 {
		return append(Rows(nil), rs...)
	}
	if len(rs) == 0 {
		return append(Rows(nil), r...)
	}

	result := make(Rows, 0, len(rs)+len(r)-1)
	result = append(result, rs[:len(rs)-1]...)

	// Join the last row of rs with the first row of r.
	last := append(Row(nil), rs[len(rs)-1]...)
	last = append(last, r[0]...)
	result = append(result, last)

	// Append the remaining rows of r.
	result = append(result, r[1:]...)

	return result
}

// delete rows[col1:col2]
func (rs *Rows) Delete(col1, col2 int) {
	*rs = slices.Delete(*rs, col1, col2)
	// return slices.Delete(*r, col1, col2)
}

// InsertRow inserts a new line at the specified index
func (rs *Rows) InsertRow(rowIndex int, row []byte) bool {
	if rowIndex < 0 || rowIndex > len(*rs) {
		return false
	}
	*rs = slices.Insert(*rs, rowIndex, row)
	return true
}

func (rs *Rows) Length() int {
	return len(*rs)
}

func (rs Rows) IsRowIndexLastRow(rowIndex int) bool {
	return rowIndex >= 0 && rowIndex == len(rs)-1
}

func (rs Rows) String(newline []byte) string {
	var s strings.Builder

	for _, r := range rs {
		s.Write(r)
		s.Write(newline)
	}

	return s.String()
}
