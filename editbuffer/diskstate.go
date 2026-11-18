package editbuffer

import (
	"os"
	"time"

	"github.com/ge-editor/editorleaf/charset"
)

// What the file looked like on disk when the buffer was last read from it or
// written to it. Comparing it with the file as it is now tells that another
// program changed the file.

// settleTime is how old the modification time of a changed file has to be
// before the change is reported. A program that is still writing the file
// would otherwise be reported (and read) half way through.
const settleTime = time.Second

type diskState struct {
	known   bool // the file existed when it was read or written
	modTime time.Time
	size    int64

	// ignored is the state of the file that the user decided not to reload.
	// It is not reported again until the file changes once more.
	ignored        bool
	ignoredModTime time.Time
	ignoredSize    int64
}

func statRegular(path string) (modTime time.Time, size int64, ok bool) {
	if path == "" {
		return time.Time{}, 0, false
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return time.Time{}, 0, false
	}
	return info.ModTime(), info.Size(), true
}

// recordDisk remembers the state of the file on disk now. A file that does
// not exist (a new buffer) is not known: nothing can have changed under it.
func (eb *EditBuffer) recordDisk() {
	mt, size, ok := statRegular(eb.path)
	eb.setDisk(mt, size, ok)
}

func (eb *EditBuffer) setDisk(mt time.Time, size int64, ok bool) {
	eb.disk = diskState{known: ok, modTime: mt, size: size}
}

// ChangedOnDisk reports whether the file was changed by someone else since
// the buffer was read from it or written to it.
//
// It is false for a buffer without a file, for a file that was deleted, for a
// file that was changed less than a second ago (it may still be written), and
// for a change the user chose not to reload (IgnoreDiskChange) until the file
// changes again. It only reads the state of the file; it is cheap enough to
// call every few seconds.
func (eb *EditBuffer) ChangedOnDisk() bool {
	if !eb.disk.known {
		return false
	}
	mt, size, ok := statRegular(eb.path)
	if !ok {
		return false
	}
	if mt.Equal(eb.disk.modTime) && size == eb.disk.size {
		return false
	}
	if eb.disk.ignored && mt.Equal(eb.disk.ignoredModTime) && size == eb.disk.ignoredSize {
		return false
	}
	if age := time.Since(mt); age >= 0 && age < settleTime {
		return false // wait until the writer is done
	}
	return true
}

// IgnoreDiskChange makes ChangedOnDisk false for the file as it is now, until
// the file is changed again. The content of the buffer is not touched.
func (eb *EditBuffer) IgnoreDiskChange() {
	mt, size, ok := statRegular(eb.path)
	if !ok {
		return
	}
	eb.disk.ignored = true
	eb.disk.ignoredModTime = mt
	eb.disk.ignoredSize = size
}

// Reload replaces the content of the buffer with the file on disk and
// forgets the undo history, which does not fit the new content. The buffer
// is not modified afterwards. The listeners are notified with ChangeReset, so
// the syntax tree, the diagnostics and the language server follow.
//
// The buffer is left as it was when the file cannot be read.
func (eb *EditBuffer) Reload() error {
	// Stat before reading: a change during the read is then reported by
	// ChangedOnDisk instead of being lost.
	mt, size, ok := statRegular(eb.path)

	raw, err := os.ReadFile(eb.path)
	if err != nil {
		return err
	}
	return eb.replaceWith(charset.Open(raw), mt, size, ok)
}

// DiskDiffers reports whether the file on disk is not the one the buffer was
// read from or written to (another program wrote it). Unlike ChangedOnDisk it
// has no settle time and no "ignored" state: it is the question before a
// save, which would overwrite that change. A deleted file does not differ.
func (eb *EditBuffer) DiskDiffers() bool {
	if !eb.disk.known {
		return false
	}
	mt, size, ok := statRegular(eb.path)
	if !ok {
		return false
	}
	return !mt.Equal(eb.disk.modTime) || size != eb.disk.size
}
