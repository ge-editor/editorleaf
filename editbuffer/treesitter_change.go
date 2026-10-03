package editbuffer

// This file provides change notifications for EditBuffer.
//
// Why here and not in UndoStack.PushAction:
// Undo and Redo modify the buffer directly and never go through PushAction,
// and PushAction merges consecutive edits into one action. Every buffer
// modification, however, goes through the four methods defined below
// (InsertRegion, RemoveRegion, SetRows, SetBytesArray), so they are the
// only reliable place to observe all edits exactly as they happen.
//
// Callers must use these EditBuffer methods, not eb.Rows.InsertRegion(...)
// directly, otherwise listeners are not notified.
//
// Threading: listeners are invoked synchronously on the goroutine that
// modifies the buffer (the UI goroutine). AddChangeListener and the
// returned remove function must be called from that goroutine too.
// A listener that needs to do heavy work should copy what it needs and
// hand it to another goroutine (for example through a channel); it must
// not read the EditBuffer from that goroutine.

// ChangeKind identifies the type of a Change.
type ChangeKind int

const (
	// ChangeInsert: Text was inserted at Start.
	ChangeInsert ChangeKind = iota

	// ChangeDelete: the region [Start, OldEnd) was removed.
	ChangeDelete

	// ChangeReset: the whole content was replaced. Text contains a complete,
	// independent snapshot of the replacement in rows.
	ChangeReset
)

// Change describes one modification of the buffer.
//
// Positions are (row, byte column) like everywhere else in the editor.
// Start and OldEnd are in the coordinates before the change, NewEnd is in
// the coordinates after the change.
//
//	Insert: OldEnd == Start, NewEnd = end of the inserted text, Text = inserted rows
//	Delete: NewEnd == Start, OldEnd = end of the removed region, Text = nil
//	Reset : Text is the complete replacement snapshot; positions are unused
type Change struct {
	Kind   ChangeKind
	Start  RowsPos
	OldEnd RowsPos
	NewEnd RowsPos

	// Text is an independent copy of inserted rows, or the replacement
	// snapshot for ChangeReset. Async consumers must not read the live buffer.
	Text Rows_
}

// TextLF returns Text as bytes with rows joined by '\n'.
// This is the representation used by LineIndex and by parsers, regardless
// of the newline type of the file.
func (c Change) TextLF() []byte {
	return joinLF(c.Text)
}

// ChangeListener receives every buffer modification. See the threading
// notes at the top of this file.
type ChangeListener func(Change)

type changeListenerEntry struct {
	id int
	fn ChangeListener
}

// changeNotifier is embedded in EditBuffer. The zero value is ready to use.
type changeNotifier struct {
	nextListenerID int
	listeners      []changeListenerEntry
}

// AddChangeListener registers fn and returns a function that removes it.
func (n *changeNotifier) AddChangeListener(fn ChangeListener) (remove func()) {
	n.nextListenerID++
	id := n.nextListenerID
	n.listeners = append(n.listeners, changeListenerEntry{id: id, fn: fn})

	return func() {
		// Build a new slice so that a listener removing itself while
		// emit is iterating does not disturb the iteration.
		next := make([]changeListenerEntry, 0, len(n.listeners))
		for _, l := range n.listeners {
			if l.id != id {
				next = append(next, l)
			}
		}
		n.listeners = next
	}
}

func (n *changeNotifier) emit(c Change) {
	for _, l := range n.listeners {
		l.fn(c)
	}
}

// InsertRegion inserts data at (rowIndex, colIndex) and notifies listeners.
// It shadows the promoted rows.Rows.InsertRegion.
func (eb *EditBuffer) InsertRegion(rowIndex, colIndex int, data Rows_) {
	if len(data) == 0 {
		return
	}

	eb.rows.insertRegion(rowIndex, colIndex, data)

	// Inserting a single empty row changes nothing.
	if len(data) == 1 && len(data[0]) == 0 {
		return
	}

	start := RowsPos{RowIndex: rowIndex, ColIndex: colIndex}
	eb.emit(Change{
		Kind:   ChangeInsert,
		Start:  start,
		OldEnd: start,
		NewEnd: endOfInserted(start, data),
		Text:   data.CloneRows(),
	})
}

// RemoveRegion removes [start, end) and returns the removed rows.
// Listeners are notified only when something was actually removed
// (rows.Rows.RemoveRegion returns nil for an invalid or empty region).
// It shadows the promoted rows.Rows.RemoveRegion.
func (eb *EditBuffer) RemoveRegion(start, end RowsPos) Rows_ {
	removed := eb.rows.removeRegion(start, end)

	if len(removed) > 0 {
		eb.emit(Change{
			Kind:   ChangeDelete,
			Start:  start,
			OldEnd: end,
			NewEnd: start,
		})
	}

	return removed
}

// SetRows replaces the whole content and notifies listeners with ChangeReset.
// It shadows the promoted rows.Rows.SetRows.
func (eb *EditBuffer) SetRows(newRows Rows_) {
	eb.rows.setRows(newRows)
	// eb.emit(Change{Kind: ChangeReset})
	eb.emit(Change{Kind: ChangeReset, Text: eb.rows.CloneRows()})
}

// SetBytesArray replaces the whole content and notifies listeners with
// ChangeReset. It shadows the promoted rows.Rows.SetBytesArray.
func (eb *EditBuffer) SetBytesArray(source [][]byte) {
	eb.rows.setBytesArray(source)
	// eb.emit(Change{Kind: ChangeReset})
	eb.emit(Change{Kind: ChangeReset, Text: eb.rows.CloneRows()})
}

// endOfInserted returns the position just after the inserted rows,
// in the coordinates after the insertion.
func endOfInserted(start RowsPos, data Rows_) RowsPos {
	last := len(data) - 1
	if last == 0 {
		return RowsPos{
			RowIndex: start.RowIndex,
			ColIndex: start.ColIndex + len(data[0]),
		}
	}

	return RowsPos{
		RowIndex: start.RowIndex + last,
		ColIndex: len(data[last]),
	}
}

// joinLF joins rows with '\n' (no trailing newline).
func joinLF(rs Rows_) []byte {
	n := 0
	for _, r := range rs {
		n += len(r) + 1
	}
	if n == 0 {
		return nil
	}

	b := make([]byte, 0, n-1)
	for i, r := range rs {
		if i > 0 {
			b = append(b, '\n')
		}
		b = append(b, r...)
	}
	return b
}
