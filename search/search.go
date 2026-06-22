// editorleaf/search/search.go
package search

import "github.com/ge-editor/gecore/screen"

type SearchStruct struct {
	CurrentSearchIndex int
	Indexes            []FoundPosition
	// Ctx                context.Context
	// Cancel             context.CancelFunc
}

func NewSearch() *SearchStruct {
	return &SearchStruct{
		CurrentSearchIndex: 0,
		Indexes:            make([]FoundPosition, 0, 256),
		// Ctx:                nil,
		// Cancel:             nil,
	}
}

// Position found in search results
type FoundPosition struct {
	Start screen.Cursor
	Stop  screen.Cursor
}

///////////////////
// とりあえず移動してきた

func NewFoundPosition(startRowIndex, startColIndex, stopRowIndex, stopColIndex int) FoundPosition {
	return FoundPosition{
		Start: screen.Cursor{
			RowIndex: startRowIndex,
			ColIndex: startColIndex,
		},
		Stop: screen.Cursor{
			RowIndex: stopRowIndex,
			ColIndex: stopColIndex,
		},
	}
}

// Returns the first index of the found search position that matches the row index.
// Return -1, not found match position.
func (ss *SearchStruct) GetFoundPosition(rowIndex int) int {
	for i := 0; i < len(ss.Indexes); i++ {
		if ss.Indexes[i].Start.RowIndex >= rowIndex {
			return i
		}
	}
	return -1
}

func (ss *SearchStruct) GetFindIndexes() []FoundPosition {
	return ss.Indexes
}
