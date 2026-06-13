package mark

import (
	"fmt"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/utils"
)

func NewMarks() *marks {
	return &marks{}
}

type marks []*Mark

func (m *marks) AddMark(a *Mark) {
	*m = append(*m, a)
}

// UnsetMarkByValue removes the last matching mark by value
func (m *marks) UnsetMarkByValue(d *Mark) bool {
	i := m.indexByValueLast(d)
	if i == -1 {
		return false
	}

	*m = append((*m)[:i], (*m)[i+1:]...)
	return true
}

func (m *marks) Prev(d *Mark) *Mark {
	i := m.indexByPointer(d)
	if i <= 0 {
		return nil
	}
	return (*m)[i-1]
}

func (m *marks) Next(d *Mark) *Mark {
	i := m.indexByPointer(d)
	if i < 0 || i >= len(*m)-1 {
		return nil
	}
	return (*m)[i+1]
}

func (m *marks) FindLastByFile(ff *editbuffer.EditBuffer) *Mark {
	for i := len(*m) - 1; i >= 0; i-- {
		if (*m)[i].File == ff {
			return (*m)[i]
		}
	}
	return nil
}

// indexByPointer finds index by identity
func (m *marks) indexByPointer(d *Mark) int {
	for i := range *m {
		if d == (*m)[i] {
			return i
		}
	}
	return -1
}

// indexByPointer のラッパー
func (m *marks) HasMark(d *Mark) bool {
	return m.indexByPointer(d) != -1
}

// indexByValueLast finds last index by value equality
func (m *marks) indexByValueLast(d *Mark) int {
	for i := len(*m) - 1; i >= 0; i-- {
		if m.equal(d, (*m)[i]) {
			return i
		}
	}
	return -1
}

func (m *marks) equal(a, b *Mark) bool {
	if a.File != b.File {
		return false
	}
	return a.RowIndex == b.RowIndex &&
		a.ColIndex == b.ColIndex
}

func (m *marks) FilterByCharacters(chars string) marks {
	items := marks{}
	for i := len(*m) - 1; i >= 0; i-- {
		mk := (*m)[i]
		text := fmt.Sprintf("%s %s", mk.File.GetBase(), utils.RemoveSymbols(mk.Content))
		if chars != "" {
			if utils.ContainsAllCharacters(text, chars) {
				items = append(items, mk)
			}
		} else {
			items = append(items, mk)
		}
	}
	return items
}
