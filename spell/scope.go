package spell

import (
	"bytes"
	"strings"

	"github.com/ge-editor/gecore/lang"
)

// A Scope tells which parts of a buffer are prose to check, and which are not
// (the code of a Markdown file, the code of a Go file around its comments).
// It only looks at the rows, never at the words.
type Scope interface {
	// Prepare is called when the rows changed, before they are checked. rows
	// are all the rows of the buffer; a scope that depends on the rows above
	// (a code block, a comment that goes on) works that out here.
	Prepare(rows [][]byte)

	// Skipped reports whether the whole row is not to be checked. It is
	// valid after Prepare.
	Skipped(row int) bool

	// Key identifies what is prose in the row. When it differs from the
	// value of the last Prepare the row is checked again, although its text
	// did not change (a comment was opened above it). It is valid after
	// Prepare.
	Key(row int) uint64

	// Mask returns the text of a row with what is not prose replaced by
	// spaces. The length is the same, so the ranges of the checked text are
	// the ranges of text. It may return text itself.
	Mask(row int, text []byte) []byte
}

// ForMode returns the scope of a language mode. A mode with delimiters (see
// lang.Delimiters) has its comments and strings checked; Markdown has its
// prose; every other mode is text, and all of it is checked.
func ForMode(m lang.Mode) Scope {
	if m == nil {
		return textScope{}
	}
	if d := lang.SpellDelimitersOf(m); d != nil {
		return &lexicalScope{d: d}
	}
	if s, ok := ScopeFor(m.Name()); ok {
		return s
	}
	return textScope{}
}

// ScopeFor returns the scope of a text-like language by its name (compared
// case-insensitively). A mode with delimiters is not here: see ForMode.
func ScopeFor(langName string) (Scope, bool) {
	switch strings.ToLower(strings.TrimSpace(langName)) {
	case "markdown":
		return &markdownScope{}, true
	case "fundamental", "org", "text":
		return textScope{}, true
	}
	return nil, false
}

// textScope: everything is prose.
type textScope struct{}

func (textScope) Prepare([][]byte)               {}
func (textScope) Skipped(int) bool               { return false }
func (textScope) Key(int) uint64                 { return 0 }
func (textScope) Mask(_ int, text []byte) []byte { return text }

// markdownScope: the fenced code blocks, the inline code, and the
// destination of links and images are not prose.
type markdownScope struct {
	skip []bool
}

func (s *markdownScope) Prepare(rows [][]byte) {
	if cap(s.skip) < len(rows) {
		s.skip = make([]bool, len(rows))
	}
	s.skip = s.skip[:len(rows)]

	var fence byte // the fence character of the block that is open, 0 if none
	for i, row := range rows {
		t := bytes.TrimLeft(row, " \t")
		c := fenceChar(t)
		switch {
		case fence == 0 && c != 0:
			fence = c
			s.skip[i] = true // the opening line (it has the language)
		case fence != 0 && c == fence:
			fence = 0
			s.skip[i] = true // the closing line
		default:
			s.skip[i] = fence != 0
		}
	}
}

// fenceChar returns '`' or '~' if t starts with a fence of at least 3.
func fenceChar(t []byte) byte {
	if len(t) < 3 {
		return 0
	}
	c := t[0]
	if c != '`' && c != '~' {
		return 0
	}
	if t[1] == c && t[2] == c {
		return c
	}
	return 0
}

func (s *markdownScope) Skipped(row int) bool {
	return row >= 0 && row < len(s.skip) && s.skip[row]
}

func (s *markdownScope) Key(row int) uint64 {
	if s.Skipped(row) {
		return 1
	}
	return 0
}

func (s *markdownScope) Mask(_ int, text []byte) []byte {
	if bytes.IndexByte(text, '`') < 0 && !bytes.Contains(text, []byte("](")) && !bytes.Contains(text, []byte("<")) {
		return text
	}
	out := make([]byte, len(text))
	copy(out, text)

	// `code`: from a backtick to the next one on the row (an unclosed one
	// does not mask anything).
	for i := 0; i < len(out); i++ {
		if out[i] != '`' {
			continue
		}
		j := bytes.IndexByte(out[i+1:], '`')
		if j < 0 {
			break
		}
		blank(out[i : i+1+j+1])
		i += j + 1
	}

	// [text](destination "title") and ![alt](src): the part in the
	// parentheses.
	for i := 0; i+1 < len(out); i++ {
		if out[i] != ']' || out[i+1] != '(' {
			continue
		}
		j := bytes.IndexByte(out[i+2:], ')')
		if j < 0 {
			break
		}
		blank(out[i+1 : i+2+j+1])
		i += j + 2
	}

	// <https://...> and HTML tags
	for i := 0; i < len(out); i++ {
		if out[i] != '<' {
			continue
		}
		j := bytes.IndexByte(out[i+1:], '>')
		if j < 0 {
			break
		}
		blank(out[i : i+1+j+1])
		i += j + 1
	}
	return out
}

func blank(b []byte) {
	for i := range b {
		b[i] = ' '
	}
}

// ---------------------------------------------------------------------------
// Comments and strings
// ---------------------------------------------------------------------------

// CodeScope is a Scope that can also hand out the code around the prose, for
// the engines that check identifiers (CodeEngine). It is off until SetCode.
type CodeScope interface {
	Scope

	// SetCode turns the code on: Skipped is then false for every row, and
	// MaskCode has something to return.
	SetCode(on bool)
	CodeOn() bool

	// MaskCode returns the text of a row with the prose (and what is not
	// text at all: an escape sequence, a character literal) replaced by
	// spaces, so what is left is the code. The length is the same.
	MaskCode(row int, text []byte) []byte
}

// rowSpans is what is known of a row.
type rowSpans struct {
	prose []Range // in a comment or a string
	dead  []Range // in a string, but not text: an escape sequence, a character literal
}

func (r *rowSpans) addProse(start, end int) {
	if end > start {
		r.prose = append(r.prose, Range{start, end})
	}
}

func (r *rowSpans) addDead(start, end int) {
	if end > start {
		r.dead = append(r.dead, Range{start, end})
	}
}

// lexicalScope finds the comments and strings of a program from the
// delimiters of its language. It reads the text the way a lexer does, with a
// state that goes from row to row (a block comment, a raw string), but it
// knows nothing else of the syntax: it needs no parser, so it works in every
// build and for every mode that has delimiters. It is wrong where the
// delimiters are not enough (heredocs, nested block comments); the cost is a
// word checked that is not prose, or one missed.
type lexicalScope struct {
	d    *lang.Delimiters
	rows []rowSpans
	code bool
}

func (s *lexicalScope) SetCode(on bool) { s.code = on }
func (s *lexicalScope) CodeOn() bool    { return s.code }

func (s *lexicalScope) Prepare(rows [][]byte) {
	if cap(s.rows) < len(rows) {
		s.rows = append(s.rows[:cap(s.rows)], make([]rowSpans, len(rows)-cap(s.rows))...)
	}
	s.rows = s.rows[:len(rows)]

	var block *lang.BlockDelimiter
	var str *lang.StringDelimiter
	for r, row := range rows {
		rs := &s.rows[r]
		rs.prose, rs.dead = rs.prose[:0], rs.dead[:0]

		i := 0
		for i < len(row) {
			switch {
			case block != nil:
				end := bytes.Index(row[i:], []byte(block.Close))
				if end < 0 {
					rs.addProse(i, len(row))
					i = len(row)
					continue
				}
				rs.addProse(i, i+end)
				i += end + len(block.Close)
				block = nil

			case str != nil:
				var closed bool
				i, closed = scanString(row, i, str, rs)
				if closed {
					str = nil
				}

			default:
				i, block, str = scanCode(row, i, s.d, rs)
			}
		}
		if str != nil && !str.Multiline {
			str = nil // it ends with the row
		}
	}
}

// scanString reads the content of a string from i. It returns where to go on
// and whether the string was closed.
func scanString(row []byte, i int, d *lang.StringDelimiter, rs *rowSpans) (int, bool) {
	start := i
	for i < len(row) {
		switch {
		case d.Escape != 0 && row[i] == d.Escape:
			if d.Code {
				i = min(i+2, len(row))
				continue
			}
			// The escape and the byte after it are not text.
			rs.addProse(start, i)
			end := min(i+2, len(row))
			rs.addDead(i, end)
			i, start = end, end

		case bytes.HasPrefix(row[i:], []byte(d.Close)):
			if d.Code {
				rs.addDead(start, i)
			} else {
				rs.addProse(start, i)
			}
			return i + len(d.Close), true

		default:
			i++
		}
	}
	if d.Code {
		rs.addDead(start, len(row))
	} else {
		rs.addProse(start, len(row))
	}
	return len(row), false
}

// scanCode reads code from i up to the start of a comment or a string. A line
// comment is prose to the end of the row; a block comment or a string that
// starts is returned, to be read by the caller.
func scanCode(row []byte, i int, d *lang.Delimiters, rs *rowSpans) (int, *lang.BlockDelimiter, *lang.StringDelimiter) {
	for j := i; j < len(row); j++ {
		rest := row[j:]
		for _, lc := range d.LineComments {
			if lc != "" && bytes.HasPrefix(rest, []byte(lc)) {
				rs.addProse(j+len(lc), len(row))
				return len(row), nil, nil
			}
		}
		for k := range d.BlockComments {
			b := &d.BlockComments[k]
			if b.Open != "" && bytes.HasPrefix(rest, []byte(b.Open)) {
				return j + len(b.Open), b, nil
			}
		}
		for k := range d.Strings {
			st := &d.Strings[k]
			if st.Open != "" && bytes.HasPrefix(rest, []byte(st.Open)) {
				return j + len(st.Open), nil, st
			}
		}
	}
	return len(row), nil, nil
}

// Skipped: a row without prose has nothing to check, unless the code is
// checked too.
func (s *lexicalScope) Skipped(row int) bool {
	return !s.code && row >= 0 && row < len(s.rows) && len(s.rows[row].prose) == 0
}

// Key is a hash of the prose ranges of the row (FNV-1a), and of the dead ones
// when the code is checked (they are blanked in the code).
func (s *lexicalScope) Key(row int) uint64 {
	if row < 0 || row >= len(s.rows) {
		return 0
	}
	h := uint64(14695981039346656037)
	mix := func(rs []Range) {
		for _, r := range rs {
			for _, v := range [2]int{r.Start, r.End} {
				h ^= uint64(v) + 1
				h *= 1099511628211
			}
		}
	}
	mix(s.rows[row].prose)
	if s.code {
		h ^= 0xff
		h *= 1099511628211
		mix(s.rows[row].dead)
	}
	return h
}

func (s *lexicalScope) Mask(row int, text []byte) []byte {
	if row < 0 || row >= len(s.rows) {
		return text
	}
	out := make([]byte, len(text))
	blank(out)
	for _, r := range s.rows[row].prose {
		end := min(r.End, len(text))
		if r.Start < end {
			copy(out[r.Start:end], text[r.Start:end])
		}
	}
	return out
}

func (s *lexicalScope) MaskCode(row int, text []byte) []byte {
	if row < 0 || row >= len(s.rows) {
		return text
	}
	out := make([]byte, len(text))
	copy(out, text)
	for _, spans := range [2][]Range{s.rows[row].prose, s.rows[row].dead} {
		for _, r := range spans {
			end := min(r.End, len(out))
			if r.Start < end {
				blank(out[r.Start:end])
			}
		}
	}
	return out
}
