package buffer

import (
	"bytes"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/utils"
)

// Find returns the edit buffer that is open for path, or nil. Unlike
// GetFileAndMeta it has no side effects (no load, no Meta taken).
func (bss *BufferSets) Find(path string) *editbuffer.EditBuffer {
	for _, bs := range *bss {
		if utils.IsSameFile(path, bs.GetPath()) {
			return bs.EditBuffer
		}
	}
	return nil
}

// Ensure opens path without showing it: the file is loaded, or an empty new
// buffer is made when it does not exist (like GetFileAndMeta). Nothing
// happens when the buffer is already open. Call it before SetStartPosition
// for a file that may not be open yet.
func (bss *BufferSets) Ensure(path string) error {
	if bss.Find(path) != nil {
		return nil
	}
	bs := newBufferSet(path)
	if err := bs.Load(); err != nil {
		if err := bs.New(); err != nil {
			return err
		}
	} else {
		bs.restoreState()
	}
	bss.Append(bs)
	return nil
}

// NewScratch adds a buffer that has no file, named name and holding data.
// Show it with OpenFile(name).
func (bss *BufferSets) NewScratch(name string, data []byte) error {
	bs := newBufferSet(name)
	if err := bs.LoadFrom(bytes.NewReader(data)); err != nil {
		return err
	}
	bss.Append(bs)
	return nil
}
