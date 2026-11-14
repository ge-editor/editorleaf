package editorleaf

import (
	"github.com/gdamore/tcell/v3"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/editorleaf/highlight"
	"github.com/ge-editor/editorleaf/search"
)

func NewFoundPosition(startRowIndex, startColIndex, stopRowIndex, stopColIndex int, color, ColorOnCursor tcell.Style) *search.SearchResult {
	return &search.SearchResult{
		Span: &highlight.Span{
			Start: editbuffer.RowsPos{
				RowIndex: startRowIndex,
				ColIndex: startColIndex,
			},
			End: editbuffer.RowsPos{
				RowIndex: stopRowIndex,
				ColIndex: stopColIndex,
			},
			Color:         color,
			ColorIfActive: ColorOnCursor,
			Priority:      highlight.LayerSearch,
		},
		// Matches: ,
	}
}

// -----------------------------------------------
// HighlightsLayer の情報から描画色を決定する
// -----------------------------------------------

// highlightPosStatus [editor/meta][priority]spansIndex
type highlightPosStatus [][]int

// layer
const (
	highlightEditor = iota
	highlightMeta
)

func (s *highlightPosStatus) Set(layer, priority, spansIndex int) {
	for len(*s) <= layer {
		*s = append(*s, nil)
	}

	for len((*s)[layer]) <= priority {
		(*s)[layer] = append((*s)[layer], -1)
	}

	(*s)[layer][priority] = spansIndex
}

func (s highlightPosStatus) Get(layer, priority int) int {
	if layer < 0 || layer >= len(s) {
		return -1
	}
	if priority < 0 || priority >= len(s[layer]) {
		return -1
	}

	return s[layer][priority]
}

func (s *highlightPosStatus) Clear() {
	*s = nil
}
