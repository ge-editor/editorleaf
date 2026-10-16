package editorleaf

import (
	"path/filepath"

	"github.com/ge-editor/editorleaf/buffer"
	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/gecore/state"
)

type fileState struct {
	RowsPos editbuffer.RowsPos
}

func saveState(path string, c editbuffer.RowsPos) {
	absPath, _ := filepath.Abs(path)
	state.AppState().Save(
		"editor:"+absPath,
		fileState{
			RowsPos: c,
		},
	)
}

func loadState(path string) *buffer.Meta {
	var st fileState

	absPath, _ := filepath.Abs(path)
	if state.AppState().Load(
		"editor:"+absPath,
		&st,
	) {
		return &buffer.Meta{
			RowsPos: st.RowsPos,
		}
	}
	return &buffer.Meta{}
}
