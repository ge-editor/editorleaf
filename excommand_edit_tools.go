package editorleaf

import (
	"context"
	"regexp"

	"github.com/ge-editor/gecore"
)

// Commands for the keys Alt+] (bracket), Alt+; (comment) and Alt+o (URL), so
// that they can be run by name too (ESC x).

func (e *Editorleaf) gotoBracketCommand() *Command {
	return &Command{
		CommandName:        "goto-matching-bracket",
		CommandDescription: "Move to the bracket that pairs with the one at the cursor",
		Run: func(ctx context.Context, args []string) (bool, error) {
			e.GotoMatchingBracket()
			return true, nil
		},
	}
}

func (e *Editorleaf) commentRegionCommand() *Command {
	return &Command{
		CommandName:        "toggle-comment-region",
		CommandDescription: "Comment the lines from the last mark to the cursor, or uncomment them if they are comments",
		Run: func(ctx context.Context, args []string) (bool, error) {
			e.ToggleCommentRegion()
			return true, nil
		},
	}
}

// var urlSchemeRe = regexp.MustCompile(`(?i)^(?:https?|ftp|file)://\S+$`)
var urlSchemeRe = regexp.MustCompile(`^(?i:https?|ftp|file)://\S+$`)

func (e *Editorleaf) openURLCommand() *Command {
	return &Command{
		CommandName:        "open-url",
		CommandDescription: "Open the URL at the cursor (open-url [URL] opens the URL given)",
		Run: func(ctx context.Context, args []string) (bool, error) {
			url := commandText(args, "open-url")
			switch {
			case url == "":
				e.OpenURL()
			case urlSchemeRe.MatchString(url):
				e.openURLString(url)
			default:
				gecore.Echo.AddText("Usage: open-url [http(s)|ftp|file://URL]")
			}

			return true, nil
		},
	}
}
