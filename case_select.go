package editorleaf

import (
	"strings"

	"github.com/gdamore/tcell/v3"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/editorleaf/highlight"
	"github.com/ge-editor/gecore"
	"github.com/ge-editor/gecore/killbuffer"
	"github.com/ge-editor/theme"
)

// The case-select mode: a selection that lives only for the mode.
//
//	[F4]      select the word around the cursor, then [F4] again changes its
//	          case (snake_case -> SCREAMING_SNAKE_CASE -> ...)
//	[Alt+F4]  select the text between the last mark and the cursor if it is
//	          at most caseSelectMarkLimit bytes; otherwise the same as [F4]
//
// The editor side is here (the range, the case state, the highlight, the
// kill buffer); the key handling is in the ge package (ge/case_select_mode.go).
// The engine that converts the text is in case_engine.go.

// caseSelectMarkLimit is the longest range, in bytes, that [Alt+F4] takes
// from the last mark to the cursor.
const caseSelectMarkLimit = 256

// caseSelectionStyle is the style of the selected range: dark text on a light
// background, which reads as inverted video. It is a complete style, so no
// other background (cursor line, ...) is mixed in.
var caseSelectionStyle = theme.ColorDefault.
	Foreground(tcell.NewRGBColor(0, 0, 0)).
	Background(tcell.NewRGBColor(200, 200, 200))

// caseSelection is the state of the mode in one Editorleaf. UI goroutine only
// (the draw goroutine reads the span in the highlight layer).
type caseSelection struct {
	active bool

	// start and end are the selected range, end exclusive. After a change of
	// case they are the range of the new text, so the range follows the
	// change of length.
	start, end editbuffer.RowsPos

	// cur is the Case the text is in now (an index of CaseTypes, -1 when it
	// is not known). It is kept here and not guessed again each time, because
	// the guess is ambiguous ("foo" is four of the cases at once).
	cur int

	// pushed is true when the kill buffer has an entry that this mode put
	// there; the next change replaces it instead of piling up entries.
	pushed bool
}

// IsCaseSelect reports whether the case-select mode is on in this editor.
func (e *Editorleaf) IsCaseSelect() bool { return e.caseSel.active }

// BeginCaseSelect starts the mode: it makes the range, shows it and puts it
// into the kill buffer. fromMark is [Alt+F4]: the range is the text between
// the last mark and the cursor if there is a mark and the distance is at most
// caseSelectMarkLimit bytes; if not, it is made as for [F4], from the word
// around the cursor.
//
// It returns false, with a message, if there is no range; the mode must not
// be entered then.
func (e *Editorleaf) BeginCaseSelect(fromMark bool) bool {
	e.EndCaseSelect()

	var start, end editbuffer.RowsPos

	ok := false
	if fromMark {
		start, end, ok = e.markRange(caseSelectMarkLimit)
	}

	if !ok {
		pos := e.meta.RowsPos

		s, en, _, found := e.GuessTargetWord(pos.RowIndex, pos.ColIndex)
		if !found {
			gecore.Echo.AddText("No word to change the case of at the cursor")
			return false
		}

		start = editbuffer.RowsPos{RowIndex: pos.RowIndex, ColIndex: s}
		end = editbuffer.RowsPos{RowIndex: pos.RowIndex, ColIndex: en}
	}

	text := e.rangeText(start, end)
	if text == "" {
		gecore.Echo.AddText("Nothing is selected")
		return false
	}

	e.caseSel = caseSelection{
		active: true,
		start:  start,
		end:    end,
		cur:    guessCurrentCase(text),
	}
	e.pushCaseSelection()
	e.paintCaseSelection()

	return true
}

// ToggleCaseSelect changes the case of the selected text to the next one (see
// nextCaseText). The range follows the new text, which is put into the kill
// buffer. When the text cannot be changed nothing happens and the range stays.
func (e *Editorleaf) ToggleCaseSelect() {
	s := &e.caseSel
	if !s.active {
		return
	}

	text := e.rangeText(s.start, s.end)

	out, next, ok := nextCaseText(text, s.cur)
	if !ok {
		gecore.Echo.AddText("The case of the selection cannot be changed")
		return
	}

	// One undo step for the change, which is a deletion and an insertion.
	undo := e.editBuffer.UndoAction
	first := undo.Index()
	undo.Seal() // do not merge into what the user typed before

	e.replaceRange(s.start, s.end, out)
	undo.MergeSince(first)

	// The text has no line break, so the new end is on the same row.
	s.end = editbuffer.RowsPos{RowIndex: s.start.RowIndex, ColIndex: s.start.ColIndex + len(out)}
	s.cur = next
	e.SetRowsPos(s.end)

	e.pushCaseSelection()
	e.paintCaseSelection()
	gecore.Echo.AddText(CaseTypes[next])
}

// DeleteCaseSelect deletes the selected text. The text is already in the kill
// buffer (it was put there when the range was made or last changed), so it
// is not put there again. The caller leaves the mode afterwards.
func (e *Editorleaf) DeleteCaseSelect() {
	s := &e.caseSel
	if !s.active {
		return
	}

	if e.deleteRegion(s.start, s.end) == nil {
		return
	}

	e.SetRowsPos(s.start)
}

// EndCaseSelect leaves the mode: the range is not shown any more. The text
// and the kill buffer stay as they are. It does nothing if the mode is off.
func (e *Editorleaf) EndCaseSelect() {
	if !e.caseSel.active {
		return
	}

	e.caseSel = caseSelection{}

	if e.meta != nil && e.meta.HighlightLayer != nil {
		e.meta.HighlightLayer.Highlights(highlight.LayerSelection).SetSpans([]*highlight.SpanTemplate{})
	}
}

// paintCaseSelection shows the range in highlight.LayerSelection.
func (e *Editorleaf) paintCaseSelection() {
	if e.meta == nil || e.meta.HighlightLayer == nil {
		return
	}

	var span highlight.SpanTemplate = &highlight.Span{
		Start:         e.caseSel.start,
		End:           e.caseSel.end,
		Color:         caseSelectionStyle,
		ColorIfActive: caseSelectionStyle,
		Priority:      highlight.LayerSelection,
	}

	e.meta.HighlightLayer.Highlights(highlight.LayerSelection).SetSpans([]*highlight.SpanTemplate{&span})
}

// pushCaseSelection puts the selected text into the kill buffer (and so into
// the clipboard). The entry that the mode put there before is replaced, so
// that the toggles of one session leave one entry, the last text.
func (e *Editorleaf) pushCaseSelection() {
	s := &e.caseSel

	region := e.editBuffer.Rows().GetRegion(s.start, s.end)
	if region == nil {
		return
	}

	if s.pushed {
		killbuffer.KillBuffer.PopKillBuffer()
	}

	if err := killbuffer.KillBuffer.PushKillBuffer(region.BytesArray(), e.editBuffer.GetNewLine().Bytes()); err != nil {
		gecore.Echo.AddText(err.Error())
	}

	s.pushed = true
}

// rangeText returns the text of [start, end), with line breaks joined as the
// buffer has them; "" if the range is not in the text.
func (e *Editorleaf) rangeText(start, end editbuffer.RowsPos) string {
	region := e.editBuffer.Rows().GetRegion(start, end)
	if region == nil {
		return ""
	}

	rows := region.BytesArray()
	parts := make([]string, len(rows))

	for i, r := range rows {
		parts[i] = string(r)
	}

	return strings.Join(parts, string(e.editBuffer.GetNewLine().Bytes()))
}

// markRange returns the range between the last mark of the file and the
// cursor, in ascending order, if there is a mark, it is not at the cursor and
// the range is at most limit bytes.
func (e *Editorleaf) markRange(limit int) (start, end editbuffer.RowsPos, ok bool) {
	mk := e.findLastByEditBuffer()
	if mk == nil {
		return start, end, false
	}

	a, b := mk.RowsPos, e.meta.RowsPos
	if a == b {
		return start, end, false
	}

	if cmpRowsPos(a, b) > 0 {
		a, b = b, a
	}

	n, within := e.bytesBetween(a, b, limit)
	if !within || n == 0 {
		return start, end, false
	}

	return a, b, true
}

// bytesBetween returns the number of bytes of [a, b) (a before b; line breaks
// are counted) and whether it is at most limit. It stops counting when the
// limit is passed, so that a mark far away costs nothing. A position that is
// not in the text gives false (a mark whose file was shortened).
func (e *Editorleaf) bytesBetween(a, b editbuffer.RowsPos, limit int) (int, bool) {
	rows := e.editBuffer.Rows()

	ra, rb := rows.Row(a.RowIndex), rows.Row(b.RowIndex)
	if ra == nil || rb == nil ||
		a.ColIndex < 0 || a.ColIndex > ra.Length() ||
		b.ColIndex < 0 || b.ColIndex > rb.Length() {
		return 0, false
	}

	if a.RowIndex == b.RowIndex {
		n := b.ColIndex - a.ColIndex
		return n, n <= limit
	}

	newline := len(e.editBuffer.GetNewLine().Bytes())

	n := ra.Length() - a.ColIndex + newline
	for i := a.RowIndex + 1; i < b.RowIndex; i++ {
		if n > limit {
			return n, false
		}

		n += rows.Row(i).Length() + newline
	}

	n += b.ColIndex

	return n, n <= limit
}
