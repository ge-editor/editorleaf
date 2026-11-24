package editorleaf

import (
	"testing"

	"github.com/gdamore/tcell/v3"
)

func TestGutterCellRune(t *testing.T) {
	red := tcell.NewRGBColor(200, 0, 0)
	blue := tcell.NewRGBColor(0, 0, 200)
	bg := tcell.NewRGBColor(40, 40, 40)

	cases := []struct {
		name        string
		top, bottom tcell.Color
		want        rune
	}{
		{"no mark", tcell.ColorDefault, tcell.ColorDefault, ' '},
		{"upper mark", red, tcell.ColorDefault, '▀'},
		{"lower mark", tcell.ColorDefault, blue, '▀'},
		{"both marks", red, blue, '▀'},
	}
	for _, c := range cases {
		if got, _ := gutterCell(c.top, c.bottom, bg); got != c.want {
			t.Errorf("%s: rune %q, want %q", c.name, got, c.want)
		}
	}
}
