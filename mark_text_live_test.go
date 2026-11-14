//go:build !marklabel

package editorleaf

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/editorleaf/mark"
)

func TestMarkTextCacheReadsFileAndFollowsChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(path, []byte("one\r\ntwo two\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	mk := mark.NewMark(path, editbuffer.RowsPos{RowIndex: 1, ColIndex: 4}, "stored label")
	c := NewMarkTextCache()

	c.Begin()
	if got := c.Text(mk); got != "two" {
		t.Errorf("Text = %q, want %q", got, "two")
	}

	// The file changes (a different size): the next rebuild sees it.
	if err := os.WriteFile(path, []byte("one\nzero\nnew text here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c.Begin()
	mk.RowsPos = editbuffer.RowsPos{RowIndex: 2, ColIndex: 4}
	if got := c.Text(mk); got != "text here" {
		t.Errorf("after the change Text = %q, want %q", got, "text here")
	}

	// A row that is not in the file any more, and a file that is gone, use the
	// stored label.
	mk.RowsPos = editbuffer.RowsPos{RowIndex: 99}
	c.Begin()
	if got := c.Text(mk); got != "stored label" {
		t.Errorf("row gone: Text = %q", got)
	}
	os.Remove(path)
	mk.RowsPos = editbuffer.RowsPos{RowIndex: 0}
	c.Begin()
	if got := c.Text(mk); got != "stored label" {
		t.Errorf("file gone: Text = %q", got)
	}
}
