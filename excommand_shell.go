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
// arguments. A leading token equal to name (case-insensitive) is removed; the
// rest is returned as is, so quoting and inner spaces survive.
func commandText(args []string, name string) string {
	text := strings.TrimSpace(strings.Join(args, " "))
	first, rest, _ := strings.Cut(text, " ")
	//for _, n := range name {
	if strings.EqualFold(first, name) {
		return strings.TrimSpace(rest)
	}
	//}
	return text
}

// shellInContentDirCommand opens an interactive shell in the directory
// containing the current buffer's file.
func (e *Editorleaf) shellInContentDirCommand() *Command {
	return &Command{
		CommandName: "shell",
		// CommandAliases:     []string{},
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

			cmd, cleanup, err := interactiveShellCommand(ctx, shellPath)
			if err != nil {
				return true, err
			}
			defer cleanup()

			term := screen.Get()
			// Call tcell's screen suspension directly. The wrapper's Suspend
			// sends SIGSTOP to ge, which is intended for Ctrl+Z job control.
			term.Screen.Suspend()
			cmd.Dir = absDir
			cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
			if cmd.Env == nil {
				cmd.Env = os.Environ()
			}
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

// interactiveShellCommand returns the command and a cleanup function.
func interactiveShellCommand(ctx context.Context, shellPath string) (*exec.Cmd, func(), error) {
	noop := func() {}
	switch path.Base(shellPath) {
	case "bash":
		f, err := os.CreateTemp("", "ge-bashrc-*")
		if err != nil {
			return nil, noop, err
		}
		cleanup := func() { os.Remove(f.Name()) }
		const rc = `[ -f ~/.bashrc ] && . ~/.bashrc
PS1='ge-shell \w> '
`
		if _, err := f.WriteString(rc); err != nil {
			f.Close()
			cleanup()
			return nil, noop, err
		}
		if err := f.Close(); err != nil {
			cleanup()
			return nil, noop, err
		}
		return exec.CommandContext(ctx, shellPath, "--rcfile", f.Name(), "-i"), cleanup, nil
	// case "zsh":
	// 	return exec.CommandContext(ctx, shellPath, "-f", "-i"), noop, nil
	case "zsh":
		dir, err := os.MkdirTemp("", "ge-zdotdir-*")
		if err != nil {
			return nil, noop, err
		}
		cleanup := func() { os.RemoveAll(dir) }

		// 元の ZDOTDIR (未設定なら $HOME) を指す。
		const zshenv = `_ge_zd="${GE_ORIG_ZDOTDIR:-$HOME}"
[[ -f "$_ge_zd/.zshenv" ]] && source "$_ge_zd/.zshenv"
unset _ge_zd
`
		const zshrc = `if [[ -n "$GE_ORIG_ZDOTDIR" ]]; then
  ZDOTDIR="$GE_ORIG_ZDOTDIR"
else
  unset ZDOTDIR
fi
unset GE_ORIG_ZDOTDIR
[[ -f "${ZDOTDIR:-$HOME}/.zshrc" ]] && source "${ZDOTDIR:-$HOME}/.zshrc"
__ge_prompt() { PROMPT='ge-shell %~> ' }
__ge_prompt
precmd_functions+=(__ge_prompt)
`
		if err := os.WriteFile(filepath.Join(dir, ".zshenv"), []byte(zshenv), 0o600); err != nil {
			cleanup()
			return nil, noop, err
		}
		if err := os.WriteFile(filepath.Join(dir, ".zshrc"), []byte(zshrc), 0o600); err != nil {
			cleanup()
			return nil, noop, err
		}

		cmd := exec.CommandContext(ctx, shellPath, "-i")
		cmd.Env = append(os.Environ(),
			"GE_ORIG_ZDOTDIR="+os.Getenv("ZDOTDIR"),
			"ZDOTDIR="+dir,
		)
		return cmd, cleanup, nil
	// case "fish":
	// 	return exec.CommandContext(ctx, shellPath, "--no-config", "-i", "-C",
	// 		"function fish_prompt; printf 'ge-shell %s> ' (prompt_pwd); end"), noop, nil
	case "fish":
		const initCmd = `set -g fish_prompt_pwd_dir_length 0
function fish_prompt
    set_color green
    printf 'ge-shell'
    set_color normal
    printf ' %s> ' (prompt_pwd)
end
function fish_right_prompt
end`
		return exec.CommandContext(ctx, shellPath, "-i", "-C", initCmd), noop, nil
	default:
		return exec.CommandContext(ctx, shellPath, "-i"), noop, nil
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
