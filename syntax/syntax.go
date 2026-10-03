// Package syntax connects EditBuffer edits to an incremental syntax
// highlighter, without depending on any specific parser library.
//
// A concrete implementation (for example one backed by tree-sitter) lives
// in another module/package and registers itself here by language name.
// That keeps cgo (if the implementation needs it) out of every package
// that does not need it: editorleaf, gecore and this package never import
// a parser library directly.
package syntax

import (
	"context"
	"strings"
	"sync"

	"github.com/ge-editor/editorleaf/highlight"
)

// Parser is implemented by one language's incremental highlighter.
// A Parser instance is owned by exactly one Worker and is only ever used
// from that Worker's goroutine; implementations do not need to be safe
// for concurrent use.
type Parser interface {
	// Parse (re)builds all state from src (the full source, rows joined
	// with '\n'). It is called once at startup and again after a
	// ChangeReset (file reload, whole-buffer reformat, ...).
	//
	// ctx is cancelled if a newer change arrives before Parse returns;
	// implementations that support cancelling a parse mid-flight should
	// check it periodically (for tree-sitter, via ParseOptions'
	// progress callback). Ignoring ctx is safe but can make ge
	// unresponsive while a very large file is (re)parsed.
	Parse(ctx context.Context, src []byte)

	// Edit applies one incremental edit on top of the previous Parse or
	// Edit, then reparses. src is the full new source after the edit.
	// e describes the edit in tree-sitter's InputEdit shape.
	Edit(ctx context.Context, e Edit, src []byte)

	// Spans returns the current highlight spans. The slice must already
	// be sorted by Start and non-overlapping: Worker copies it into a
	// highlight.HighlightsLayer as-is (see FindHighlightSpan in
	// editorleaf/vcommand_search.go, which walks spans in order and
	// never sorts them itself).
	//
	// Spans is called from the same goroutine as Parse/Edit, always
	// after them, never concurrently with them.
	Spans() []highlight.Span

	// Close releases resources (for a cgo-backed parser: the native
	// parser and syntax tree). Called exactly once, after the last
	// call to Parse/Edit/Spans.
	Close()
}

// Edit is tree-sitter's InputEdit, spelled out so that this package (and
// anything importing only this package) does not need a parser library's
// types. Positions are byte offsets / (row, byte column) in a text where
// rows are joined with a single '\n', matching editbuffer.LineIndex.
type Edit struct {
	StartByte, OldEndByte, NewEndByte    int
	StartRow, StartCol                   int
	OldEndRow, OldEndCol                 int
	NewEndRow, NewEndCol                 int
}

// Factory creates one Parser instance. A Worker calls it once, when a
// buffer of the matching language is attached.
type Factory func() Parser

var (
	mu        sync.Mutex
	factories = map[string]Factory{}
)

// Register makes a Factory available under langName (a lang.Mode's
// Name(), e.g. "go"). Intended to be called from an implementation
// package's init(). langName is matched case-insensitively against
// Lookup's argument (lang.Mode implementations are free to spell their
// Name() however they like -- e.g. go_mode.GoMode.Name() returns "Go",
// not "go" -- and this package should not require every implementation
// package to agree on casing). Registering the same name twice replaces
// the previous factory (last import wins); ge only ever links in one
// implementation per language, so this should not happen in practice.
func Register(langName string, f Factory) {
	mu.Lock()
	defer mu.Unlock()
	factories[normalizeLangName(langName)] = f
}

// Lookup returns the Factory registered for langName, if any. See
// Register: the match is case-insensitive.
func Lookup(langName string) (Factory, bool) {
	mu.Lock()
	defer mu.Unlock()
	f, ok := factories[normalizeLangName(langName)]
	return f, ok
}

func normalizeLangName(langName string) string {
	return strings.ToLower(strings.TrimSpace(langName))
}
