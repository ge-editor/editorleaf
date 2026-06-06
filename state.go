package editorleaf

import (
	"path/filepath"

	"github.com/ge-editor/editorleaf/buffer"
	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/gecore"
)

type fileState struct {
	Cursor editbuffer.Cursor
}

func saveState(path string, c editbuffer.Cursor) {
	absPath, _ := filepath.Abs(path)
	gecore.AppState().Save(
		"editor:"+absPath,
		fileState{
			Cursor: c,
		},
	)
}

func loadState(path string) *buffer.Meta {
	var st fileState

	absPath, _ := filepath.Abs(path)
	if gecore.AppState().Load(
		"editor:"+absPath,
		&st,
	) {
		return &buffer.Meta{
			Cursor: st.Cursor,
		}
	}
	return &buffer.Meta{}
}
