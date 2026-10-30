package editorleaf

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/gecore/tree"
)

type workspaceEditorState struct {
	Path   string             `json:"path"`
	Cursor editbuffer.RowsPos `json:"cursor"`
}

var _ tree.WorkspaceLeafData = (*Editorleaf)(nil)

// MarshalWorkspaceData saves the editor buffer path and cursor position.
//
// Only buffers backed by an existing file are recorded. Buffers such as
// "*stdin*" or "unnamed" (and files that were never saved) get an empty
// path, so restoring the workspace does not turn them into new files.
func (e *Editorleaf) MarshalWorkspaceData() (json.RawMessage, error) {
	path := e.editBuffer.GetPath()
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		return json.Marshal(workspaceEditorState{})
	}
	return json.Marshal(workspaceEditorState{Path: path, Cursor: e.meta.RowsPos})
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
	// A file that has been deleted since the workspace was saved is skipped
	// (the leaf keeps its initial buffer) instead of becoming a new empty
	// file buffer.
	if info, err := os.Stat(state.Path); err != nil || !info.Mode().IsRegular() {
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
