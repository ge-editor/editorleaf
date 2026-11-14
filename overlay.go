package editorleaf

import (
	"sort"

	"github.com/gdamore/tcell/v3"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/editorleaf/highlight"
	"github.com/ge-editor/gecore/marks"
	"github.com/ge-editor/locale"
	"github.com/ge-editor/theme"
)

// Backgrounds that do not belong to the syntax style of a character:
//
//   - the logical row of the cursor
//   - spaces and tabs at the end of a line
//   - the marks of the file
//
// The cached cell styles (RowLayout.RuneWidthCache) never contain them, so
// moving the cursor or setting a mark does not make a row recompute.

// ---------------------------------------------------------------------------
// The cursor row
// ---------------------------------------------------------------------------

// cursorLineStyle returns the style a cell at (x, y) of the edit area is
// drawn with. On the logical row of the cursor a cell whose background is
// the default one gets the cursor-line background, or, beyond the
// recommended column width, the cursor-line background for that area.
// Cells with another background (search results, ...) are kept.
//
// "has background c" is tested by setting c and comparing: tcell.Style is
// comparable, and setting the color it already has changes nothing.
func (e *Editorleaf) cursorLineStyle(x, y, chWidth int, style tcell.Style) tcell.Style {
	if y != e.cursorLineRow1-1 {
		return style
	}

	switch {
	case style.Background(theme.ColorColumnLimitOverflowBackground) == style:
		return style.Background(theme.ColorCursorLineColumnOverBackground)

	case style.Background(theme.ColorDefaultBackground) == style:
		if e.isColumnOver(x, y, chWidth) {
			return style.Background(theme.ColorCursorLineColumnOverBackground)
		}
		return style.Background(theme.ColorCursorLineBackground)
	}

	return style
}

// ---------------------------------------------------------------------------
// Marks
// ---------------------------------------------------------------------------

// markHighlightKey is the state the spans of highlight.LayerMark were built
// from.
type markHighlightKey struct {
	version uint64 // marks.Marks.Version()
	path    string // the file of the buffer
	valid   bool
}

// refreshMarkHighlights rebuilds the spans of highlight.LayerMark when the
// marks or the file of the buffer changed since the last call. There is one
// span for every mark of the file; the span of the newest mark has
// theme.ColorLastMark, the others theme.ColorMark. The layer is only read
// again (and the rows restyled) when the spans really differ.
func (e *Editorleaf) refreshMarkHighlights() {
	if e.mode != ModeEditor || e.meta == nil || e.meta.HighlightLayer == nil {
		return
	}
	hl := e.meta.HighlightLayer.Peek(highlight.LayerMark)
	if hl == nil {
		return
	}

	key := markHighlightKey{
		version: marks.Marks.Version(),
		path:    e.editBuffer.GetPath(),
		valid:   true,
	}
	if e.markKey == key {
		return
	}
	e.markKey = key

	// The marks are kept newest first (marks.Add puts the new one in front),
	// so the first mark of the file is the last one that was set.
	isLast := map[editbuffer.RowsPos]bool{}
	order := []editbuffer.RowsPos{}
	first := true
	for _, a := range marks.Marks.Items() {
		m := toEditorMark(a)
		if m == nil || m.ExtraData == nil || !sameMarkFile(m.FilePath, key.path) {
			continue
		}
		if _, ok := isLast[m.RowsPos]; !ok {
			order = append(order, m.RowsPos)
			isLast[m.RowsPos] = false
		}
		if first {
			isLast[m.RowsPos] = true
			first = false
		}
	}

	// Spans have to be in ascending order (see Highlights.MatcheFirstRegenSpanIndex).
	sort.Slice(order, func(i, j int) bool { return cmpRowsPos(order[i], order[j]) < 0 })

	if sameMarkSpans(hl, order, isLast) {
		return
	}

	spans := make([]*highlight.SpanTemplate, 0, len(order))
	for _, p := range order {
		color := theme.ColorMark
		if isLast[p] {
			color = theme.ColorLastMark
		}
		var t highlight.SpanTemplate = &highlight.Span{
			Start:         p,
			End:           editbuffer.RowsPos{RowIndex: p.RowIndex, ColIndex: p.ColIndex + 1},
			Color:         color,
			ColorIfActive: color,
			Priority:      highlight.LayerMark,
		}
		spans = append(spans, &t)
	}
	hl.SetSpans(spans)
}

func sameMarkSpans(hl *highlight.Highlights, order []editbuffer.RowsPos, isLast map[editbuffer.RowsPos]bool) bool {
	if hl.SpansLength() != len(order) {
		return false
	}
	for i, p := range order {
		sp := hl.GetSpan(i)
		if sp.Start != p || (sp.Color == theme.ColorLastMark) != isLast[p] {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Trailing whitespace and marks, painted over a drawn row
// ---------------------------------------------------------------------------

// paintRowOverlays draws the background of trailing whitespace and of marks
// on the cells of a row that was just drawn. cache is the row's cell cache
// (rune positions → cell, see cacheCellInfo), startScreenY the edit-area row
// of its first logical row (negative when the row is scrolled off at the
// top) and hangingIndentWidth the indent of its wrapped logical rows.
//
// Painting afterwards, from the cache, keeps this independent of the many
// places that draw a cell (wrapping, control codes, ...), and a mark wins
// over trailing whitespace.
func (e *Editorleaf) paintRowOverlays(rowIndex, startScreenY int, cache []locale.Cell, hangingIndentWidth int) {
	if e.mode != ModeEditor || len(cache) == 0 {
		return
	}

	// Trailing whitespace. The last cell is the LF or the EOF.
	if (*e.editBuffer.GetLangMode()).ShowTrailingWhitespace() {
		if start := trailingWhitespaceStart(cache); start >= 0 {
			style := theme.ColorTrailingWhitespaceBackground
			for p := start; p < len(cache)-1; {
				cell := cache[p]
				if cell.Size == 0 {
					p++
					continue
				}
				e.paintCell(startScreenY, hangingIndentWidth, cell, cell.Style.Background(style))
				p += cell.Size
			}
		}
	}

	// Marks, then the cursors of the multicursor mode on top of them
	if e.meta == nil || e.meta.HighlightLayer == nil {
		return
	}
	if hl := e.meta.HighlightLayer.Peek(highlight.LayerMark); hl != nil {
		for i, n := 0, hl.SpansLength(); i < n; i++ {
			sp := hl.GetSpan(i)
			cell, ok := cellAt(cache, sp, rowIndex)
			if !ok {
				continue
			}

			bg := theme.ColorMarkBackground
			if sp.Color == theme.ColorLastMark {
				bg = theme.ColorLastMarkBackground
			}
			e.paintCell(startScreenY, hangingIndentWidth, cell, cell.Style.Background(bg))
		}
	}
	if hl := e.meta.HighlightLayer.Peek(highlight.LayerCursor); hl != nil {
		for i, n := 0, hl.SpansLength(); i < n; i++ {
			cell, ok := cellAt(cache, hl.GetSpan(i), rowIndex)
			if !ok {
				continue
			}
			style := cell.Style.
				Foreground(theme.ColorMultiCursorForeground).
				Background(theme.ColorMultiCursorBackground)
			e.paintCell(startScreenY, hangingIndentWidth, cell, style)
		}
	}
}

// cellAt returns the cell the span starts at, if the span is on the row and
// the position is the start of a character of the row as it is now.
func cellAt(cache []locale.Cell, sp *highlight.Span, rowIndex int) (locale.Cell, bool) {
	if sp.Start.RowIndex != rowIndex {
		return locale.Cell{}, false
	}
	p := sp.Start.ColIndex
	if p < 0 || p >= len(cache) || cache[p].Size == 0 {
		return locale.Cell{}, false // the row is shorter now, or p is not the start of a character
	}
	return cache[p], true
}

// trailingWhitespaceStart returns the byte position where the spaces and
// tabs at the end of the row begin, or -1. The LF/EOF cell at the end of the
// cache is not looked at. Positions without a cell (the continuation bytes
// of a multi-byte character) have Size 0 and are skipped.
func trailingWhitespaceStart(cache []locale.Cell) int {
	start := -1
	for p := 0; p < len(cache)-1; {
		cell := cache[p]
		if cell.Size == 0 {
			p++
			continue
		}
		if cell.Ch == ' ' || locale.Is(cell, locale.TAB) {
			if start < 0 {
				start = p
			}
		} else {
			start = -1
		}
		p += cell.Size
	}
	return start
}

// paintCell draws cell again with style, at the position it was cached for.
func (e *Editorleaf) paintCell(startScreenY, hangingIndentWidth int, cell locale.Cell, style tcell.Style) {
	y := startScreenY + cell.LogicalRowIndex
	x := cell.TotalWidthLogicalRow
	if cell.LogicalRowIndex > 0 {
		x += hangingIndentWidth // cacheCellInfo leaves it out
	}

	if locale.Is(cell, locale.CONTROLCODE) {
		// ^X is drawn as two cells
		e.setCellInEditArea(x, y, style, '^', 1)
		e.setCellInEditArea(x+1, y, style, cell.Ch+64, 1)
		return
	}
	e.setCellInEditArea(x, y, style, cell.Ch, cell.Width)
}
