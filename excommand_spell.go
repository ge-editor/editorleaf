package editorleaf

import (
	"context"
	"strings"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/editorleaf/spell"
	"github.com/ge-editor/gecore"
	"github.com/ge-editor/theme"
)

// The spell check of a buffer (package spell). The engine is only there when
// ge was built with it (-tags hunspell); without it these commands say so.
//
//	ESC x spell                  turn the check of the buffer on or off
//	ESC x spell-correct WORD     replace the misspelled word at the cursor
//	                             (Tab lists the suggestions)
//	ESC x spell-add              put the word at the cursor in the personal dictionary
//	ESC x spell-ignore           accept the word at the cursor until the check is turned off
//	ESC x spell-identifiers      also check the names of variables, functions and types (toggle)

// maxSuggestions is how many corrections spell-correct offers.
const maxSuggestions = 10

// spellCommand is the ESC x "spell" command.
func (e *Editorleaf) spellCommand() *Command {
	cmd := &Command{
		CommandName:        "spell",
		CommandDescription: "Turn the spell check of the buffer on or off",
	}
	cmd.Run = func(ctx context.Context, args []string) (bool, error) {
		eb := e.editBuffer
		if spell.Enabled(eb) {
			spell.Disable(eb)
			gecore.Echo.AddText("Spell check: off")
			return true, nil
		}
		if e.enableSpell(false) {
			gecore.Echo.AddText(e.spellOnMessage())
		}
		return true, nil
	}
	return cmd
}

// enableSpell turns the spell check on for the buffer. It says why if it
// cannot, and reports whether it is on.
//
// quiet: without the engine nothing is said, for the start of a buffer: ge
// built without it would otherwise complain about every file it opens.
// What goes wrong with a dictionary is said in any case.
//
// The identifiers are checked from the start if IdentifiersByDefault says so.
func (e *Editorleaf) enableSpell(quiet bool) bool {
	if !spell.Available() {
		if !quiet {
			gecore.Echo.AddText(spell.ErrNoEngine.Error()).WithStyle(theme.ColorEchoYellow)
		}
		return false
	}
	opts := spell.Options{
		Scope:    e.spellScope(),
		Dispatch: dispatchUI,
		FocusRow: func() int {
			if e.meta == nil {
				return 0
			}
			return e.meta.RowsPos.RowIndex
		},
	}
	if err := spell.Enable(e.editBuffer, e.langModeName(), "", opts); err != nil {
		gecore.Echo.AddText("Spell check: " + err.Error()).WithStyle(theme.ColorEchoRed)
		return false
	}

	if spell.IdentifiersAtStart(e.langModeName()) {
		_ = spell.SetCode(e.editBuffer, true) // not for a mode that has no code: nothing to say
	}
	if w := spell.Warnings(e.editBuffer); len(w) > 0 {
		gecore.Echo.AddText("Spell dictionary: " + strings.Join(w, "; ")).WithStyle(theme.ColorEchoYellow)
	}
	return true
}

// spellOnMessage says that the spell check is on, and with which
// dictionaries of the language mode.
func (e *Editorleaf) spellOnMessage() string {
	msg := "Spell check: on"
	if d := spell.Dictionaries(e.editBuffer); len(d) > 0 {
		msg += " (+" + strings.Join(d, ", +") + ")"
	}
	if spell.CodeOn(e.editBuffer) {
		msg += ", identifiers too"
	}
	return msg
}

// autoSpell starts the spell check of a buffer that is shown for the first
// time, in a language mode that OnByDefault names.
func (e *Editorleaf) autoSpell() {
	if e.mode != ModeEditor || e.editBuffer == nil || e.meta == nil {
		return // not an editor (the minibuffer), or not set up
	}
	if !spell.OnAtStart(e.langModeName()) || !spell.TakeStart(e.editBuffer) {
		return
	}
	e.enableSpell(true)
}

// spellScope is what is checked in the buffer: the comments and strings of a
// program, the prose of a Markdown file, all of a text.
func (e *Editorleaf) spellScope() spell.Scope {
	if m := e.editBuffer.GetLangMode(); m != nil {
		return spell.ForMode(*m)
	}
	return spell.ForMode(nil)
}

// langModeName is the name of the language mode of the buffer.
func (e *Editorleaf) langModeName() string {
	if m := e.editBuffer.GetLangMode(); m != nil && *m != nil {
		return (*m).Name()
	}
	return ""
}

// misspelledAtCursor returns the misspelled word at the cursor, and says what
// is wrong if there is none.
func (e *Editorleaf) misspelledAtCursor() (word string, r spell.Range, row int, ok bool) {
	if !spell.Enabled(e.editBuffer) {
		gecore.Echo.AddText("The spell check is off: ESC x spell").WithStyle(theme.ColorEchoYellow)
		return "", spell.Range{}, 0, false
	}
	if e.meta == nil {
		return "", spell.Range{}, 0, false
	}
	pos := e.meta.RowsPos
	word, r, ok = spell.WordAt(e.editBuffer, pos)
	if !ok {
		gecore.Echo.AddText("No misspelled word at the cursor").WithStyle(theme.ColorEchoYellow)
		return "", spell.Range{}, 0, false
	}
	return word, r, pos.RowIndex, true
}

// spellCorrectCommand is the ESC x "spell-correct" command. The completion
// of its argument is the list of suggestions for the word at the cursor.
func (e *Editorleaf) spellCorrectCommand() *Command {
	cmd := &Command{
		CommandName:        "spell-correct",
		CommandDescription: "Replace the misspelled word at the cursor",
	}
	cmd.Complete = func(input string) []string {
		if e.meta == nil || !spell.Enabled(e.editBuffer) {
			return []string{}
		}
		word, _, ok := spell.WordAt(e.editBuffer, e.meta.RowsPos)
		if !ok {
			return []string{}
		}
		input = strings.TrimSpace(input)
		out := []string{}
		for _, s := range spell.Suggest(e.editBuffer, word, maxSuggestions) {
			if input == "" || strings.HasPrefix(strings.ToLower(s), strings.ToLower(input)) {
				out = append(out, s)
			}
		}
		return out
	}
	cmd.Run = func(ctx context.Context, args []string) (bool, error) {
		word, r, row, ok := e.misspelledAtCursor()
		if !ok {
			return true, nil
		}
		repl := strings.TrimSpace(commandText(args, cmd.CommandName))
		if repl == "" {
			sug := spell.Suggest(e.editBuffer, word, maxSuggestions)
			if len(sug) == 0 {
				gecore.Echo.AddText("No suggestion for " + word)
			} else {
				gecore.Echo.AddText(word + " → " + strings.Join(sug, ", ") + "  (spell-correct WORD)")
			}
			return true, nil
		}
		if strings.ContainsAny(repl, "\r\n") {
			gecore.Echo.AddText("A word cannot have a newline").WithStyle(theme.ColorEchoRed)
			return true, nil
		}
		e.replaceRange(
			editbuffer.RowsPos{RowIndex: row, ColIndex: r.Start},
			editbuffer.RowsPos{RowIndex: row, ColIndex: r.End},
			repl,
		)
		return true, nil
	}
	return cmd
}

// spellAddCommand is the ESC x "spell-add" command.
func (e *Editorleaf) spellAddCommand() *Command {
	cmd := &Command{
		CommandName:        "spell-add",
		CommandDescription: "Put the word at the cursor in the personal dictionary",
	}
	cmd.Run = func(ctx context.Context, args []string) (bool, error) {
		word, _, _, ok := e.misspelledAtCursor()
		if !ok {
			return true, nil
		}
		if err := spell.Add(e.editBuffer, word); err != nil {
			gecore.Echo.AddText("Not added: " + err.Error()).WithStyle(theme.ColorEchoRed)
			return true, nil
		}
		gecore.Echo.AddText("Added to the dictionary: " + word).WithStyle(theme.ColorEchoGreen)
		return true, nil
	}
	return cmd
}

// spellIgnoreCommand is the ESC x "spell-ignore" command.
func (e *Editorleaf) spellIgnoreCommand() *Command {
	cmd := &Command{
		CommandName:        "spell-ignore",
		CommandDescription: "Accept the word at the cursor until the spell check is turned off",
	}
	cmd.Run = func(ctx context.Context, args []string) (bool, error) {
		word, _, _, ok := e.misspelledAtCursor()
		if !ok {
			return true, nil
		}
		spell.Ignore(e.editBuffer, word)
		gecore.Echo.AddText("Ignored: " + word)
		return true, nil
	}
	return cmd
}

// spellIdentifiersCommand is the ESC x "spell-identifiers" command: it turns
// the check of the code of a program on or off, and turns the spell check on
// if it is off. Off is the default: the comments and strings only.
func (e *Editorleaf) spellIdentifiersCommand() *Command {
	cmd := &Command{
		CommandName:        "spell-identifiers",
		CommandDescription: "Also check the names of variables, functions and types (toggle)",
	}
	cmd.Run = func(ctx context.Context, args []string) (bool, error) {
		eb := e.editBuffer
		// If the spell check has to be turned on, the identifiers are what
		// the user asked for, whatever IdentifiersByDefault says.
		started := !spell.Enabled(eb)
		if started && !e.enableSpell(false) {
			return true, nil
		}
		on := started || !spell.CodeOn(eb)
		if err := spell.SetCode(eb, on); err != nil {
			gecore.Echo.AddText("Spell check: " + err.Error()).WithStyle(theme.ColorEchoYellow)
			return true, nil
		}
		if on {
			gecore.Echo.AddText("Spell check of identifiers: on")
		} else {
			gecore.Echo.AddText("Spell check of identifiers: off")
		}
		return true, nil
	}
	return cmd
}
