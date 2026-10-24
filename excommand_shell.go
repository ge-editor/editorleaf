package editorleaf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/gecore"
	"github.com/ge-editor/theme"
	"github.com/ge-editor/utils"
)

// commandText returns the argument text of an ESC x input. ESC x passes the
// raw input as one element ("! ls -la"); other callers may pass only the
// arguments. A leading token equal to one of names (case-insensitive) is
// removed; the rest is returned as is, so quoting and inner spaces survive.
func commandText(args []string, names ...string) string {
	text := strings.TrimSpace(strings.Join(args, " "))
	first, rest, _ := strings.Cut(text, " ")
	for _, n := range names {
		if strings.EqualFold(first, n) {
			return strings.TrimSpace(rest)
		}
	}
	return text
}

// shellCommand runs ! COMMAND with sh -c and inserts stdout at the cursor.
func (e *Editorleaf) shellCommand() *Command {
	return &Command{
		CommandName:        "!",
		CommandDescription: "Shell command: ! COMMAND inserts its output",
		Run: func(ctx context.Context, args []string) (bool, error) {
			cmdline := commandText(args, "!")
			if cmdline == "" {
				gecore.Echo.AddText("Usage: ! COMMAND")
				return true, nil
			}
			result, err := utils.ExecShellCommand(cmdline)
			if err != nil {
				echoShellError(err, result)
				return true, err
			}
			if result != "" {
				e.InsertString(result)
			}
			return true, nil
		},
	}
}

// shellRegionCommand pipes the marked region to a shell command and replaces
// it with stdout when the command succeeds and produces output.
func (e *Editorleaf) shellRegionCommand() *Command {
	return &Command{
		CommandName:        "|",
		CommandDescription: "Shell command on region: | COMMAND replaces the region with its output",
		Run: func(ctx context.Context, args []string) (bool, error) {
			cmdline := commandText(args, "|")
			if cmdline == "" {
				gecore.Echo.AddText("Usage: | COMMAND")
				return true, nil
			}
			start, stop, err := e.regionBounds()
			if err != nil {
				gecore.Echo.AddText(err.Error())
				return true, nil
			}
			input := ""
			if rows := e.editBuffer.Rows().GetRegion(start, stop); rows != nil {
				input = rows.JoinString(e.editBuffer.GetNewLine().Bytes())
			}
			out, err := runShell(ctx, cmdline, input)
			if err != nil {
				echoShellError(err, "")
				return true, err
			}
			if out == "" {
				gecore.Echo.AddText("Shell command succeeded with no output")
				return true, nil
			}
			e.killRegion(start, stop)
			e.InsertString(out)
			return true, nil
		},
	}
}

func (e *Editorleaf) regionBounds() (start, stop editbuffer.RowsPos, err error) {
	mark := e.findLastByEditBuffer()
	if mark == nil {
		return start, stop, errors.New("The mark is not set now, so there is no region")
	}
	cur := e.meta.RowsPos
	if mark.RowIndex == cur.RowIndex && mark.ColIndex == cur.ColIndex {
		return start, stop, errors.New("Mark and cursor position are the same, so there is no region")
	}
	markFirst := mark.RowIndex < cur.RowIndex ||
		(mark.RowIndex == cur.RowIndex && mark.ColIndex < cur.ColIndex)
	if markFirst {
		return mark.RowsPos, cur, nil
	}
	return cur, mark.RowsPos, nil
}

func runShell(ctx context.Context, cmdline, stdin string) (string, error) {
	c := exec.CommandContext(ctx, "sh", "-c", cmdline)
	c.Stdin = strings.NewReader(stdin)
	var out, errOut bytes.Buffer
	c.Stdout, c.Stderr = &out, &errOut
	if err := c.Run(); err != nil {
		if msg := firstLine(errOut.String()); msg != "" {
			return "", fmt.Errorf("%w: %s", err, msg)
		}
		return "", err
	}
	return out.String(), nil
}

func echoShellError(err error, stderr string) {
	msg := firstLine(stderr)
	if msg == "" {
		msg = err.Error()
	}
	gecore.Echo.AddText(msg).WithStyle(theme.ColorEchoRed)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	line, _, _ := strings.Cut(s, "\n")
	return line
}
