package editorleaf

import (
	"testing"

	"github.com/gdamore/tcell/v3"

	"github.com/ge-editor/theme"
)

// The area beyond the recommended column width has its own background, but
// it must not hide the background of a search result.
func TestOverflowBackground(t *testing.T) {
	over := theme.ColorColumnLimitOverflowBackground

	// A default background (with or without a syntax color) becomes the
	// overflow one.
	for _, c := range []struct {
		name  string
		style tcell.Style
	}{
		{"default", theme.ColorDefault},
		{"syntax color", theme.CodeColors["comment"]},
		{"tab", theme.ColorTab},
	} {
		if got := overflowBackground(c.style); got != c.style.Background(over) {
			t.Errorf("%s: the overflow background was not set", c.name)
		}
	}

	// A background of its own is kept.
	for _, c := range []struct {
		name  string
		style tcell.Style
	}{
		{"search result", theme.ColorSearchFound},
		{"search result on the cursor", theme.ColorSearchFoundOnCursor},
		{"mark", theme.ColorMark},
		{"last mark", theme.ColorLastMark},
	} {
		if got := overflowBackground(c.style); got != c.style {
			t.Errorf("%s: its background was replaced", c.name)
		}
	}
}

// Applying it twice changes nothing more: the style that has the overflow
// background already is not default any more, and is the one that cursorLineStyle
// turns into the cursor-line overflow background.
func TestOverflowBackgroundTwice(t *testing.T) {
	once := overflowBackground(theme.ColorDefault)
	if twice := overflowBackground(once); twice != once {
		t.Fatal("not stable")
	}
}
