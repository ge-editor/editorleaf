package editorleaf

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ge-editor/gecore/tree"
)

func TestReplaceWorkspaceEditChecksOldText(t *testing.T) {
	if BufferSets == nil {
		t.Skip("BufferSets is not set up in this test binary")
	}
	path := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(path, []byte("one foo\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	good := tree.ReplaceRequest{Files: []tree.ReplaceFile{{
		Path:  path,
		Edits: []tree.ReplaceEdit{{Line: 1, Col: 5, Old: "foo", New: "bar"}},
	}}}
	we, err := replaceWorkspaceEdit(good)
	if err != nil {
		t.Fatal(err)
	}
	ed := we.Files[0].Edits[0]
	if ed.Start.RowIndex != 0 || ed.Start.ColIndex != 4 || ed.End.ColIndex != 7 || ed.NewText != "bar" {
		t.Fatalf("edit = %+v", ed)
	}

	stale := good
	stale.Files = []tree.ReplaceFile{{
		Path:  path,
		Edits: []tree.ReplaceEdit{{Line: 1, Col: 5, Old: "xyz", New: "bar"}},
	}}
	if _, err := replaceWorkspaceEdit(stale); err == nil {
		t.Fatal("changed text must be refused")
	}
}
