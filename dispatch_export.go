package editorleaf

// DispatchUI queues fn to run on the UI goroutine (the event loop applies
// it before the next redraw) and wakes the loop. It never blocks, and any
// goroutine may call it.
//
// It exports the mechanism the syntax workers already use, for the
// application's own background services such as the language servers.
func DispatchUI(fn func()) {
	dispatchUI(fn)
}
