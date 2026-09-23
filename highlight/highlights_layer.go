package highlight

// Highlights は SpanTemplate をコピーして保持する。
// SpanTemplate にポインタを渡した場合、元の struct とデータを共有する。
type HighlightsLayer []*Highlights

func (h *HighlightsLayer) GetSpanTemplate(priority, index int) *SpanTemplate {
	if priority >= len(*h) {
		return nil
	}

	if index >= len((*h)[priority].Spans) {
		return nil
	}

	return (*h)[priority].Spans[index]
}

func (h *HighlightsLayer) Highlights(priorityIndex int) *Highlights {
	for len(*h) <= priorityIndex {
		*h = append(*h, &Highlights{})
	}

	return (*h)[priorityIndex]
}

func (h *HighlightsLayer) MaxPriority() int {
	return len(*h) - 1
}

func (h *HighlightsLayer) SpanLength(priority int) int {
	if priority >= len(*h) {
		return 0
	}

	return len((*h)[priority].Spans)
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
	for len(*h) <= priority {
		*h = append(*h, &Highlights{})
	}

	for len((*h)[priority].Spans) <= index {
		(*h)[priority].Spans = append((*h)[priority].Spans, nil)
	}

	(*h)[priority].Spans[index] = span
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
