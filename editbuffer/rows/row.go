package rows

import (
	"slices"
	"unicode/utf8"
)

type Row []byte

// Length returns the number of bytes in the row.
func (r Row) Length() int {
	return len(r)
}

// Bytes returns the underlying byte slice of the row.
func (r Row) Bytes() []byte {
	return r
}

// row: "abc"
//
// colIndex:
// 0 1 2 3
// ^ ^ ^ ^
// | | | └─ 行末
// | | └─── c の前
// | └───── b の前
// └─────── a の前

// IsColIndexAtRowEnd reports whether colIndex is at the end of the row.
// colIndex is a byte offset; len(r) is the row-end position.
func (r Row) IsColIndexAtRowEnd(colIndex int) bool {
	return colIndex == len(r)
}

// CopyBytes returns a copy of the specified byte range.
func (r Row) CopyBytes(col1, col2 int) []byte {
	if col1 < 0 || col2 < col1 || col2 > len(r) {
		return nil
	}

	return slices.Clone(r[col1:col2])
}

// CopyBytes returns a copy of the specified byte range.
func (r Row) Copy(col1, col2 int) Row {
	if col1 < 0 || col2 < col1 || col2 > len(r) {
		return nil
	}

	return slices.Clone(r[col1:col2])
}

// Copy returns a copy Row.
func (r Row) Clone() Row {
	return slices.Clone(r)
}

// Delete removes bytes in the range [col1:col2].
func (r *Row) Delete(col1, col2 int) {
	*r = slices.Delete(*r, col1, col2)
}

/* func (r *row) Delete(col1, col2 int) row {
	*r = slices.Delete(*r, col1, col2)
	return *r
}
*/

// Add appends bytes to the row.
func (r *Row) Add(b []byte) {
	*r = append(*r, b...)
}

// DecodeRune decodes a rune at the specified byte position.
func (r Row) DecodeRune(colIndex int) (ch rune, size int, ok bool) {
	if colIndex < 0 || colIndex >= len(r) {
		return 0, 0, false
	}

	ch, size = utf8.DecodeRune(r[colIndex:])
	return ch, size, true
}

// DecodeEndRune decodes the last rune in the row.
func (r Row) DecodeEndRune() (ch rune, size, colIndex int, ok bool) {
	return r.DecodePrevRune(len(r))
}

// DecodePrevRune decodes the rune immediately before colIndex.
func (r Row) DecodePrevRune(colIndex int) (ch rune, size, prevColIndex int, ok bool) {
	if colIndex <= 0 || colIndex > len(r) {
		return 0, 0, 0, false
	}

	prevColIndex = colIndex - 1

	// Move back to find the start of the previous rune.
	for prevColIndex >= 0 && !utf8.RuneStart(r[prevColIndex]) {
		prevColIndex--
	}

	if prevColIndex < 0 {
		return 0, 0, 0, false
	}

	ch, size = utf8.DecodeRune(r[prevColIndex:])

	// Reject invalid UTF-8.
	if ch == utf8.RuneError && size == 1 {
		return 0, 0, 0, false
	}

	// colIndex must be exactly at the end of the decoded rune.
	if colIndex-prevColIndex != size {
		return 0, 0, 0, false
	}

	return ch, size, prevColIndex, true
}
