package editbuffer

import (
	"fmt"

	"github.com/ge-editor/editorleaf/editbuffer/rows"
	"github.com/ge-editor/gelog"
)

// NewUndoStack creates a new UndoStack.
func NewUndoStack() *UndoStack {
	return &UndoStack{
		stack:    make([]*actionGroup, 0, 64),
		index:    0,
		saveMark: 0,
	}
}

type UndoStack struct {
	stack    []*actionGroup
	index    int
	saveMark int
}

// actionGroup is a group of edit actions that should be undone/redone
// as a single operation.
type actionGroup struct {
	actions []*EditAction
}

// Push adds an action to the group.
func (g *actionGroup) Push(a *EditAction) {
	if a == nil {
		return
	}

	g.actions = append(g.actions, a)

}

// Actions returns copies of the edit actions in this group.
//
// Each EditAction is shallow-copied so that modifying the returned
// EditAction does not modify the action stored in the undo stack.
// Fields referenced by pointers, slices, maps, etc. are not deep-copied.
func (g *actionGroup) Actions() []*EditAction {
	copied := make([]*EditAction, len(g.actions))
	for i, ea := range g.actions {
		if ea == nil {
			continue
		}

		shallowCopy := *ea
		copied[i] = &shallowCopy
	}
	return copied
}

type ActionClass int

const (
	INSERT ActionClass = iota
	DELETE
	DELETE_BACKWARD
)

type EditAction struct {
	Class  ActionClass
	Before rows.RowsPos
	After  rows.RowsPos
	Data   rows.Rows
}

func (a *EditAction) Clone() *EditAction {
	if a == nil {
		return nil
	}

	cloned := *a
	cloned.Data = a.Data.Clone()

	return &cloned
}

// PushActionGroup pushes a copy of the action group onto the undo stack.
//
// The action group and each EditAction are copied so that subsequent
// modifications to the original group do not affect the undo stack.
// The copy is shallow with respect to fields contained in EditAction.
func (u *UndoStack) PushActionGroup(ag *actionGroup) {
	if ag == nil || len(ag.actions) == 0 {
		return
	}

	u.clearRedo()

	copied := &actionGroup{
		actions: make([]*EditAction, len(ag.actions)),
	}

	for i, ea := range ag.actions {
		if ea == nil {
			continue
		}

		actionCopy := *ea
		copied.actions[i] = &actionCopy
	}

	u.stack = append(u.stack, copied)
	u.index++
}

// PushAction pushes a copy of the edit action onto the undo stack.
//
// The original EditAction is never stored directly in the stack.
// A copy is created first so that subsequent modifications to the
// caller's EditAction cannot modify the undo history.
//
// Data is also copied to prevent the undo stack from sharing mutable
// row data with the caller.
func (u *UndoStack) PushAction(a *EditAction, beAbleToJoin bool) {
	if a == nil {
		return
	}

	gelog.Debug(
		"PushAction",
		"after", fmt.Sprintf("%d:%d", a.After.RowIndex, a.After.ColIndex),
		"before", fmt.Sprintf("%d:%d", a.Before.RowIndex, a.Before.ColIndex),
		"data", a.Data.String([]byte{'\n'}),
	)

	// Make a complete copy before storing the action in the undo stack.
	actionCopy := *a
	actionCopy.Data = a.Data.Clone()

	// If we are not at the end of the stack, the redo history must be
	// discarded before adding a new action.
	u.clearRedo()

	// There is no previous action to join with.
	if !beAbleToJoin || u.index == 0 {
		u.pushNewGroup(&actionCopy)
		return
	}

	// Do not join across the saved-state boundary.
	//
	// This ensures that the first edit after saving starts a new
	// undo group.
	if u.index == u.saveMark {
		u.pushNewGroup(&actionCopy)
		return
	}

	lastActionGroup := u.stack[u.index-1]

	if len(lastActionGroup.actions) == 0 {
		u.pushNewGroup(&actionCopy)
		return
	}

	lastAction := lastActionGroup.actions[len(lastActionGroup.actions)-1]

	if u.canJoin(lastAction, &actionCopy) {
		u.joinActions(lastAction, &actionCopy)
		return
	}

	// The action cannot be joined with the previous action, so start
	// a new action group.
	u.pushNewGroup(&actionCopy)
}

// Undo returns the most recent action group and moves the current
// position backward.
func (u *UndoStack) Undo() *actionGroup {
	if u.index == 0 {
		return nil
	}

	u.index--
	return u.stack[u.index]
}

// Redo returns the next action group and moves the current position
// forward.
func (u *UndoStack) Redo() *actionGroup {
	if u.index >= len(u.stack) {
		return nil
	}

	ag := u.stack[u.index]
	u.index++

	return ag
}

// IsUndoEmpty reports whether there is nothing to undo.
func (u *UndoStack) IsUndoEmpty() bool {
	return u.index == 0
}

// IsRedoEmpty reports whether there is nothing to redo.
func (u *UndoStack) IsRedoEmpty() bool {
	return u.index >= len(u.stack)
}

// MarkSaved marks the current position as the saved state.
func (u *UndoStack) MarkSaved() {
	u.saveMark = u.index
}

// IsDirty reports whether the current position differs from the saved state.
/* func (u *UndoStack) IsDirty() bool {
	return u.index != u.saveMark
}
*/

// IsDirty reports whether the current position differs from the saved state.
func (u *UndoStack) IsDirty() bool {
	return u.saveMark < 0 || u.index != u.saveMark
}

// clearRedo discards all actions after the current position.
/* func (u *UndoStack) clearRedo() {
	if u.index >= len(u.stack) {
		return
	}

	u.stack = u.stack[:u.index]
}
*/

// clearRedo discards all actions after the current position.
func (u *UndoStack) clearRedo() {
	if u.index >= len(u.stack) {
		return
	}

	u.stack = u.stack[:u.index]

	if u.saveMark > u.index {
		u.saveMark = -1
	}
}

// pushNewGroup creates a new action group and advances the stack position.
func (u *UndoStack) pushNewGroup(a *EditAction) {
	ag := &actionGroup{
		actions: []*EditAction{a},
	}

	u.stack = append(u.stack, ag)
	u.index++
}

// canJoin reports whether two consecutive actions can be merged.
func (u *UndoStack) canJoin(last, current *EditAction) bool {
	if last == nil || current == nil {
		return false
	}

	if last.Class != current.Class {
		return false
	}

	switch current.Class {
	case INSERT:
		// Normal insertion proceeds forward from the previous cursor.
		return last.After.Equals(current.Before)

	case DELETE:
		// Forward deletion keeps the cursor at the same position.
		return last.After.Equals(current.Before)

	case DELETE_BACKWARD:
		// Backward deletion moves the cursor toward the beginning.
		return last.Before.Equals(current.After)

	default:
		return false
	}
}

// joinActions merges current into last.
func (u *UndoStack) joinActions(last, current *EditAction) {
	switch current.Class {
	case INSERT:
		// abc + def => abcdef
		last.Data = last.Data.Joined(current.Data)
		last.After = current.After

	case DELETE:
		// delete abc + delete def => delete abcdef
		last.Data = last.Data.Joined(current.Data)
		last.After = current.After

	case DELETE_BACKWARD:
		// Backspace is performed in reverse document order.
		//
		// First: delete "c"
		// Then : delete "b"
		//
		// Undo must restore "bc", therefore current.Data comes first.
		last.Data = current.Data.Joined(last.Data)
		last.After = current.After
	}
}
