// Package diagnostic keeps the diagnostics (errors, warnings, ...) of each
// EditBuffer and shows them through the highlight layers of the panes.
//
// It is the counterpart of package syntax for a second kind of buffer
// information. It knows nothing about language servers: whoever produces
// diagnostics (the application, connecting a language server) calls Set, and
// editorleaf panes call Subscribe, the same direction of dependency as
// syntax.Subscribe.
//
// All functions must be called on the UI goroutine. Writing into a
// HighlightsLayer is safe against the draw goroutine reading it, because
// Highlights replaces its spans as a whole under its own lock.
package diagnostic

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/gdamore/tcell/v3"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/editorleaf/highlight"
	"github.com/ge-editor/theme"
)

// Severity uses the LSP numbering, so that a smaller value is more severe.
type Severity int

const (
	Error       Severity = 1
	Warning     Severity = 2
	Information Severity = 3
	Hint        Severity = 4
)

// Item is one diagnostic. Start and End are (row, byte column); End is
// exclusive. Start == End is allowed (a point) and is shown over one byte.
type Item struct {
	Start, End editbuffer.RowsPos
	Severity   Severity
	Message    string
	Source     string
	Code       string
}

// Styles of the diagnostic spans. A span replaces the style of the text it
// covers (the highlight layers do not merge), so each style carries a
// foreground and a background that stand out on the default theme. They are
// variables so that a theme can change them.
var (
	StyleError       = theme.ColorDefault.Foreground(tcell.NewRGBColor(255, 120, 120)).Background(tcell.NewRGBColor(72, 28, 28))
	StyleWarning     = theme.ColorDefault.Foreground(tcell.NewRGBColor(235, 195, 80)).Background(tcell.NewRGBColor(62, 52, 20))
	StyleInformation = theme.ColorDefault.Foreground(tcell.NewRGBColor(120, 175, 235)).Background(tcell.NewRGBColor(28, 44, 66))
	StyleHint        = theme.ColorDefault.Foreground(tcell.NewRGBColor(150, 150, 150)).Background(tcell.NewRGBColor(44, 44, 44))
)

func styleOf(s Severity) tcell.Style {
	switch s {
	case Error:
		return StyleError
	case Warning:
		return StyleWarning
	case Information:
		return StyleInformation
	default:
		return StyleHint
	}
}

// sink is one destination layer (normally a pane's HighlightLayer).
type sink struct {
	target   *highlight.HighlightsLayer
	priority int
	refs     int // Subscribe calls sharing this (target, priority)
}

// install replaces the spans at s.priority. The whole slice is built first
// and swapped in at once, so the draw goroutine never sees it half filled.
func (s *sink) install(spans []highlight.Span) {
	if len(spans) == 0 {
		s.target.Highlights(s.priority).Clear()
		return
	}
	templates := make([]*highlight.SpanTemplate, len(spans))
	for i := range spans {
		sp := spans[i]
		sp.Priority = s.priority
		var st highlight.SpanTemplate = &sp
		templates[i] = &st
	}
	s.target.Highlights(s.priority).SetSpans(templates)
}

// entry is the diagnostic state of one EditBuffer.
type entry struct {
	eb    *editbuffer.EditBuffer
	sinks []*sink
	items []Item           // sorted by Start, then severity
	spans []highlight.Span // items flattened for drawing

	// removeListener unregisters the change listener that keeps items on
	// their text while the buffer is edited (see onChange). It exists only
	// while there are items.
	removeListener func()
}

// The registry is only touched on the UI goroutine; the mutex only guards
// against a misuse turning into a corrupted map.
var registry = struct {
	sync.Mutex
	entries map[*editbuffer.EditBuffer]*entry
}{entries: map[*editbuffer.EditBuffer]*entry{}}

func entryOf(eb *editbuffer.EditBuffer, create bool) *entry {
	e := registry.entries[eb]
	if e == nil && create {
		e = &entry{eb: eb}
		registry.entries[eb] = e
	}
	return e
}

// Subscribe makes the diagnostics of eb appear in target at priority
// (a pane's e.meta.HighlightLayer and highlight.LayerDiagnostic). The
// diagnostics already known are installed before Subscribe returns, and
// later calls of Set update target automatically.
//
// Subscribing the same (target, priority) twice is reference counted, like
// syntax.Subscribe. unsubscribe removes this subscription only, and may be
// called more than once. The diagnostics themselves stay with the buffer
// (a pane that switches away and back sees them again) until Release.
func Subscribe(eb *editbuffer.EditBuffer, target *highlight.HighlightsLayer, priority int) (unsubscribe func()) {
	if eb == nil || target == nil {
		return func() {}
	}

	registry.Lock()
	defer registry.Unlock()

	e := entryOf(eb, true)

	var s *sink
	for _, x := range e.sinks {
		if x.target == target && x.priority == priority {
			s = x
			break
		}
	}
	if s != nil {
		s.refs++
	} else {
		s = &sink{target: target, priority: priority, refs: 1}
		e.sinks = append(e.sinks, s)
		s.install(e.spans) // replays the current spans; clears stale ones if none
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			registry.Lock()
			defer registry.Unlock()
			s.refs--
			if s.refs <= 0 {
				e.sinks = slices.DeleteFunc(e.sinks, func(x *sink) bool { return x == s })
			}
		})
	}
}

// Set replaces all diagnostics of eb. nil or an empty slice clears them.
// items is copied.
func Set(eb *editbuffer.EditBuffer, items []Item) {
	if eb == nil {
		return
	}

	registry.Lock()
	defer registry.Unlock()

	e := entryOf(eb, len(items) > 0)
	if e == nil {
		return // nothing stored and nothing to store
	}

	e.items = slices.Clone(items)
	slices.SortStableFunc(e.items, func(a, b Item) int {
		if c := cmpPos(a.Start, b.Start); c != 0 {
			return c
		}
		return int(a.Severity) - int(b.Severity)
	})
	e.spans = spansOf(e.items)

	for _, s := range e.sinks {
		s.install(e.spans)
	}
	e.syncListener()
}

// Items returns the diagnostics of eb, sorted by position. The caller must
// not modify the result.
func Items(eb *editbuffer.EditBuffer) []Item {
	registry.Lock()
	defer registry.Unlock()

	if e := entryOf(eb, false); e != nil {
		return e.items
	}
	return nil
}

// At returns the diagnostics that cover pos (a point diagnostic covers its
// own position), the most severe first. It is meant for showing the message
// of what is under the cursor.
func At(eb *editbuffer.EditBuffer, pos editbuffer.RowsPos) []Item {
	var out []Item
	for _, it := range Items(eb) {
		end := it.End
		if !before(it.Start, end) {
			end = editbuffer.RowsPos{RowIndex: it.Start.RowIndex, ColIndex: it.Start.ColIndex + 1}
		}
		if !before(pos, it.Start) && before(pos, end) {
			out = append(out, it)
		}
	}
	slices.SortStableFunc(out, func(a, b Item) int { return int(a.Severity) - int(b.Severity) })
	return out
}

// Release forgets everything about eb. Call it when the buffer is killed,
// not when a pane stops showing it.
func Release(eb *editbuffer.EditBuffer) {
	registry.Lock()
	e := entryOf(eb, false)
	delete(registry.entries, eb)
	registry.Unlock()

	if e != nil {
		if e.removeListener != nil {
			e.removeListener()
			e.removeListener = nil
		}
		for _, s := range e.sinks {
			s.install(nil)
		}
	}
}

// ---- flattening ----

func before(a, b editbuffer.RowsPos) bool {
	return a.RowIndex < b.RowIndex || (a.RowIndex == b.RowIndex && a.ColIndex < b.ColIndex)
}

func cmpPos(a, b editbuffer.RowsPos) int {
	switch {
	case before(a, b):
		return -1
	case before(b, a):
		return 1
	}
	return 0
}

// spansOf turns possibly overlapping items into sorted, non-overlapping
// spans. A highlight slot is searched by position and returns the first
// span that contains it, so overlaps would hide later spans; where items
// overlap, the most severe one wins (the earlier one among equals).
//
// A point item is widened to one byte, otherwise it would cover no
// position at all.
func spansOf(items []Item) []highlight.Span {
	type norm struct {
		start, end editbuffer.RowsPos
		sev        Severity
		idx        int
	}

	ns := make([]norm, 0, len(items))
	for i, it := range items {
		end := it.End
		if !before(it.Start, end) {
			end = editbuffer.RowsPos{RowIndex: it.Start.RowIndex, ColIndex: it.Start.ColIndex + 1}
		}
		ns = append(ns, norm{start: it.Start, end: end, sev: it.Severity, idx: i})
	}
	slices.SortStableFunc(ns, func(a, b norm) int { return cmpPos(a.start, b.start) })

	bounds := make([]editbuffer.RowsPos, 0, 2*len(ns))
	for _, n := range ns {
		bounds = append(bounds, n.start, n.end)
	}
	slices.SortFunc(bounds, cmpPos)
	bounds = slices.Compact(bounds)

	var (
		out     []highlight.Span
		active  []norm
		next    int
		lastIdx = -1
	)
	for i := 0; i+1 < len(bounds); i++ {
		a, b := bounds[i], bounds[i+1]

		for next < len(ns) && !before(a, ns[next].start) { // start <= a
			active = append(active, ns[next])
			next++
		}
		active = slices.DeleteFunc(active, func(n norm) bool { return !before(a, n.end) }) // end <= a
		if len(active) == 0 {
			continue
		}

		best := active[0]
		for _, n := range active[1:] {
			if n.sev < best.sev {
				best = n
			}
		}

		if last := len(out) - 1; last >= 0 && lastIdx == best.idx && out[last].End == a {
			out[last].End = b // the same item continues
			continue
		}
		style := styleOf(best.sev)
		out = append(out, highlight.Span{
			Start:         a,
			End:           b,
			Color:         style,
			ColorIfActive: style.Bold(true),
		})
		lastIdx = best.idx
	}
	return out
}

// Summary describes the first of items (At returns the most severe first)
// in one line for the echo area:
//
//	error: undefined: x [compiler] (+1)
//
// Only the first line of the message is used, and (+n) says how many more
// diagnostics cover the same position. ok is false when items is empty.
func Summary(items []Item) (text string, severity Severity, ok bool) {
	if len(items) == 0 {
		return "", 0, false
	}

	first := items[0]
	msg, _, _ := strings.Cut(strings.TrimSpace(first.Message), "\n")

	text = label(first.Severity) + ": " + strings.TrimSpace(msg)
	if first.Source != "" {
		text += " [" + first.Source + "]"
	}
	if more := len(items) - 1; more > 0 {
		text += fmt.Sprintf(" (+%d)", more)
	}
	return text, first.Severity, true
}

func label(s Severity) string {
	switch s {
	case Error:
		return "error"
	case Warning:
		return "warning"
	case Information:
		return "info"
	default:
		return "hint"
	}
}

// ---- following edits ----
//
// A language server analyzes the text it was last sent and publishes a while
// later; in between the user keeps typing. Without help the highlighted
// ranges would stay at their old coordinates and slide off the text they
// describe. So while a buffer has diagnostics, every change moves them the
// way the text moved, until the server's next result replaces them.

// syncListener keeps the change listener registered exactly while there are
// items. The registry lock must be held.
func (e *entry) syncListener() {
	switch {
	case len(e.items) > 0 && e.removeListener == nil:
		e.removeListener = e.eb.AddChangeListener(e.onChange)
	case len(e.items) == 0 && e.removeListener != nil:
		e.removeListener()
		e.removeListener = nil
	}
}

// onChange is the buffer's change listener (UI goroutine, synchronous).
func (e *entry) onChange(c editbuffer.Change) {
	registry.Lock()
	defer registry.Unlock()

	if registry.entries[e.eb] != e {
		return // released
	}

	if c.Kind == editbuffer.ChangeReset {
		// The whole text was replaced (reload, save-time formatting):
		// positions mean nothing any more. The server will publish again.
		e.items = nil
	} else {
		e.items = shiftItems(e.items, c)
	}
	e.spans = spansOf(e.items)
	for _, s := range e.sinks {
		s.install(e.spans)
	}
	e.syncListener()
}

// shiftItems moves items through one Insert or Delete change. Items whose
// text was deleted entirely are dropped; point items are never dropped.
func shiftItems(items []Item, c editbuffer.Change) []Item {
	out := make([]Item, 0, len(items))
	for _, it := range items {
		wasPoint := !before(it.Start, it.End)

		start := shiftPos(it.Start, c, false)
		end := shiftPos(it.End, c, true)
		if before(end, start) {
			end = start
		}
		if !wasPoint && !before(start, end) {
			continue // nothing of it is left
		}

		it.Start, it.End = start, end
		out = append(out, it)
	}
	return out
}

// shiftPos maps a position in the text before c to the text after it.
//
// isEnd selects the side of an insertion exactly at p: text typed at the
// start of a range lands before it and pushes it along, text typed at the
// end of a range lands after it and leaves it alone.
func shiftPos(p editbuffer.RowsPos, c editbuffer.Change, isEnd bool) editbuffer.RowsPos {
	switch c.Kind {
	case editbuffer.ChangeInsert:
		if before(p, c.Start) || (isEnd && p == c.Start) {
			return p
		}
		if p.RowIndex == c.Start.RowIndex {
			// The rest of the line moves behind the inserted text.
			return editbuffer.RowsPos{
				RowIndex: c.NewEnd.RowIndex,
				ColIndex: c.NewEnd.ColIndex + (p.ColIndex - c.Start.ColIndex),
			}
		}
		return editbuffer.RowsPos{
			RowIndex: p.RowIndex + (c.NewEnd.RowIndex - c.Start.RowIndex),
			ColIndex: p.ColIndex,
		}

	case editbuffer.ChangeDelete:
		if !before(c.Start, p) {
			return p // at or before the deleted region
		}
		if before(p, c.OldEnd) {
			return c.Start // inside it
		}
		if p.RowIndex == c.OldEnd.RowIndex {
			return editbuffer.RowsPos{
				RowIndex: c.Start.RowIndex,
				ColIndex: c.Start.ColIndex + (p.ColIndex - c.OldEnd.ColIndex),
			}
		}
		return editbuffer.RowsPos{
			RowIndex: p.RowIndex - (c.OldEnd.RowIndex - c.Start.RowIndex),
			ColIndex: p.ColIndex,
		}
	}
	return p
}
