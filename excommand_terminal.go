package editorleaf

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"

	"github.com/ge-editor/gecore"
	"github.com/ge-editor/gecore/excommand"
	"github.com/ge-editor/gecore/lang"
	"github.com/ge-editor/gecore/screen"
	"github.com/ge-editor/theme"
)

// modeCommands returns the ESC x commands that the mode of the buffer offers
// (lang.TerminalCommandProvider). They exist only while the buffer has that
// mode.
func (e *Editorleaf) modeCommands() []excommand.ExCommand {
	if e.editBuffer == nil {
		return nil
	}
	pm := e.editBuffer.GetLangMode()
	if pm == nil || *pm == nil {
		return nil
	}
	provider, ok := (*pm).(lang.TerminalCommandProvider)
	if !ok {
		return nil
	}
	var cmds []excommand.ExCommand
	for _, tc := range provider.TerminalCommands() {
		tc := tc
		cmds = append(cmds, &Command{
			CommandName:        tc.Name,
			CommandDescription: tc.Description,
			Run: func(ctx context.Context, args []string) (bool, error) {
				return true, runTerminalCommand(ctx, tc)
			},
		})
	}
	return cmds
}

// runTerminalCommand leaves the editor screen, runs the steps of tc with the
// terminal as their input and output, and returns to the editor. It waits for
// Enter before returning when a step failed, or when tc.PauseOnSuccess is set.
//
// The programs are looked for before the screen is left, so a missing `go`
// is an error in the echo line and not a flash of the terminal.
func runTerminalCommand(ctx context.Context, tc lang.TerminalCommand) error {
	for _, step := range tc.Steps {
		if _, err := exec.LookPath(step.Program); err != nil {
			return fmt.Errorf("%s: %s not found in PATH", tc.Name, step.Program)
		}
	}

	// Not in the directory of a file: `go install pkg@version` does not
	// depend on a module, and a go.mod or go.work near the file must not
	// change what is installed.
	dir := os.TempDir()

	// In the terminal ^C is a signal for every process of the foreground
	// group, ge included. Catching it keeps ge alive (a program that is run
	// starts with the default handler, so it is still interrupted).
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt)
	defer signal.Stop(sigc)

	term := screen.Get()
	// Call tcell's screen suspension directly: the wrapper's Suspend sends
	// SIGSTOP to ge, which is for Ctrl+Z job control.
	term.Screen.Suspend()

	out := os.Stdout
	fmt.Fprintf(out, "\n== %s ==\n", tc.Description)

	var failed []string
	for _, step := range tc.Steps {
		line := step.Program + " " + strings.Join(step.Args, " ")
		fmt.Fprintf(out, "\n$ %s\n", line)

		cmd := exec.CommandContext(ctx, step.Program, step.Args...)
		cmd.Dir = dir
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(out, "FAILED: %v\n", err)
			failed = append(failed, line)
		}
	}

	if len(failed) == 0 {
		fmt.Fprintf(out, "\n== done: %d ok ==\n", len(tc.Steps))
	} else {
		fmt.Fprintf(out, "\n== done: %d ok, %d failed ==\n", len(tc.Steps)-len(failed), len(failed))
		for _, f := range failed {
			fmt.Fprintf(out, "  failed: %s\n", f)
		}
	}
	if len(failed) > 0 || tc.PauseOnSuccess {
		fmt.Fprint(out, "Press Enter to return to ge ")
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	}

	resumeErr := term.Resume()

	if len(failed) > 0 {
		gecore.Echo.AddText(fmt.Sprintf("%s: %d of %d failed", tc.Name, len(failed), len(tc.Steps))).
			WithStyle(theme.ColorEchoRed)
	} else {
		gecore.Echo.AddText(fmt.Sprintf("%s: %d ok", tc.Name, len(tc.Steps))).
			WithStyle(theme.ColorEchoGreen)
	}
	return resumeErr
}
