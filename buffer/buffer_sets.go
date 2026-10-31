package buffer

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"slices"
	"unicode/utf8"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/editorleaf/syntax"
	"github.com/ge-editor/gecore/screen"
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
	return NewBufferSetsFromStdin(paths, "", nil)
}

// NewBufferSetsFromStdin is NewBufferSets with one more buffer, named
// stdinName and holding stdin, placed first (so it is the one shown at start
// up). The buffer is not backed by a file. stdinName == "" means no stdin
// buffer.
func NewBufferSetsFromStdin(paths []string, stdinName string, stdin []byte) (bss *BufferSets, errs error) {
	bss = &BufferSets{}

	if stdinName != "" {
		buff := newBufferSet(stdinName)
		if err := buff.LoadFrom(bytes.NewReader(stdin)); err != nil {
			errs = errors.Join(errs, err)
		} else {
			bss.Append(buff)
		}
	}

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
			if err == nil {
				// Saved position; "+LINE[:COL]" (SetStartPosition) overrides it.
				buff.restoreState()
			}
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

// SetStartPosition sets where the cursor starts when the buffer of path is
// first shown (command line "+LINE[:COLUMN] path"). line and col are 1-based,
// col counts characters and 0 means column 1; out of range values are
// clamped. Call it before an Editorleaf takes the buffer's Meta.
func (bss *BufferSets) SetStartPosition(path string, line, col int) {
	for _, buffSet := range *bss {
		if utils.IsSameFile(path, buffSet.GetPath()) {
			buffSet.setStartPosition(line, col)
			return
		}
	}
}

// SetStartPositionByBase is SetStartPosition for the buffer whose base name
// is name (the buffer made from stdin, which has no file).
func (bss *BufferSets) SetStartPositionByBase(name string, line, col int) {
	for _, buffSet := range *bss {
		if buffSet.GetBase() == name {
			buffSet.setStartPosition(line, col)
			return
		}
	}
}

func (bs *bufferSet) setStartPosition(line, col int) {
	rows := bs.Rows()
	if line < 1 || rows == nil || rows.Length() == 0 {
		return
	}
	rowIndex := min(line, rows.Length()) - 1

	// Column: the (col-1)th character of the row, as a byte index.
	row := rows.Row(rowIndex).Bytes()
	colIndex := 0
	for n := col - 1; n > 0 && colIndex < len(row); n-- {
		_, size := utf8.DecodeRune(row[colIndex:])
		colIndex += size
	}

	m := bs.PopMeta()
	m.RowsPos = editbuffer.RowsPos{RowIndex: rowIndex, ColIndex: colIndex}
	m.PrevScreenPos.Col = colIndex
	if s := screen.Get(); s != nil {
		m.ScreenPos.Row = (s.Height - 1) / 2 // like MoveCursorGoToLine: centered
	}
	bs.PushMeta(m)
}

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
		if utils.IsSameFile(filePath, buffSet.GetPath()) {
			meta := buffSet.PopMeta()
			meta.ClampRowsPos(buffSet.EditBuffer)
			return buffSet.EditBuffer, meta, result, nil
		}
	}

	buffSet := newBufferSet(filePath)
	err := buffSet.Load()
	if err != nil {
		if err = buffSet.New(); err != nil {
			return buffSet.EditBuffer, buffSet.PopMeta(), result, err
		}
		result = editbuffer.ResultNewFile
	} else {
		result = editbuffer.ResultLoadedFile
		// A workspace restore sets its own cursor after this call,
		// so the workspace position wins over the saved one.
		buffSet.restoreState()
	}
	bss.Append(buffSet)

	meta := buffSet.PopMeta()
	meta.ClampRowsPos(buffSet.EditBuffer)
	return buffSet.EditBuffer, meta, result, err
}

/* func (bss *BufferSets) GetFileAndMeta(filePath string) (*editbuffer.EditBuffer, *Meta, editbuffer.Result, error) {
	result := editbuffer.ResultNone

	for _, buffSet := range *bss {
		if utils.IsSameFile(filePath, buffSet.GetPath()) {
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
} */

// ClampRowsPos adjusts RowsPos so that it points inside the current rows.
// RowIndex is limited to the last row, and ColIndex to the end of that row
// and moved back to the start of a UTF-8 character.
func (m *Meta) ClampRowsPos(eb *editbuffer.EditBuffer) {
	rows := eb.Rows()

	n := rows.Length()
	if n == 0 {
		m.RowsPos = editbuffer.RowsPos{}
		return
	}

	pos := &m.RowsPos
	pos.RowIndex = min(max(pos.RowIndex, 0), n-1)

	row := rows.Row(pos.RowIndex).Bytes()
	pos.ColIndex = min(max(pos.ColIndex, 0), len(row))

	// Do not stop in the middle of a multibyte character.
	// ColIndex == len(row) is the end-of-line position and is left as it is.
	for pos.ColIndex > 0 && pos.ColIndex < len(row) && !utf8.RuneStart(row[pos.ColIndex]) {
		pos.ColIndex--
	}
}

// Contains reports whether a buffer for path is open. The file need not exist
// on disk (a new file that was not saved yet).
func (bss *BufferSets) Contains(path string) bool {
	for _, buffSet := range *bss {
		if utils.IsSameFile(path, buffSet.GetPath()) {
			return true
		}
	}
	return false
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

	// The buffer is gone: stop its syntax Worker (parser + tree).
	syntax.Release(ff)
	*bss = slices.Delete(*bss, i, i+1)
	return i
}

/* func (bss *BufferSets) RemoveByBufferIndex(index int) int {
	if index >= 0 && index < len(*bss) {
		syntax.Release((*bss)[index].EditBuffer)
	}
	*bss = slices.Delete(*bss, index, index+1)
	return index
} */

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
