// worker_a.go
// 2026-10-03 Sat

package syntax

import (
	"context"
	"sync"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/editorleaf/highlight"
)

// Worker drives one Parser from the changes of one EditBuffer and publishes
// spans to its owning entry (see manager.go).
//
// Ownership and threading:
//   - At most one Worker per EditBuffer, owned by its entry. The entry
//     manages worker lifetime as the buffer's language mode changes.
//   - onChange runs on the UI goroutine (it is called synchronously from
//     EditBuffer.emit) and must never block. It only takes a short-lived
//     mutex and appends to a slice / clones rows.Rows; it never touches
//     the Parser.
//   - run is the only goroutine that calls Parser.Parse/Edit/Spans and
//     that reads/writes li and src. It never touches EditBuffer directly.
//   - Published spans are handed to the entry by closures passed to
//     dispatch, i.e. on the UI goroutine, never from run().
type Worker struct {
	eb       *editbuffer.EditBuffer
	parser   Parser
	out      *entry
	dispatch func(func())

	removeListener func() // unregisters onChange from eb

	wake chan struct{}
	quit chan struct{}
	done chan struct{}

	mu           sync.Mutex
	pending      []editbuffer.Change
	needsReset   bool
	snapshot     editbuffer.Rows_ // valid when needsReset
	activeCancel context.CancelFunc
	stopOnce     sync.Once

	// Owned by run() only.
	li  *editbuffer.LineIndex
	src []byte

	// fullParse is set when a Parse (after a reset) was cancelled, so the
	// parser's tree does not describe src. Until a Parse completes, changes
	// only update li/src and the parser is fully reparsed at the end of the
	// batch instead of receiving incremental Edit calls.
	fullParse bool

	// desynced is set when a Change could not be applied to li (see
	// applyChange). It means li/src no longer describe the buffer.
	// Rather than guessing, the worker clears the highlight layer and
	// waits for the next ChangeReset, which resnapshots from scratch.
	desynced bool
}

// newWorker builds a Worker for e's buffer. It is not running yet: the
// caller stores it in e.worker first and then calls start.
func newWorker(e *entry, factory Factory) *Worker {
	return &Worker{
		eb:       e.eb,
		parser:   factory(),
		out:      e,
		dispatch: e.dispatch,
		wake:     make(chan struct{}, 1),
		quit:     make(chan struct{}),
		done:     make(chan struct{}),
		li:       editbuffer.NewLineIndex(*e.eb.Rows()),
	}
}

// start registers the change listener, launches the goroutine and primes
// the parser with the current content. Call on the UI goroutine.
func (w *Worker) start() {
	w.src = []byte(w.eb.Rows().JoinString([]byte{'\n'}))
	w.removeListener = w.eb.AddChangeListener(w.onChange)

	go w.run()

	// Prime the parser with the current content, the same way a
	// ChangeReset would. This runs on the UI goroutine before the worker
	// has done anything, which is fine: run() has not touched
	// w.src/w.li yet, and onChange cannot fire before AddChangeListener
	// returned above, so there is no race with it.
	w.mu.Lock()
	w.needsReset = true
	w.snapshot = w.eb.Rows().CloneRows()
	w.mu.Unlock()
	w.wakeUp()
}

// stop ends the worker goroutine and closes the parser. Do not call while
// holding entry.sinkMu: run() may be inside setSpans.
func (w *Worker) stop() {
	w.stopOnce.Do(func() {
		w.removeListener()
		w.mu.Lock()
		if w.activeCancel != nil {
			w.activeCancel()
		}
		close(w.quit)
		w.mu.Unlock()
	})
	<-w.done
}

func (w *Worker) wakeUp() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// onChange is the EditBuffer change listener. See the threading note on
// Worker: this runs on the UI goroutine and must be fast and non-blocking.
func (w *Worker) onChange(c editbuffer.Change) {
	// gelog.Debug("syntax", "onChange", c)

	w.mu.Lock()
	// Cancel the parse currently in flight. Doing this while holding the
	// same mutex used by run() to publish activeCancel closes the race
	// between a new edit and starting a parse for an older snapshot.
	if w.activeCancel != nil && !dbgNoCancel { // GE_TS_NOCANCEL
		w.activeCancel()
	}
	switch {
	case c.Kind == editbuffer.ChangeReset:
		w.needsReset = true
		w.snapshot = w.eb.Rows().CloneRows()
		w.pending = w.pending[:0]

	case w.needsReset:
		// A reset is already queued and the worker has not drained it
		// yet. Re-snapshot instead of trying to replay this edit on
		// top of a snapshot the worker has not applied yet; Reset is
		// rare (reload, format-on-save), so this is not a hot path.
		w.snapshot = w.eb.Rows().CloneRows()

	default:
		w.pending = append(w.pending, c)
	}
	w.mu.Unlock()

	w.wakeUp()
}

func (w *Worker) run() {
	defer close(w.done)
	defer w.parser.Close()

	for {
		select {
		case <-w.quit:
			return
		case <-w.wake:
		}

		// GE_TS_DEBOUNCE_MS: let a burst of edits settle first.
		if dbgDebounce > 0 && !w.waitQuiet() {
			return
		}

		w.mu.Lock()
		select {
		case <-w.quit:
			w.mu.Unlock()
			return
		default:
		}
		reset, snapshot := w.needsReset, w.snapshot
		changes := w.pending
		w.needsReset, w.snapshot, w.pending = false, nil, nil
		ctx, cancel := context.WithCancel(context.Background())
		w.activeCancel = cancel
		w.mu.Unlock()

		finish := func() {
			cancel()
			w.mu.Lock()
			w.activeCancel = nil
			w.mu.Unlock()
		}

		// --- Reset: rebuild everything from the snapshot ---
		if reset {
			w.li.Reset(snapshot)
			w.src = []byte(snapshot.JoinString([]byte{'\n'}))
			w.desynced = false
			w.fullParse = false

			w.parser.Parse(ctx, w.src)
			canceled := ctx.Err() != nil
			finish()
			if canceled {
				// The parser's tree does not match src. Changes that are
				// already pending (or arrive later) are applied to li/src
				// only, then the parser reparses from scratch.
				w.fullParse = true
				w.wakeUp()
				continue
			}
			w.publish()
			continue
		}

		if w.desynced || (len(changes) == 0 && !w.fullParse) {
			finish()
			continue
		}

		// --- Incremental: apply changes[0:next] ---
		ok := true
		next := 0 // index of the first change that has NOT been applied
		for next < len(changes) {
			if ctx.Err() != nil {
				break
			}
			if !w.applyChange(ctx, changes[next]) {
				ok = false
				break
			}
			// Applied to li/src (and the parser's tree) even if ctx was
			// cancelled while the parser was reparsing, so count it.
			next++
		}

		canceled := ctx.Err() != nil

		// Recovering from a cancelled reset parse: all changes were applied
		// to li/src only, so reparse the full source once.
		if ok && w.fullParse && next == len(changes) && !canceled {
			w.parser.Parse(ctx, w.src)
			canceled = ctx.Err() != nil
			if !canceled {
				w.fullParse = false
			}
		}

		finish()

		// Cancelled mid-batch: put the unapplied changes back in front of
		// anything that arrived meanwhile, instead of dropping them.
		if ok && next < len(changes) {
			w.requeue(changes[next:])
			continue
		}

		// On desync, applyChange already cleared the layer; publishing here
		// would immediately refill it with stale spans.
		if ok && !canceled && !w.fullParse {
			w.publish()
		}
	}
}

// requeue puts changes that were taken from pending but not applied back at
// the front of the pending list, preserving order.
func (w *Worker) requeue(rest []editbuffer.Change) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// A reset arrived meanwhile: its snapshot already reflects every change,
	// so replaying the old ones would corrupt li/src.
	if w.needsReset {
		return
	}

	merged := make([]editbuffer.Change, 0, len(rest)+len(w.pending))
	merged = append(merged, rest...)
	merged = append(merged, w.pending...)
	w.pending = merged

	w.wakeUp()
}

/* func (w *Worker) run() {
	defer close(w.done)
	defer w.parser.Close()

	for {
		select {
		case <-w.quit:
			return
		case <-w.wake:
		}

		w.mu.Lock()
		select {
		case <-w.quit:
			w.mu.Unlock()
			return
		default:
		}
		reset, snapshot := w.needsReset, w.snapshot
		changes := w.pending
		w.needsReset, w.snapshot, w.pending = false, nil, nil
		ctx, cancel := context.WithCancel(context.Background())
		w.activeCancel = cancel
		w.mu.Unlock()
		deferCancel := func() {
			cancel()
			w.mu.Lock()
			if w.activeCancel != nil {
				w.activeCancel = nil
			}
			w.mu.Unlock()
		}

		if reset {
			w.li.Reset(snapshot)
			w.src = []byte(snapshot.JoinString([]byte{'\n'}))
			w.desynced = false

			w.parser.Parse(ctx, w.src)
			canceled := ctx.Err() != nil
			deferCancel()
			if !canceled {
				w.publish()
			}
			continue
		}

		if w.desynced || len(changes) == 0 {
			deferCancel()
			continue
		}

		ok := true
		for _, c := range changes {
			if ctx.Err() != nil {
				break
			}
			if !w.applyChange(ctx, c) {
				ok = false
				break
			}
		}
		// On desync, applyChange already cleared the layer; publishing
		// here would immediately refill it with the parser's now-stale
		// spans, undoing that.
		canceled := ctx.Err() != nil
		deferCancel()
		if ok && !canceled {
			w.publish()
		}
	}
} */

// applyChange updates li/src for one change and feeds it to the parser.
// It returns false if the change could not be applied (li is out of sync
// with the buffer); the caller stops processing the rest of the batch and
// waits for a ChangeReset to recover. See the desynced field comment.
/* func (w *Worker) applyChange(ctx context.Context, c editbuffer.Change) bool {
	e, ok := w.li.Apply(c)
	if !ok {
		w.desynced = true
		w.dispatchUI(func() {
			clearLayer(w.target, w.priority) // avoid showing highlighting for a now-wrong tree
		})
		return false
	}

	w.src = spliceBytes(w.src, e.StartByte, e.OldEndByte, e.NewEndByte, c.TextLF())
	w.parser.Edit(ctx, toSyntaxEdit(e), w.src)
	return true
} */
func (w *Worker) applyChange(ctx context.Context, c editbuffer.Change) bool {
	e, ok := w.li.Apply(c)
	if !ok {
		w.desynced = true
		// avoid showing highlighting for a now-wrong tree
		w.dispatchUI(func() { w.out.setSpans(w, nil) })
		return false
	}

	w.src = spliceBytes(w.src, e.StartByte, e.OldEndByte, e.NewEndByte, c.TextLF())

	// The parser's tree is stale (a reset Parse was cancelled): skip the
	// incremental edit; run() reparses the whole source afterwards.
	if w.fullParse {
		return true
	}

	w.parser.Edit(ctx, toSyntaxEdit(e), w.src)
	return true
}

// publish copies the parser's current spans and schedules their installation
// into every sink on the UI goroutine (see setSpans).
func (w *Worker) publish() {
	spans := append([]highlight.Span(nil), w.parser.Spans()...)
	w.dispatchUI(func() { w.out.setSpans(w, spans) })
}

func (w *Worker) dispatchUI(fn func()) {
	if w.dispatch != nil {
		w.dispatch(fn)
		return
	}
	fn()
}

// clearLayer clears priority's spans, without the panic
// highlight.HighlightsLayer.Clear would raise if that priority slot has
// never been written to yet (its backing slice starts at length 0).
func clearLayer(h *highlight.HighlightsLayer, priority int) {
	if priority < len(*h) {
		// h.Clear(priority)
		h.Highlights(priority).Clear()
	}
}

func toSyntaxEdit(e editbuffer.Edit) Edit {
	return Edit{
		StartByte: e.StartByte, OldEndByte: e.OldEndByte, NewEndByte: e.NewEndByte,
		StartRow: e.StartPoint.Row, StartCol: e.StartPoint.Column,
		OldEndRow: e.OldEndPoint.Row, OldEndCol: e.OldEndPoint.Column,
		NewEndRow: e.NewEndPoint.Row, NewEndCol: e.NewEndPoint.Column,
	}
}

// spliceBytes returns a new slice equal to
// src[:startByte] + repl + src[oldEndByte:].
// newEndByte is only used to preallocate the right capacity.
func spliceBytes(src []byte, startByte, oldEndByte, newEndByte int, repl []byte) []byte {
	out := make([]byte, 0, newEndByte+(len(src)-oldEndByte))
	out = append(out, src[:startByte]...)
	out = append(out, repl...)
	out = append(out, src[oldEndByte:]...)
	return out
}
