package spell

import (
	"slices"
	"sync"
	"time"

	"github.com/gdamore/tcell/v3"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/editorleaf/highlight"
)

// Options of Enable.
type Options struct {
	// Dispatch runs a function on the UI goroutine. Required: the timer
	// that starts a check runs on another goroutine.
	Dispatch func(func())

	// FocusRow returns the row to check first (the cursor row), so that
	// what the user is looking at is checked before the rest of a big file.
	// nil starts at the top.
	FocusRow func() int

	// Delay is the time without an edit before the rows are checked.
	// Zero means DefaultDelay.
	Delay time.Duration

	// Scope tells which parts of the rows are prose (ForMode). nil: the
	// scope of the language name given to Enable (ScopeFor).
	Scope Scope
}

const (
	// DefaultDelay is the quiet time after an edit before a check.
	DefaultDelay = 300 * time.Millisecond

	// rowsPerStep rows are checked in one step on the UI goroutine; the
	// rest is left to the next step, so a big file does not block the UI.
	rowsPerStep = 400
)

// Style is the Color of the spans of highlight.LayerSpell. The editor paints
// an underline over the cells (it does not draw the style), so this only
// marks the spans.
var Style = tcell.StyleDefault

type rowState struct {
	ranges  []Range
	dirty   bool   // to be checked
	skipped bool   // not prose (see Scope.Skipped); never has ranges
	key     uint64 // Scope.Key when it was last looked at
}

// sink is one destination layer (normally a pane's HighlightLayer).
type sink struct {
	target   *highlight.HighlightsLayer
	priority int
	refs     int
}

// entry is the spell check state of one EditBuffer.
type entry struct {
	eb     *editbuffer.EditBuffer
	engine Engine
	scope  Scope
	opts   Options

	rows []rowState
	// dicts and warnings: what the engine loaded for the language mode (see
	// ModeEngine).
	dicts, warnings []string
	// scopeStale: the rows changed since Scope.Prepare was called.
	scopeStale     bool
	removeListener func()
	gen            int // the last edit; a timer of an older one does nothing
	closed         bool
}

// state is what is known of a buffer: its sinks (which may be there before
// the check is enabled) and its entry, if enabled.
type state struct {
	sinks   []*sink
	e       *entry
	started bool // see TakeStart
}

// The registry is only touched on the UI goroutine; the mutex only guards
// against a misuse turning into a corrupted map.
var registry = struct {
	sync.Mutex
	states map[*editbuffer.EditBuffer]*state
}{states: map[*editbuffer.EditBuffer]*state{}}

func stateOf(eb *editbuffer.EditBuffer, create bool) *state {
	s := registry.states[eb]
	if s == nil && create {
		s = &state{}
		registry.states[eb] = s
	}
	return s
}

// Subscribe makes the misspelled words of eb appear in target at priority
// (a pane's e.meta.HighlightLayer and highlight.LayerSpell). It may be called
// before Enable: the layer then fills when the check starts. Subscribing the
// same (target, priority) twice is reference counted; unsubscribe removes this
// subscription only and may be called more than once.
func Subscribe(eb *editbuffer.EditBuffer, target *highlight.HighlightsLayer, priority int) (unsubscribe func()) {
	if eb == nil || target == nil {
		return func() {}
	}

	registry.Lock()
	defer registry.Unlock()

	st := stateOf(eb, true)

	var s *sink
	for _, x := range st.sinks {
		if x.target == target && x.priority == priority {
			s = x
			break
		}
	}
	if s != nil {
		s.refs++
	} else {
		s = &sink{target: target, priority: priority, refs: 1}
		st.sinks = append(st.sinks, s)
		if st.e != nil {
			install(s, st.e.flat())
		} else {
			target.Highlights(priority).Clear()
		}
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			registry.Lock()
			defer registry.Unlock()
			s.refs--
			if s.refs <= 0 {
				st.sinks = slices.DeleteFunc(st.sinks, func(x *sink) bool { return x == s })
			}
		})
	}
}

// Enabled reports whether the check is on for eb.
func Enabled(eb *editbuffer.EditBuffer) bool {
	registry.Lock()
	defer registry.Unlock()
	st := registry.states[eb]
	return st != nil && st.e != nil
}

// Enable starts checking eb: langName is the lang.Mode of the buffer (it
// decides the Scope), lang the dictionary. It returns ErrNoEngine,
// ErrUnsupportedMode, or the error of the engine (no dictionary, ...).
func Enable(eb *editbuffer.EditBuffer, langName, lang string, opts Options) error {
	if eb == nil || opts.Dispatch == nil {
		return errNeedBuffer
	}
	if Enabled(eb) {
		return nil
	}
	scope := opts.Scope
	if scope == nil {
		var ok bool
		if scope, ok = ScopeFor(langName); !ok {
			return ErrUnsupportedMode
		}
	}
	if lang == "" {
		lang = DefaultLanguage
	}
	engine, err := open(lang)
	if err != nil {
		return err
	}
	if opts.Delay <= 0 {
		opts.Delay = DefaultDelay
	}

	e := &entry{eb: eb, engine: engine, scope: scope, opts: opts}
	if me, ok := engine.(ModeEngine); ok && langName != "" {
		e.dicts, e.warnings = me.LoadMode(langName)
	}
	e.resetRows(eb.Rows().Length())
	e.removeListener = eb.AddChangeListener(e.onChange)

	registry.Lock()
	stateOf(eb, true).e = e
	registry.Unlock()

	e.schedule(0)
	return nil
}

// Disable stops checking eb and removes the underlines.
func Disable(eb *editbuffer.EditBuffer) {
	registry.Lock()
	st := registry.states[eb]
	var e *entry
	if st != nil {
		e, st.e = st.e, nil
	}
	registry.Unlock()
	if e == nil {
		return
	}
	e.close()
	for _, s := range st.sinks {
		s.target.Highlights(s.priority).Clear()
	}
}

// Release forgets everything about eb. Called when the buffer is killed.
func Release(eb *editbuffer.EditBuffer) {
	Disable(eb)
	registry.Lock()
	delete(registry.states, eb)
	registry.Unlock()
}

func (e *entry) close() {
	if e.closed {
		return
	}
	e.closed = true
	e.removeListener()
	e.engine.Close()
}

func (e *entry) resetRows(n int) {
	e.scopeStale = true
	e.rows = make([]rowState, n)
	for i := range e.rows {
		e.rows[i].dirty = true
	}
}

// onChange keeps the rows aligned with the buffer. The rows an edit touched
// lose their ranges at once (their columns are not valid any more) and are
// checked again after the quiet time; the rows below only move.
func (e *entry) onChange(c editbuffer.Change) {
	if e.closed {
		return
	}
	n := e.eb.Rows().Length()

	if c.Kind == editbuffer.ChangeReset {
		e.resetRows(n)
	} else {
		start := c.Start.RowIndex
		oldN := c.OldEnd.RowIndex - start + 1
		newN := c.NewEnd.RowIndex - start + 1
		if start < 0 || oldN < 1 || newN < 1 || start+oldN > len(e.rows) {
			e.resetRows(n)
		} else {
			fresh := make([]rowState, newN)
			for i := range fresh {
				fresh[i].dirty = true
			}
			e.rows = slices.Replace(e.rows, start, start+oldN, fresh...)
			e.scopeStale = true
			if len(e.rows) != n {
				e.resetRows(n) // out of step: start again rather than guess
			}
		}
	}

	e.publish()
	e.schedule(e.opts.Delay)
}

// schedule starts a check after delay. A later edit makes an earlier timer
// do nothing, so the check runs after the quiet time, not during typing.
func (e *entry) schedule(delay time.Duration) {
	e.gen++
	g := e.gen
	time.AfterFunc(delay, func() {
		e.opts.Dispatch(func() {
			if !e.closed && e.gen == g {
				e.step()
			}
		})
	})
}

// step checks up to rowsPerStep dirty rows, from the focus row on, and
// schedules itself again if there are more.
func (e *entry) step() {
	rows := e.eb.Rows().BytesArray()
	if len(rows) != len(e.rows) {
		e.resetRows(len(rows))
	}

	// What is prose may have changed in rows that were not edited: a block
	// comment or a code block that was opened or closed above them. Those
	// rows are checked again.
	if e.scopeStale {
		e.scope.Prepare(rows)
		e.scopeStale = false
		for i := range e.rows {
			st := &e.rows[i]
			if sk, key := e.scope.Skipped(i), e.scope.Key(i); sk != st.skipped || key != st.key {
				st.skipped, st.key = sk, key
				st.dirty = true
			}
		}
	}

	n := len(e.rows)
	first := 0
	if e.opts.FocusRow != nil {
		first = min(max(e.opts.FocusRow(), 0), max(n-1, 0))
	}

	budget := rowsPerStep
	changed := false
	more := false
	for k := 0; k < n; k++ {
		i := (first + k) % n
		st := &e.rows[i]
		if !st.dirty {
			continue
		}
		if budget == 0 {
			more = true
			break
		}
		budget--
		st.dirty = false
		st.ranges = nil
		if !st.skipped && len(rows[i]) > 0 {
			st.ranges = e.check(i, rows[i])
		}
		changed = true
	}

	if changed {
		e.publish()
	}
	if more {
		e.schedule(time.Millisecond)
	}
}

// check returns the misspelled words of a row: those of the prose and, if the
// code is checked, those of the code.
func (e *entry) check(i int, text []byte) []Range {
	out := e.engine.Misspelled(e.scope.Mask(i, text))

	cs, ok := e.scope.(CodeScope)
	if !ok || !cs.CodeOn() {
		return out
	}
	ce, ok := e.engine.(CodeEngine)
	if !ok {
		return out
	}
	// The prose and the code of a row are apart, so the ranges do not
	// overlap; they only have to be in order.
	out = append(out, ce.MisspelledCode(cs.MaskCode(i, text))...)
	slices.SortFunc(out, func(a, b Range) int { return a.Start - b.Start })
	return out
}

// wordPos is one misspelled word: its row and byte columns.
type wordPos struct {
	row, start, end int
}

// flat lists the misspelled words of all rows, in ascending order.
func (e *entry) flat() []wordPos {
	var out []wordPos
	for i := range e.rows {
		for _, r := range e.rows[i].ranges {
			out = append(out, wordPos{row: i, start: r.Start, end: r.End})
		}
	}
	return out
}

// publish replaces the spans of every sink that differs from the words.
func (e *entry) publish() {
	words := e.flat()

	registry.Lock()
	defer registry.Unlock()
	st := registry.states[e.eb]
	if st == nil {
		return
	}
	for _, s := range st.sinks {
		install(s, words)
	}
}

// install puts the words in the layer of s. A layer that already shows
// exactly these words is left alone: the version of a layer is part of what
// the rows' cached styles were made from, so every change of the spans makes
// the rows recompute, and an edit that does not change a misspelled word (most
// of them) must not do that.
func install(s *sink, words []wordPos) {
	h := s.target.Highlights(s.priority)
	if sameWords(h, words) {
		return
	}
	if len(words) == 0 {
		h.Clear()
		return
	}

	spans := make([]*highlight.SpanTemplate, 0, len(words))
	for _, w := range words {
		sp := &highlight.Span{
			Start:         editbuffer.RowsPos{RowIndex: w.row, ColIndex: w.start},
			End:           editbuffer.RowsPos{RowIndex: w.row, ColIndex: w.end},
			Color:         Style,
			ColorIfActive: Style,
			Priority:      highlight.LayerSpell,
		}
		var t highlight.SpanTemplate = sp
		spans = append(spans, &t)
	}
	h.SetSpans(spans) // the whole slice at once: the draw goroutine never sees it half filled
}

func sameWords(h *highlight.Highlights, words []wordPos) bool {
	if h.SpansLength() != len(words) {
		return false
	}
	for i, w := range words {
		sp := h.GetSpan(i)
		if sp.Start.RowIndex != w.row || sp.Start.ColIndex != w.start ||
			sp.End.RowIndex != w.row || sp.End.ColIndex != w.end {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// What the commands need
// ---------------------------------------------------------------------------

func entryOf(eb *editbuffer.EditBuffer) *entry {
	registry.Lock()
	defer registry.Unlock()
	if st := registry.states[eb]; st != nil {
		return st.e
	}
	return nil
}

// WordAt returns the misspelled word at pos: the one that contains it, or
// that ends right before it (the cursor sits after the word it just typed).
func WordAt(eb *editbuffer.EditBuffer, pos editbuffer.RowsPos) (word string, r Range, ok bool) {
	e := entryOf(eb)
	if e == nil || pos.RowIndex < 0 || pos.RowIndex >= len(e.rows) {
		return "", Range{}, false
	}
	row := eb.Rows().Row(pos.RowIndex)
	if row == nil {
		return "", Range{}, false
	}
	for _, rg := range e.rows[pos.RowIndex].ranges {
		if pos.ColIndex >= rg.Start && pos.ColIndex <= rg.End && rg.End <= len(*row) {
			return string((*row)[rg.Start:rg.End]), rg, true
		}
	}
	return "", Range{}, false
}

// Suggest returns up to n corrections for word.
func Suggest(eb *editbuffer.EditBuffer, word string, n int) []string {
	if e := entryOf(eb); e != nil {
		return e.engine.Suggest(word, n)
	}
	return nil
}

// Add puts word in the personal dictionary and checks the buffer again.
func Add(eb *editbuffer.EditBuffer, word string) error {
	e := entryOf(eb)
	if e == nil {
		return nil
	}
	if err := e.engine.Add(word); err != nil {
		return err
	}
	e.recheckAll()
	return nil
}

// Ignore accepts word until the check is turned off, and checks again.
func Ignore(eb *editbuffer.EditBuffer, word string) {
	if e := entryOf(eb); e != nil {
		e.engine.Ignore(word)
		e.recheckAll()
	}
}

func (e *entry) recheckAll() {
	for i := range e.rows {
		e.rows[i].dirty = true
	}
	e.schedule(0)
}

// SetCode turns the check of the code of a program (variable and function
// names, ...) on or off for eb. The comments and strings are checked either
// way. It fails if the spell check is off, if the language mode has no code
// apart from its prose, or if the engine cannot check identifiers.
func SetCode(eb *editbuffer.EditBuffer, on bool) error {
	e := entryOf(eb)
	if e == nil {
		return ErrNotEnabled
	}
	cs, ok := e.scope.(CodeScope)
	if !ok {
		return ErrNoCodeScope
	}
	if _, ok := e.engine.(CodeEngine); !ok {
		return ErrNoCodeEngine
	}
	if cs.CodeOn() == on {
		return nil
	}
	cs.SetCode(on)
	e.scopeStale = true
	e.recheckAll()
	return nil
}

// CodeOn reports whether the code of eb is checked.
func CodeOn(eb *editbuffer.EditBuffer) bool {
	e := entryOf(eb)
	if e == nil {
		return false
	}
	cs, ok := e.scope.(CodeScope)
	return ok && cs.CodeOn()
}

// Dictionaries returns the names of the dictionaries of the language mode
// that were loaded (go for ~/.config/ge/spell/go.dic).
func Dictionaries(eb *editbuffer.EditBuffer) []string {
	if e := entryOf(eb); e != nil {
		return slices.Clone(e.dicts)
	}
	return nil
}

// Warnings returns what went wrong when the dictionaries of the language
// mode were loaded.
func Warnings(eb *editbuffer.EditBuffer) []string {
	if e := entryOf(eb); e != nil {
		return slices.Clone(e.warnings)
	}
	return nil
}

// TakeStart reports whether the buffer is at its start: it returns true the
// first time it is called for eb, and false after that, so that what a
// buffer starts with (see OnAtStart) is done once, and the user turning it
// off is not undone when the buffer is shown in another pane.
func TakeStart(eb *editbuffer.EditBuffer) bool {
	registry.Lock()
	defer registry.Unlock()
	st := stateOf(eb, true)
	if st.started {
		return false
	}
	st.started = true
	return true
}
