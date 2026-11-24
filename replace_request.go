package editorleaf

import (
	"errors"
	"fmt"

	"github.com/ge-editor/editorleaf/charset"
	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/gecore/tree"
)

// replaceRequest handles a tree.ReplaceRequest (replace in files, from the
// search list of the explorer).
//
// Every edit is checked against the text of the file as it is now (the open
// buffer, or the file on disk): the text at the place must be what the sender
// saw (Old). If any place differs, nothing is touched; the search result is
// out of date and must be made again. The valid edits then go the same way as
// a rename (offerWorkspaceEdit): the user sees one line per changed row and
// confirms, the files are edited as buffers in their own undo history, and
// nothing is saved. workspace-edit-undo takes it back in all files.
func (e *Editorleaf) replaceRequest(req tree.ReplaceRequest) error {
	we, err := replaceWorkspaceEdit(req)
	if err != nil {
		return err
	}
	if len(we.Files) == 0 {
		return errors.New("Nothing to replace")
	}
	e.offerWorkspaceEdit(req.Title, "Replaced", we, true)
	return nil
}

// replaceWorkspaceEdit checks req and turns it into a WorkspaceEdit.
func replaceWorkspaceEdit(req tree.ReplaceRequest) (*WorkspaceEdit, error) {
	we := &WorkspaceEdit{}
	for _, rf := range req.Files {
		if len(rf.Edits) == 0 {
			continue
		}
		name := shortPath(rf.Path)

		var rows [][]byte
		if eb := findBuffer(rf.Path); eb != nil {
			rows = rowsOf(eb)
		} else {
			file, err := charset.ReadFile(rf.Path)
			if err != nil {
				return nil, fmt.Errorf("cannot read %s: %w", name, err)
			}
			rows = splitLines(file.Text)
		}

		fe := FileEdit{Path: rf.Path}
		for _, ed := range rf.Edits {
			if ed.Line < 1 || ed.Line > len(rows) || ed.Col < 1 || ed.Old == "" {
				return nil, fmt.Errorf("%s:%d: changed since the search", name, ed.Line)
			}
			row := rows[ed.Line-1]
			start, end := ed.Col-1, ed.Col-1+len(ed.Old)
			if end > len(row) || string(row[start:end]) != ed.Old {
				return nil, fmt.Errorf("%s:%d: changed since the search", name, ed.Line)
			}
			fe.Edits = append(fe.Edits, TextEdit{
				Start:   editbuffer.RowsPos{RowIndex: ed.Line - 1, ColIndex: start},
				End:     editbuffer.RowsPos{RowIndex: ed.Line - 1, ColIndex: end},
				NewText: ed.New,
			})
		}
		we.Files = append(we.Files, fe)
	}
	return we, nil
}
