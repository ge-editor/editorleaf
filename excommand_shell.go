package editorleaf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/gecore"
	"github.com/ge-editor/gecore/screen"
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

// shellInContentDirCommand opens an interactive shell in the directory
// containing the current buffer's file.
func (e *Editorleaf) shellInContentDirCommand() *Command {
	return &Command{
		CommandName:        "shell",
		CommandAliases:     []string{"sh"},
		CommandDescription: "Open an interactive shell in the content directory",
		Run: func(ctx context.Context, args []string) (bool, error) {
			path := e.editBuffer.GetPath()
			dir := "."
			if path != "" {
				dir = filepath.Dir(path)
			}
			absDir, err := filepath.Abs(dir)
			if err != nil {
				return true, err
			}

			shell := os.Getenv("SHELL")
			if shell == "" {
				shell = "sh"
			}
			shellPath, err := exec.LookPath(shell)
			if err != nil {
				shellPath, err = exec.LookPath("sh")
				if err != nil {
					return true, err
				}
			}
			cmd := interactiveShellCommand(ctx, shellPath)
			term := screen.Get()
			// Call tcell's screen suspension directly. The wrapper's Suspend
			// sends SIGSTOP to ge, which is intended for Ctrl+Z job control.
			term.Screen.Suspend()
			cmd.Dir = absDir
			cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
			cmd.Env = os.Environ()
			ps1Set := false
			for i, env := range cmd.Env {
				if strings.HasPrefix(env, "PS1=") {
					cmd.Env[i] = "PS1=ge-shell> "
					ps1Set = true
				}
			}
			if !ps1Set {
				cmd.Env = append(cmd.Env, "PS1=ge-shell> ")
			}
			runErr := cmd.Run()
			resumeErr := term.Resume()
			if runErr != nil {
				return true, runErr
			}
			return true, resumeErr
		},
	}
}

func interactiveShellCommand(ctx context.Context, shellPath string) *exec.Cmd {
	switch path.Base(shellPath) {
	case "bash":
		// Skip startup files so they cannot replace the custom prompt.
		return exec.CommandContext(ctx, shellPath, "--noprofile", "--norc", "-i")
	case "zsh":
		return exec.CommandContext(ctx, shellPath, "-f", "-i")
	case "fish":
		return exec.CommandContext(ctx, shellPath, "--no-config", "-i", "-C", "function fish_prompt; printf 'ge-shell> '; end")
	default:
		return exec.CommandContext(ctx, shellPath, "-i")
	}
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
