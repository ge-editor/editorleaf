package editorleaf

import (
	"encoding/json"
	"fmt"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/gecore/tree"
)

type workspaceEditorState struct {
	Path   string             `json:"path"`
	Cursor editbuffer.RowsPos `json:"cursor"`
}

var _ tree.WorkspaceLeafData = (*Editorleaf)(nil)

// MarshalWorkspaceData saves the editor buffer path and cursor position.
func (e *Editorleaf) MarshalWorkspaceData() (json.RawMessage, error) {
	return json.Marshal(workspaceEditorState{Path: e.editBuffer.GetPath(), Cursor: e.meta.RowsPos})
}

// UnmarshalWorkspaceData restores the editor buffer path and cursor position.
func (e *Editorleaf) UnmarshalWorkspaceData(data json.RawMessage) error {
	var state workspaceEditorState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	if state.Path == "" {
		return nil
	}
	if BufferSets == nil {
		return fmt.Errorf("editorleaf: buffer sets are not initialized")
	}
	eb, meta, _, err := BufferSets.GetFileAndMeta(state.Path)
	if err != nil {
		return err
	}
	e.setBuffer(eb, meta)
	e.meta.RowsPos = state.Cursor
	e.meta.ClampRowsPos(eb)
	return nil
}
