package editorleaf

import (
	"context"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v3"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/editorleaf/syntax"
	"github.com/ge-editor/gecore"
	"github.com/ge-editor/gecore/screen"
)

// Sticky Scroll: while the cursor is inside a function, a type, a Markdown
// section, ..., the first row of that region is kept at the top of the view
// when it has scrolled out of it.
//
// It is an overlay: the top rows of the edit area are painted over after the
// normal draw (drawSticky is the one call drawEditorleaf makes). Nothing of
// the layout, the scrolling or the cursor handling changes, and the covered
// rows are never the cursor's: at most as many rows are covered as there are
// screen rows above the cursor.
//
// What is a region comes from the parser of the buffer (syntax.Scope, see
// syntax.OutlineProvider). The rows are drawn by drawLine, so tabs, wide
// characters, highlighting and the line number are the usual ones; a row that
// wraps shows only its first screen row.

// StickyScrollMax is the most rows Sticky Scroll takes. 0 turns it off.
var StickyScrollMax = defaultStickyScrollMax

const defaultStickyScrollMax = 3

// stickyBackground is the background of the rows of Sticky Scroll. (Move it to
// package theme with the other colours.)
var stickyBackground = tcell.NewRGBColor(34, 38, 50)

// stickyState is what one Editorleaf remembers between draws: the chain of
// header rows around the cursor, which only changes with the outline or the
// cursor row.
type stickyState struct {
	valid bool
	eb    *editbuffer.EditBuffer
	ver   uint64
	row   int
	chain []int
}

// drawSticky paints the Sticky Scroll rows over the top of the edit area. Call
// it at the end of a draw of an Editorleaf in ModeEditor.
func (e *Editorleaf) drawSticky(ctx context.Context) {
	if e.mode != ModeEditor || e.editBuffer == nil || e.meta == nil || StickyScrollMax <= 0 {
		return
	}
	rows := e.stickyRows()
	if len(rows) == 0 {
		return
	}

	area, cursorLine := e.editArea, e.cursorLineRow1
	e.stickyDrawing, e.cursorLineRow1 = true, 0
	defer func() {
		e.editArea, e.cursorLineRow1, e.stickyDrawing = area, cursorLine, false
	}()

	for i, h := range rows {
		if h < 0 || h >= e.RowsLength() {
			continue
		}
		e.editArea = area
		if e.bsArray.NeedsCompute(h) {
			if _, canceled := e.drawLineWithCompute(ctx, 0, h, -1, false); canceled {
				return
			}
		}
		// A one row edit area: drawLine stops after the first screen row of
		// the row, and everything it paints lands on screen row i.
		e.editArea = screen.Rect{X: area.X, Y: area.Y + i, Width: area.Width, Height: 1}
		if _, canceled := e.drawLine(ctx, 0, h, -1, true); canceled {
			return
		}
	}
}

// stickyRows returns the rows (buffer row indexes, outer first) to show, as
// they are on the screen right now.
//
// A header is covered or off screen when its first screen row is above row k
// of the edit area, where k is the number of rows shown; so k is the fixed
// point of k = #{headers above row k}. The count only grows with k, so
// starting from 0 reaches it in a few steps. If there are more than the limit
// the innermost ones (the ones nearest to the cursor) are kept.
func (e *Editorleaf) stickyRows() []int {
	limit := StickyScrollMax
	if cy := e.meta.ScreenPos.Row; cy < limit {
		limit = cy // never cover the cursor's screen row
	}
	if limit <= 0 {
		return nil
	}

	scopes, ver := syntax.OutlineOf(e.editBuffer)
	if len(scopes) == 0 {
		return nil
	}
	chain := e.stickyChain(scopes, ver, e.meta.RowsPos.RowIndex)
	if len(chain) == 0 {
		return nil
	}

	top, y0, ok := e.stickyViewTop()
	if !ok {
		return nil
	}

	// ys[i]: screen row (relative to the edit area) of the first screen row of
	// chain[i]; -1 for a header above the first row drawn.
	ys := make([]int, len(chain))
	r, y := top, y0
	for i, h := range chain {
		if h < top {
			ys[i] = -1
			continue
		}
		for ; r < h; r++ {
			if e.bsArray.NeedsCompute(r) {
				return nil
			}
			y += e.bsArray.BoundariesLen(r)
		}
		ys[i] = y
	}

	k := 0
	for {
		n := 0
		for _, y := range ys {
			if y < k {
				n++
			}
		}
		if n > limit {
			n = limit
		}
		if n <= k {
			break
		}
		k = n
	}
	if k == 0 {
		return nil
	}

	shown := make([]int, 0, k)
	for i, y := range ys {
		if y < k {
			shown = append(shown, chain[i])
		}
	}
	if len(shown) > k {
		shown = shown[len(shown)-k:]
	}
	return shown
}

// stickyChain returns the header rows of the scopes that contain row, outer
// first. The scopes are sorted by HeaderRow (see syntax.OutlineProvider).
func (e *Editorleaf) stickyChain(scopes []syntax.Scope, ver uint64, row int) []int {
	st := &e.sticky
	if st.valid && st.eb == e.editBuffer && st.ver == ver && st.row == row {
		return st.chain
	}

	chain := st.chain[:0]
	for _, s := range scopes {
		if s.HeaderRow > row {
			break
		}
		if s.EndRow >= row {
			chain = append(chain, s.HeaderRow)
		}
	}
	*st = stickyState{valid: true, eb: e.editBuffer, ver: ver, row: row, chain: chain}
	return chain
}

// stickyViewTop finds the row at the top of the edit area in the draw that
// has just been done, and the screen row of its first screen row (0 when it
// starts exactly at the top, negative when its first screen rows are cut
// off). It walks up from the cursor row over rows that this draw has laid out.
func (e *Editorleaf) stickyViewTop() (top, y0 int, ok bool) {
	row := e.meta.RowsPos.RowIndex
	if e.bsArray.NeedsCompute(row) {
		return 0, 0, false
	}
	_, lcy, err := e.bsArray.CursorPositionOnScreenLogicalRow(row, e.meta.RowsPos.ColIndex)
	if err != nil {
		return 0, 0, false
	}

	y := e.meta.ScreenPos.Row - lcy
	for y > 0 && row > 0 {
		row--
		if e.bsArray.NeedsCompute(row) {
			return 0, 0, false
		}
		y -= e.bsArray.BoundariesLen(row)
	}
	return row, y, true
}

// stickyCommand is the ESC x "sticky-scroll" command.
//
//	ESC x sticky-scroll        turn it on or off
//	ESC x sticky-scroll N      show at most N rows (0: off)
func (e *Editorleaf) stickyCommand() *Command {
	cmd := &Command{
		CommandName:        "sticky-scroll",
		CommandDescription: "Turn Sticky Scroll on or off, or set the number of rows",
	}
	cmd.Run = func(ctx context.Context, args []string) (bool, error) {
		arg := strings.TrimSpace(commandText(args, cmd.CommandName))
		switch {
		case arg == "":
			if StickyScrollMax > 0 {
				StickyScrollMax = 0
			} else {
				StickyScrollMax = defaultStickyScrollMax
			}
		default:
			n, err := strconv.Atoi(arg)
			if err != nil || n < 0 {
				gecore.Echo.AddText("Usage: sticky-scroll [ROWS]")
				return true, nil
			}
			StickyScrollMax = n
		}
		if StickyScrollMax == 0 {
			gecore.Echo.AddText("Sticky Scroll: off")
		} else {
			gecore.Echo.AddText("Sticky Scroll: " + strconv.Itoa(StickyScrollMax) + " rows")
		}
		return true, nil
	}
	return cmd
}
