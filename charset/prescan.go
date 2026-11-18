package charset

import (
	"bytes"
	"unicode"
	"unicode/utf8"
)

// Info is what Prescan finds out about the bytes of a file.
type Info struct {
	Spec
	Binary bool // it looks like binary data
	Lossy  bool // no encoding reads it without loss
}

// Prescan finds out the encoding of a file before it is read into a buffer.
//
// The order: a BOM; NUL bytes (UTF-16 without a BOM, or else binary);
// ISO-2022-JP (7 bit with escape sequences, which would pass as UTF-8); valid
// UTF-8; then CP932 and EUC-JP, each only if the text converts back to the
// very same bytes. When both do, the one that reads as more natural Japanese
// wins. When nothing fits the result is Lossy (UTF-8 with U+FFFD): the buffer
// shows it but cannot write it, and ESC x reload-encoding can try another.
//
// The whole file is looked at, not only its beginning: a file that is ASCII
// for the first 64KB is still not UTF-8 if Shift_JIS follows.
func Prescan(raw []byte) Info {
	switch {
	case len(raw) == 0:
		return Info{}
	case bytes.HasPrefix(raw, bomUTF8):
		return Info{Spec: Spec{UTF8, true}}
	case bytes.HasPrefix(raw, bomUTF16LE):
		return Info{Spec: Spec{UTF16LE, true}}
	case bytes.HasPrefix(raw, bomUTF16BE):
		return Info{Spec: Spec{UTF16BE, true}}
	}

	if bytes.IndexByte(raw, 0) >= 0 {
		if spec, ok := guessUTF16(raw); ok {
			return Info{Spec: spec}
		}
		return Info{Binary: true, Lossy: true}
	}

	if isISO2022JP(raw) {
		if _, ok := Decode(raw, Spec{ISO2022JP, false}); ok {
			return Info{Spec: Spec{ISO2022JP, false}}
		}
	}

	if utf8.Valid(raw) {
		return Info{}
	}

	cpText, cpOK := Decode(raw, Spec{CP932, false})
	euText, euOK := Decode(raw, Spec{EUCJP, false})
	switch {
	case cpOK && euOK:
		if score(euText) > score(cpText) {
			return Info{Spec: Spec{EUCJP, false}}
		}
		return Info{Spec: Spec{CP932, false}}
	case cpOK:
		return Info{Spec: Spec{CP932, false}}
	case euOK:
		return Info{Spec: Spec{EUCJP, false}}
	}
	return Info{Lossy: true}
}

// guessUTF16 looks for UTF-16 without a BOM: mostly ASCII text has a NUL in
// every other byte.
func guessUTF16(raw []byte) (Spec, bool) {
	n := len(raw) / 2 * 2
	if n < 2 {
		return Spec{}, false
	}
	var even, odd int // NULs at even and odd offsets
	for i := 0; i < n; i++ {
		if raw[i] == 0 {
			if i%2 == 0 {
				even++
			} else {
				odd++
			}
		}
	}
	pairs := n / 2
	var spec Spec
	switch {
	case odd*3 >= pairs && even*20 <= pairs:
		spec = Spec{UTF16LE, false}
	case even*3 >= pairs && odd*20 <= pairs:
		spec = Spec{UTF16BE, false}
	default:
		return Spec{}, false
	}
	_, ok := Decode(raw, spec)
	return spec, ok
}

// isISO2022JP reports whether the bytes are 7 bit and switch to JIS X 0208
// (ESC $ B or ESC $ @).
func isISO2022JP(b []byte) bool {
	found := false
	for i, c := range b {
		if c >= 0x80 {
			return false
		}
		if c == 0x1B && i+2 < len(b) && b[i+1] == '$' && (b[i+2] == 'B' || b[i+2] == '@') {
			found = true
		}
	}
	return found
}

// score rates how natural a text is as Japanese: kana and kanji count for it;
// half-width katakana (what EUC-JP bytes look like in CP932), U+FFFD, private
// use and control characters count against it.
func score(text []byte) int {
	s := 0
	for i := 0; i < len(text); {
		r, n := utf8.DecodeRune(text[i:])
		i += n
		switch {
		case r >= 0x3041 && r <= 0x30FF, r >= 0x4E00 && r <= 0x9FFF:
			s += 2
		case r >= 0xFF61 && r <= 0xFF9F:
			s -= 3
		case r == utf8.RuneError, r >= 0xE000 && r <= 0xF8FF:
			s -= 5
		case r >= 0x80 && r < 0xA0, r < 0x20 && !unicode.IsSpace(r):
			s -= 5
		}
	}
	return s
}
