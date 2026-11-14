// editorleaf/buffer/buffer_set.go
// bufferSet
// - Holds data and information of a file
// - Stores information necessary for editing the file, such as cursor position
//   These are kept together as a set.

package buffer

import "github.com/ge-editor/editorleaf/editbuffer"

func newBufferSet(filePath string) *bufferSet {
	file := editbuffer.NewFile(filePath)
	return &bufferSet{
		EditBuffer: file,
		metas:      make([]*Meta, 0, 2),
	}
}

type bufferSet struct {
	*editbuffer.EditBuffer
	// metas is a stack of the views (cursor position, ...) that were left
	// when a leaf switched to another buffer. The last one was left most
	// recently, so PopMeta gives the most recently used view first.
	metas []*Meta
}

// GetMetas returns the stacked metas, the most recently left one last.
func (bs *bufferSet) GetMetas() []*Meta {
	return bs.metas
}

// PushMeta stacks m as the most recently used meta.
func (bs *bufferSet) PushMeta(m *Meta) {
	bs.metas = append(bs.metas, m)
}

// PopMeta takes the most recently pushed meta (the view that was left last).
// With no meta it returns a new one.
func (bs *bufferSet) PopMeta() *Meta {
	if len(bs.metas) > 0 {
		lastMeta := bs.metas[len(bs.metas)-1]
		bs.metas = bs.metas[:len(bs.metas)-1]
		return lastMeta
	}
	return newMeta()
}
