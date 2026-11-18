// Package spell shows the misspelled words of a buffer.
//
// It is the counterpart of package syntax for a third kind of buffer
// information, and it knows nothing about how a word is checked: an Engine
// does that. The engine lives in another module (language/spell, which calls
// libhunspell through cgo) and registers itself here with Register, so that
// editorleaf, gecore and this package never need a C compiler.
//
// The misspelled words are kept per row and published to the
// highlight.LayerSpell slot of every pane that shows the buffer (Subscribe).
// The editor does not draw them as a span style (a span would replace the
// syntax color); it paints an underline over the drawn cells, like the marks.
//
// Everything here runs on the UI goroutine: the buffer is edited there, the
// change listener is called there, and so is the check (a few hundred rows
// per step, see Options), so neither the engine nor the rows need locking.
package spell

import (
	"errors"
	"sync"
)

// DefaultLanguage is the language used when none is asked for.
const DefaultLanguage = "en_US"

// Range is a word in a row: byte columns, End exclusive.
type Range struct {
	Start, End int
}

// Engine checks words. One Engine is used by one buffer and only from the UI
// goroutine.
type Engine interface {
	// Misspelled returns the words of text (one row) that are not spelled
	// correctly, as byte ranges of text. Words that are not to be checked
	// (numbers, URLs, identifiers, ...) are the engine's business.
	Misspelled(text []byte) []Range

	// Suggest returns up to n corrections for word.
	Suggest(word string, n int) []string

	// Add puts word in the personal dictionary, which is kept.
	Add(word string) error

	// Ignore accepts word until the engine is closed.
	Ignore(word string)

	Close()
}

// CodeEngine is an Engine that can also check the code of a program: the
// names of its variables, functions and types. The words of an identifier
// are found differently from those of a text (split at "_", digits and
// capitals), and the words that every programmer knows (fmt, ctx, strconv)
// are not flagged, so it is a separate method.
type CodeEngine interface {
	// MisspelledCode returns the words of text (one row, with only the code
	// left in it) that are not spelled correctly, as byte ranges of text.
	MisspelledCode(text []byte) []Range
}

// ModeEngine is an Engine that can add the dictionaries of a language mode
// (the words of Go for the Go mode): the files named after the mode next to
// the dictionary of the language.
type ModeEngine interface {
	// LoadMode adds the dictionaries of the mode (its name as the
	// lang.Mode gives it). It returns the names of those that were loaded,
	// and what went wrong with the others, which does not stop the check.
	LoadMode(mode string) (loaded, warnings []string)
}

// Factory opens an Engine for a language such as "en_US".
type Factory func(lang string) (Engine, error)

var (
	mu      sync.Mutex
	factory Factory
)

// Register makes the engine available. It is called from the init() of the
// package that implements it (language/spell); registering again replaces the
// previous one.
func Register(f Factory) {
	mu.Lock()
	defer mu.Unlock()
	factory = f
}

// Available reports whether an engine is registered (ge was built with it).
func Available() bool {
	mu.Lock()
	defer mu.Unlock()
	return factory != nil
}

func open(lang string) (Engine, error) {
	mu.Lock()
	f := factory
	mu.Unlock()
	if f == nil {
		return nil, ErrNoEngine
	}
	return f(lang)
}

var (
	// ErrNoEngine: no engine is registered.
	ErrNoEngine = errors.New("spell checker is not built in (build ge with -tags hunspell)")
	// ErrUnsupportedMode: the language mode of the buffer has no Scope.
	ErrUnsupportedMode = errors.New("spell check is not supported in this language mode yet")

	// ErrNotEnabled: the spell check is not on for the buffer.
	ErrNotEnabled = errors.New("the spell check is off")
	// ErrNoCodeScope: the whole text of the language mode is checked
	// already, there is no code to add.
	ErrNoCodeScope = errors.New("all of the text of this language mode is checked already")
	// ErrNoCodeEngine: the engine cannot check identifiers.
	ErrNoCodeEngine = errors.New("the spell checker cannot check identifiers")

	errNeedBuffer = errors.New("spell: no buffer or no dispatch function")
)
