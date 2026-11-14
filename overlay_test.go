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
