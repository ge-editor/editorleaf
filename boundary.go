package editorleaf

import (
	"fmt"
	"slices"

	"github.com/ge-editor/editorleaf/search"
	"github.com/ge-editor/gecore"
	"github.com/ge-editor/gelog"
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
	// within the virtual row.
	// The virtual row includes LF for non-final rows and EOF for
	// the final row.
	StartLogicalRowByteIndex int

	// StopLogicalRowByteIndex is the exclusive end byte index
	// within the virtual row.
	StopLogicalRowByteIndex int

	/* 	// StartLogicalRowByteIndex is the inclusive start byte index
	   	// within the original row.
	   	StartLogicalRowByteIndex int

	   	// StopLogicalRowByteIndex is the exclusive end byte index
	   	// within the original row.
	   	StopLogicalRowByteIndex int
	*/

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

/* func (b *BoundariesArray) Editor() *Editorleaf {
	return b.editor
}
*/

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

	// 範囲外アクセス
	if rowIndex >= len(b.rows) {
		// 呼び出し元情報 + 指定インデックス + 実際の配列長を一緒に出力
		err := fmt.Errorf("[INVALID INDEX] Called from %s | Target: [rowIndex:%d, colIndex:%d] | Actual limits: [rowsLen:%d, cacheLen:%d]", gelog.CallerInfo(), rowIndex, len(b.rows))
		gelog.Error(err.Error())
		gecore.Echo.AddText(err.Error())
		panic(err)
		// return 0 // 適切なエラー処理または初期値を返す
	}

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

// ClearFrom clears all cached layout information from rowIndex onward.
func (b *BoundariesArray) ClearFrom(rowIndex int) {
	if rowIndex < 0 || rowIndex >= len(b.rows) {
		return
	}

	b.rows = b.rows[:rowIndex]
}

// Clear clears the cached layout information for the specified row.
func (b *BoundariesArray) ClearRow(rowIndex int) {
	if len(b.rows) <= rowIndex {
		return
	}
	b.rows[rowIndex] = RowLayout{}
}

// ------------------------------------------------------------------
// Lazy evaluation support

// NeedsCompute reports whether layout information for rowIndex
// needs to be recomputed.
/* func (b *BoundariesArray) NeedsCompute(rowIndex int) bool {
	if b == nil {
		return true
	}
	if rowIndex < 0 || rowIndex >= len(b.rows) {
		return true
	}

	return b.rows[rowIndex].Boundaries == nil
} */

func (b *BoundariesArray) NeedsCompute(rowIndex int) bool {
	if b == nil {
		return true
	}

	if rowIndex < 0 || rowIndex >= len(b.rows) {
		return true
	}

	row := b.rows[rowIndex]

	// これは、Rows バッファ初期状態における判定に有効です。
	// 内容が比較されるわけではないので注意
	// This check is useful for validating the initial state of the Rows buffer.
	// It does not compare the actual row contents.
	// RuneWidthCache always includes one extra entry for LF or EOF.
	if len(row.RuneWidthCache) != b.editor.editBuffer.Rows.Row(rowIndex).Length()+1 {
		return true
	}

	return row.Boundaries == nil ||
		row.RuneWidthCache == nil
}

/* func (b *BoundariesArray) Invalidate(rowIndex int) {
	if b == nil {
		return
	}
	if rowIndex < 0 || rowIndex >= len(b.rows) {
		return
	}

	b.rows[rowIndex].Boundaries = nil
	b.rows[rowIndex].RuneWidthCache = nil
}
*/

// beAvailable ensures layout data for rowIndex is computed.
func (b *BoundariesArray) beAvailable(rowIndex int) {
	if b.NeedsCompute(rowIndex) {
		b.editor.drawLineWithCompute(
			0, rowIndex, -1, false, -1, []search.FoundPosition{},
		)
	}
}

func (b *BoundariesArray) GetIndexOfLastLogicalRow(rowIndex int) int {
	b.beAvailable(rowIndex)
	return len(b.rows[rowIndex].Boundaries) - 1
}

// Returns the screen position of the cursor corresponding from cached array to the specified column index in logical rows.
func (b *BoundariesArray) CursorPositionOnScreenLogicalRow(rowIndex, colIndex int) (lx, ly int) {
	b.beAvailable(rowIndex)

	// 範囲外アクセス
	if b == nil ||
		rowIndex < 0 ||
		rowIndex >= len(b.rows) ||
		colIndex < 0 ||
		colIndex >= len(b.rows[rowIndex].RuneWidthCache) {

		// 呼び出し元情報 + 指定インデックス + 実際の配列長を一緒に出力
		err := fmt.Errorf("[INVALID INDEX] Called from %s | Target: [rowIndex:%d, colIndex:%d] | Actual limits: [rowsLen:%d, cacheLen:%d]", gelog.CallerInfo(), rowIndex, colIndex, len(b.rows), len(b.rows[rowIndex].RuneWidthCache))
		gelog.Error(err.Error())
		gecore.Echo.AddText(err.Error())
		// return 0, 0 // 適切なエラー処理または初期値を返す
		panic(err)
	}

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
