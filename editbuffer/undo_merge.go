package editbuffer

// Support for edits that are made as one operation from the user's point of
// view but are carried out as many small ones, such as a rename that touches
// many places in a file.

// Index returns the position in the undo history: the number of groups that
// Undo can step back over. Remember it before an operation and hand it to
// MergeSince afterwards.
func (u *UndoStack) Index() int {
	return u.index
}

// Seal makes the most recent action unjoinable, so that the actions pushed
// next cannot be merged into it. Without it, an insertion that starts exactly
// where the user's last typed text ended would become part of that text's
// undo step.
func (u *UndoStack) Seal() {
	if u.index == 0 {
		return
	}
	g := u.stack[u.index-1]
	if n := len(g.actions); n > 0 && g.actions[n-1] != nil {
		g.actions[n-1].NoJoin = true
	}
}

// MergeSince merges every group pushed since Index() returned index into one,
// so that a single Undo (and a single Redo) covers them all. Undo applies the
// actions of a group in reverse order, so the merged group undoes exactly what
// the separate groups would have, in the same order.
//
// It must be called while the history is at its end (nothing to redo), which
// is the case right after pushing, and does nothing if fewer than two groups
// were pushed. If the saved state lay inside the merged range it can no longer
// be reached by undo, and the history is marked as modified for good.
func (u *UndoStack) MergeSince(index int) {
	if index < 0 || index >= u.index-1 || u.index != len(u.stack) {
		return
	}

	merged := &actionGroup{}
	for _, g := range u.stack[index:u.index] {
		merged.actions = append(merged.actions, g.actions...)
	}

	u.stack = append(u.stack[:index], merged)
	u.index = index + 1

	if u.saveMark > index {
		u.saveMark = -1
	}
}
