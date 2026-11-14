package editorleaf

import (
	"encoding/json"
	"testing"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/editorleaf/mark"
	"github.com/ge-editor/gecore/marks"
)

func storedMarkAt(t *testing.T, id, path string, row, col int) *marks.StoredMark {
	t.Helper()
	extra, err := json.Marshal(&mark.ExtraData{
		FilePath: path,
		RowsPos:  editbuffer.RowsPos{RowIndex: row, ColIndex: col},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &marks.StoredMark{ID: id, ExtraData: extra}
}

// The last mark has to be found also when it was loaded from marks.json
// (a StoredMark): the highlight shows such a mark as the last one, and the
// region commands must use the same mark.
func TestLastMarkInFindsStoredMarkAfterUnset(t *testing.T) {
	const path = "/tmp/ge-last-mark-test/a.go"

	items := []marks.Mark{
		storedMarkAt(t, "newest", path, 10, 0),
		storedMarkAt(t, "other-file", "/tmp/ge-last-mark-test/b.go", 1, 0),
		mark.NewMark(path, editbuffer.RowsPos{RowIndex: 3, ColIndex: 2}, "older"),
	}

	got := lastMarkIn(items, path)
	if got == nil || got.RowIndex != 10 {
		t.Fatalf("last mark = %+v, want the stored mark at row 10", got)
	}

	// Unset the newest: the next mark of the file is the last one now.
	got = lastMarkIn(items[1:], path)
	if got == nil || got.RowIndex != 3 || got.ColIndex != 2 {
		t.Fatalf("after unset, last mark = %+v, want row 3 col 2", got)
	}

	if got := lastMarkIn(items[1:2], path); got != nil {
		t.Fatalf("no mark of the file is left, got %+v", got)
	}
}
