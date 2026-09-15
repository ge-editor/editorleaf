package editorleaf

import (
	"path/filepath"

	"github.com/ge-editor/editorleaf/buffer"
	"github.com/ge-editor/editorleaf/editbuffer/rows"
	"github.com/ge-editor/gecore"
)

type fileState struct {
	RowsPos rows.RowsPos
}

func saveState(path string, c rows.RowsPos) {
	absPath, _ := filepath.Abs(path)
	gecore.AppState().Save(
		"editor:"+absPath,
		fileState{
			RowsPos: c,
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
			RowsPos: st.RowsPos,
		}
	}
	return &buffer.Meta{}
}
