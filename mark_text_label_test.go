//go:build marklabel

package editorleaf

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/editorleaf/mark"
)

// With -tags marklabel the stored label is shown even though the file has
// other text at the mark.
func TestMarkTextCacheUsesStoredLabel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(path, []byte("one\ntwo two\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mk := mark.NewMark(path, editbuffer.RowsPos{RowIndex: 1, ColIndex: 4}, "stored label")
	c := NewMarkTextCache()
	c.Begin()
	if got := c.Text(mk); got != "stored label" {
		t.Errorf("Text = %q, want the stored label", got)
	}
}
