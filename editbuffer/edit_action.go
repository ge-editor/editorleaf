package editbuffer

import (
	"github.com/ge-editor/gecore/screen"
	"github.com/ge-editor/utils"
)

// Undoable interface
type Undoable interface {
	Undo() []*EditAction
	Redo() []*EditAction
}

type ActionClass int

const (
	INSERT ActionClass = iota
	DELETE
	DELETE_BACKWARD
)

// Smallest unit of an edit operation
type EditAction struct {
	Class  ActionClass
	Before screen.Cursor
	After  screen.Cursor
	Data   []byte
}

func (e *EditAction) Undo() []*EditAction {
	return []*EditAction{e}
}

func (e *EditAction) Redo() []*EditAction {
	return []*EditAction{e}
}

// -------------------------
// Group of actions
// -------------------------
type EditGroup struct {
	Actions []*EditAction
}

func (g *EditGroup) Undo() []*EditAction {
	rev := make([]*EditAction, len(g.Actions))
	for i, a := range g.Actions {
		rev[len(g.Actions)-1-i] = a
	}
	return rev
}

func (g *EditGroup) Redo() []*EditAction {
	return g.Actions
}

// -------------------------
// UndoStack
// -------------------------
type UndoStack struct {
	stack    []Undoable
	index    int
	saveMark int
}

// Create a new UndoStack
func NewUndoStack() *UndoStack {
	return &UndoStack{
		stack:    make([]Undoable, 0),
		index:    0,
		saveMark: 0,
	}
}

// Push a new action (merge with the last one if it's the same class and position)
func (u *UndoStack) PushAction(a *EditAction) {
	if u.index > 0 {
		// If the previous action is the same class and cursor position,
		// merge it into the last action instead of pushing a new one.
		if prev, ok := u.stack[u.index-1].(*EditAction); ok {
			if prev.Class == a.Class {
				if a.Class == DELETE_BACKWARD && a.Before.Equals(prev.After) {
					utils.ReverseUTF8Bytes(a.Data)
					prev.Data = append(a.Data, prev.Data...)
					prev.After = a.After
					return
				} else if a.Class == INSERT && prev.After.Equals(a.Before) {
					prev.Data = append(prev.Data, a.Data...)
					prev.After = a.After
					return
				} else if a.Class == DELETE && prev.After.Equals(a.Before) {
					prev.Data = append(prev.Data, a.Data...)
					prev.After = a.After
					return
				}
			}
		}
	}

	if u.index < len(u.stack) {
		u.stack = u.stack[:u.index] // Clear redo buffer
	}
	u.stack = append(u.stack, a)
	u.index++
}

// Push a group of actions (for macros, replace operations, etc.)
func (u *UndoStack) PushGroup(g *EditGroup) {
	if u.index < len(u.stack) {
		u.stack = u.stack[:u.index] // Clear redo buffer
	}
	u.stack = append(u.stack, g)
	u.index++
}

// Undo the last action/group
func (u *UndoStack) Undo() []*EditAction {
	if u.index == 0 {
		return nil
	}
	u.index--
	return u.stack[u.index].Undo()
}

// Redo the next action/group
func (u *UndoStack) Redo() []*EditAction {
	if u.index >= len(u.stack) {
		return nil
	}
	res := u.stack[u.index].Redo()
	u.index++
	return res
}

// Check if undo stack is empty
func (u *UndoStack) IsUndoEmpty() bool {
	return u.index == 0
}

// Check if redo stack is empty
func (u *UndoStack) IsRedoEmpty() bool {
	return u.index >= len(u.stack)
}

// Mark the current position as "saved"
func (u *UndoStack) MarkSaved() {
	u.saveMark = u.index
}

// Check if the buffer is dirty (modified after last save)
func (u *UndoStack) IsDirty() bool {
	return u.index != u.saveMark
}
