package editbuffer

import "testing"

func group(rows ...string) *actionGroup {
	g := &actionGroup{}
	for _, r := range rows {
		g.actions = append(g.actions, &EditAction{Class: INSERT, Data: Rows_{Row_(r)}})
	}
	return g
}

func texts(g *actionGroup) []string {
	var out []string
	for _, a := range g.actions {
		out = append(out, string(a.Data[0]))
	}
	return out
}

func TestMergeSince(t *testing.T) {
	u := NewUndoStack()
	u.PushActionGroup(group("before"))

	start := u.Index()
	if start != 1 {
		t.Fatalf("Index = %d, want 1", start)
	}
	u.PushActionGroup(group("a"))
	u.PushActionGroup(group("b", "c"))
	u.PushActionGroup(group("d"))

	u.MergeSince(start)

	if u.Index() != 2 {
		t.Fatalf("Index after merge = %d, want 2", u.Index())
	}

	// One Undo steps over all three, with the actions in their original order
	// (Undo applies them in reverse).
	g := u.Undo()
	got := texts(g)
	want := []string{"a", "b", "c", "d"}
	if len(got) != len(want) {
		t.Fatalf("merged actions = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("merged actions = %v, want %v", got, want)
		}
	}

	// The group before it is untouched, and nothing else is left.
	if g2 := u.Undo(); g2 == nil || texts(g2)[0] != "before" {
		t.Fatalf("the earlier group was damaged: %+v", g2)
	}
	if !u.IsUndoEmpty() {
		t.Fatal("history should be empty")
	}

	// And one Redo brings the whole merged group back.
	u.Redo()
	if r := u.Redo(); r == nil || len(r.actions) != 4 {
		t.Fatalf("Redo of the merged group = %+v", r)
	}
	if !u.IsRedoEmpty() {
		t.Fatal("nothing should be left to redo")
	}
}

func TestMergeSinceDoesNothingForOneOrZeroGroups(t *testing.T) {
	u := NewUndoStack()
	u.PushActionGroup(group("x"))

	start := u.Index()
	u.MergeSince(start) // nothing pushed since
	if u.Index() != 1 {
		t.Fatalf("Index = %d", u.Index())
	}

	u.PushActionGroup(group("y"))
	u.MergeSince(start) // exactly one group since
	if u.Index() != 2 {
		t.Fatalf("Index = %d, want 2 (a single group is left alone)", u.Index())
	}
}

func TestMergeSinceAndTheSavedState(t *testing.T) {
	// Saved before the operation: undoing it all returns to the saved state.
	u := NewUndoStack()
	u.PushActionGroup(group("x"))
	u.SetSavePoint()
	start := u.Index()
	u.PushActionGroup(group("a"))
	u.PushActionGroup(group("b"))
	u.MergeSince(start)

	if !u.IsDirty() {
		t.Fatal("after the operation the buffer is modified")
	}
	u.Undo()
	if u.IsDirty() {
		t.Fatal("undoing the merged group must reach the saved state again")
	}

	// Saved in the middle of the merged range: that state is gone.
	u = NewUndoStack()
	start = u.Index()
	u.PushActionGroup(group("a"))
	u.SetSavePoint()
	u.PushActionGroup(group("b"))
	u.MergeSince(start)
	u.Undo()
	if !u.IsDirty() {
		t.Fatal("a saved state inside the merged range cannot be reached any more")
	}
}

func TestSealPreventsJoining(t *testing.T) {
	u := NewUndoStack()
	u.PushActionGroup(group("typed"))

	u.Seal()
	if !u.stack[0].actions[0].NoJoin {
		t.Fatal("Seal must mark the last action")
	}

	// No history: nothing to seal, no panic.
	NewUndoStack().Seal()
}
