package editorleaf

import (
	"fmt"
	"slices"

	"github.com/ge-editor/editorleaf/search"
	"github.com/ge-editor/gecore"
	"github.com/ge-editor/locale"
	"github.com/ge-editor/utils"
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

	RuneWidthCache []locale.Cell
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
	boundaries []Boundary, // index is logical row number
	hangingIndentWidth int,
	runeWidths []locale.Cell, // index is rowIndex byte position
) {
	utils.EnsureSize(&b.rows, rowIndex)

	b.rows[rowIndex] = RowLayout{
		Boundaries:         boundaries,
		HangingIndentWidth: hangingIndentWidth,
		RuneWidthCache:     runeWidths,
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
func (b *BoundariesArray) Boundary(rowIndex, logicalRowIndex int) *Boundary {
	b.beAvailable(rowIndex)
	return &b.rows[rowIndex].Boundaries[logicalRowIndex]
}

// LastBoundary returns the last logical row boundary
// for the specified physical row.
func (b *BoundariesArray) LastBoundary(rowIndex int) *Boundary {
	b.beAvailable(rowIndex)
	row := b.rows[rowIndex]
	return &row.Boundaries[len(row.Boundaries)-1]
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

func (b *BoundariesArray) Clear(rowIndex int) {
	if len(b.rows) <= rowIndex {
		return
	}
	b.rows[rowIndex] = RowLayout{}
}

// ------------------------------------------------------------------
// Lazy evaluation support

// NeedsReparse reports whether layout information for rowIndex
// needs to be recomputed.
func (b *BoundariesArray) NeedsReparse(rowIndex int) bool {
	if b == nil {
		return true
	}

	if rowIndex < 0 || rowIndex >= len(b.rows) {
		return true
	}

	row := b.rows[rowIndex]

	if len(row.RuneWidthCache) != len(*b.editor.editBuffer.Rows().Row(rowIndex)) {
		return true
	}

	// nil = never parsed
	return row.Boundaries == nil
}

// beAvailable ensures layout data for rowIndex is computed.
func (b *BoundariesArray) beAvailable(rowIndex int) {
	if b.NeedsReparse(rowIndex) {
		b.editor.drawLineWithCompute(
			0, rowIndex, -1, false, -1, []search.FoundPosition{},
		)
	}
}

// Returns the screen position of the cursor corresponding from cached array to the specified column index in logical rows.
func (b *BoundariesArray) CursorPositionOnScreenLogicalRow(rowIndex, colIndex int) (lx, ly int) {
	b.beAvailable(rowIndex)
	cell := b.rows[rowIndex].RuneWidthCache[colIndex]
	return cell.TotalWidthLogicalRow, cell.LogicalRowIndex
}

func (b *BoundariesArray) GetIndexOfLogicalRow(rowIndex, colIndex int) int {
	b.beAvailable(rowIndex)
	cell := b.rows[rowIndex].RuneWidthCache[colIndex]
	return cell.LogicalRowIndex
}

// Check if the column index is within the last boundary of the specified row
// Return false: out of index or not initialized.
func (b *BoundariesArray) OnEndOfLogicalRow(rowIndex, colIndex int) bool {
	b.beAvailable(rowIndex)
	lastBoundary := b.LastBoundary(rowIndex)
	return colIndex >= lastBoundary.StartLogicalRowByteIndex && colIndex < lastBoundary.StopLogicalRowByteIndex
}

func (b *BoundariesArray) IsEndOfLogicalRow(rowIndex, colIndex int) bool {
	b.beAvailable(rowIndex)
	cell := b.rows[rowIndex].RuneWidthCache[colIndex]
	return b.rows[rowIndex].Boundaries[cell.LogicalRowIndex].StopLogicalRowByteIndex == colIndex
}
