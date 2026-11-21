package editorleaf

import (
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"github.com/ge-editor/gecore"
	"github.com/ge-editor/gecore/killbuffer"
)

// urlRe finds a URL in a row: a scheme that is safe to hand to the system,
// then everything up to a space or a character that does not belong to a URL.
// Characters outside ASCII end it too: in Japanese text a URL is often
// followed by the next word with no space ("https://example.com/aです"), and
// browsers copy a URL in its percent-encoded (ASCII) form. The punctuation
// that ends a sentence is taken off by trimURL.
var urlRe = regexp.MustCompile("(?i)(?:https?|ftp|file)://[^\\s<>\"'`\\x00-\\x1f\\x{80}-\\x{10FFFF}]+")

// trimURL takes off what a URL is followed by in text: the punctuation of a
// sentence, and a closing bracket that has no opening one in the URL
// ("(see https://example.com/a)" and "[x](https://example.com/a)").
func trimURL(u string) string {
	for u != "" {
		last := u[len(u)-1]

		switch {
		case strings.IndexByte(".,;:!?*", last) >= 0:
			u = u[:len(u)-1]

		case last == ')' && strings.Count(u, ")") > strings.Count(u, "("),
			last == ']' && strings.Count(u, "]") > strings.Count(u, "["),
			last == '}' && strings.Count(u, "}") > strings.Count(u, "{"):
			u = u[:len(u)-1]

		default:
			return u
		}
	}

	return u
}

// findURL returns the URL at byte column col of row: the one that covers the
// cursor (or ends right at it). Failing that, the only URL of the row, so that
// the command works from anywhere on a line that has one.
func findURL(row []byte, col int) (url string, ok bool) {
	var all []string

	for _, m := range urlRe.FindAllIndex(row, -1) {
		u := trimURL(string(row[m[0]:m[1]]))
		if u == "" {
			continue
		}

		if end := m[0] + len(u); col >= m[0] && col <= end {
			return u, true
		}

		all = append(all, u)
	}

	if len(all) == 1 {
		return all[0], true
	}

	return "", false
}

// openCommand returns the program that opens a URL on this system.
func openCommand(url string) *exec.Cmd {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url)
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return exec.Command("xdg-open", url)
	}
}

// openURL hands the URL to the system's opener and does not wait for it.
func openURL(url string) error {
	cmd := openCommand(url)

	if err := cmd.Start(); err != nil {
		return err
	}

	go func() { _ = cmd.Wait() }() // do not leave a zombie

	return nil
}

// OpenURL opens the URL at the cursor (see findURL) with the system's
// opener. When it cannot be opened (no opener, no desktop: ge over ssh) the
// URL is put in the kill buffer, which is the clipboard too.
func (e *Editorleaf) OpenURL() {
	pos := e.meta.RowsPos

	row := e.editBuffer.Rows().Row(pos.RowIndex)
	if row == nil {
		return
	}

	url, ok := findURL(row.Bytes(), pos.ColIndex)
	if !ok {
		gecore.Echo.AddText("No URL at the cursor")
		return
	}

	e.openURLString(url)
}

func (e *Editorleaf) openURLString(url string) {
	if err := openURL(url); err != nil {
		gecore.Echo.AddText(fmt.Sprintf("Cannot open %s: %v (copied to the kill buffer)", url, err))

		_ = killbuffer.KillBuffer.PushKillBuffer([][]byte{[]byte(url)}, e.editBuffer.GetNewLine().Bytes())

		return
	}

	gecore.Echo.AddText("Opening " + url)
}
