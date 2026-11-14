package editorleaf

import "github.com/ge-editor/editorleaf/editbuffer"

// Location is a place in a file: where a definition is.
type Location struct {
	Path string
	Pos  editbuffer.RowsPos // row and byte column

	// Text is the line of the location, for a list ("" if unknown).
	Text string
}

// LanguageService is what the editor asks of the language servers. editorleaf
// does not know how they work (the packages that do depend on editorleaf,
// not the other way round), so the application installs an implementation
// with SetLanguageService, the same direction of dependency as
// syntax.Register.
//
// Both methods are called on the UI goroutine and answer on the UI
// goroutine, later: done is called exactly once, with an error if the
// request could not be made or failed.
type LanguageService interface {
	// Hover returns the documentation of the symbol at pos ("" if none).
	Hover(eb *editbuffer.EditBuffer, pos editbuffer.RowsPos, done func(text string, err error))

	// Definition returns where the symbol at pos is defined.
	Definition(eb *editbuffer.EditBuffer, pos editbuffer.RowsPos, done func(locs []Location, err error))

	// References returns the places that use the symbol at pos.
	References(eb *editbuffer.EditBuffer, pos editbuffer.RowsPos, done func(locs []Location, err error))

	// DocumentSymbols returns the outline of the buffer, in document order.
	DocumentSymbols(eb *editbuffer.EditBuffer, done func(symbols []Symbol, err error))

	// WorkspaceSymbols returns the symbols of the whole workspace that match
	// query, for the language of eb.
	WorkspaceSymbols(eb *editbuffer.EditBuffer, query string, done func(symbols []Symbol, err error))

	// CodeActions returns the actions available for the range [start, end]
	// of the buffer (a cursor is start == end). Only actions that carry their
	// edit are returned; skipped counts the others the server offered.
	CodeActions(eb *editbuffer.EditBuffer, start, end editbuffer.RowsPos, done func(actions []CodeAction, skipped int, err error))

	// Format returns the edits that format the whole buffer.
	Format(eb *editbuffer.EditBuffer, done func(res FormatResult, err error))

	// Rename returns the edits that rename the symbol at pos to newName, in
	// all the files it touches. Nothing is changed by asking.
	Rename(eb *editbuffer.EditBuffer, pos editbuffer.RowsPos, newName string, done func(we *WorkspaceEdit, err error))

	// Complete returns the completion candidates at pos, in the server's
	// order of preference.
	Complete(eb *editbuffer.EditBuffer, pos editbuffer.RowsPos, done func(items []CompletionItem, err error))
}

// TextEdit replaces [Start, End) with NewText (row and byte column).
type TextEdit struct {
	Start, End editbuffer.RowsPos
	NewText    string
}

// CompletionItem is one completion candidate with the edit that accepting
// it performs.
type CompletionItem struct {
	Label  string
	Kind   string // "func", "var", ... or ""
	Detail string
	Filter string // what typing is matched against; Label if empty

	// Edit is applied to the buffer when the item is accepted.
	Edit TextEdit

	// Additional are further edits that belong to it, such as an added
	// import. Only those that lie entirely above the line of Edit are
	// applied; they cannot disturb the cursor beyond a number of lines.
	Additional []TextEdit
}

// CompletionPicker lets the user choose among items and calls accept with
// the chosen one (or does not call it if the user cancels). prefix is the
// identifier already typed before the cursor, a good initial filter. The
// application installs it with SetCompletionPicker; the user interface is
// not editorleaf's concern.
type CompletionPicker func(prefix string, items []CompletionItem, accept func(CompletionItem))

var completionPicker CompletionPicker

// SetCompletionPicker installs the completion picker. Call it from init()
// or main().
func SetCompletionPicker(p CompletionPicker) {
	completionPicker = p
}

var languageService LanguageService

// SetLanguageService installs the language service. Call it from init() or
// main(); without one the hover and definition commands only say so.
func SetLanguageService(s LanguageService) {
	languageService = s
}

// LocationPicker lets the user choose among locations (title says what
// they are) and calls accept with the chosen one, or does not call it if the
// user cancels. The application installs it with SetLocationPicker.
type LocationPicker func(title string, locs []Location, accept func(Location))

var locationPicker LocationPicker

// SetLocationPicker installs the location picker. Call it from init() or
// main().
func SetLocationPicker(p LocationPicker) {
	locationPicker = p
}

// FileEdit is the edits to one file.
type FileEdit struct {
	Path  string
	Edits []TextEdit // valid for the text the server saw; no particular order

	// Open: the server worked on the open buffer of this file, and Version
	// is the buffer's version (EditBuffer.Version) the edits are valid for.
	// If the buffer has another version, the edits must not be applied.
	// For a file that is not Open the server read the file from disk.
	Open    bool
	Version uint64
}

// WorkspaceEdit is a set of edits across files, such as the result of a
// rename. It is applied as a whole or not at all.
type WorkspaceEdit struct {
	Files []FileEdit
}

// PreviewPicker shows details (what is about to happen) and lets the user
// confirm: apply is called only if the user chooses the apply line, never on
// cancel. title is the prompt, applyLabel the text of the confirming line.
// The application installs it with SetPreviewPicker.
type PreviewPicker func(title, applyLabel string, details []string, apply func())

var previewPicker PreviewPicker

// SetPreviewPicker installs the preview picker. Call it from init() or main().
func SetPreviewPicker(p PreviewPicker) {
	previewPicker = p
}

// Symbol is a named thing at a place.
type Symbol struct {
	Name      string
	Kind      string
	Container string
	Depth     int // nesting in an outline
	Location  Location
}

// FormatResult is the answer to a formatting request: edits that are valid
// for the buffer at Version, and only for it.
type FormatResult struct {
	Edits   []TextEdit
	Version uint64
}

// ListPicker lets the user choose one of lines and calls accept with its
// index, or does not call it if the user cancels. The application installs it
// with SetListPicker.
type ListPicker func(title string, lines []string, accept func(index int))

var listPicker ListPicker

// SetListPicker installs the list picker. Call it from init() or main().
func SetListPicker(p ListPicker) {
	listPicker = p
}

// CodeAction is something the server offers to do at a place: fix a problem,
// extract a function, organize the imports, ...
type CodeAction struct {
	Title     string
	Kind      string
	Preferred bool // the server's suggestion
	Edit      *WorkspaceEdit
}

