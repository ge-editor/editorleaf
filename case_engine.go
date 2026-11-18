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

// splitCaseWords splits s into its words: at every character that is not an
// ASCII letter or digit, and inside a piece at the places where a camel-case
// word starts ("fooBar" -> foo, Bar; "HTTPServer" -> HTTP, Server). A digit
// belongs to the word before it ("foo2Bar" -> foo2, Bar).
func splitCaseWords(s string) []string {
	var words []string

	for _, piece := range strings.FieldsFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') &&
			!(r >= 'A' && r <= 'Z') &&
			!(r >= '0' && r <= '9')
	}) {
		words = append(words, splitCamel(piece)...)
	}

	return words
}

// splitCamel splits one piece that has only ASCII letters and digits at the
// places where a camel-case word starts.
func splitCamel(piece string) []string {
	isUp := func(c byte) bool { return c >= 'A' && c <= 'Z' }
	isLow := func(c byte) bool { return c >= 'a' && c <= 'z' }
	isDig := func(c byte) bool { return c >= '0' && c <= '9' }

	var words []string

	start := 0

	for i := 1; i < len(piece); i++ {
		prev, cur := piece[i-1], piece[i]

		switch {
		// fooBar, foo2Bar
		case isUp(cur) && (isLow(prev) || isDig(prev)):
			words = append(words, piece[start:i])
			start = i

		// HTTPServer: the last capital of a run starts the next word
		case isUp(cur) && isUp(prev) && i+1 < len(piece) && isLow(piece[i+1]):
			words = append(words, piece[start:i])
			start = i
		}
	}

	return append(words, piece[start:])
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

// identifierCases are the cases that keep one token (no spaces). They are the
// cycle of ChangeCase for a text that has no space.
var identifierCases = []Case{
	CaseSnake, CaseScreamingSnake, CaseKebab, CaseTrain, CaseLowerCamel, CaseUpperCamel,
}

// phraseCases are all the cases. They are the cycle for a text that has a
// space (a phrase).
var phraseCases = []Case{
	CaseSnake, CaseScreamingSnake, CaseKebab, CaseTrain, CaseLowerCamel, CaseUpperCamel,
	CaseLower, CaseUpper, CaseWord,
}

// caseIndexOf returns the Case that has the name, or -1 ("unknown").
func caseIndexOf(name string) int {
	for i, n := range CaseTypes {
		if n == name {
			return i
		}
	}

	return -1
}

const caseAffixChars = "_- "

// splitCaseAffixes cuts the '_', '-' and ' ' at both ends off text, which
// would be lost by the conversion ("_privateName" stays "_private_name").
// core is empty when text has nothing else.
func splitCaseAffixes(text string) (prefix, core, suffix string) {
	core = strings.Trim(text, caseAffixChars)
	if core == "" {
		return "", "", ""
	}

	prefix = text[:len(text)-len(strings.TrimLeft(text, caseAffixChars))]
	suffix = text[len(strings.TrimRight(text, caseAffixChars)):]

	return prefix, core, suffix
}

// isCaseConvertible reports whether every character of core is one that the
// conversion keeps: ASCII letters, digits, '_', '-' and ' '. Anything else
// (other scripts, tabs, line breaks) would be lost when the words are joined
// again, so such a text is left alone.
func isCaseConvertible(core string) bool {
	for i := 0; i < len(core); i++ {
		c := core[i]
		if !isCaseWordChar(c) && c != ' ' {
			return false
		}
	}

	return true
}

// guessCurrentCase returns the Case that text seems to be in, or -1.
func guessCurrentCase(text string) int {
	_, core, _ := splitCaseAffixes(text)

	return caseIndexOf(guessCase(core))
}

// caseCandidate is one step of the cycle: a case and the text it gives.
type caseCandidate struct {
	c    Case
	text string
}

// caseCandidates returns the cycle of the words, in order, with every
// distinct text once (the first case that gives it). One word "foo" gives
// snake_case, kebab-case and lowerCamelCase the same text; without this the
// cycle would go foo, FOO, foo, Foo, foo, Foo, ...
func caseCandidates(prefix string, words []string, suffix string, cycle []Case) []caseCandidate {
	out := make([]caseCandidate, 0, len(cycle))
	seen := make(map[string]bool, len(cycle))

	for _, c := range cycle {
		t := prefix + formatCase(words, CaseTypes[c]) + suffix
		if seen[t] {
			continue
		}

		seen[t] = true

		out = append(out, caseCandidate{c: c, text: t})
	}

	return out
}

// nextCaseText converts text to the case after cur and returns the new text
// and that case.
//
// cur is the case text is known to be in (-1 if unknown). It is kept by the
// caller and handed back, because guessing it from the text is ambiguous: "foo"
// is lowercase, snake_case, kebab-case and lowerCamelCase at once. When cur
// is not a step of the cycle (unknown, or from the cycle of another kind of
// text), the step whose text is the text itself is taken as the place.
//
// A step that gives the text itself is skipped, and at most one turn of the
// cycle is made, so there is no endless loop: ok is false when no step gives
// another text (or text cannot be converted), and nothing should be changed.
func nextCaseText(text string, cur int) (out string, next int, ok bool) {
	prefix, core, suffix := splitCaseAffixes(text)
	if core == "" || !isCaseConvertible(core) {
		return "", cur, false
	}

	words := splitCaseWords(core)
	if len(words) == 0 {
		return "", cur, false
	}

	cycle := identifierCases
	if strings.Contains(core, " ") {
		cycle = phraseCases
	}

	steps := caseCandidates(prefix, words, suffix, cycle)

	pos := -1

	for i, st := range steps {
		if int(st.c) == cur {
			pos = i
			break
		}
	}

	if pos < 0 {
		for i, st := range steps {
			if st.text == text {
				pos = i
				break
			}
		}
	}

	// pos is -1 when the text is in no step: start at the first one.
	for k := 1; k <= len(steps); k++ {
		st := steps[(pos+k)%len(steps)]
		if st.text != text {
			return st.text, int(st.c), true
		}
	}

	return "", cur, false
}

// ---- the word around the cursor ----

// GuessTargetWord finds the word around the byte column character of row
// line.
//
// A target consists of ASCII letters, digits, '_' and '-'.
// '-' at the beginning or end is excluded from the target.
// The first character of the resulting word must be an ASCII letter and the
// last one an ASCII letter or digit.
// A word must contain at least two characters.
//
// start and end are byte columns, end exclusive.
func (e *Editorleaf) GuessTargetWord(line, character int) (start, end int, word string, ok bool) {
	row := e.editBuffer.Rows().Row(line)
	if row == nil {
		return 0, 0, "", false
	}

	return guessTargetWord(row.Bytes(), character)
}

// guessTargetWord is GuessTargetWord on the bytes of one row.
func guessTargetWord(text []byte, character int) (start, end int, word string, ok bool) {
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

	// '-' at the beginning and at the end.
	for start < end && text[start] == '-' {
		start++
	}

	for end > start && text[end-1] == '-' {
		end--
	}

	if end-start < 2 {
		return 0, 0, "", false
	}

	// First character must be an ASCII letter.
	if !isCaseLetter(text[start]) {
		return 0, 0, "", false
	}

	// Last character must be an ASCII letter or digit.
	if last := text[end-1]; !isCaseLetter(last) && !isCaseDigit(last) {
		return 0, 0, "", false
	}

	return start, end, string(text[start:end]), true
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
