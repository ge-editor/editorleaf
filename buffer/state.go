// editorleaf/buffer/state.go
//
// Per-file cursor position kept in the application state (state.json).
// It lives in package buffer (not editorleaf) because BufferSets creates
// buffers and must restore the position when a buffer is first created.

package buffer

import (
	"path/filepath"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/gecore/state"
)

// fileState is the value stored under stateKey(path). Keep its JSON shape
// compatible with the entries already written to state.json.
type fileState struct {
	RowsPos editbuffer.RowsPos
}

func stateKey(path string) string {
	absPath, _ := filepath.Abs(path)
	return "editor:" + absPath
}

// SaveState stores the cursor position of the file at path.
func SaveState(path string, pos editbuffer.RowsPos) {
	state.AppState().Save(stateKey(path), fileState{RowsPos: pos})
}

func loadState(path string) (editbuffer.RowsPos, bool) {
	var st fileState
	if !state.AppState().Load(stateKey(path), &st) {
		return editbuffer.RowsPos{}, false
	}
	return st.RowsPos, true
}

// restoreState gives a freshly loaded bufferSet the cursor position saved
// for its file, clamped to the current rows (the file may have changed
// since). It does nothing when no position was saved.
//
// Call it only when the bufferSet is created, never for an already open one.
func (bs *bufferSet) restoreState() {
	pos, ok := loadState(bs.GetPath())
	if !ok {
		return
	}
	m := newMeta()
	m.RowsPos = pos
	m.ClampRowsPos(bs.EditBuffer)
	m.PrevScreenPos.Col = m.RowsPos.ColIndex
	bs.PushMeta(m)
}
