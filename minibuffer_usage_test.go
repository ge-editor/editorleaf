package editorleaf

import (
	"slices"
	"testing"

	"github.com/ge-editor/gecore"
)

// A session with a usage shows it while it is open and takes it away when it
// ends; a session without one leaves the echo line alone.
func TestMinibufferSessionUsage(t *testing.T) {
	gecore.Echo.ClearUsage()
	m := &MinibufferManagerStruct{}
	withUsage := &Session{Usage: []string{"Mark", "C-g:quit"}}

	m.session = withUsage
	m.showUsage()
	if got := gecore.Echo.Usage(); !slices.Equal(got, withUsage.Usage) {
		t.Fatalf("usage = %v", got)
	}

	m.clearUsage()
	if got := gecore.Echo.Usage(); len(got) != 0 {
		t.Fatalf("usage stays after the session ended: %v", got)
	}

	// A session without a usage does not touch the usage of a mode under it.
	gecore.Echo.SetUsage("MultiCursor", "C-g:quit")
	m.session = &Session{}
	m.showUsage()
	m.clearUsage()
	if got := gecore.Echo.Usage(); !slices.Equal(got, []string{"MultiCursor", "C-g:quit"}) {
		t.Fatalf("the usage of the mode below was changed: %v", got)
	}
	gecore.Echo.ClearUsage()

	// No session (Cancel after Close): nothing to do.
	m.session = nil
	m.showUsage()
	m.clearUsage()
}
