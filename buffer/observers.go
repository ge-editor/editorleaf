package buffer

import "github.com/ge-editor/editorleaf/editbuffer"

// Observers of the buffer collection.
//
// editorleaf sits below the packages that know about language servers
// (language depends on editorleaf, not the other way round), so it cannot
// call them. Instead it announces when a buffer appears in or leaves
// BufferSets, and the application (package ge) subscribes, the same
// direction of dependency as syntax.Register.
//
// Both callbacks run on the UI goroutine, synchronously inside Append /
// RemoveByBufferFile, so they must be quick and must not modify BufferSets.

var (
	appendObservers []func(*editbuffer.EditBuffer)
	removeObservers []func(*editbuffer.EditBuffer)
)

// OnAppend registers fn to be called after a buffer was added to BufferSets.
// The buffer has already been loaded: its rows hold the file's content.
// Call it from init() or main(), before buffers are created.
func OnAppend(fn func(*editbuffer.EditBuffer)) {
	appendObservers = append(appendObservers, fn)
}

// OnRemove registers fn to be called just before a buffer is removed from
// BufferSets (killed). The buffer is still intact.
func OnRemove(fn func(*editbuffer.EditBuffer)) {
	removeObservers = append(removeObservers, fn)
}

func notifyAppend(eb *editbuffer.EditBuffer) {
	for _, fn := range appendObservers {
		fn(eb)
	}
}

func notifyRemove(eb *editbuffer.EditBuffer) {
	for _, fn := range removeObservers {
		fn(eb)
	}
}
