package editorleaf

import (
	"path/filepath"

	"github.com/ge-editor/editorleaf/buffer"
	"github.com/ge-editor/gecore"
	"github.com/ge-editor/gecore/screen"
)

type fileState struct {
	Cursor screen.Cursor
}

func saveState(path string, c screen.Cursor) {
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
