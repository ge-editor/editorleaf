package editorleaf

import (
	"bytes"
	"context"
	"regexp"
	"strings"

	"github.com/gdamore/tcell/v3"

	"github.com/ge-editor/editorleaf/editbuffer/rows"
	"github.com/ge-editor/editorleaf/highlight"
	"github.com/ge-editor/editorleaf/search"
	"github.com/ge-editor/gecore"
	"github.com/ge-editor/theme"
)

func (e *Editorleaf) MoveNextFoundWord() {
	searchResults := e.meta.Search
	// gelog.Debug("MoveNextFoundWord", "searchResults", searchResults)

	h := e.meta.HighlightLayer
	l := h.SpanLength(highlight.LayerSearch)
	/* gelog.Debug(
		"MoveNextFoundWord HighlightLayer",
		"h", fmt.Sprintf("%p", &e.meta.HighlightLayer),
		"len", len(*e.meta.HighlightLayer),
	) */

	// gelog.Debug("MoveNextFoundWord", "searchResults", searchResults, "SpanLength", l)

	if l == 0 {
		return
	}

	if searchResults.CurrentSearchIndex == -1 {
		for i := 0; i < l; i++ {
			s := h.GetSpanTemplate(highlight.LayerSearch, i)
			span, ok := (*s).(*search.SearchResult)
			if !ok {
				continue
			}
			if span.Start.RowIndex >= e.meta.RowsPos.RowIndex {
				searchResults.CurrentSearchIndex = i
				break
			}
		}
	} else if searchResults.CurrentSearchIndex == l {
		searchResults.CurrentSearchIndex = 0
	} else {
		searchResults.CurrentSearchIndex++
	}

	if searchResults.CurrentSearchIndex < 0 {
		searchResults.CurrentSearchIndex = 0
	} else if searchResults.CurrentSearchIndex >= l {
		searchResults.CurrentSearchIndex = l - 1
	}

	s := h.GetSpanTemplate(highlight.LayerSearch, searchResults.CurrentSearchIndex)
	f, ok := (*s).(*search.SearchResult)
	if !ok {
		return
	}

	e.meta.RowsPos = f.Start
}

func (e *Editorleaf) MovePrevFoundWord() {
	searchResults := e.meta.Search

	h := e.meta.HighlightLayer

	l := h.SpanLength(highlight.LayerSearch)
	if l == 0 {
		return
	}

	if searchResults.CurrentSearchIndex == -1 {
		for i := l - 1; i >= 0; i-- {
			s := h.GetSpanTemplate(highlight.LayerSearch, i)
			span, ok := (*s).(*search.SearchResult)
			if !ok {
				continue
			}
			if span.Start.RowIndex <= e.meta.RowsPos.RowIndex {
				searchResults.CurrentSearchIndex = i
				break
			}
		}
		/* don't loop
		} else if searchResults.CurrentSearchIndex == 0 {
		searchResults.CurrentSearchIndex = l - 1
		*/
	} else {
		searchResults.CurrentSearchIndex--
	}

	if searchResults.CurrentSearchIndex < 0 {
		searchResults.CurrentSearchIndex = 0
	} else if searchResults.CurrentSearchIndex >= l {
		searchResults.CurrentSearchIndex = l - 1
	}

	s := h.GetSpanTemplate(highlight.LayerSearch, searchResults.CurrentSearchIndex)
	f, ok := (*s).(*search.SearchResult)
	if !ok {
		return
	}

	e.meta.RowsPos = f.Start
}

func (e *Editorleaf) SearchHistory() []string {
	return e.meta.Search.History.Items
}

// When not using regular expressions
func (e *Editorleaf) SearchText(text string, caseSensitive, isRegexp bool) {
	// gelog.Debug("SearchText", "search", searchResults)

	searchResults := e.meta.Search
	searchResults.CurrentSearchIndex = -1

	textLen := len(text)
	if textLen == 0 {
		return
	}

	if isRegexp {
		e.SearchRegexp(text, caseSensitive)
	} else {
		e.searchText(text, caseSensitive)
	}
}

func (e *Editorleaf) SearchRegexp(searchTerm string, caseSensitive bool) {
	e.meta.Search.SearchMu.Lock()
	defer e.meta.Search.SearchMu.Unlock()

	// 進行中の検索があればキャンセル
	if e.meta.Search.SearchCancel != nil {
		e.meta.Search.SearchCancel()
	}

	// 新しい検索用の Context と CancelFunc を作成
	ctx, cancel := context.WithCancel(context.Background())
	e.meta.Search.SearchCancel = cancel

	go func(ctx context.Context) {
		e.meta.Search.History.Add(searchTerm)

		hls := e.meta.HighlightLayer.Highlights(highlight.LayerSearch)
		hls.Clear()

		rows := e.editBuffer.Rows
		re, err := regexp.Compile(searchTerm)
		if err != nil {
			return
		}

		for rowIndex := 0; rowIndex < rows.Length(); rowIndex++ {
			matches := re.FindAllSubmatchIndex((*rows)[rowIndex], -1)
			if matches == nil {
				continue
			}
			for _, match := range matches {
				select {
				case <-ctx.Done():
					return
				default:
					a := NewFoundPosition(rowIndex, match[0], rowIndex, match[1], theme.ColorSearchFound)
					b := (highlight.SpanTemplate)(a)
					hls.AppendSpan(&b)
					// searchIndex++
				}
			}
		}
	}(ctx)
}

func (e *Editorleaf) searchText(text string, caseSensitive bool) {
	e.meta.Search.SearchMu.Lock()
	defer e.meta.Search.SearchMu.Unlock()

	// 進行中の検索があればキャンセル
	if e.meta.Search.SearchCancel != nil {
		e.meta.Search.SearchCancel()
	}

	// 新しい検索用の Context と CancelFunc を作成
	ctx, cancel := context.WithCancel(context.Background())
	e.meta.Search.SearchCancel = cancel

	go func(ctx context.Context) {
		e.meta.Search.History.Add(text)

		hls := e.meta.HighlightLayer.Highlights(highlight.LayerSearch)
		hls.Clear()

		if !caseSensitive {
			text = strings.ToLower(text)
		}
		textBytes := []byte(text)
		textBytesLen := len(textBytes)

		lines := e.editBuffer.Rows
		for i := 0; i < lines.Length(); i++ {
			line := (*lines)[i]
			index := 0
		loop:
			for limit := 0; ; limit++ {
				select {
				case <-ctx.Done():
					return
				default:
					substring := line[index:]
					if !caseSensitive {
						substring = bytes.ToLower(substring)
					}
					findIndex := bytes.Index(substring, textBytes)
					// gelog.Debug("searchText", "findIndex", findIndex, "substring", substring, "textBytes", textBytes)
					if findIndex == -1 {
						break loop
					}
					startIndex := len(line[:index+findIndex])
					stopIndex := startIndex + textBytesLen
					a := NewFoundPosition(i, startIndex, i, stopIndex, theme.ColorSearchFound)
					b := (highlight.SpanTemplate)(a)
					hls.AppendSpan(&b)
					index += findIndex + textBytesLen
				}

				if limit > 100_000 {
					gecore.Echo.AddText("Search text over 100,000")
					return
				}
			}
		}
	}(ctx)
}

func (e *Editorleaf) ReplaceCurrentSearchString(str string) {
	searchResults := e.meta.Search

	h := e.meta.HighlightLayer
	le := h.SpanLength(highlight.LayerSearch)
	if le == 0 {
		return
	}

	if searchResults.CurrentSearchIndex == -1 {
		return
	}

	// foundPosition := search.Results[search.CurrentSearchIndex]
	s := h.GetSpanTemplate(highlight.LayerSearch, searchResults.CurrentSearchIndex)
	foundPosition, ok := (*s).(*search.SearchResult)
	if !ok {
		return
	}
	e.killRegion(rows.RowsPos{RowIndex: foundPosition.Start.RowIndex, ColIndex: foundPosition.Start.ColIndex},
		rows.RowsPos{RowIndex: foundPosition.Start.RowIndex, ColIndex: foundPosition.End.ColIndex})
	e.InsertString(str)

	// Correct the changed Results within the same line where replacement is made
	// How many rune characters change due to replacement?
	l := len([]byte(str)) - (foundPosition.End.ColIndex - foundPosition.Start.ColIndex)
	// RowIndex where replacement is made
	// rowIndex := search.Results[search.CurrentSearchIndex].Start.RowIndex
	rowIndex := foundPosition.Start.RowIndex
	// Correct the changed Results due to replacement within the same line
	for i := searchResults.CurrentSearchIndex + 1; i < le; /* len(search.Results)  && search.Results[i].Start.RowIndex == rowIndex*/ i++ {

		s := h.GetSpanTemplate(highlight.LayerSearch, i)
		span, ok := (*s).(*search.SearchResult)
		if !ok {
			continue
		}
		if span.Start.RowIndex != rowIndex {
			break
		}

		span.Start.ColIndex += l
		span.End.ColIndex += l
	}
	// Exclude the replaced search result
	// What if the replacement still matches the search after replacement? No consideration for now

	// これを追加すること！！！
	// search.Results = slices.Delete(search.Results, search.CurrentSearchIndex, search.CurrentSearchIndex+1)
}

func NewFoundPosition(startRowIndex, startColIndex, stopRowIndex, stopColIndex int, color tcell.Style) *search.SearchResult {
	return &search.SearchResult{
		Span: &highlight.Span{
			Start: rows.RowsPos{
				RowIndex: startRowIndex,
				ColIndex: startColIndex,
			},
			End: rows.RowsPos{
				RowIndex: stopRowIndex,
				ColIndex: stopColIndex,
			},
			Color:    color,
			Priority: highlight.LayerSearch,
		},
		// Matches: ,
	}
}

// -----------------------------------------------
// HighlightsLayer の情報から描画色を決定する
// -----------------------------------------------

// highlightPosStatus [editor/meta][priority]spansIndex
type highlightPosStatus [][]int

// layer
const (
	highlightEditor = iota
	highlightMeta
)

func (s *highlightPosStatus) Set(layer, priority, spansIndex int) {
	for len(*s) <= layer {
		*s = append(*s, nil)
	}

	for len((*s)[layer]) <= priority {
		(*s)[layer] = append((*s)[layer], -1)
	}

	(*s)[layer][priority] = spansIndex
}

func (s highlightPosStatus) Get(layer, priority int) int {
	if layer < 0 || layer >= len(s) {
		return -1
	}
	if priority < 0 || priority >= len(s[layer]) {
		return -1
	}

	return s[layer][priority]
}

func (s *highlightPosStatus) Clear() {
	*s = nil
}

// HighlightsLayer の情報から描画色を決定する
// highlightPosStatus 以降で pos と Traverse な関係にある highlights.Span を返す。
func (e Editorleaf) FindHighlightSpan(
	pos rows.RowsPos,
	stat highlightPosStatus,
) (*highlight.Span, highlightPosStatus) {

	maxPriority := max(
		e.meta.HighlightLayer.MaxPriority(),
		e.highlightLayer.MaxPriority(),
	)

	for priorityIndex := maxPriority; priorityIndex >= 0; priorityIndex-- {

		// e.meta.HighlightLayer と e.highlightLayer では、
		// 同一 priorityIndex において、両方が Highlights を持つことはない。
		// 両方が Highlights を持たないことはある。
		layerIndex := highlightMeta
		hl := e.meta.HighlightLayer.Highlights(priorityIndex)

		if hl == nil {
			layerIndex = highlightEditor
			hl = e.highlightLayer.Highlights(priorityIndex)
		}

		if hl == nil {
			continue
		}

		currentSpanIndex := stat.Get(layerIndex, priorityIndex)

		// 現在位置とマッチしているか確認
		if currentSpanIndex >= 0 &&
			currentSpanIndex < hl.SpansLength() {

			span := hl.GetSpan(currentSpanIndex)

			if highlight.PosInSpan(pos, span) {
				return span, stat
			}
		}

		// 現在位置から再検索
		spanIndex := hl.MatcheFirstRegenSpanIndex(
			pos,
			max(0, currentSpanIndex),
		)

		if spanIndex == -1 {
			continue
		}

		span := hl.GetSpan(spanIndex)
		if span == nil {
			continue
		}

		stat.Set(layerIndex, priorityIndex, spanIndex)

		return span, stat
	}

	return nil, stat
}
