package editorleaf

import (
	"strings"
	"unicode"
)

type Case int

const (
	// CaseUnknown Case = iota
	CaseSnake Case = iota
	CaseScreamingSnake
	CaseKebab
	CaseTrain
	CaseLowerCamel
	CaseUpperCamel
	CaseLower
	CaseUpper
	CaseWord
)

var CaseTypes = [...]string{
	"snake_case",
	"SCREAMING_SNAKE_CASE",
	"kebab-case",
	"Train-Case",
	"lowerCamelCase",
	"UpperCamelCase",
	"lowercase",
	"UPPERCASE",
	"Word",
}

type CaseChangeState struct {
	Start int
	End   int
	Text  string
	Case  Case
}

func splitCaseWords(s string) []string {
	// Keep the behavior of the original extension:
	// ASCII letters and numbers are treated as word components.
	return strings.FieldsFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') &&
			!(r >= 'A' && r <= 'Z') &&
			!(r >= '0' && r <= '9')
	})
}

func formatCase(words []string, typ string) string {
	switch typ {
	case "snake_case":
		return strings.Join(lowerWords(words), "_")

	case "SCREAMING_SNAKE_CASE":
		return strings.Join(upperWords(words), "_")

	case "kebab-case":
		return strings.Join(lowerWords(words), "-")

	case "Train-Case":
		return strings.Join(titleWords(words), "-")

	case "lowerCamelCase":
		if len(words) == 0 {
			return ""
		}

		result := make([]string, len(words))
		result[0] = strings.ToLower(words[0])

		for i := 1; i < len(words); i++ {
			result[i] = upperFirst(words[i])
		}

		return strings.Join(result, "")

	case "UpperCamelCase":
		result := make([]string, len(words))

		for i, word := range words {
			result[i] = upperFirst(word)
		}

		return strings.Join(result, "")

	case "lowercase":
		return strings.Join(lowerWords(words), " ")

	case "UPPERCASE":
		return strings.Join(upperWords(words), " ")

	case "Word":
		return strings.Join(titleWords(words), " ")

	default:
		return ""
	}
}

func lowerWords(words []string) []string {
	result := make([]string, len(words))

	for i, word := range words {
		result[i] = strings.ToLower(word)
	}

	return result
}

func upperWords(words []string) []string {
	result := make([]string, len(words))

	for i, word := range words {
		result[i] = strings.ToUpper(word)
	}

	return result
}

func titleWords(words []string) []string {
	result := make([]string, len(words))

	for i, word := range words {
		result[i] = upperFirst(word)
	}

	return result
}

func upperFirst(s string) string {
	if s == "" {
		return ""
	}

	runes := []rune(strings.ToLower(s))
	runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]

	return string(runes)
}

///////////////////////

func guessCase(word string) string {
	switch {
	case isTrainCase(word):
		return "Train-Case"

	case isKebabCase(word):
		return "kebab-case"

	case isScreamingSnakeCase(word):
		return "SCREAMING_SNAKE_CASE"

	case isSnakeCase(word):
		return "snake_case"

	case isUpperCamelCase(word):
		return "UpperCamelCase"

	case isLowerCamelCase(word):
		return "lowerCamelCase"

	case isUppercase(word):
		return "UPPERCASE"

	case isLowercase(word):
		return "lowercase"

	case isWordCase(word):
		return "Word"
	}

	return ""
}

func isTrainCase(s string) bool {
	words := strings.Split(s, "-")
	if len(words) < 2 {
		return false
	}

	for _, word := range words {
		if !isTitleWord(word) {
			return false
		}
	}

	return true
}

func isKebabCase(s string) bool {
	words := strings.Split(s, "-")
	return len(words) >= 2 && allLowerOrNumber(words)
}

func isScreamingSnakeCase(s string) bool {
	words := strings.Split(s, "_")
	return len(words) >= 2 && allUpperOrNumber(words)
}

func isSnakeCase(s string) bool {
	words := strings.Split(s, "_")
	return len(words) >= 2 && allLowerOrNumber(words)
}

func isUpperCamelCase(s string) bool {
	words := camelWords(s)
	return len(words) >= 2 && isTitleWords(words)
}

func isLowerCamelCase(s string) bool {
	words := camelWords(s)
	return len(words) >= 2 &&
		isLowerWord(words[0]) &&
		isTitleWords(words[1:])
}

func isUppercase(s string) bool {
	words := strings.Fields(s)
	return len(words) >= 2 && allUpperOrNumber(words)
}

func isLowercase(s string) bool {
	words := strings.Fields(s)
	return len(words) >= 2 && allLowerOrNumber(words)
}

func isWordCase(s string) bool {
	words := strings.Fields(s)
	return len(words) >= 2 && isTitleWords(words)
}

// ///
func isTitleWord(s string) bool {
	if s == "" {
		return false
	}

	runes := []rune(s)
	if !unicode.IsUpper(runes[0]) {
		return false
	}

	for _, r := range runes[1:] {
		if !unicode.IsLower(r) && !unicode.IsDigit(r) {
			return false
		}
	}

	return true
}

func allLowerOrNumber(words []string) bool {
	if len(words) == 0 {
		return false
	}

	for _, word := range words {
		if word == "" {
			return false
		}

		for _, r := range word {
			if !unicode.IsLower(r) && !unicode.IsDigit(r) {
				return false
			}
		}
	}

	return true
}

func allUpperOrNumber(words []string) bool {
	if len(words) == 0 {
		return false
	}

	for _, word := range words {
		if word == "" {
			return false
		}

		for _, r := range word {
			if !unicode.IsUpper(r) && !unicode.IsDigit(r) {
				return false
			}
		}
	}

	return true
}

func camelWords(s string) []string {
	if s == "" {
		return nil
	}

	runes := []rune(s)
	words := make([]string, 0)

	start := 0

	for i := 1; i < len(runes); i++ {
		if unicode.IsUpper(runes[i]) {
			words = append(words, string(runes[start:i]))
			start = i
		}
	}

	words = append(words, string(runes[start:]))

	return words
}

func isTitleWords(words []string) bool {
	if len(words) == 0 {
		return false
	}

	for _, word := range words {
		if !isTitleWord(word) {
			return false
		}
	}

	return true
}

func isLowerWord(s string) bool {
	if s == "" {
		return false
	}

	for _, r := range s {
		if !unicode.IsLower(r) && !unicode.IsDigit(r) {
			return false
		}
	}

	return true
}

/////

/*
// ChangeCase changes word to the next case.
// If word does not have a recognized case, snake_case is returned.
func ChangeCase(word string) string {
	words := splitCaseWords(word)
	if len(words) == 0 {
		return word
	}

	current := guessCase(word)
	next := nextCaseType(current)

	return formatCase(words, next)
}
*/

////////////

// ChangeCase changes the word under the cursor to the next case.
/*
func (e *Editorleaf) ChangeCase() {
	start, end, ok := e.RowsPos()
	if !ok {
		return
	}

	text := e.TextRange(start, end)
	if text == "" {
		return
	}

	var next Case

	if e.caseChange.Start == start &&
		e.caseChange.End == end &&
		e.caseChange.Text == text {

		next = NextCase(e.caseChange.Case)

	} else {
		current := GuessCase(text)
		next = NextCase(current)
	}

	replaced := FormatCase(SplitCaseWords(text), next)
	if replaced == text {
		return
	}

	e.Replace(start, end, replaced)

	e.caseChange = CaseChangeState{
		Start: start,
		End:   end,
		Text:  replaced,
		Case:  next,
	}
}
*/

/*
// GuessTargetWord finds the word around the specified column.
//
// A target consists of ASCII letters, digits, '_' and '-'.
// '-' at the beginning or end is excluded from the target.
// The first and last characters of the resulting word must be ASCII letters.
// A word must contain at least two characters.
func (e *Editorleaf) GuessTargetWord(line, character int) (start, end int, word string, ok bool) {
	row := e.editBuffer.Rows.Row(line)
	text := row.String()

	if character < 0 || character > len(text) {
		return 0, 0, "", false
	}

	// Find start position.
	start = character

	for start > 0 && isCaseWordChar(text[start-1]) {
		start--
	}

	// Find end position.
	end = character

	for end < len(text) && isCaseWordChar(text[end]) {
		end++
	}

	if start == end {
		return 0, 0, "", false
	}

	word = text[start:end]

	// '-' at the beginning.
	for len(word) > 0 && word[0] == '-' {
		word = word[1:]
		start++
	}

	// '-' at the end.
	for len(word) > 0 && word[len(word)-1] == '-' {
		word = word[:len(word)-1]
		end--
	}

	if len(word) < 2 {
		return 0, 0, "", false
	}

	// First character must be an ASCII letter.
	if !isCaseLetter(word[0]) {
		return 0, 0, "", false
	}

	// Last character must be an ASCII letter or digit.
	if !isCaseLetter(word[len(word)-1]) &&
		!isCaseDigit(word[len(word)-1]) {
		return 0, 0, "", false
	}

	return start, end, word, true
}

func isCaseWordChar(c byte) bool {
	return isCaseLetter(c) ||
		isCaseDigit(c) ||
		c == '_' ||
		c == '-'
}

func isCaseLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z')
}

func isCaseDigit(c byte) bool {
	return c >= '0' && c <= '9'
}
*/
