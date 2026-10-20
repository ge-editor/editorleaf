package editorleaf

import (
	"sync"

	"github.com/gdamore/tcell/v3"

	"github.com/ge-editor/editorleaf/buffer"
	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/editorleaf/highlight"
	"github.com/ge-editor/editorleaf/syntax"
	"github.com/ge-editor/gecore/screen"
)

func (e *Editorleaf) InitTreesitter() {
	// Idempotent: e.editBuffer can be reassigned after this Editorleaf
	// was created (open-file into an existing pane, buffer switch, or a
	// new sibling pane cloning its neighbor's buffer -- see
	// editorleaf/vcommand2.go, editorleaf.go's buffer-set replace path,
	// and leaftype.go's NewSiblingLeaf). Unsubscribe from any previous
	// buffer first so re-calling Init for the same pane never subscribes
	// twice. Only this pane's HighlightLayer is (un)registered; the
	// syntax.Worker (parser + tree) belongs to the EditBuffer and is
	// reused, so none of this reparses anything.
	e.WillCloseTreesitter()

	e.syntaxUnsubscribe = syntax.Subscribe(
		e.editBuffer,
		e.meta.HighlightLayer,
		highlight.LayerSyntax,
		dispatchUI,
	)
}

// setBuffer is the only place that may replace e.editBuffer / e.meta.
//
// Everything derived from the pair must change together:
//  1. the subscription of e.meta.HighlightLayer to the old buffer's syntax
//     Worker is dropped BEFORE the swap (the Worker keeps running);
//  2. the boundary cache (bsArray) belongs to the old rows;
//  3. e.meta.HighlightLayer is subscribed to the new buffer's Worker AFTER
//     the swap (created on first use, otherwise its current spans are
//     installed immediately).
//
// Callers that need to save the old meta (PushMeta) must do so before
// calling setBuffer. m must not be nil.
func (e *Editorleaf) setBuffer(eb *editbuffer.EditBuffer, m *buffer.Meta) {
	e.WillCloseTreesitter()
	e.editBuffer, e.meta = eb, m
	e.bsArray.ClearAll()
	e.InitTreesitter()
}

// WillCloseTreesitter drops this pane's subscription. It does not stop the
// buffer's Worker: that is done by syntax.Release when the buffer is killed.
func (e *Editorleaf) WillCloseTreesitter() {
	if e.syntaxUnsubscribe != nil {
		e.syntaxUnsubscribe()
		e.syntaxUnsubscribe = nil
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

/* func postSyntaxUpdateWake() {
	// EventQ is closed by Fini. A worker finishing as the app shuts down
	// must not panic while trying to schedule its final UI update.
	defer func() { _ = recover() }()
	select {
	// case screen.Get().EventQ() <- tcell.NewEventInterrupt("ge-syntax-update"):
	// Redraw on resize event
	case screen.Get().EventQ() <- tcell.NewEventResize(screen.Get().Size()):
	default:
	}
} */

func postSyntaxUpdateWake() {
	defer func() { _ = recover() }()

	q := screen.Get().EventQ()

	select {
	case q <- tcell.NewEventResize(screen.Get().Size()):
	default:
	}

	// debug 用
	// syntax highlighting の完了を通知する
	select {
	case q <- tcell.NewEventInterrupt("ge-syntax-update"):
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
