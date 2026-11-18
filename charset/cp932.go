package charset

import (
	"fmt"
	"sync"
	"unicode/utf8"
)

// CP932 (Windows-31J): ASCII, half-width katakana 0xA1-0xDF, and double byte
// codes (lead 0x81-0x9F, 0xE0-0xFC; trail 0x40-0x7E, 0x80-0xFC) from the
// table in cp932_table.go. 0x5C and 0x7E are the ASCII characters, unlike
// JIS X 0201. x/text has no CP932 (its Shift_JIS lacks the NEC and IBM
// extensions that Windows files use).

const (
	cp932TrailMin   = 0x40
	cp932TrailCount = 0xFC - 0x40 + 1 // 189 slots; 0x7F is never assigned
	cp932LeadMin    = 0x81
)

func isCP932Lead(b byte) bool {
	return (b >= 0x81 && b <= 0x9F) || (b >= 0xE0 && b <= 0xFC)
}

// cp932Decode converts to UTF-8. ok is false when a byte could not be read;
// U+FFFD is put in its place.
func cp932Decode(src []byte) (out []byte, ok bool) {
	out = make([]byte, 0, len(src)+len(src)/2)
	ok = true
	for i := 0; i < len(src); {
		b := src[i]
		switch {
		case b < 0x80:
			out = append(out, b)
			i++
		case b >= 0xA1 && b <= 0xDF:
			out = utf8.AppendRune(out, 0xFF61+rune(b-0xA1))
			i++
		case isCP932Lead(b) && i+1 < len(src):
			t := src[i+1]
			if t >= cp932TrailMin && t <= 0xFC && t != 0x7F {
				if r := cp932Double[int(b-cp932LeadMin)*cp932TrailCount+int(t-cp932TrailMin)]; r != 0 {
					out = utf8.AppendRune(out, rune(r))
					i += 2
					continue
				}
			}
			ok = false
			out = utf8.AppendRune(out, utf8.RuneError)
			i++
		default:
			ok = false
			out = utf8.AppendRune(out, utf8.RuneError)
			i++
		}
	}
	return out, ok
}

// cp932Rank orders the codes that have the same character; the lowest wins
// when writing, as Windows does: JIS X 0208 (and the second level), then the
// NEC row 13, then the IBM extension, then the NEC selection of the IBM
// extension (rows 0xED and 0xEE).
func cp932Rank(lead int) int {
	switch {
	case lead <= 0x84, lead >= 0x88 && lead <= 0x9F, lead >= 0xE0 && lead <= 0xEA:
		return 0
	case lead == 0x87:
		return 1
	case lead >= 0xFA:
		return 2
	case lead == 0xED || lead == 0xEE:
		return 3
	}
	return 4 // user defined area
}

var (
	cp932Once   sync.Once
	cp932ToCode map[rune]uint16
)

func buildCP932Encode() {
	cp932ToCode = make(map[rune]uint16, 9300)
	for lead := 0x81; lead <= 0xFC; lead++ {
		for trail := 0x40; trail <= 0xFC; trail++ {
			r := rune(cp932Double[(lead-cp932LeadMin)*cp932TrailCount+(trail-cp932TrailMin)])
			if r == 0 {
				continue
			}
			code := uint16(lead<<8 | trail)
			if old, exists := cp932ToCode[r]; !exists || cp932Rank(lead) < cp932Rank(int(old>>8)) {
				cp932ToCode[r] = code
			}
		}
	}
}

type cp932Error struct {
	offset int
	r      rune
}

func (e *cp932Error) Error() string {
	return fmt.Sprintf("U+%04X cannot be written as CP932", e.r)
}

// cp932Encode converts UTF-8 to CP932. A character that CP932 does not have
// (or invalid UTF-8) is a *cp932Error.
func cp932Encode(src []byte) ([]byte, error) {
	cp932Once.Do(buildCP932Encode)
	out := make([]byte, 0, len(src))
	for i := 0; i < len(src); {
		b := src[i]
		if b < 0x80 {
			out = append(out, b)
			i++
			continue
		}
		r, n := utf8.DecodeRune(src[i:])
		if r == utf8.RuneError && n <= 1 {
			return nil, &cp932Error{offset: i, r: r}
		}
		if r >= 0xFF61 && r <= 0xFF9F {
			out = append(out, byte(r-0xFF61)+0xA1)
			i += n
			continue
		}
		code, ok := cp932ToCode[r]
		if !ok {
			return nil, &cp932Error{offset: i, r: r}
		}
		out = append(out, byte(code>>8), byte(code))
		i += n
	}
	return out, nil
}
