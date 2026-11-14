package editorleaf

import (
	"slices"
	"testing"

	"github.com/ge-editor/editorleaf/editbuffer"
)

func TestNormalizeCursorPositions(t *testing.T) {
	p := func(r, c int) editbuffer.RowsPos { return editbuffer.RowsPos{RowIndex: r, ColIndex: c} }

	in := []editbuffer.RowsPos{p(3, 1), p(1, 5), p(1, 2), p(3, 1), p(1, 2)}
	want := []editbuffer.RowsPos{p(1, 2), p(1, 5), p(3, 1)}

	if got := normalizeCursorPositions(in); !slices.Equal(got, want) {
		t.Errorf("normalizeCursorPositions = %v, want %v", got, want)
	}
	if got := normalizeCursorPositions(nil); len(got) != 0 {
		t.Errorf("normalizeCursorPositions(nil) = %v, want empty", got)
	}
}
