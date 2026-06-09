package rows

import (
	"slices"
	"unicode/utf8"

	"github.com/ge-editor/utils"
)

type row []byte

// Return the number of bytes in the row
func (r *row) Length() int {
	return len(*r)
}

func (r *row) IsColIndexAtRowEnd(colIndex int) bool {
	_, size := utf8.DecodeRune((*r)[colIndex:])
	return len(*r) == colIndex+size
}

func (r row) Bytes() []byte {
	return r
}

func (r *row) SubBytes(col1, col2 int) []byte {
	removed := make([]byte, col2-col1)
	copy(removed, (*r)[col1:col2])
	return removed
}

func (r *row) Delete(col1, col2 int) row {
	return slices.Delete(*r, col1, col2)
}

// Add bytes to row
func (r *row) Add(b []byte) {
	*r = append(*r, b...)
}

// DecodeRune decodes a rune from the specified line and position
func (r row) DecodeRune(colIndex int) (ch rune, size int, ok bool) {
	if colIndex < 0 || colIndex >= len(r) {
		return 0, 0, false
	}
	ch, size = utf8.DecodeRune(r[colIndex:])
	// encoding is invalid
	// if ch == utf8.RuneError && size == 1 {
	// 許容する
	if size == 0 {
		return 0, 0, false
	}
	return ch, size, true
}

// DecodeRune decodes a rune from the specified line and position
func (r *row) DecodeEndRune() (ch rune, size, colIndex int, ok bool) {
	return r.DecodePrevRune(len(*r)) // +1
}

// DecodePrevRune decodes the previous rune from the specified line and position
// i: colIndex
func (r *row) DecodePrevRune(colIndex int) (ch rune, size, i int, ok bool) {
	if colIndex <= 0 || colIndex > len((*r)) {
		return 0, 0, 0, false
	}
	// Move back to find the start of the previous rune
	i = colIndex - 1
	for i >= 0 && !utf8.RuneStart((*r)[i]) {
		i--
	}
	if i < 0 {
		return 0, 0, 0, false
	}
	ch, size = utf8.DecodeRune((*r)[i:])
	if ch == utf8.RuneError && size == 1 {
		return 0, 0, 0, false
	}
	if colIndex-i != size {
		return 0, 0, 0, false
	}
	return ch, size, i, true
}

// **********************************

type RowsStruct struct {
	rows [][]byte
}

// New creates a new buffer with an initial capacity of 64
func New() *RowsStruct {
	return &RowsStruct{
		rows: make([][]byte, 0, 64),
	}
}

func (rs *RowsStruct) Rows() *rows {
	return (*rows)(&rs.rows)
}

func (rs *RowsStruct) SetRows(r rows) {
	rs.rows = r
}

func (rs *RowsStruct) BytesArray() [][]byte {
	return (*rs).rows
}

// return with remove EOF
func (rs *RowsStruct) Bytes() ([]byte, []int, error) {
	bytes, ints, err := utils.JoinBytes((*rs).rows)

	if len(bytes) == 0 {
		return bytes, ints, err
	}

	return bytes[:len(bytes)-1], ints, err // remove EOF
}

// RowsLength returns the number of lines
func (rs *RowsStruct) RowsLength() int {
	return len((*rs).rows)
}

// **********************************

type rows [][]byte

func (r *rows) Row(rowIndex int) *row {
	return (*row)(&(*r)[rowIndex])
}

// AddRow adds a new []byte to the lines **slices**
func (r *rows) Add(data []byte) {
	*r = append(*r, data)
}

// delete rows[col1:col2]
func (r *rows) Delete(col1, col2 int) {
	*r = slices.Delete(*r, col1, col2)
	// return slices.Delete(*r, col1, col2)
}

// InsertRow inserts a new line at the specified index
func (r *rows) InsertRow(rowIndex int, row []byte) bool {
	if rowIndex < 0 || rowIndex > len(*r) {
		return false
	}
	*r = slices.Insert(*r, rowIndex, row)
	return true
}

// SetRow sets the content of a specific line by index
func (r *rows) SetRow(rowIndex int, row []byte) bool {
	if rowIndex < 0 || rowIndex >= len(*r) {
		return false
	}
	(*r)[rowIndex] = row
	return true
}

func (r *rows) Length() int {
	return len(*r)
}

func (r *rows) IsRowIndexLastRow(rowIndex int) bool {
	return len(*r)-1 == rowIndex
}
