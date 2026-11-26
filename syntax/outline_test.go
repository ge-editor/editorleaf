package syntax

import (
	"reflect"
	"testing"

	"github.com/ge-editor/editorleaf/editbuffer"
)

func pos(row, col int) editbuffer.RowsPos { return editbuffer.RowsPos{RowIndex: row, ColIndex: col} }

func TestShiftOutline(t *testing.T) {
	base := []Scope{{0, 3}, {5, 9}, {11, 20}}

	tests := []struct {
		name string
		c    editbuffer.Change
		want []Scope
	}{
		{
			name: "edit inside one row moves nothing",
			c:    editbuffer.Change{Kind: editbuffer.ChangeInsert, Start: pos(6, 1), OldEnd: pos(6, 1), NewEnd: pos(6, 4)},
			want: base,
		},
		{
			name: "newline inside the first scope",
			c:    editbuffer.Change{Kind: editbuffer.ChangeInsert, Start: pos(1, 2), OldEnd: pos(1, 2), NewEnd: pos(2, 0)},
			want: []Scope{{0, 4}, {6, 10}, {12, 21}},
		},
		{
			name: "newline in the header row keeps the header",
			c:    editbuffer.Change{Kind: editbuffer.ChangeInsert, Start: pos(5, 3), OldEnd: pos(5, 3), NewEnd: pos(6, 0)},
			want: []Scope{{0, 3}, {5, 10}, {12, 21}},
		},
		{
			name: "rows deleted between the scopes",
			c:    editbuffer.Change{Kind: editbuffer.ChangeDelete, Start: pos(3, 0), OldEnd: pos(5, 0), NewEnd: pos(3, 0)},
			want: []Scope{{0, 3}, {9, 18}},
		},
		{
			name: "reset drops everything",
			c:    editbuffer.Change{Kind: editbuffer.ChangeReset},
			want: nil,
		},
	}
	for _, tt := range tests {
		e := &entry{outline: append([]Scope(nil), base...)}
		e.shiftOutline(tt.c)
		if !reflect.DeepEqual(e.outline, tt.want) {
			t.Errorf("%s: got %v, want %v", tt.name, e.outline, tt.want)
		}
	}
}

func TestShiftOutlineKeepsOldSlice(t *testing.T) {
	old := []Scope{{0, 3}, {5, 9}}
	e := &entry{outline: old}
	ver := e.outlineVer
	e.shiftOutline(editbuffer.Change{Kind: editbuffer.ChangeInsert, Start: pos(1, 0), OldEnd: pos(1, 0), NewEnd: pos(2, 0)})

	if !reflect.DeepEqual(old, []Scope{{0, 3}, {5, 9}}) {
		t.Errorf("the slice that readers may hold was modified: %v", old)
	}
	if e.outlineVer == ver {
		t.Error("outlineVer did not change")
	}
}
