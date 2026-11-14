package editbuffer

import "testing"

func TestRowsPosAdjustForDeletion(t *testing.T) {
	p := func(r, c int) RowsPos { return RowsPos{RowIndex: r, ColIndex: c} }

	tests := []struct {
		name       string
		start, end RowsPos
		pos, want  RowsPos
	}{
		// the deletion is inside one row
		{"before the row", p(1, 2), p(1, 4), p(0, 9), p(0, 9)},
		{"before the deletion", p(1, 2), p(1, 4), p(1, 1), p(1, 1)},
		{"at the start", p(1, 2), p(1, 4), p(1, 2), p(1, 2)},
		{"inside", p(1, 2), p(1, 4), p(1, 3), p(1, 2)},
		{"at the end", p(1, 2), p(1, 4), p(1, 4), p(1, 2)},
		{"behind it, same row", p(1, 2), p(1, 4), p(1, 10), p(1, 8)},
		{"below", p(1, 2), p(1, 4), p(2, 5), p(2, 5)},

		// the deletion joins rows 1..3
		{"inside, first row", p(1, 5), p(3, 2), p(1, 7), p(1, 5)},
		{"inside, middle row", p(1, 5), p(3, 2), p(2, 0), p(1, 5)},
		{"inside, last row", p(1, 5), p(3, 2), p(3, 1), p(1, 5)},
		{"behind it, last row", p(1, 5), p(3, 2), p(3, 7), p(1, 10)},
		{"below", p(1, 5), p(3, 2), p(4, 3), p(2, 3)},
		{"above", p(1, 5), p(3, 2), p(0, 9), p(0, 9)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.pos
			got.AdjustForDeletion(tt.start, tt.end)
			if got != tt.want {
				t.Errorf("AdjustForDeletion(%v, %v) of %v = %v, want %v",
					tt.start, tt.end, tt.pos, got, tt.want)
			}
		})
	}
}
