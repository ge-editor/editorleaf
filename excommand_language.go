package editorleaf

import (
	"context"
	"fmt"
	"strings"

	"github.com/ge-editor/gecore"
	"github.com/ge-editor/gecore/lang"
	"github.com/ge-editor/theme"
	"github.com/ge-editor/utils"
)

// languageCommand is the ESC x "language" command: it sets the language mode
// of this editor's buffer with EditBuffer.SetLangMode. The syntax Worker of
// the buffer follows the change (see syntax.Subscribe).
//
//	ESC x language go
//	ESC x lang <Enter>      -- then pick from the filtered list
func (e *Editorleaf) languageCommand() *Command {
	cmd := &Command{
		CommandName:        "language",
		CommandAliases:     []string{"lang", "set-language"},
		CommandDescription: "Set the language mode of the buffer",
		Complete:           LanguageNames,
	}
	cmd.Run = func(ctx context.Context, args []string) (bool, error) {
		name := languageArg(cmd, args)
		if name == "" {
			gecore.Echo.AddText("Usage: language NAME")
			return true, nil
		}
		if err := e.SetLanguage(name); err != nil {
			gecore.Echo.AddText(err.Error()).WithStyle(theme.ColorEchoRed)
			return true, err
		}
		return true, nil
	}
	return cmd
}

// languageArg extracts the language name from args. ESC x passes the raw
// input ("language go") as one element, while other callers may pass only the
// arguments; both forms are accepted.
func languageArg(cmd *Command, args []string) string {
	return commandText(args, append([]string{cmd.CommandName}, cmd.CommandAliases...)...)
}

// LanguageNames returns the names of the registered lang.Modes, in
// registration order, that contain all characters of input in order
// (case-insensitive, like the buffer list filter). An empty input matches all.
func LanguageNames(input string) []string {
	input = strings.TrimSpace(input)
	names := []string{}
	for _, m := range lang.Modes.GetModes() {
		if input == "" || utils.ContainsAllCharacters(m.Name(), input) {
			names = append(names, m.Name())
		}
	}
	return names
}

// SetLanguage sets the lang.Mode of this editor's buffer by name. The name
// is matched case-insensitively; if there is no exact match, a filter that
// matches exactly one language is accepted. It must be called on the UI
// goroutine.
func (e *Editorleaf) SetLanguage(name string) error {
	modes := lang.Modes.GetModes()
	find := func(n string) int {
		for i := range modes {
			if strings.EqualFold(modes[i].Name(), n) {
				return i
			}
		}
		return -1
	}

	i := find(name)
	if i < 0 {
		if names := LanguageNames(name); len(names) == 1 {
			i = find(names[0])
		}
	}
	if i < 0 {
		return fmt.Errorf("unknown language: %s", name)
	}

	// &modes[i] points into the registry's slice, the same *Mode that
	// lang.Modes.GetMode hands out.
	e.editBuffer.SetLangMode(&modes[i])
	gecore.Echo.AddText("Language: " + modes[i].Name())
	return nil
}
