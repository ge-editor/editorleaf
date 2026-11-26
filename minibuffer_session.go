// editorleaf/minibuffer_session.go

package editorleaf

import (
	"github.com/ge-editor/gecore/tree"
	"github.com/ge-editor/keychord"
	"github.com/ge-editor/utils"
)

type Mode int

const (
	ModeEditor Mode = iota
	ModeMinibuffer
)

type Session struct {
	// Editor is the editor that gets the keys: the only one of a plain
	// session, the focused field of a form.
	Editor      *Editorleaf
	Prompt      string
	PromptWidth int
	Mode        Mode

	// A form has a header line and several fields, each with its own
	// editor (multi-line, soft-wrapped, scrolled by itself) and label.
	// Fields is nil for a plain session.
	Fields []*Field
	Focus  int
	// Header returns the text of the header line, for example the check
	// boxes of a search. It is called at every draw.
	Header func() string

	// A form can have a row of options after its fields (the check boxes of
	// a search). Tab and Shift+Tab move the focus Search -> Replace -> the
	// options -> Search. While the options have the focus no editor is
	// active, and the keys go to OptionKeys (Space, the cursor keys, ...).
	// OptionKeys is nil for a form without options.
	OptionKeys  *keychord.RootNode
	OptionFocus bool

	// Usage is what the echo line shows while the session is open: its name
	// and its keys (see gecore.Echo.SetUsage). The minibuffer manager sets
	// it when the session starts and clears it when the session ends, however
	// it ends (Enter, C-g, another session). nil shows nothing.
	Usage []string
}

// Field is one input of a form session.
type Field struct {
	Label      string
	LabelWidth int
	Editor     *Editorleaf
	Hidden     bool // a hidden field takes no room and gets no focus
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
	// Not have parent leaf
	editor.parentLeafType = tree.LeafTypes.GetDefaultLeafType()
	editor.parentLeafType.SetCtx(&tree.LeafContext{
		CancelManager: tree.ECM,
	})

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

// NewFormSession makes a session with a header line and one field per label.
// bind is called once per field to map the keys of that field's editor.
func NewFormSession(
	labels []string,
	bind func(field int, km *keychord.RootNode, editor *Editorleaf),
) *Session {
	s := &Session{Mode: ModeMinibuffer}

	labelWidth := 0
	for _, l := range labels {
		labelWidth = max(labelWidth, utils.WidthOnScreen([]byte(l)))
	}

	for i, l := range labels {
		i := i
		sub := NewSession(l, func(km *keychord.RootNode, editor *Editorleaf) {
			bind(i, km, editor)
		})
		sub.Editor.Active(false)
		s.Fields = append(s.Fields, &Field{Label: l, LabelWidth: labelWidth, Editor: sub.Editor})
	}

	s.Focus = 0
	s.Editor = s.Fields[0].Editor
	s.Editor.Active(true)
	return s
}

// SetFocus moves the keys to field i. A hidden or missing field is ignored.
// While the options have the focus the field is chosen but stays inactive.
func (s *Session) SetFocus(i int) {
	if i < 0 || i >= len(s.Fields) || s.Fields[i].Hidden {
		return
	}
	s.Editor.Active(false)
	s.Focus = i
	s.Editor = s.Fields[i].Editor
	s.Editor.Active(!s.OptionFocus)
}

// FocusNext moves the focus to the next visible field, then to the options
// (when the form has them), round.
func (s *Session) FocusNext() { s.moveFocus(1) }

// FocusPrev moves the focus the other way round.
func (s *Session) FocusPrev() { s.moveFocus(-1) }

func (s *Session) moveFocus(dir int) {
	// A slot is the index of a visible field, or -1 for the options.
	var slots []int
	for i, f := range s.Fields {
		if !f.Hidden {
			slots = append(slots, i)
		}
	}
	if s.OptionKeys != nil {
		slots = append(slots, -1)
	}
	if len(slots) < 2 {
		return
	}

	cur := s.Focus
	if s.OptionFocus {
		cur = -1
	}
	pos := 0
	for k, slot := range slots {
		if slot == cur {
			pos = k
			break
		}
	}
	next := slots[(pos+dir+len(slots))%len(slots)]

	if next == -1 {
		s.Editor.Active(false)
		s.OptionFocus = true
		return
	}
	s.OptionFocus = false
	s.SetFocus(next)
	s.Editor.Active(true)
}

// SetHidden shows or hides field i. If the focused field is hidden the focus
// goes to the first field.
func (s *Session) SetHidden(i int, hidden bool) {
	if i < 0 || i >= len(s.Fields) {
		return
	}
	s.Fields[i].Hidden = hidden
	if hidden && s.Focus == i {
		s.SetFocus(0)
	}
}

// deactivate takes the focus away from every editor of the session.
func (s *Session) deactivate() {
	if s.Fields == nil {
		s.Editor.Active(false)
		return
	}
	for _, f := range s.Fields {
		f.Editor.Active(false)
	}
}
