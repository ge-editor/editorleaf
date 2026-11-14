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
	// Version is the buffer's version right after this change. It starts
	// at 0 for a new buffer, increases by exactly 1 per Change (also for
	// ChangeReset), and never goes back, not even on Undo (an undo is just
	// another edit). Consumers that talk to the outside world, such as the
	// LSP client's textDocument/didChange, use it as the document version.
	// See EditBuffer.Version.
	Version uint64

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
	version uint64 // see EditBuffer.Version

	nextSaveListenerID int
	saveListeners      []saveListenerEntry

	nextListenerID int
	listeners      []changeListenerEntry

	langListeners []langListenerEntry // see AddLangModeListener
}

type langListenerEntry struct {
	id int
	fn func()
}

// AddLangModeListener registers fn, which is called after
// EditBuffer.SetLangMode replaced the buffer's lang.Mode, and returns a
// function that removes it. Like change listeners, it is invoked
// synchronously on the goroutine that calls SetLangMode (the UI goroutine),
// and AddLangModeListener / remove must be called from there too.
func (n *changeNotifier) AddLangModeListener(fn func()) (remove func()) {
	n.nextListenerID++
	id := n.nextListenerID
	n.langListeners = append(n.langListeners, langListenerEntry{id: id, fn: fn})

	return func() {
		next := make([]langListenerEntry, 0, len(n.langListeners))
		for _, l := range n.langListeners {
			if l.id != id {
				next = append(next, l)
			}
		}
		n.langListeners = next
	}
}

func (n *changeNotifier) emitLangMode() {
	for _, l := range n.langListeners {
		l.fn()
	}
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

// Version returns the buffer's current version: the Version of the most
// recent Change, or 0 if the buffer has not been modified through the
// notifying methods yet. Call it on the UI goroutine.
//
// LoadFrom / Load / New set the initial content without a Change, so they
// do not advance the version. They run before anyone listens; a consumer
// that opens the buffer later (didOpen) takes its text and Version() at the
// same moment and then applies only Changes with a greater Version.
func (n *changeNotifier) Version() uint64 {
	return n.version
}

func (n *changeNotifier) emit(c Change) {
	// The version advances even when nobody listens, so a listener added
	// later still sees a consistent, gap-free sequence from then on.
	n.version++
	c.Version = n.version

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

// SaveListener is called after the buffer was written to disk
// successfully, on the goroutine that called Save (the UI goroutine).
type SaveListener func()

type saveListenerEntry struct {
	id int
	fn SaveListener
}

// AddSaveListener registers fn to be called after every successful Save
// and returns the function that unregisters it.
//
// It is the counterpart of AddChangeListener for "the file on disk now
// equals the buffer": the LSP client turns it into textDocument/didSave.
// Save runs a save-time formatting first, which is reported as a
// ChangeReset before the write, so a save listener always runs after the
// change listeners have seen the final text.
func (n *changeNotifier) AddSaveListener(fn SaveListener) (remove func()) {
	n.nextSaveListenerID++
	id := n.nextSaveListenerID
	n.saveListeners = append(n.saveListeners, saveListenerEntry{id: id, fn: fn})

	return func() {
		next := make([]saveListenerEntry, 0, len(n.saveListeners))
		for _, l := range n.saveListeners {
			if l.id != id {
				next = append(next, l)
			}
		}
		n.saveListeners = next
	}
}

func (n *changeNotifier) emitSaved() {
	for _, l := range n.saveListeners {
		l.fn()
	}
}
