package editorleaf

import (
	"github.com/gdamore/tcell/v3"

	"github.com/ge-editor/gecore"
	"github.com/ge-editor/gecore/overlay"
	"github.com/ge-editor/gecore/screen"
	"github.com/ge-editor/keychord"
	"github.com/ge-editor/theme"
	"github.com/ge-editor/utils"
)

// Minibuffer Manager is Singleton
var minibufferManager = &MinibufferManagerStruct{}

func MinibufferManager() *MinibufferManagerStruct {
	gecore.CancelManager().Register(minibufferManager)
	return minibufferManager
}

// Required implement Overlay interface
type MinibufferManagerStruct struct {
	active bool

	session    *Session
	onComplete func(result keychord.KeyDispatchTransition)

	overlayRect utils.Rect
}

func (m *MinibufferManagerStruct) Start(session *Session, onComplete func(result keychord.KeyDispatchTransition)) {
	m.session = session
	m.onComplete = onComplete
	m.MinibufferResize(m.overlayRect)
	m.active = true

	overlay.OverlayManager().Layout(screen.Get().Rect)
}

func (m *MinibufferManagerStruct) Close() {
	if !m.active {
		return
	}
	m.Cancel()
	/* 	m.session.Editor.Active(false)
	   	m.session = nil
	   	m.active = false
	*/
	overlay.OverlayManager().Layout(screen.Get().Rect)
}

func (m *MinibufferManagerStruct) Cancel() {
	m.session.Editor.Active(false)
	/* var leafEditor tree.Leaf
	leafEditor = m.session.Editor
	m.session.Editor.Kill(&leafEditor, false)
	*/
	m.session = nil
	m.active = false
}

func (m *MinibufferManagerStruct) Priority() int {
	return 32 // 適当
}

func (m *MinibufferManagerStruct) Active(b bool) {
	m.active = b
}

func (m *MinibufferManagerStruct) IsActive() bool {
	return m.active
}

func (m *MinibufferManagerStruct) Editor() *Editorleaf {
	return m.session.Editor
}

// Return input content
func (m *MinibufferManagerStruct) GetBytes() ([]byte, []int, error) {
	if m.session == nil {
		return nil, nil, nil
	}
	return m.session.Editor.GetBytes()
}

func (m *MinibufferManagerStruct) SetPrompt(prompt string) {
	m.session.Prompt = prompt
	m.session.PromptWidth = utils.WidthOnScreen([]byte(prompt))

	/* 	editorRect := overlayRect
	   	editorRect.X += m.session.PromptWidth
	   	editorRect.Width -= m.session.PromptWidth
	   	editorRect.Height = height
	   	m.session.Editor.Resize(editorRect)
	*/
}

func (m *MinibufferManagerStruct) SetString(content string) {
	m.SetBytes([]byte(content))
}

func (m *MinibufferManagerStruct) SetBytes(content []byte) {
	if m.session == nil {
		return
	}

	rows, err := SplitRows(content)
	if err != nil {
		panic(err)
	}
	m.session.Editor.SetRows(rows)
}

// return minibuffer height
func (m *MinibufferManagerStruct) RequiredHeight() int {
	if !m.active {
		return 0
	}

	return m.MinibufferResize(m.overlayRect)
}

func (m *MinibufferManagerStruct) Resize(overlayRect utils.Rect) {
	m.overlayRect = overlayRect

	if !m.active {
		return
	}

	m.MinibufferResize(m.overlayRect)
}

// return Remaining height
// overlayRect.Height: この値のみ画面の高さ
func (m *MinibufferManagerStruct) MinibufferResize(overlayRect utils.Rect) int {
	// gelog.Info("overlayRect", "height", overlayRect.Height)

	maxHeight := int(float32(overlayRect.Height) * 0.2)
	if maxHeight < 5 {
		maxHeight = 5
	}
	height := m.session.Editor.RowsLength()
	if height > maxHeight {
		height = maxHeight
	}
	// gelog.Info("maxHeight", maxHeight)

	editorRect := overlayRect
	editorRect.X += m.session.PromptWidth
	editorRect.Width -= m.session.PromptWidth
	editorRect.Height = height
	m.session.Editor.Resize(editorRect)

	return height
}

func (m *MinibufferManagerStruct) Draw(s tcell.Screen) bool {
	if !m.active {
		return false
	}

	screen.Get().FillRect(m.overlayRect, 0, theme.ColorDefault)
	screen.Get().DrawLabel(m.overlayRect, &screen.LabelParams{Style: theme.ColorDefault}, m.session.Prompt)
	m.session.Editor.Draw()
	return false
}

func (m *MinibufferManagerStruct) UniversalCancel() {
	m.Close()
}

func (m *MinibufferManagerStruct) Dispatch(ev tcell.EventKey) keychord.KeyDispatchTransition {
	if !m.active {
		return keychord.DispatchNotFound
	}

	_, res := m.session.Editor.DispatchKey(ev)

	if m.onComplete != nil {
		m.onComplete(res)
	}
	return res
}
