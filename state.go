package editorleaf

import (
	"os"

	"github.com/ge-editor/editorleaf/buffer"
	"github.com/ge-editor/editorleaf/editbuffer"
)

// saveState stores the cursor position of path.
//
// Restoring is done by buffer.BufferSets when it creates a buffer
// (buffer/state.go), because package buffer cannot import editorleaf.
func saveState(path string, c editbuffer.RowsPos) {
	buffer.SaveState(path, c)
}

// saveStateIfFile is saveState for buffers backed by an existing regular
// file only (not "unnamed", "*stdin*" or a file never saved).
func saveStateIfFile(path string, c editbuffer.RowsPos) {
	if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
		saveState(path, c)
	}
}
