package search

import (
	"bytes"
	"context"
	"errors"
	"regexp"
	"sort"
	"unicode"
	"unicode/utf8"
)

// This file is the search engine: standard library only, no editor types, so
// that it can be tested alone. It works on a Doc, which is the rows of a
// buffer joined with '\n'. Searching the joined text is what makes a search
// string that contains a newline work.

// MaxMatches limits the matches of one search.
const MaxMatches = 1_000_000

var ErrTooManyMatches = errors.New("too many matches")

// Pos is a position in the buffer: Row is a row index, Col a byte offset in
// the row (the same meaning as editbuffer.RowsPos).
type Pos struct {
	Row, Col int
}

// Before reports whether p is strictly before q.
func (p Pos) Before(q Pos) bool {
	if p.Row != q.Row {
		return p.Row < q.Row
	}
	return p.Col < q.Col
}

// Span is the range [Start, End) of one search result.
type Span struct {
	Start, End Pos
}

// ----------------------
// Doc
// ----------------------

// Doc is the text to search. It owns a copy of the rows, so it can be handed
// to another goroutine while the buffer keeps being edited.
type Doc struct {
	Text      []byte
	lineStart []int // byte offset of the start of every row in Text
}

// NewDoc joins rows with '\n'. There is always at least one row.
func NewDoc(rows [][]byte) *Doc {
	if len(rows) == 0 {
		rows = [][]byte{nil}
	}
	n := len(rows) - 1
	for _, r := range rows {
		n += len(r)
	}
	d := &Doc{Text: make([]byte, 0, n), lineStart: make([]int, len(rows))}
	for i, r := range rows {
		if i > 0 {
			d.Text = append(d.Text, '\n')
		}
		d.lineStart[i] = len(d.Text)
		d.Text = append(d.Text, r...)
	}
	return d
}

// Pos converts a byte offset of Text into a position.
func (d *Doc) Pos(off int) Pos {
	// The last row whose start is <= off.
	i := sort.Search(len(d.lineStart), func(i int) bool { return d.lineStart[i] > off }) - 1
	if i < 0 {
		i = 0
	}
	return Pos{Row: i, Col: off - d.lineStart[i]}
}

// Offset converts a position into a byte offset of Text.
func (d *Doc) Offset(p Pos) int {
	if p.Row < 0 {
		return 0
	}
	if p.Row >= len(d.lineStart) {
		return len(d.Text)
	}
	return d.lineStart[p.Row] + p.Col
}

// ----------------------
// Options
// ----------------------

// Options are the conditions of a search (the line of check boxes).
type Options struct {
	Text      string
	MatchCase bool
	WholeWord bool
	Regex     bool
}

// Compile builds the regexp. ^ and $ match at every row (?m); . does not
// match a newline (no ?s). Without Regex the text is searched literally,
// newlines included.
func (o Options) Compile() (*regexp.Regexp, error) {
	pattern := o.Text
	if !o.Regex {
		pattern = regexp.QuoteMeta(pattern)
	}
	flags := "(?m"
	if !o.MatchCase {
		flags += "i"
	}
	flags += ")"
	return regexp.Compile(flags + pattern)
}

// Match is one result. Index is the submatch indexes into the Text of the Doc
// it was found in, as returned by the regexp package (used by Expand).
type Match struct {
	Span
	Index []int
}

// Expand returns the replacement for this match. With Regex, $1 and ${name}
// in template are expanded; otherwise template is used as it is.
func (m Match) Expand(re *regexp.Regexp, d *Doc, template string, regex bool) []byte {
	if !regex {
		return []byte(template)
	}
	return re.Expand(nil, []byte(template), d.Text, m.Index)
}

// Find returns the matches of o in d, in order, without overlaps. Empty
// matches (for example ^ alone) are left out: they have nothing to show or to
// replace. ctx is checked while the matches are collected.
func Find(ctx context.Context, d *Doc, o Options) (*regexp.Regexp, []Match, error) {
	if o.Text == "" {
		return nil, nil, nil
	}
	re, err := o.Compile()
	if err != nil {
		return nil, nil, err
	}

	all := re.FindAllSubmatchIndex(d.Text, MaxMatches+1)
	if len(all) > MaxMatches {
		return re, nil, ErrTooManyMatches
	}

	out := make([]Match, 0, len(all))
	for i, idx := range all {
		if i&0xfff == 0 {
			if err := ctx.Err(); err != nil {
				return re, nil, err
			}
		}
		start, end := idx[0], idx[1]
		if start == end {
			continue
		}
		if o.WholeWord && !isWholeWord(d.Text, start, end) {
			continue
		}
		out = append(out, Match{
			Span:  Span{Start: d.Pos(start), End: d.Pos(end)},
			Index: idx,
		})
	}
	return re, out, nil
}

// Go's \b only knows ASCII, so a whole word is checked here. A word is made of
// letters, digits and '_' (letters include Japanese).
func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func isWholeWord(text []byte, start, end int) bool {
	if start > 0 {
		if r, _ := utf8.DecodeLastRune(text[:start]); isWordRune(r) {
			return false
		}
	}
	if end < len(text) {
		if r, _ := utf8.DecodeRune(text[end:]); isWordRune(r) {
			return false
		}
	}
	return true
}

// Rows splits text into rows the way a buffer holds them.
func Rows(text string) [][]byte {
	return bytes.Split([]byte(text), []byte{'\n'})
}

// ----------------------
// Union of generations
// ----------------------

// Union merges the spans of several generations into one list that is sorted
// by Start and has no overlaps; this is what the highlight layer needs. The
// results of different searches cannot be told apart once they overlap.
// overlapped reports whether any two spans overlapped (spans that only touch
// do not). Spans of one generation never overlap each other.
func Union(gens ...[]Span) (merged []Span, overlapped bool) {
	n := 0
	for _, g := range gens {
		n += len(g)
	}
	all := make([]Span, 0, n)
	for _, g := range gens {
		all = append(all, g...)
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].Start != all[j].Start {
			return all[i].Start.Before(all[j].Start)
		}
		return all[j].End.Before(all[i].End) // the longer first
	})

	merged = make([]Span, 0, len(all))
	for _, s := range all {
		if k := len(merged) - 1; k >= 0 && s.Start.Before(merged[k].End) {
			overlapped = true
			if merged[k].End.Before(s.End) {
				merged[k].End = s.End
			}
			continue
		}
		merged = append(merged, s)
	}
	return merged, overlapped
}
