package editorleaf

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ge-editor/editorleaf/editbuffer"
)

// Pure helpers for planning a multi-file edit (a rename). Nothing here
// touches a buffer: they work on rows, so that everything can be checked
// before the first edit is made.

// splitLines splits file content into rows the way a buffer holds them: at
// '\n', with a CR before it removed. There is always at least one row.
func splitLines(data []byte) [][]byte {
	rows := bytes.Split(data, []byte{'\n'})
	for i, r := range rows {
		rows[i] = bytes.TrimRight(r, "\r")
	}
	return rows
}

// rowsOf copies the rows of a buffer.
func rowsOf(eb *editbuffer.EditBuffer) [][]byte {
	rs := eb.Rows()
	if rs == nil || rs.Length() == 0 {
		return [][]byte{{}}
	}
	out := make([][]byte, rs.Length())
	for i, r := range *rs {
		out[i] = bytes.Clone(r)
	}
	return out
}

// sameRows compares two texts row by row, ignoring empty rows at the end
// (whether a file's final newline shows as an empty last row differs between
// a file and a buffer).
func sameRows(a, b [][]byte) bool {
	trim := func(rows [][]byte) [][]byte {
		for len(rows) > 0 && len(rows[len(rows)-1]) == 0 {
			rows = rows[:len(rows)-1]
		}
		return rows
	}
	a, b = trim(a), trim(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !bytes.Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}

// validateEdits checks that edits fit rows: every position inside the text,
// no range reversed, no two ranges overlapping. It returns the edits sorted
// by position (touching ranges are fine).
func validateEdits(rows [][]byte, edits []TextEdit) ([]TextEdit, error) {
	sorted := slices.Clone(edits)
	slices.SortStableFunc(sorted, func(a, b TextEdit) int {
		if c := cmpRowsPos(a.Start, b.Start); c != 0 {
			return c
		}
		return cmpRowsPos(a.End, b.End)
	})

	inside := func(p editbuffer.RowsPos) bool {
		return p.RowIndex >= 0 && p.RowIndex < len(rows) &&
			p.ColIndex >= 0 && p.ColIndex <= len(rows[p.RowIndex])
	}

	for i, ed := range sorted {
		switch {
		case !inside(ed.Start) || !inside(ed.End):
			return nil, fmt.Errorf("edit %d (%d:%d-%d:%d) is outside the text",
				i+1, ed.Start.RowIndex+1, ed.Start.ColIndex, ed.End.RowIndex+1, ed.End.ColIndex)
		case cmpRowsPos(ed.End, ed.Start) < 0:
			return nil, fmt.Errorf("edit %d has its end before its start", i+1)
		case i > 0 && cmpRowsPos(ed.Start, sorted[i-1].End) < 0:
			return nil, fmt.Errorf("edits %d and %d overlap", i, i+1)
		}
	}
	return sorted, nil
}

func joinBytes(parts ...[]byte) []byte {
	var n int
	for _, p := range parts {
		n += len(p)
	}
	out := make([]byte, 0, n)
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// applyToRows returns rows with edits applied. edits must be sorted and
// non-overlapping (validateEdits); rows itself is not modified.
func applyToRows(rows [][]byte, edits []TextEdit) [][]byte {
	out := slices.Clone(rows)

	// Bottom to top, so that the positions of the edits still to do stay valid.
	for i := len(edits) - 1; i >= 0; i-- {
		ed := edits[i]
		before := out[ed.Start.RowIndex][:ed.Start.ColIndex]
		after := out[ed.End.RowIndex][ed.End.ColIndex:]

		parts := bytes.Split([]byte(ed.NewText), []byte{'\n'})
		var replacement [][]byte
		if len(parts) == 1 {
			replacement = [][]byte{joinBytes(before, parts[0], after)}
		} else {
			last := len(parts) - 1
			replacement = append(replacement, joinBytes(before, parts[0]))
			replacement = append(replacement, parts[1:last]...)
			replacement = append(replacement, joinBytes(parts[last], after))
		}
		out = slices.Replace(out, ed.Start.RowIndex, ed.End.RowIndex+1, replacement...)
	}
	return out
}

// previewLines describes the edits to one file, one line per changed row:
//
//	path:42: old text -> new text
//
// A row touched by an edit that spans lines (or inserts a newline) is
// reported as such instead, since its "new text" is not one line.
func previewLines(path string, rows [][]byte, edits []TextEdit) []string {
	singleByRow := map[int][]TextEdit{}
	multi := map[int]bool{}
	var order []int

	note := func(row int) {
		if !slices.Contains(order, row) {
			order = append(order, row)
		}
	}
	for _, ed := range edits {
		row := ed.Start.RowIndex
		note(row)
		if ed.Start.RowIndex != ed.End.RowIndex || strings.Contains(ed.NewText, "\n") {
			multi[row] = true
			continue
		}
		singleByRow[row] = append(singleByRow[row], ed)
	}
	slices.Sort(order)

	lines := make([]string, 0, len(order))
	for _, row := range order {
		if multi[row] {
			lines = append(lines, fmt.Sprintf("%s:%d: (an edit spanning lines)", path, row+1))
			continue
		}
		// Single-line edits do not change the number of rows, so the row
		// keeps its index.
		after := applyToRows(rows, singleByRow[row])[row]
		lines = append(lines, fmt.Sprintf("%s:%d: %s -> %s",
			path, row+1, strings.TrimSpace(string(rows[row])), strings.TrimSpace(string(after))))
	}
	return lines
}

// cursorAfterEdits maps the cursor through edits for a rename: the cursor
// stays on the same thing, which means the start of the edited text if it was
// inside one, shifted by earlier edits on its line (and by lines added above
// it). edits must be sorted and non-overlapping.
func cursorAfterEdits(pos editbuffer.RowsPos, edits []TextEdit) editbuffer.RowsPos {
	return mapCursor(pos, edits, func(ed TextEdit, _ editbuffer.RowsPos) (editbuffer.RowsPos, int) {
		return ed.Start, 0
	})
}

// cursorAfterFormat maps the cursor through formatting edits: the same as a
// rename, except that a cursor inside a one-line edit keeps its distance from
// the start of the edit (as far as the new text reaches), so that reindenting
// a line does not throw the cursor to its start.
func cursorAfterFormat(pos editbuffer.RowsPos, edits []TextEdit) editbuffer.RowsPos {
	return mapCursor(pos, edits, func(ed TextEdit, pos editbuffer.RowsPos) (editbuffer.RowsPos, int) {
		if ed.Start.RowIndex == ed.End.RowIndex && ed.Start.RowIndex == pos.RowIndex && !strings.Contains(ed.NewText, "\n") {
			return ed.Start, min(pos.ColIndex-ed.Start.ColIndex, len(ed.NewText))
		}
		return ed.Start, 0
	})
}

// mapCursor moves pos through edits (sorted, non-overlapping). inside decides
// where a cursor that lies within an edit goes: an anchor in the old text and
// a number of bytes to add after the edits before it have been accounted for.
func mapCursor(pos editbuffer.RowsPos, edits []TextEdit,
	inside func(ed TextEdit, pos editbuffer.RowsPos) (anchor editbuffer.RowsPos, extra int)) editbuffer.RowsPos {

	anchor, extra := pos, 0
	containing := -1
	for i, ed := range edits {
		if cmpRowsPos(ed.Start, pos) <= 0 && cmpRowsPos(pos, ed.End) <= 0 {
			anchor, extra = inside(ed, pos)
			containing = i
			break
		}
	}

	out := anchor
	for i, ed := range edits {
		if i == containing || cmpRowsPos(ed.End, anchor) > 0 {
			continue // not entirely before the anchor
		}

		oldLines := ed.End.RowIndex - ed.Start.RowIndex
		newLines := strings.Count(ed.NewText, "\n")
		out.RowIndex += newLines - oldLines

		// An edit that ends on the anchor's row moves it along that row.
		if ed.End.RowIndex == anchor.RowIndex {
			if newLines == 0 {
				out.ColIndex += len(ed.NewText) - (ed.End.ColIndex - ed.Start.ColIndex)
				if oldLines != 0 {
					out.ColIndex = len(ed.NewText) + (anchor.ColIndex - ed.End.ColIndex) + ed.Start.ColIndex
				}
			} else {
				lastLine := ed.NewText[strings.LastIndex(ed.NewText, "\n")+1:]
				out.ColIndex = len(lastLine) + (anchor.ColIndex - ed.End.ColIndex)
			}
		}
	}
	out.ColIndex += extra
	return out
}

// shortPath shows path relative to the working directory when it lies below
// it.
func shortPath(path string) string {
	if wd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(wd, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return rel
		}
	}
	return path
}
