package editorleaf

import (
	"bytes"
	"context"

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

	overlayRect screen.Rect
}

func (m *MinibufferManagerStruct) Start(session *Session, onComplete func(result keychord.KeyDispatchTransition)) {
	m.clearUsage() // of the session that this one replaces
	m.session = session
	m.onComplete = onComplete
	m.MinibufferResize(m.overlayRect)
	m.active = true
	m.showUsage()

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

// showUsage puts the usage of the session in the echo line.
func (m *MinibufferManagerStruct) showUsage() {
	if m.session != nil && len(m.session.Usage) > 0 {
		gecore.Echo.SetUsage(m.session.Usage...)
	}
}

// clearUsage takes the usage of the session out of the echo line. A session
// without one leaves the echo line alone: the usage of a mode that is open
// under it stays.
func (m *MinibufferManagerStruct) clearUsage() {
	if m.session != nil && len(m.session.Usage) > 0 {
		gecore.Echo.ClearUsage()
	}
}

func (m *MinibufferManagerStruct) Cancel() {
	m.clearUsage()
	m.session.deactivate()
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

// ---- form sessions ----

// FieldBytes returns the content of field i of a form session (rows joined
// with '\n').
func (m *MinibufferManagerStruct) FieldBytes(i int) []byte {
	if m.session == nil || i < 0 || i >= len(m.session.Fields) {
		return nil
	}
	b, _, _ := m.session.Fields[i].Editor.GetBytes()
	return b
}

// SetFieldString replaces the content of field i with content.
func (m *MinibufferManagerStruct) SetFieldString(i int, content string) {
	if m.session == nil || i < 0 || i >= len(m.session.Fields) {
		return
	}
	rows, err := SplitRows([]byte(content))
	if err != nil {
		return
	}
	m.session.Fields[i].Editor.SetRows(rows)
}

func (m *MinibufferManagerStruct) FocusField(i int) {
	if m.session != nil {
		m.session.SetFocus(i)
	}
}

func (m *MinibufferManagerStruct) FocusNextField() {
	if m.session != nil {
		m.session.FocusNext()
	}
}

func (m *MinibufferManagerStruct) SetFieldHidden(i int, hidden bool) {
	if m.session != nil {
		m.session.SetHidden(i, hidden)
	}
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

// SetStringAtEnd replaces the content with content and puts the cursor at its
// end. Unlike SetString followed by a cursor movement command, it is safe when
// the new content is shorter than the old cursor position.
func (m *MinibufferManagerStruct) SetStringAtEnd(content string) {
	m.SetString(content)
	if m.session == nil {
		return
	}
	m.session.Editor.moveCursorToEndOfContent()
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

func (m *MinibufferManagerStruct) Resize(overlayRect screen.Rect) {
	m.overlayRect = overlayRect

	if !m.active {
		return
	}

	m.MinibufferResize(m.overlayRect)
}

// return Remaining height
// overlayRect.Height: この値のみ画面の高さ
func (m *MinibufferManagerStruct) MinibufferResize(overlayRect screen.Rect) int {
	if m.session.Fields != nil {
		return m.formResize(overlayRect)
	}
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

// visualHeight is the number of screen rows the content of e takes when its
// rows are wrapped at width.
func visualHeight(e *Editorleaf, width int) int {
	if width < 1 {
		return 1
	}
	b, _, _ := e.GetBytes()
	h := 0
	for _, row := range bytes.Split(b, []byte{'\n'}) {
		w := utils.WidthOnScreen(row)
		h += max(1, (w+width-1)/width)
	}
	return max(h, 1)
}

// formHeights decides the height of every field from the heights they want
// (wanted) and the rows available (avail; the header row is not included).
// Every field gets what it wants when that fits. Otherwise the rows are shared
// equally, and the share that a short field does not need goes to the longer
// ones. A field that gets less than it wants scrolls.
func formHeights(wanted []int, avail int) []int {
	h := make([]int, len(wanted))
	total := 0
	for _, w := range wanted {
		total += w
	}
	if total <= avail {
		copy(h, wanted)
		return h
	}

	// Water-filling: give every field an equal share, and hand the share a
	// short field does not need to the others.
	left := avail
	remaining := len(wanted)
	done := make([]bool, len(wanted))
	for changed := true; changed && remaining > 0; {
		changed = false
		share := left / remaining
		for i, w := range wanted {
			if !done[i] && w <= share {
				h[i], done[i] = w, true
				left -= w
				remaining--
				changed = true
			}
		}
	}
	for i := range wanted {
		if !done[i] {
			h[i] = max(1, left/remaining)
		}
	}
	return h
}

// formResize lays out a form session from overlayRect.Y downwards and
// returns its height.
func (m *MinibufferManagerStruct) formResize(overlayRect screen.Rect) int {
	s := m.session

	maxHeight := int(float32(overlayRect.Height) * 0.2)
	if maxHeight < 5 {
		maxHeight = 5
	}

	var visible []*Field
	var wanted []int
	for _, f := range s.Fields {
		if f.Hidden {
			continue
		}
		width := overlayRect.Width - f.LabelWidth
		visible = append(visible, f)
		wanted = append(wanted, visualHeight(f.Editor, width))
	}

	heights := formHeights(wanted, max(len(visible), maxHeight-1)) // 1 row: the header

	y := overlayRect.Y + 1
	for i, f := range visible {
		r := screen.Rect{
			X:      overlayRect.X + f.LabelWidth,
			Y:      y,
			Width:  overlayRect.Width - f.LabelWidth,
			Height: heights[i],
		}
		f.Editor.Resize(r)
		y += heights[i]
	}
	return y - overlayRect.Y
}

func (m *MinibufferManagerStruct) drawForm(ctx context.Context) {
	s := m.session
	rect := m.overlayRect

	if s.Header != nil {
		hr := rect
		hr.Height = 1
		screen.Get().DrawLabel(hr, &screen.LabelParams{Style: theme.ColorDefault}, s.Header())
	}

	for _, f := range s.Fields {
		if f.Hidden {
			continue
		}
		// The label is on the first row of the field.
		lr := screen.Rect{X: rect.X, Y: f.Editor.viewArea.Y, Width: f.LabelWidth, Height: 1}
		screen.Get().DrawLabel(lr, &screen.LabelParams{Style: theme.ColorDefault}, f.Label)
		f.Editor.Draw(ctx)
	}
}

func (m *MinibufferManagerStruct) Draw(ctx context.Context) bool {
	if !m.active {
		return false
	}
	if m.session.Fields != nil {
		screen.Get().FillRect(m.overlayRect, 0, theme.ColorDefault)
		m.drawForm(ctx)
		return false
	}

	screen.Get().FillRect(m.overlayRect, 0, theme.ColorDefault)
	screen.Get().DrawLabel(m.overlayRect, &screen.LabelParams{Style: theme.ColorDefault}, m.session.Prompt)
	m.session.Editor.Draw(ctx)
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
