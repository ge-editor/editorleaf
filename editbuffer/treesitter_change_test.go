package editbuffer

import (
	"strings"
	"testing"
)

func newLoaded(t *testing.T, text string) *EditBuffer {
	t.Helper()
	eb := NewEditBuffer(nil)
	if err := eb.LoadFrom(strings.NewReader(text)); err != nil {
		t.Fatal(err)
	}
	return eb
}

func TestChangeVersionIncreasesByOne(t *testing.T) {
	eb := newLoaded(t, "hello\nworld")
	if got := eb.Version(); got != 0 {
		t.Fatalf("Version after load = %d, want 0 (load does not notify)", got)
	}

	var got []Change
	remove := eb.AddChangeListener(func(c Change) { got = append(got, c) })
	defer remove()

	eb.InsertRegion(0, 5, Rows_{Row_(" there")})
	eb.RemoveRegion(
		RowsPos{RowIndex: 0, ColIndex: 5},
		RowsPos{RowIndex: 0, ColIndex: 11},
	)
	eb.SetBytesArray([][]byte{[]byte("x")})

	wantKinds := []ChangeKind{ChangeInsert, ChangeDelete, ChangeReset}
	if len(got) != len(wantKinds) {
		t.Fatalf("got %d changes, want %d", len(got), len(wantKinds))
	}
	for i, c := range got {
		if c.Kind != wantKinds[i] {
			t.Errorf("change %d: kind = %v, want %v", i, c.Kind, wantKinds[i])
		}
		if want := uint64(i + 1); c.Version != want {
			t.Errorf("change %d: Version = %d, want %d", i, c.Version, want)
		}
	}
	if eb.Version() != 3 {
		t.Fatalf("Version = %d, want 3", eb.Version())
	}
}

func TestVersionNotAdvancedByNoOps(t *testing.T) {
	eb := newLoaded(t, "abc")

	called := 0
	remove := eb.AddChangeListener(func(Change) { called++ })
	defer remove()

	// Nothing to insert, a single empty row, an empty region.
	eb.InsertRegion(0, 0, Rows_{})
	eb.InsertRegion(0, 0, Rows_{Row_(nil)})
	eb.RemoveRegion(RowsPos{}, RowsPos{})

	if called != 0 {
		t.Fatalf("listener called %d times for no-op edits", called)
	}
	if eb.Version() != 0 {
		t.Fatalf("Version = %d, want 0", eb.Version())
	}
}

func TestVersionAdvancesWithoutListeners(t *testing.T) {
	eb := newLoaded(t, "abc")

	// Nobody listens yet: the version still moves, so a listener added
	// later (the LSP client opening the document) continues the sequence.
	eb.InsertRegion(0, 3, Rows_{Row_("d")})
	if eb.Version() != 1 {
		t.Fatalf("Version = %d, want 1", eb.Version())
	}

	var got Change
	remove := eb.AddChangeListener(func(c Change) { got = c })
	defer remove()

	eb.InsertRegion(0, 4, Rows_{Row_("e")})
	if got.Version != 2 {
		t.Fatalf("Change.Version = %d, want 2", got.Version)
	}
}

func TestVersionSharedByAllListeners(t *testing.T) {
	eb := newLoaded(t, "abc")

	var a, b uint64
	removeA := eb.AddChangeListener(func(c Change) { a = c.Version })
	removeB := eb.AddChangeListener(func(c Change) { b = c.Version })
	defer removeA()
	defer removeB()

	eb.InsertRegion(0, 0, Rows_{Row_("x")})
	if a != 1 || b != 1 {
		t.Fatalf("listeners saw versions %d and %d, want 1 and 1", a, b)
	}
}

func TestSaveListeners(t *testing.T) {
	eb := newLoaded(t, "abc")

	var a, b int
	removeA := eb.AddSaveListener(func() { a++ })
	removeB := eb.AddSaveListener(func() { b++ })

	eb.emitSaved()
	if a != 1 || b != 1 {
		t.Fatalf("after first save: a=%d b=%d, want 1 and 1", a, b)
	}

	removeA()
	eb.emitSaved()
	if a != 1 || b != 2 {
		t.Fatalf("after removing a: a=%d b=%d, want 1 and 2", a, b)
	}

	// A listener may remove itself while it is being notified.
	var removeSelf func()
	calls := 0
	removeSelf = eb.AddSaveListener(func() {
		calls++
		removeSelf()
	})
	eb.emitSaved()
	eb.emitSaved()
	if calls != 1 {
		t.Fatalf("self-removing listener called %d times, want 1", calls)
	}
	removeB()
}

