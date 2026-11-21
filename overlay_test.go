package editorleaf

import (
	"testing"

	"github.com/ge-editor/locale"
)

// cacheOf builds a cell cache like drawLineWithCompute does: one cell per
// character at the byte position of its first byte, none at the
// continuation bytes, and the LF at the end.
func cacheOf(cells ...locale.Cell) []locale.Cell {
	n := 0
	for _, c := range cells {
		n += c.Size
	}
	cache := make([]locale.Cell, n)
	p := 0
	for _, c := range cells {
		cache[p] = c
		p += c.Size
	}
	return cache
}

func TestTrailingWhitespaceStart(t *testing.T) {
	ch := func(r rune, size int) locale.Cell { return locale.Cell{Ch: r, Size: size} }
	space := ch(' ', 1)
	tab := locale.Cell{Ch: '»', Size: 1, Class: locale.TAB}
	lf := ch('¬', 1)

	tests := []struct {
		name  string
		cache []locale.Cell
		want  int
	}{
		{"empty row", cacheOf(lf), -1},
		{"no trailing", cacheOf(ch('a', 1), ch('b', 1), lf), -1},
		{"spaces", cacheOf(ch('a', 1), space, space, lf), 1},
		{"space and tab", cacheOf(ch('a', 1), ch('b', 1), space, tab, lf), 2},
		{"space inside only", cacheOf(ch('a', 1), space, ch('b', 1), lf), -1},
		{"only whitespace", cacheOf(tab, space, lf), 0},
		{"after a multi-byte character", cacheOf(ch('あ', 3), space, lf), 3},
		{"multi-byte after a space", cacheOf(space, ch('あ', 3), lf), -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := trailingWhitespaceStart(tt.cache); got != tt.want {
				t.Errorf("trailingWhitespaceStart = %d, want %d", got, tt.want)
			}
		})
	}
}

// A mark on a tab colors the first column of it, not the width of the tab.
func TestMarkCellWidth(t *testing.T) {
	for _, tc := range []struct {
		name string
		cell locale.Cell
		want int
	}{
		{"tab of 4", locale.Cell{Ch: '»', Size: 1, Width: 4, Class: locale.TAB}, 1},
		{"tab of 8", locale.Cell{Ch: '»', Size: 1, Width: 8, Class: locale.TAB}, 1},
		{"tab of 1 (at the last column of a tab stop)", locale.Cell{Ch: '»', Size: 1, Width: 1, Class: locale.TAB}, 1},
		{"letter", locale.Cell{Ch: 'a', Size: 1, Width: 1}, 1},
		{"wide character", locale.Cell{Ch: 'あ', Size: 3, Width: 2}, 2}, // all of it
		{"control code ^X", locale.Cell{Ch: 1, Size: 1, Width: 2, Class: locale.CONTROLCODE}, 2},
	} {
		if got := markCellWidth(tc.cell); got != tc.want {
			t.Errorf("%s: markCellWidth = %d, want %d", tc.name, got, tc.want)
		}
	}
}
