package editorleaf

import "github.com/ge-editor/editorleaf/editbuffer"

// ReplaceAllBytes replaces the whole text of the buffer with data, as
// undoable edits (a delete of everything and an insert), and puts the cursor
// at the start. It is used by "ge-remote --replace-buffer-stdin".
func (e *Editorleaf) ReplaceAllBytes(data []byte) error {
	rs, err := SplitRows(data)
	if err != nil {
		return err
	}
	rows := e.editBuffer.Rows()
	if n := rows.Length(); n > 0 {
		last := rows.Row(n - 1)
		end := editbuffer.RowsPos{RowIndex: n - 1, ColIndex: last.Length()}
		e.deleteRegion(editbuffer.RowsPos{}, end)
	}
	e.meta.RowsPos = editbuffer.RowsPos{}
	e.insertRows(rs, true)
	e.SetRowsPos(editbuffer.RowsPos{})
	return nil
}
