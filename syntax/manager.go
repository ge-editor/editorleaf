package syntax

import (
	"slices"
	"sync"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/editorleaf/highlight"
)

// This file manages syntax state per EditBuffer.
//
// Lifetime:
//
//	entry   : one per EditBuffer that has at least one subscribed pane (or
//	          a running Worker). It outlives panes and language changes and
//	          owns the sinks and the last published spans.
//	Worker  : at most one per entry; owns the parser and the syntax tree.
//	          Created when the buffer's lang.Mode has a registered Parser,
//	          replaced when EditBuffer.SetLangMode changes the language,
//	          stopped by Release (kill-buffer).
//	sink    : one per destination HighlightsLayer (normally a pane's
//	          buffer.Meta.HighlightLayer). Added/removed by Subscribe and
//	          the returned unsubscribe func when a pane is created,
//	          destroyed or switched to another buffer. A new sink receives
//	          the latest spans immediately, so no reparse is needed.
//
// Subscribe, Release and EditBuffer.SetLangMode must be called on the UI
// goroutine.

// sink is one destination of an entry's spans.
type sink struct {
	target   *highlight.HighlightsLayer
	priority int
	refs     int // number of Subscribe calls sharing this (target, priority)
}

// install replaces the spans at s.priority with spans.
func (s *sink) install(spans []highlight.Span) {
	clearLayer(s.target, s.priority)
	for i := range spans {
		sp := spans[i]
		sp.Priority = s.priority
		var st highlight.SpanTemplate = &sp
		s.target.Highlights(s.priority).AppendSpan(&st)
	}
}

// entry is the syntax state of one EditBuffer.
type entry struct {
	eb       *editbuffer.EditBuffer
	dispatch func(func())

	removeLangListener func()
	lang               string // normalized name the current worker was built for; "" if none (UI goroutine only)

	sinkMu sync.Mutex // guards worker, sinks and latest
	worker *Worker
	sinks  []*sink
	latest []highlight.Span // last published spans; replayed to new sinks
}

var registry = struct {
	sync.Mutex
	entries map[*editbuffer.EditBuffer]*entry
}{entries: map[*editbuffer.EditBuffer]*entry{}}

// Subscribe makes the syntax spans of eb appear in target at priority
// (typically a pane's e.meta.HighlightLayer and highlight.LayerSyntax).
//
// The parser is chosen by eb's lang.Mode name (see Register) and follows
// EditBuffer.SetLangMode: while the mode has no registered Parser nothing is
// published, and once one exists the existing subscriptions start receiving
// spans without subscribing again. Subscribers to the same buffer share one
// Worker, so a second pane showing the buffer, or a pane that switches back
// to it, does not create another parser.
//
// If spans have already been published they are installed into target
// before Subscribe returns; otherwise stale spans at that priority are
// cleared. Subscribing the same (target, priority) twice is reference
// counted: panes whose Meta share one HighlightLayer (Meta.DeepCopy copies
// the pointer) are written once, and the layer stays registered until both
// have unsubscribed.
//
// dispatch is called from the worker goroutine with a closure that updates
// the layers. It must enqueue the closure on the UI goroutine and must not
// block. All callers are expected to pass the same dispatch; the one passed
// when the buffer's entry is created is used for its whole life.
//
// unsubscribe removes this subscription only. It never stops the Worker
// (see Release) and is safe to call more than once. When the last
// subscription goes away and the buffer has no Worker, the buffer's entry is
// dropped.
func Subscribe(
	eb *editbuffer.EditBuffer,
	target *highlight.HighlightsLayer,
	priority int,
	dispatch func(func()),
) (unsubscribe func()) {
	if eb == nil || target == nil {
		return func() {}
	}

	registry.Lock()
	e := registry.entries[eb]
	if e == nil {
		e = &entry{eb: eb, dispatch: dispatch}
		registry.entries[eb] = e
		e.removeLangListener = eb.AddLangModeListener(e.syncWorker)
		e.syncWorker()
	}
	s := e.addSink(target, priority)
	registry.Unlock()

	var once sync.Once
	return func() { once.Do(func() { e.removeSink(s) }) }
}

// Release stops and forgets the syntax state of eb. Call it when the buffer
// itself goes away (kill-buffer), not when a pane stops showing it.
// Outstanding unsubscribe funcs stay valid and become no-ops.
func Release(eb *editbuffer.EditBuffer) {
	registry.Lock()
	e := registry.entries[eb]
	delete(registry.entries, eb)
	registry.Unlock()

	if e != nil {
		e.destroy()
	}
}

// syncWorker makes e.worker match the buffer's current lang.Mode. It runs
// when the entry is created and on every EditBuffer.SetLangMode.
func (e *entry) syncWorker() {
	name := normalizeLangName(langNameOf(e.eb))
	factory, ok := Lookup(name)
	if !ok {
		name = ""
	}
	if name == e.lang {
		return
	}
	e.lang = name

	// The old worker (and the spans it published) belongs to the previous
	// language. Detach it first, and wait for it outside sinkMu: its
	// goroutine may be inside setSpans.
	old := e.detachWorker()
	if old != nil {
		old.stop()
	}

	if !ok {
		return
	}
	w := newWorker(e, factory)
	e.sinkMu.Lock()
	e.worker = w
	e.sinkMu.Unlock()
	w.start()
}

// detachWorker unlinks the current worker (if any) and clears what it
// published from all sinks. The caller stops the returned worker.
func (e *entry) detachWorker() *Worker {
	e.sinkMu.Lock()
	defer e.sinkMu.Unlock()

	old := e.worker
	e.worker = nil
	if old != nil {
		e.latest = nil
		for _, s := range e.sinks {
			s.install(nil)
		}
	}
	return old
}

// destroy implements Release.
func (e *entry) destroy() {
	e.removeLangListener()
	old := e.detachWorker()
	if old != nil {
		old.stop()
	}
	e.sinkMu.Lock()
	e.sinks = nil
	e.sinkMu.Unlock()
}

func (e *entry) addSink(target *highlight.HighlightsLayer, priority int) *sink {
	e.sinkMu.Lock()
	defer e.sinkMu.Unlock()

	for _, s := range e.sinks {
		if s.target == target && s.priority == priority {
			s.refs++
			return s // already current
		}
	}

	s := &sink{target: target, priority: priority, refs: 1}
	e.sinks = append(e.sinks, s)
	s.install(e.latest) // replays the last spans; clears stale ones if none
	return s
}

func (e *entry) removeSink(s *sink) {
	registry.Lock()
	defer registry.Unlock()

	e.sinkMu.Lock()
	s.refs--
	if s.refs <= 0 {
		e.sinks = slices.DeleteFunc(e.sinks, func(x *sink) bool { return x == s })
	}
	idle := len(e.sinks) == 0 && e.worker == nil
	e.sinkMu.Unlock()

	// Nobody shows this buffer and there is nothing to keep warm.
	if idle && registry.entries[e.eb] == e {
		delete(registry.entries, e.eb)
		e.removeLangListener()
	}
}

// setSpans records spans as the latest result of w and installs them into
// every sink. It runs on the UI goroutine (it is the body of the closures
// the Worker gives to dispatch). Spans of a Worker that is no longer the
// entry's current one are dropped. nil spans clear the sinks.
func (e *entry) setSpans(w *Worker, spans []highlight.Span) {
	e.sinkMu.Lock()
	defer e.sinkMu.Unlock()

	if e.worker != w {
		return
	}
	e.latest = spans
	for _, s := range e.sinks {
		s.install(spans)
	}
}

func langNameOf(eb *editbuffer.EditBuffer) string {
	m := eb.GetLangMode()
	if m == nil || *m == nil {
		return ""
	}
	return (*m).Name()
}
