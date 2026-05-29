package buffer

import (
	"errors"
	"io/fs"
	"os"
	"slices"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/utils"
)

// Create editorleaf.Editor.Buffers from
// Files (command line arguments)
// Always one will be created
//
// # Create bufferSet and append to Buffers from path string array
//
// From paths (command line argument)
// create editorleaf.Editor.Buffers
// Create one bufferSet, even if there is none.
func NewBufferSets(paths []string) (bss *BufferSets, errs error) {
	bss = &BufferSets{}

	for _, path := range paths {
		var err error
		var mode fs.FileMode
		buff := newBufferSet(path)

		fileInfo, err := os.Stat(path)
		if os.IsNotExist(err) {
			err = buff.New()
			goto last
		}

		if err != nil {
			goto last
		}

		mode = fileInfo.Mode()
		if mode.IsRegular() {
			err = buff.Load()
			buff.SetReadonly(mode.Perm()&0200 == 0)
		} else {
			continue // such directory or ...
		}

	last:
		if err != nil {
			errs = errors.Join(errs, err)
		} else {
			bss.Append(buff)
		}
	} // for files

	// create one buffer if nothing
	if len(*bss) == 0 {
		buff := newBufferSet("")
		buff.New()
		bss.Append(buff)
	}
	return bss, errs
}

// Buffers
type BufferSets []*bufferSet

// Find a buffer from the bufferSetArray
// If found, return the meta from metas
// If metas does not exist, return a new meta
// If there exists an Editor, you could call "func (e *Editor) GetMeta() *meta {}"
func (bss *BufferSets) GetMeta(ff *editbuffer.EditBuffer) *Meta {
	for _, buffSet := range *bss {
		if buffSet.EditBuffer == ff {
			return buffSet.PopMeta()
		}
	}
	return newMeta()
}

// Find BufferSet in BufferSetArray
func (bss *BufferSets) BufferSet(ff *editbuffer.EditBuffer) *bufferSet {
	for _, buffSet := range *bss {
		if buffSet.EditBuffer == ff {
			return buffSet
		}
	}
	return nil
}

// Find the buffer and meta for filePath from Buffers
// Create a new buffer otherwise
// Load into buffer if the file exists
// Register into Buffers
// Return buffer and meta
func (bss *BufferSets) GetFileAndMeta(filePath string) (*editbuffer.EditBuffer, *Meta, editbuffer.Result, error) {
	result := editbuffer.ResultNone

	for _, buffSet := range *bss {
		if utils.SameFile(filePath, buffSet.GetPath()) {
			return buffSet.EditBuffer, buffSet.PopMeta(), result, nil
		}
	}

	var err error
	buffSet := newBufferSet(filePath)
	if err = buffSet.Load(); err != nil {
		if err = buffSet.New(); err != nil {
			return buffSet.EditBuffer, buffSet.PopMeta(), result, err
		}
		result = editbuffer.ResultNewFile
	} else {
		result = editbuffer.ResultLoadedFile
	}
	bss.Append(buffSet)
	return buffSet.EditBuffer, buffSet.PopMeta(), result, err
}

func (bss *BufferSets) Append(buffSet *bufferSet) {
	*bss = append(*bss, buffSet)
}

/*
func (bss *BufferSets) Remove(filePath string) {
	for i, buffSet := range *bss {
		if utils.SameFile(filePath, buffSet.GetPath()) {
			*bss = append((*bss)[:i], (*bss)[i+1:]...)
			return
		}
	}
}
*/

// Remove bufferSet match *editbuffer.File in BufferSets.
// Return removed index of bufferSet in BufferSets.
// Return -1 if not match.
func (bss *BufferSets) RemoveByBufferFile(ff *editbuffer.EditBuffer) int {
	i := bss.GetIndexByBufferFile(ff)
	if i == -1 {
		return -1
	}

	*bss = slices.Delete(*bss, i, i+1)
	return i
}

func (bss *BufferSets) RemoveByBufferIndex(index int) int {
	*bss = slices.Delete(*bss, index, index+1)
	return index
}

// Check if the specified *editbuffer.File exists in BufferSets
// If found, return its index
// If not found, return -1
func (bss *BufferSets) GetIndexByBufferFile(ff *editbuffer.EditBuffer) int {
	for i, buffSet := range *bss {
		if buffSet.EditBuffer == ff {
			return i
		}
	}
	return -1 // Return -1 if the element is not found
}
