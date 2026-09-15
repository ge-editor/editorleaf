package buffer

import (
	"github.com/ge-editor/editorleaf/editbuffer/rows"
	"github.com/ge-editor/editorleaf/highlight"
	"github.com/ge-editor/editorleaf/mark"
	"github.com/ge-editor/editorleaf/search"
	"github.com/ge-editor/gecore/screen"
)

func newMeta() *Meta {
	return &Meta{
		RowsPos: rows.RowsPos{
			RowIndex: 0,
			ColIndex: 0,
		},

		ScreenPos: screen.Cursor{
			Row: 0,
			Col: 0,
		},
		PrevScreenPos: screen.Cursor{
			Row: 0,
			Col: 0,
		},

		LogicalPos: screen.Cursor{
			Row: 0,
			Col: 0,
		},
		PrevLogicalPos: screen.Cursor{
			Row: 0,
			Col: 0,
		},

		// PrevCx:              0, // Horizontal position of the cursor when vertically moving the cursor
		// PrevDrawnY:          0, // Up to which line number was drawn
		// PrevRowIndex:        0, // When the logical number of lines increases
		// PrevNumberOfLogical: 0, // When the logical number of lines increases
		// PrevLogicalCY:       0, // When the logical number of lines increases

		ModelineCx: 0, // Number of columns to display
		Mark:       nil,

		Search:         &search.SearchResults{},
		HighlightLayer: &highlight.HighlightsLayer{},
	}
}

type Meta struct {
	RowsPos rows.RowsPos

	ScreenPos     screen.Cursor
	PrevScreenPos screen.Cursor // Horizontal position of the cursor when vertically moving the cursor

	LogicalPos     screen.Cursor
	PrevLogicalPos screen.Cursor

	// PrevCx              int                  // Horizontal position of the cursor when vertically moving the cursor
	// PrevDrawnY          int // Up to which line number was drawn
	// PrevRowIndex        int // When the logical number of lines increases
	// PrevNumberOfLogical int // When the logical number of lines increases
	//PrevLogicalCY       int                  // When the logical number of lines increases

	ModelineCx int        // Number of columns to display in the modeline
	Mark       *mark.Mark // ※ 参照型

	StartDrawRowIndex     int
	StartDrawLogicalIndex int
	EndDrawRowIndex       int

	Search         *search.SearchResults
	HighlightLayer *highlight.HighlightsLayer
}

func (m *Meta) DeepCopy() *Meta {
	if m == nil {
		return nil
	}

	nm := *m // 構造体の値コピー（ここが重要）

	// --- ポインタフィールドだけ個別処理 ---
	if m.Mark != nil {
		markCopy := *m.Mark
		nm.Mark = &markCopy
	} else {
		nm.Mark = nil
	}

	/*
		if m.Search != nil {
			searchCopy := *m.Search
			nm.Search = &searchCopy
		} else {
			nm.Search = nil
		}
	*/

	return &nm
}
