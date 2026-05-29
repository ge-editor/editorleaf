package editorleaf

import (
	"fmt"
	"slices"

	"github.com/ge-editor/editorleaf/search"
	"github.com/ge-editor/gecore"
)

// Boundary represents a single wrapped logical row segment
// within a physical line.
//
// A physical line (row) may be split into multiple logical rows
// due to wrapping. Each Boundary describes one such segment.
type Boundary struct {
	// StartLogicalRowByteIndex is the inclusive start byte index
	// within the original row.
	StartLogicalRowByteIndex int

	// StopLogicalRowByteIndex is the exclusive end byte index
	// within the original row.
	StopLogicalRowByteIndex int

	// LogicalRowWidth is the rendered width (in cells)
	// of this logical row.
	LogicalRowWidth int

	// TotalCellWidth includes tab expansion width.
	TotalCellWidth int
}

// Clear resets the Boundary to its zero state.
func (b *Boundary) Clear() {
	*b = Boundary{}
}

// IsEmpty reports whether the Boundary is uninitialized.
func (b *Boundary) IsEmpty() bool {
	return b.StopLogicalRowByteIndex == 0 &&
		b.LogicalRowWidth == 0
}

// ------------------------------------------------------------------

// RowLayout represents layout information for a single physical row.
//
// It contains all computed wrapping boundaries and additional
// layout metadata such as hanging indentation width.
type RowLayout struct {
	// Boundaries contains all wrapped logical row segments.
	Boundaries []Boundary

	// HangingIndentWidth is the indentation width applied to
	// wrapped logical rows after the first one.
	HangingIndentWidth int
}

// ------------------------------------------------------------------

// BoundariesArray manages layout cache information per physical row.
//
// It lazily computes wrapping information when needed and
// stores per-row layout metadata.
type BoundariesArray struct {
	editor *Editorleaf
	rows   []RowLayout
}

// NewBoundariesArray creates a new BoundariesArray.
func NewBoundariesArray(editor *Editorleaf) BoundariesArray {
	return BoundariesArray{
		editor: editor,
		rows:   make([]RowLayout, 0, 64),
	}
}

// Len returns the number of rows currently stored.
func (b *BoundariesArray) Len() int {
	return len(b.rows)
}

// Set stores layout information for the specified row index.
//
// It overwrites any existing layout data for the row.
func (b *BoundariesArray) Set(
	rowIndex int,
	boundaries []Boundary,
	hangingIndentWidth int,
) {
	b.ensureSize(rowIndex)

	b.rows[rowIndex] = RowLayout{
		Boundaries:         boundaries,
		HangingIndentWidth: hangingIndentWidth,
	}
}

// BoundariesLen returns the number of logical rows
// (wrapped segments) for the specified physical row.
func (b *BoundariesArray) BoundariesLen(rowIndex int) int {
	b.beAvailable(rowIndex)
	return len(b.rows[rowIndex].Boundaries)
}

// Boundary returns the Boundary for the specified row
// and logical row index.
func (b *BoundariesArray) Boundary(rowIndex, logicalRowIndex int) Boundary {
	b.beAvailable(rowIndex)
	return b.rows[rowIndex].Boundaries[logicalRowIndex]
}

// LastBoundary returns the last logical row boundary
// for the specified physical row.
func (b *BoundariesArray) LastBoundary(rowIndex int) Boundary {
	b.beAvailable(rowIndex)
	row := b.rows[rowIndex]
	return row.Boundaries[len(row.Boundaries)-1]
}

// GetHangingIndentWidth returns the hanging indentation width
// for the specified row. It returns 0 if the row is out of range.
func (b *BoundariesArray) GetHangingIndentWidth(rowIndex int) int {
	if rowIndex < 0 || rowIndex >= len(b.rows) {
		return 0
	}
	return b.rows[rowIndex].HangingIndentWidth
}

// Insert inserts count empty rows at rowIndex.
func (b *BoundariesArray) Insert(rowIndex, count int) {
	if rowIndex < 0 || count < 0 {
		gecore.Echo.AddText(fmt.Sprintf(
			"Error: rowIndex and count must be non-negative, rowIndex: %d, count: %d",
			rowIndex, count,
		))
		return
	}

	appendCount := rowIndex - (b.Len() - 1)
	if appendCount > 0 {
		b.rows = append(b.rows, make([]RowLayout, appendCount)...)
	}

	b.rows = slices.Insert(b.rows, rowIndex, make([]RowLayout, count)...)
}

// Delete removes count rows starting from rowIndex.
func (b *BoundariesArray) Delete(rowIndex, count int) error {
	if rowIndex < 0 || count < 0 {
		return fmt.Errorf("rowIndex and count must be non-negative")
	}
	if rowIndex >= len(b.rows) {
		return fmt.Errorf("rowIndex out of range")
	}
	if rowIndex+count > len(b.rows) {
		count = len(b.rows) - rowIndex
	}

	b.rows = slices.Delete(b.rows, rowIndex, rowIndex+count)
	return nil
}

// ClearAll clears all cached layout information.
func (b *BoundariesArray) ClearAll() {
	b.rows = nil
}

// ------------------------------------------------------------------
// Lazy evaluation support

// isDirty reports whether layout information for rowIndex
// needs to be recomputed.
func (b *BoundariesArray) isDirty(rowIndex int) bool {
	return rowIndex >= len(b.rows) ||
		len(b.rows[rowIndex].Boundaries) == 0
}

// beAvailable ensures layout data for rowIndex is computed.
func (b *BoundariesArray) beAvailable(rowIndex int) {
	if b.isDirty(rowIndex) {
		b.editor.drawLineWithCompute(
			0, rowIndex, -1, false, -1, []search.FoundPosition{},
		)
	}
}

// ------------------------------------------------------------------
// Internal helpers

// ensureSize expands the internal slice so that rowIndex
// becomes addressable.
func (b *BoundariesArray) ensureSize(rowIndex int) {
	if rowIndex < len(b.rows) {
		return
	}

	newSize := rowIndex + 1

	if newSize > cap(b.rows) {
		newCap := cap(b.rows) * 2
		if newCap < newSize {
			newCap = newSize
		}
		newSlice := make([]RowLayout, newSize, newCap)
		copy(newSlice, b.rows)
		b.rows = newSlice
	} else {
		b.rows = b.rows[:newSize]
	}
}
