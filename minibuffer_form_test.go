package editorleaf

import (
	"slices"
	"testing"
)

func TestFormHeights(t *testing.T) {
	cases := []struct {
		wanted []int
		avail  int
		want   []int
	}{
		{[]int{1, 1}, 4, []int{1, 1}},   // everything fits
		{[]int{2, 2}, 4, []int{2, 2}},   // exactly
		{[]int{10, 10}, 4, []int{2, 2}}, // both want more: half each
		{[]int{1, 10}, 4, []int{1, 3}},  // the short one leaves its room
		{[]int{10, 1}, 4, []int{3, 1}},
		{[]int{10}, 4, []int{4}}, // search only (the replace field is hidden)
	}
	for _, c := range cases {
		got := formHeights(c.wanted, c.avail)
		if !slices.Equal(got, c.want) {
			t.Errorf("formHeights(%v, %d) = %v, want %v", c.wanted, c.avail, got, c.want)
		}
		sum := 0
		for _, h := range got {
			sum += h
		}
		if sum > c.avail {
			t.Errorf("formHeights(%v, %d) = %v: over %d", c.wanted, c.avail, got, c.avail)
		}
	}
}
