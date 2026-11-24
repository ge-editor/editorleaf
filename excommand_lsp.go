package editorleaf

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/gecore"
	"github.com/ge-editor/theme"
	"github.com/ge-editor/utils"
)

// Commands backed by the language service (see LanguageService):
//
//	ESC x lsp-hover            documentation of the symbol at the cursor (F1), shown by the hover viewer (SetHoverViewer)
//	ESC x lsp-goto-definition  jump to its definition
//	ESC x lsp-find-references  where the symbol at the cursor is used, chosen from a list
//	ESC x lsp-completion       completion at the cursor, chosen from a list
//
// lsp-goto-definition leaves a mark where it was called (SetMarkAtCursor), so
// the way back is the usual mark commands.

func (e *Editorleaf) hoverCommand() *Command {
	return &Command{
		CommandName: "lsp-hover",
		// CommandAliases:     []string{"lsp-hover"},
		CommandDescription: "Show the documentation of the symbol at the cursor",
		Run: func(ctx context.Context, args []string) (bool, error) {
			e.hover()
			return true, nil
		},
	}
}

func (e *Editorleaf) definitionCommand() *Command {
	return &Command{
		CommandName: "lsp-goto-definition",
		// CommandAliases:     []string{"goto-definition", "lsp-definition"},
		CommandDescription: "Jump to the definition of the symbol at the cursor",
		Run: func(ctx context.Context, args []string) (bool, error) {
			e.LspGotoDefinition()
			return true, nil
		},
	}
}

func echoError(err error) {
	gecore.Echo.AddText(err.Error()).WithStyle(theme.ColorEchoRed)
}

// LspHoverText asks the language server for the documentation of the symbol
// at the cursor and calls show with it, ready to read (see hoverDisplayText),
// when it comes: not at all if the cursor has moved on, there is nothing to
// show or an error (those are said in the echo area). With show nil the first
// lines of the documentation go to the echo area. Most callers want LspHover,
// which uses the hover viewer of the application.
//
// When another minibuffer is open by then (a prompt that the user started
// while the server was answering) it is not taken over: the answer goes to
// the echo area, and show is not called.
func (e *Editorleaf) LspHoverText(show func(text string)) {
	if languageService == nil {
		gecore.Echo.AddText("No language service")
		return
	}

	eb, pos := e.editBuffer, e.meta.RowsPos
	languageService.Hover(eb, pos, func(text string, err error) {
		if err != nil {
			echoError(err)
			return
		}
		if e.editBuffer != eb || e.meta.RowsPos != pos {
			return // the cursor has moved on; the answer is no longer about what it is on
		}

		shown := hoverDisplayText(text)
		if shown == "" {
			gecore.Echo.AddText("No documentation here")
			return
		}

		if show == nil || MinibufferManager().IsActive() {
			gecore.Echo.AddText(firstLines(shown, 3))
			return
		}

		show(shown)
	})
}

// HoverViewer shows the documentation of lsp-hover (ready to read, several
// rows) to the user: the application's way, installed with SetHoverViewer.
type HoverViewer func(text string)

var hoverViewer HoverViewer

// SetHoverViewer installs the viewer of the documentation of lsp-hover, for
// the command and for the key (LspHover). Call it from init() or main();
// without one the documentation goes to the echo area, its first lines.
func SetHoverViewer(v HoverViewer) {
	hoverViewer = v
}

// LspHover is lsp-hover: the documentation of the symbol at the cursor, shown
// by the hover viewer (see SetHoverViewer). The key F1 runs it.
func (e *Editorleaf) LspHover() {
	e.LspHoverText(hoverViewer)
}

func (e *Editorleaf) hover() {
	e.LspHover()
}

// hoverDisplayText makes the documentation that a language server returns
// (Markdown) ready to read in the minibuffer: the lines of the code fences
// are taken out, blank lines are not doubled, and no blank is left at the
// ends of the lines and of the text.
func hoverDisplayText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")

	var out []string

	blank := true // no blank line at the start

	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, " \t\r")

		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			continue // the fence; the code in it stays
		}

		if line == "" {
			if blank {
				continue
			}

			blank = true
		} else {
			blank = false
		}

		out = append(out, line)
	}

	return strings.TrimRight(strings.Join(out, "\n"), "\n")
}

func (e *Editorleaf) LspGotoDefinition() {
	if languageService == nil {
		gecore.Echo.AddText("No language service")
		return
	}

	eb, pos := e.editBuffer, e.meta.RowsPos
	version := eb.Version()
	languageService.Definition(eb, pos, func(locs []Location, err error) {
		if err != nil {
			echoError(err)
			return
		}
		// Jumping after the user went elsewhere or edited would be a
		// surprise, and the location may no longer be right.
		if e.editBuffer != eb || eb.Version() != version || e.meta.RowsPos != pos {
			return
		}
		if len(locs) == 0 {
			gecore.Echo.AddText("No definition found")
			return
		}
		if err := e.gotoLocation(locs[0]); err != nil {
			echoError(err)
			return
		}
		if more := len(locs) - 1; more > 0 {
			gecore.Echo.AddText(fmt.Sprintf("(+%d more definitions)", more))
		}
	})
}

// gotoLocation moves to loc, opening its file in this pane if it is another
// file. A mark is left at the current cursor first.
func (e *Editorleaf) gotoLocation(loc Location) error {
	e.SetMarkAtCursor()
	if !utils.IsSameFile(e.GetPath(), loc.Path) {
		if _, err := e.OpenFile(loc.Path); err != nil {
			return err
		}
	}
	e.SetRowsPos(loc.Pos)
	return nil
}

// firstLines joins the first n non-empty lines of text into one echo line.
func firstLines(text string, n int) string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
			if len(lines) == n {
				break
			}
		}
	}
	return strings.Join(lines, " | ")
}

func (e *Editorleaf) completeCommand() *Command {
	return &Command{
		CommandName: "lsp-completion",
		// CommandAliases:     []string{"completion", "lsp-complete"},
		CommandDescription: "Complete the identifier at the cursor",
		Run: func(ctx context.Context, args []string) (bool, error) {
			e.complete()
			return true, nil
		},
	}
}

func (e *Editorleaf) complete() {
	if languageService == nil {
		gecore.Echo.AddText("No language service")
		return
	}
	if completionPicker == nil {
		gecore.Echo.AddText("No completion picker")
		return
	}

	eb, pos := e.editBuffer, e.meta.RowsPos
	version := eb.Version()
	languageService.Complete(eb, pos, func(items []CompletionItem, err error) {
		if err != nil {
			echoError(err)
			return
		}
		// The candidates are for the text and the cursor as they were.
		if e.editBuffer != eb || eb.Version() != version || e.meta.RowsPos != pos {
			return
		}
		if len(items) == 0 {
			gecore.Echo.AddText("No completions")
			return
		}

		accept := func(item CompletionItem) {
			if e.editBuffer != eb || eb.Version() != version {
				gecore.Echo.AddText("The buffer changed; completion cancelled").WithStyle(theme.ColorEchoRed)
				return
			}
			e.applyCompletion(item)
		}

		if len(items) == 1 {
			accept(items[0]) // nothing to choose
			return
		}
		completionPicker(e.identifierBeforeCursor(), items, accept)
	})
}

// identifierBeforeCursor returns the identifier typed right before the
// cursor, "" if there is none.
func (e *Editorleaf) identifierBeforeCursor() string {
	rows := e.editBuffer.Rows()
	if rows == nil || e.meta.RowsPos.RowIndex >= rows.Length() {
		return ""
	}
	line := []byte((*rows)[e.meta.RowsPos.RowIndex])
	col := min(e.meta.RowsPos.ColIndex, len(line))
	return string(line[identifierStart(line, col):col])
}

// identifierStart is the byte column where the identifier that ends at col
// begins (col itself if the character before it is not part of one).
func identifierStart(line []byte, col int) int {
	i := col
	for i > 0 {
		r, size := utf8.DecodeLastRune(line[:i])
		if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			break
		}
		i -= size
	}
	return i
}

// applyCompletion performs the edit of an accepted item and leaves the
// cursor behind the inserted text.
//
// The main edit goes first. Additional edits that lie entirely above its
// line (an added import) follow, bottom to top, so that each one's
// coordinates are still valid when it is applied; they can only move the
// cursor down by the number of lines they added. Other additional edits are
// not applied, and the user is told.
//
// Each replacement is a delete and an insert, two steps of undo.
func (e *Editorleaf) applyCompletion(item CompletionItem) {
	main := item.Edit

	var above []TextEdit
	skipped := 0
	for _, a := range item.Additional {
		if a.End.RowIndex < main.Start.RowIndex {
			above = append(above, a)
		} else {
			skipped++
		}
	}
	slices.SortFunc(above, func(a, b TextEdit) int { return -cmpRowsPos(a.Start, b.Start) })

	e.replaceRange(main.Start, main.End, main.NewText)
	cursor := e.meta.RowsPos

	for _, a := range above {
		before := e.editBuffer.Rows().Length()
		e.replaceRange(a.Start, a.End, a.NewText)
		cursor.RowIndex += e.editBuffer.Rows().Length() - before
	}
	e.SetRowsPos(cursor)

	if skipped > 0 {
		gecore.Echo.AddText(fmt.Sprintf("(%d additional edits not applied)", skipped))
	}
}

// replaceRange replaces [start, end) with text, leaving the cursor behind
// the text. The replaced text does not go to the kill buffer.
func (e *Editorleaf) replaceRange(start, end editbuffer.RowsPos, text string) {
	if cmpRowsPos(start, end) < 0 {
		e.deleteRegion(start, end)
	}
	e.meta.RowsPos = start
	if text != "" {
		e.InsertString(text)
	}
}

func cmpRowsPos(a, b editbuffer.RowsPos) int {
	switch {
	case a.RowIndex != b.RowIndex:
		return a.RowIndex - b.RowIndex
	default:
		return a.ColIndex - b.ColIndex
	}
}

func (e *Editorleaf) referencesCommand() *Command {
	return &Command{
		CommandName: "lsp-find-references",
		// CommandAliases:     []string{"find-references", "lsp-references"},
		CommandDescription: "List where the symbol at the cursor is used, and jump to one",
		Run: func(ctx context.Context, args []string) (bool, error) {
			e.references()
			return true, nil
		},
	}
}

func (e *Editorleaf) references() {
	if languageService == nil {
		gecore.Echo.AddText("No language service")
		return
	}

	eb, pos := e.editBuffer, e.meta.RowsPos
	version := eb.Version()
	languageService.References(eb, pos, func(locs []Location, err error) {
		if err != nil {
			echoError(err)
			return
		}
		if e.editBuffer != eb || eb.Version() != version || e.meta.RowsPos != pos {
			return // the locations are for the text and cursor as they were
		}

		switch {
		case len(locs) == 0:
			gecore.Echo.AddText("No references found")
		case len(locs) == 1:
			if err := e.gotoLocation(locs[0]); err != nil {
				echoError(err)
			}
		case locationPicker == nil:
			gecore.Echo.AddText(fmt.Sprintf("%d references (no location picker)", len(locs)))
		default:
			title := fmt.Sprintf("References (%d): ", len(locs))
			locationPicker(title, locs, func(loc Location) {
				if e.editBuffer != eb || eb.Version() != version {
					gecore.Echo.AddText("The buffer changed; jump cancelled").WithStyle(theme.ColorEchoRed)
					return
				}
				if err := e.gotoLocation(loc); err != nil {
					echoError(err)
				}
			})
		}
	})
}
