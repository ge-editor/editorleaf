package editbuffer

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeAt(t *testing.T, path, content string, mod time.Time) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
}

func TestChangedOnDiskIgnoreAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.txt")
	base := time.Now().Add(-time.Hour)
	writeAt(t, path, "one\n", base)

	eb := NewEditBuffer(nil)
	eb.UndoAction = NewUndoStack()
	eb.SetPath(path)
	if err := eb.Load(); err != nil {
		t.Fatal(err)
	}
	if eb.ChangedOnDisk() {
		t.Fatal("a file that was just read is not changed")
	}

	writeAt(t, path, "two\nthree\n", base.Add(time.Minute))
	if !eb.ChangedOnDisk() {
		t.Fatal("the change by another program was not noticed")
	}

	// "No": not asked again for this change ...
	eb.IgnoreDiskChange()
	if eb.ChangedOnDisk() {
		t.Fatal("an ignored change is reported again")
	}
	// ... but asked again when the file changes once more.
	writeAt(t, path, "four\n", base.Add(2*time.Minute))
	if !eb.ChangedOnDisk() {
		t.Fatal("a newer change was not noticed after an ignored one")
	}

	if err := eb.Reload(); err != nil {
		t.Fatal(err)
	}
	if eb.ChangedOnDisk() {
		t.Fatal("the file was reloaded, so it is not changed")
	}
	// "four" and the empty row after the final newline, like Load.
	if got := eb.Rows().Length(); got != 2 {
		t.Fatalf("rows = %d, want 2", got)
	}
	if eb.IsDirtyFlag() {
		t.Fatal("a reloaded buffer is not modified")
	}
}

func TestChangedOnDiskWaitsUntilTheWriterIsDone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.txt")
	writeAt(t, path, "one\n", time.Now().Add(-time.Hour))

	eb := NewEditBuffer(nil)
	eb.SetPath(path)
	if err := eb.Load(); err != nil {
		t.Fatal(err)
	}

	// Changed just now: it may still be written.
	writeAt(t, path, "two\n", time.Now())
	if eb.ChangedOnDisk() {
		t.Fatal("a file that is being written is reported")
	}
}

func TestDiskDiffersHasNoSettleTimeAndSetPathResets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	writeAt(t, path, "one\n", time.Now().Add(-time.Hour))

	eb := NewEditBuffer(nil)
	eb.SetPath(path)
	if err := eb.Load(); err != nil {
		t.Fatal(err)
	}
	if eb.DiskDiffers() {
		t.Fatal("just read")
	}
	writeAt(t, path, "two\n", time.Now()) // changed just now
	if !eb.DiskDiffers() {
		t.Fatal("an overwrite has to be noticed at once")
	}

	// Save as: the state of the old path is not compared with the new one.
	other := filepath.Join(dir, "b.txt")
	writeAt(t, other, "x\n", time.Now().Add(-time.Hour))
	eb.SetPath(other)
	if eb.DiskDiffers() {
		t.Fatal("the new path was compared with the state of the old one")
	}
}
