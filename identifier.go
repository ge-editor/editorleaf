package editorleaf

import (
	"unicode"
	"unicode/utf8"
)

func isIdentifierRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// identifierAt returns the identifier (letters, digits and '_' of any script)
// that the byte column col of row is on, or ends at: "" if there is none.
func identifierAt(row []byte, col int) string {
	if col < 0 || col > len(row) {
		return ""
	}

	start := col
	for start > 0 {
		r, size := utf8.DecodeLastRune(row[:start])
		if r == utf8.RuneError || !isIdentifierRune(r) {
			break
		}

		start -= size
	}

	end := col
	for end < len(row) {
		r, size := utf8.DecodeRune(row[end:])
		if r == utf8.RuneError || !isIdentifierRune(r) {
			break
		}

		end += size
	}

	return string(row[start:end])
}

// IdentifierAtCursor returns the identifier under the cursor, or right before
// it; "" if there is none. It is the old name offered when a symbol is renamed.
func (e *Editorleaf) IdentifierAtCursor() string {
	pos := e.meta.RowsPos

	row := e.editBuffer.Rows().Row(pos.RowIndex)
	if row == nil {
		return ""
	}

	return identifierAt(row.Bytes(), pos.ColIndex)
}
