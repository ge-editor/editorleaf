package editorleaf

import (
	"sync"

	"github.com/gdamore/tcell/v3"

	"github.com/ge-editor/editorleaf/highlight"
	"github.com/ge-editor/editorleaf/syntax"
	"github.com/ge-editor/gecore/screen"
	"github.com/ge-editor/gelog"
)

func (e *Editorleaf) InitTreesitter() {
	// Idempotent: e.editBuffer can be reassigned after this Editorleaf
	// was created (open-file into an existing pane, buffer switch, or a
	// new sibling pane cloning its neighbor's buffer -- see
	// editorleaf/vcommand2.go, editorleaf.go's buffer-set replace path,
	// and leaftype.go's NewSiblingLeaf). Detach any previous worker
	// first so re-calling Init for the same pane never attaches two.
	e.WillCloseTreesitter()

	langMode := e.editBuffer.GetLangMode()
	if langMode == nil || *langMode == nil {
		return // no file / no matching lang.Mode: nothing to highlight
	}

	detach, ok := syntax.Attach(
		e.editBuffer,
		(*langMode).Name(),
		e.highlightLayer,
		highlight.LayerSyntax,
		dispatchUI,
	)
	if ok {
		e.syntaxDetach = detach
	} else {
		// Expected for most lang.Modes today (only Go has a registered
		// syntax.Parser); logged at Debug so it doesn't look like an
		// error, but is visible if you're trying to figure out why a
		// given file isn't getting syntax highlighting.
		gelog.Debug("syntax.Attach: no Parser registered", "lang", (*langMode).Name())
	}
}

func (e *Editorleaf) WillCloseTreesitter() {
	if e.syntaxDetach != nil {
		e.syntaxDetach()
		e.syntaxDetach = nil
	}
}

// dispatchUI queues worker-owned highlight updates on the UI event loop.
// The event loop applies the closure before starting its normal redraw.
var syntaxUIUpdates struct {
	sync.Mutex
	queue []func()
}

func dispatchUI(fn func()) {
	syntaxUIUpdates.Lock()
	syntaxUIUpdates.queue = append(syntaxUIUpdates.queue, fn)
	syntaxUIUpdates.Unlock()

	// tcell/v3 injects events by sending to EventQ. Keep this non-blocking:
	// if the channel is full, the queued update will be drained on the next
	// event already waiting in the queue.
	postSyntaxUpdateWake()
}

func postSyntaxUpdateWake() {
	// EventQ is closed by Fini. A worker finishing as the app shuts down
	// must not panic while trying to schedule its final UI update.
	defer func() { _ = recover() }()
	select {
	// case screen.Get().EventQ() <- tcell.NewEventInterrupt("ge-syntax-update"):
	// Redraw on resize event
	case screen.Get().EventQ() <- tcell.NewEventResize(screen.Get().Size()):
	default:
	}
}

// ApplyPendingSyntaxUpdates runs queued highlight-layer changes on the UI
// event loop. Call before handling an event so the following draw sees them.
func ApplyPendingSyntaxUpdates() {
	syntaxUIUpdates.Lock()
	updates := syntaxUIUpdates.queue
	syntaxUIUpdates.queue = nil
	syntaxUIUpdates.Unlock()

	for _, update := range updates {
		update()
	}
}
