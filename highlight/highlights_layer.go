package highlight

// Highlights は SpanTemplate をコピーして保持する。
// SpanTemplate にポインタを渡した場合、元の struct とデータを共有する。
type HighlightsLayer []*Highlights

// NewHighlightsLayer returns a layer with a slot for every priority already
// in place. The slots of a layer are then never added while the editor is
// running, so the draw goroutine can read the layer while the event loop
// replaces the spans of a slot.
func NewHighlightsLayer() *HighlightsLayer {
	h := make(HighlightsLayer, LayerCursor+1)
	for i := range h {
		h[i] = NewHighlights()
	}
	return &h
}

func (h *HighlightsLayer) GetSpanTemplate(priority, index int) *SpanTemplate {
	if priority < 0 || priority >= len(*h) {
		return nil
	}

	return (*h)[priority].spanTemplate(index)
}

func (h *HighlightsLayer) Highlights(priorityIndex int) *Highlights {
	for len(*h) <= priorityIndex {
		*h = append(*h, &Highlights{})
	}

	return (*h)[priorityIndex]
}

// Peek は Highlights と違い、priorityIndex のスロットを自動生成しない。
// まだ存在しなければ nil を返す。FindHighlightSpan のように
// 「このレイヤーに priorityIndex のデータがあるか」を副作用なしに
// 確認したい参照側で使う。Highlights() は書き込み側 (AppendSpan 前の
// 確保、Clear) 用で、呼ぶだけでスロットが生えてしまうため nil
// チェックの代わりには使えない。
func (h *HighlightsLayer) Peek(priorityIndex int) *Highlights {
	if priorityIndex < 0 || priorityIndex >= len(*h) {
		return nil
	}

	return (*h)[priorityIndex]
}

// Version is the sum of slot versions. Each slot version increases whenever
// its spans change, so the sum invalidates cached row styles on updates.
func (h *HighlightsLayer) Version() uint64 {
	var v uint64
	for _, hl := range *h {
		if hl != nil {
			v += hl.Version()
		}
	}
	return v
}

func (h *HighlightsLayer) MaxPriority() int {
	return len(*h) - 1
}

func (h *HighlightsLayer) SpanLength(priority int) int {
	if priority < 0 || priority >= len(*h) {
		return 0
	}

	return (*h)[priority].SpansLength()
}

/* func (h *HighlightsLayer) AppendSpan(
	priority int,
	span *SpanTemplate,
) {
	for len(*h) <= priority {
		*h = append(*h, &Highlights{})
	}

	(*h)[priority].Spans = append((*h)[priority].Spans, span)
}
*/

func (h *HighlightsLayer) SetSpan(
	priority int,
	index int,
	span *SpanTemplate,
) {
	h.Highlights(priority).SetSpan(index, span)
}

/*
func (h *HighlightsLayer) Clear(
	priority int,
) {
	(*h)[priority].Spans = (*h)[priority].Spans[:0]
}
*/

/*
func (h *HighlightsLayer) Set(
	priority int,
	highlight *Highlights,
) {
	for len(*h) <= priority {
		*h = append(*h, &Highlights{})
	}

	(*h)[priority] = highlight
}
*/

/*
func (h *HighlightsLayer) Get(priority int) *Highlights {
	if len(*h) <= priority {
		return nil
	}
	return (*h)[priority]
}
*/
