// editorleaf/minibuffer_session.go

package editorleaf

import (
	"github.com/ge-editor/keychord"
	"github.com/ge-editor/utils"
)

type Mode int

const (
	ModeEditor Mode = iota
	ModeMinibuffer
)

type Session struct {
	Editor      *Editorleaf
	Prompt      string
	PromptWidth int
	Mode        Mode
}

func NewSession(
	prompt string,
	bind func(*keychord.RootNode, *Editorleaf),
) *Session {
	editor := newMinibuffer()
	editor.MinibufferMode(ModeMinibuffer)
	km := keychord.NewRootNode()
	bind(km, editor)
	editor.SetKeyDispatcher(km)
	editor.Active(true)

	return &Session{
		Editor:      editor,
		Prompt:      prompt,
		PromptWidth: utils.WidthOnScreen([]byte(prompt)),
		Mode:        ModeMinibuffer,
	}
}

func (s *Session) SetPrompt(prompt string) {
	s.Prompt = prompt
	s.PromptWidth = utils.WidthOnScreen([]byte(prompt))
}
