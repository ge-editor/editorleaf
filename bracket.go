package editorleaf

import (
	"bytes"
	"unicode/utf8"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/editorleaf/spell"
	"github.com/ge-editor/gecore"
)

// bracketPairs are the brackets that match: ASCII and the full-width ones of
// Japanese text. '<' '>' are not here: they are mostly operators.
var bracketPairs = [][2]rune{
	{'(', ')'}, {'[', ']'}, {'{', '}'},
	{'（', '）'}, {'［', '］'}, {'｛', '｝'},
	{'「', '」'}, {'『', '』'}, {'【', '】'},
	{'〈', '〉'}, {'《', '》'}, {'〔', '〕'},
}

// bracketKind returns the pair of r, and whether r is the opening one.
func bracketKind(r rune) (pair [2]rune, isOpen, ok bool) {
	for _, p := range bracketPairs {
		switch r {
		case p[0]:
			return p, true, true
		case p[1]:
			return p, false, true
		}
	}

	return [2]rune{}, false, false
}

// bracketAtCursor finds the bracket the cursor is on, or else the one just
// before it, so that it works with the cursor behind a ")". It returns where
// the bracket starts.
func bracketAtCursor(rows [][]byte, row, col int) (r, c int, ok bool) {
	if row < 0 || row >= len(rows) || col < 0 || col > len(rows[row]) {
		return 0, 0, false
	}

	line := rows[row]

	if col < len(line) {
		if ch, _ := utf8.DecodeRune(line[col:]); ch != utf8.RuneError {
			if _, _, isBracket := bracketKind(ch); isBracket {
				return row, col, true
			}
		}
	}

	if col > 0 {
		ch, size := utf8.DecodeLastRune(line[:col])
		if ch != utf8.RuneError {
			if _, _, isBracket := bracketKind(ch); isBracket {
				return row, col - size, true
			}
		}
	}

	return 0, 0, false
}

// matchBracketAt returns the position of the bracket that pairs with the one
// at (row, col) of text, which must start there. Only brackets of the same
// kind are counted, so "(]" is not confused with "()".
func matchBracketAt(text [][]byte, row, col int) (mrow, mcol int, found bool) {
	ch, _ := utf8.DecodeRune(text[row][col:])

	pair, isOpen, ok := bracketKind(ch)
	if !ok {
		return 0, 0, false
	}

	depth := 0

	if isOpen {
		for r := row; r < len(text); r++ {
			start := 0
			if r == row {
				start = col + utf8.RuneLen(ch)
			}

			for i := start; i < len(text[r]); {
				cur, size := utf8.DecodeRune(text[r][i:])

				switch cur {
				case pair[0]:
					depth++
				case pair[1]:
					if depth == 0 {
						return r, i, true
					}

					depth--
				}

				i += size
			}
		}

		return 0, 0, false
	}

	for r := row; r >= 0; r-- {
		end := len(text[r])
		if r == row {
			end = col
		}

		for i := end; i > 0; {
			cur, size := utf8.DecodeLastRune(text[r][:i])
			i -= size

			switch cur {
			case pair[1]:
				depth++
			case pair[0]:
				if depth == 0 {
					return r, i, true
				}

				depth--
			}
		}
	}

	return 0, 0, false
}

// codeRows returns rows with the comments and strings blanked out (same
// lengths), for the modes that know them, so that a bracket in a string or a
// comment does not count. Other modes get rows itself.
func (e *Editorleaf) codeRows(rows [][]byte) [][]byte {
	cs, ok := spell.ForMode(*e.editBuffer.GetLangMode()).(spell.CodeScope)
	if !ok {
		return rows
	}

	cs.SetCode(true)
	cs.Prepare(rows)

	code := make([][]byte, len(rows))
	for i, r := range rows {
		code[i] = cs.MaskCode(i, r)
	}

	return code
}

// GotoMatchingBracket moves the cursor to the bracket that pairs with the one
// at the cursor (or just before it). Brackets in strings and comments are
// skipped; a bracket that is itself in a string or comment is matched within
// the text as it is.
func (e *Editorleaf) GotoMatchingBracket() {
	rows := e.editBuffer.Rows().BytesArray()
	pos := e.meta.RowsPos

	r, c, ok := bracketAtCursor(rows, pos.RowIndex, pos.ColIndex)
	if !ok {
		gecore.Echo.AddText("No bracket at the cursor")
		return
	}

	text := e.codeRows(rows)

	// The bracket is in a string or a comment: it is blank in the code.
	ch, size := utf8.DecodeRune(rows[r][c:])
	if !bytes.HasPrefix(text[r][c:], rows[r][c:c+size]) || ch == utf8.RuneError {
		text = rows
	}

	mr, mc, found := matchBracketAt(text, r, c)
	if !found {
		gecore.Echo.AddText("No matching bracket")
		return
	}

	e.SetRowsPos(editbuffer.RowsPos{RowIndex: mr, ColIndex: mc})
}
