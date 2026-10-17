package mark

import (
	"encoding/json"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/gecore/marks"
	"github.com/ge-editor/gecore/tree"
	"github.com/ge-editor/gecore/workspace"
)

func NewMark(path string, current editbuffer.RowsPos, content string) *Mark {
	var wsName, trName string
	ws := workspace.Current()
	if ws != nil {
		wsName = ws.Name
	}
	tr := tree.CurrentRootTree()
	if tr != nil {
		trName = tr.Name
	}

	return &Mark{
		Label:         content,
		WorkspaceName: wsName,
		RootTreeName:  trName,
		ExtraData: &ExtraData{
			FilePath: path,
			RowsPos:  current,
		},
	}
}

// implements gecore.marks.Mark interface
type Mark struct {
	id                string
	Label             string
	WorkspaceName     string
	RootTreeName      string
	WorkspaceFilePath string

	*ExtraData
}

func (m *Mark) SetID(id string) {
	m.id = id
}

func (m *Mark) GetID() string {
	return m.id
}

func (m *Mark) UpdateExtraData() {}

func (m *Mark) GetStoredMark() (marks.StoredMark, error) {
	if m == nil {
		return marks.StoredMark{}, nil
	}

	var extraData json.RawMessage
	if m.ExtraData != nil {
		b, err := json.Marshal(m.ExtraData)
		if err != nil {
			return marks.StoredMark{}, err
		}
		extraData = b
	}

	return marks.StoredMark{
		ID:                m.id,
		Label:             m.Label,
		WorkspaceName:     m.WorkspaceName,
		RootTreeName:      m.RootTreeName,
		WorkspaceFilePath: m.WorkspaceFilePath,
		ExtraData:         extraData,
	}, nil
}

// ExtraData contains editor-specific data persisted with a mark.
type ExtraData struct {
	FilePath string // *editbuffer.GetPath()
	editbuffer.RowsPos
}

func Equals(a, b *Mark) bool {
	// gelog.Debug("mark Equals", "m", *a, "a", *b)

	if a.WorkspaceName != b.WorkspaceName ||
		a.RootTreeName != b.RootTreeName ||
		a.WorkspaceFilePath != b.WorkspaceFilePath ||
		!a.RowsPos.Equals(b.RowsPos) {
		return false
	}
	return true
}

// AdjustForInsertion updates the mark position after text insertion.
// insertStart: insertion start position
// insertEnd: insertion end position after insertion
func (m *Mark) AdjustForInsertion(insertStart, insertEnd editbuffer.RowsPos) {
	// If the mark is located before the insertion row,
	// its position is unaffected.
	if m == nil || m.RowIndex < insertStart.RowIndex {
		return
	}

	// Number of rows added by the insertion.
	rowOffset := insertEnd.RowIndex - insertStart.RowIndex

	// Mark is on the same row as insertion start.
	if m.RowIndex == insertStart.RowIndex {
		// Only adjust when insertion occurs before or at the mark position.
		if m.ColIndex >= insertStart.ColIndex {

			// Single-line insertion:
			// shift only the column position.
			if rowOffset == 0 {
				m.ColIndex += insertEnd.ColIndex - insertStart.ColIndex
			} else {
				// Multi-line insertion:
				// move the mark to the corresponding position
				// in the inserted end row.
				m.ColIndex = m.ColIndex - insertStart.ColIndex + insertEnd.ColIndex
				m.RowIndex += rowOffset
			}
		}
		return
	}

	// Mark is below the insertion area:
	// shift downward by the inserted row count.
	m.RowIndex += rowOffset
}

// AdjustForDeletion updates the mark position after text deletion.
// deleteStart: deletion start position
// deleteEnd: deletion end position
func (m *Mark) AdjustForDeletion(deleteStart, deleteEnd editbuffer.RowsPos) {
	// If the mark is before the deletion start row,
	// its position is unaffected.
	if m == nil || m.RowIndex < deleteStart.RowIndex {
		return
	}

	// Mark is on the same row as deletion start.
	if m.RowIndex == deleteStart.RowIndex {

		// If the mark is at or before the deletion start,
		// no adjustment is needed.
		if m.ColIndex <= deleteStart.ColIndex {
			return
		}

		// Otherwise, move the mark to the deletion start.
		m.ColIndex = deleteStart.ColIndex
		return
	}

	// Mark is below the deleted range:
	// shift upward by the number of removed rows.
	if m.RowIndex > deleteEnd.RowIndex {
		m.RowIndex -= deleteEnd.RowIndex - deleteStart.RowIndex
		return
	}

	// Mark is inside the deleted range:
	// move it to the deletion start position.
	m.RowIndex = deleteStart.RowIndex
	m.ColIndex = deleteStart.ColIndex
}

// IsInDeletionRange reports whether the mark position is inside the deleted
// range. The start is inclusive and the end is exclusive.
func (m *Mark) IsInDeletionRange(deleteStart, deleteEnd editbuffer.RowsPos) bool {
	if m == nil {
		return false
	}
	return (m.RowIndex > deleteStart.RowIndex ||
		(m.RowIndex == deleteStart.RowIndex && m.ColIndex > deleteStart.ColIndex)) &&
		(m.RowIndex < deleteEnd.RowIndex ||
			(m.RowIndex == deleteEnd.RowIndex && m.ColIndex < deleteEnd.ColIndex))
}
