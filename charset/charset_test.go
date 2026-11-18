package charset

import (
	"bytes"
	"errors"
	"testing"
)

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b := make([]byte, 0, len(s)/2)
	for i := 0; i+1 < len(s); i += 2 {
		var v byte
		for _, c := range []byte(s[i : i+2]) {
			v <<= 4
			switch {
			case c >= '0' && c <= '9':
				v |= c - '0'
			case c >= 'a' && c <= 'f':
				v |= c - 'a' + 10
			default:
				t.Fatalf("bad hex %q", s)
			}
		}
		b = append(b, v)
	}
	return b
}

func TestParseSpec(t *testing.T) {
	for in, want := range map[string]Spec{
		"UTF-8":        {UTF8, false},
		"utf8":         {UTF8, false},
		"utf-8-bom":    {UTF8, true},
		"UTF-16LE":     {UTF16LE, false},
		"utf-16be-bom": {UTF16BE, true},
		"Shift_JIS":    {CP932, false},
		"sjis":         {CP932, false},
		"cp932":        {CP932, false},
		"EUC-JP":       {EUCJP, false},
		"iso-2022-jp":  {ISO2022JP, false},
	} {
		got, ok := ParseSpec(in)
		if !ok || got != want {
			t.Errorf("ParseSpec(%q) = %v, %v; want %v", in, got, ok, want)
		}
	}
	if _, ok := ParseSpec("latin1"); ok {
		t.Error("an unknown name was accepted")
	}
	// The names offered for completion are accepted.
	for _, n := range SpecNames() {
		if s, ok := ParseSpec(n); !ok || s.String() != n {
			t.Errorf("ParseSpec(%q) = %v, %v", n, s, ok)
		}
	}
}

func TestCP932Bytes(t *testing.T) {
	// あ ①(NEC row 13) ㈱(NEC row 13, not the IBM extension) 髙(IBM
	// extension, not the NEC selection) ｱ(half-width) and ASCII \ ~.
	text := []byte("あ①㈱髙ｱ\\~")
	want := unhex(t, "82a08740878afbfcb15c7e")

	got, err := Encode(text, Spec{CP932, false}, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("encoded %x, want %x", got, want)
	}
	back, lossless := Decode(got, Spec{CP932, false})
	if !lossless || !bytes.Equal(back, text) {
		t.Fatalf("decoded %q (lossless %v)", back, lossless)
	}
}

func TestPrescan(t *testing.T) {
	jp := "日本語のテキスト"
	for _, c := range []struct {
		name string
		raw  []byte
		want Spec
	}{
		{"empty", nil, Spec{UTF8, false}},
		{"ascii", []byte("hello\n"), Spec{UTF8, false}},
		{"utf8", []byte(jp), Spec{UTF8, false}},
		{"utf8 bom", append([]byte{0xEF, 0xBB, 0xBF}, jp...), Spec{UTF8, true}},
		{"utf16le bom", []byte{0xFF, 0xFE, 'a', 0}, Spec{UTF16LE, true}},
		{"utf16be bom", []byte{0xFE, 0xFF, 0, 'a'}, Spec{UTF16BE, true}},
		{"utf16le", []byte{'a', 0, 'b', 0, '\n', 0}, Spec{UTF16LE, false}},
		{"utf16be", []byte{0, 'a', 0, 'b', 0, '\n'}, Spec{UTF16BE, false}},
		{"cp932", unhex(t, "93fa967b8cea82cc8365834c835883678141"), Spec{CP932, false}},
		{"euc-jp", unhex(t, "c6fccbdcb8eca4cea5c6a5ada5b9a5c8"), Spec{EUCJP, false}},
		{"iso-2022-jp", unhex(t, "1b2442467c4b5c386c244e2546252d253925481b2842"), Spec{ISO2022JP, false}},
	} {
		info := Prescan(c.raw)
		if info.Spec != c.want || info.Binary || info.Lossy {
			t.Errorf("%s: %+v, want %v", c.name, info, c.want)
		}
	}

	// Not text, and not any encoding that is known.
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	if info := Prescan(png); !info.Binary || !info.Lossy {
		t.Errorf("binary: %+v", info)
	}
	if info := Prescan([]byte{'a', 0x81}); !info.Lossy || info.Binary {
		t.Errorf("a lone lead byte: %+v", info)
	}
}

// The whole file is looked at: Shift_JIS after a long ASCII start is not UTF-8.
func TestPrescanLooksPastTheStart(t *testing.T) {
	raw := append(bytes.Repeat([]byte("a"), 100*1024), unhex(t, "93fa967b8cea")...)
	if info := Prescan(raw); info.Spec != (Spec{CP932, false}) {
		t.Fatalf("%+v", info)
	}
}

func TestOpenMacNFDWholeFileOnly(t *testing.T) {
	nfd := "か\u3099き\u3099\n" // が ぎ decomposed
	f := Open([]byte(nfd))
	if !f.MacNFD || string(f.Text) != "がぎ\n" {
		t.Fatalf("MacNFD %v, text %q", f.MacNFD, f.Text)
	}
	back, err := Encode(f.Text, f.Spec, f.MacNFD, nil)
	if err != nil || string(back) != nfd {
		t.Fatalf("written back %q, %v", back, err)
	}

	// NFC and NFD mixed: left as it is, so a save changes nothing.
	mixed := "が\nか\u3099\n"
	f = Open([]byte(mixed))
	if f.MacNFD || string(f.Text) != mixed {
		t.Fatalf("mixed: MacNFD %v, text %q", f.MacNFD, f.Text)
	}
	if IsMacNFD([]byte("plain ascii")) {
		t.Fatal("ASCII is not UTF-8-mac")
	}
}

func TestEncodeBOMAndNewline(t *testing.T) {
	got, err := Encode([]byte("a\nb"), Spec{UTF16LE, true}, false, []byte("\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if want := unhex(t, "fffe61000d000a006200"); !bytes.Equal(got, want) {
		t.Fatalf("%x, want %x", got, want)
	}

	f := Open(got)
	if f.Spec != (Spec{UTF16LE, true}) || f.Lossy || string(f.Text) != "a\r\nb" {
		t.Fatalf("%+v %q", f.Spec, f.Text)
	}

	got, _ = Encode([]byte("x"), Spec{UTF8, true}, false, nil)
	if !bytes.Equal(got, []byte{0xEF, 0xBB, 0xBF, 'x'}) {
		t.Fatalf("%x", got)
	}
}

func TestEncodeErrorNamesTheLine(t *testing.T) {
	for _, spec := range []Spec{{CP932, false}, {EUCJP, false}, {ISO2022JP, false}} {
		_, err := Encode([]byte("a\nb\n😀\n"), spec, false, []byte("\r\n"))
		var ee *EncodeError
		if !errors.As(err, &ee) {
			t.Fatalf("%v: err = %v", spec, err)
		}
		if ee.Line != 3 || ee.Rune != '😀' {
			t.Errorf("%v: line %d, rune %q", spec, ee.Line, ee.Rune)
		}
	}
}

func TestLossyIsNotWrittenBack(t *testing.T) {
	// NEC selected IBM extension (0xEEE0) comes back as 0xFBFC: not the same
	// bytes, so the file is Lossy and the buffer will not write it.
	raw := unhex(t, "eee0")
	f := OpenAs(raw, Spec{CP932, false})
	if !f.Lossy {
		t.Fatal("a file that does not come back byte for byte is not Lossy")
	}
}

func TestOpenAsOtherEncoding(t *testing.T) {
	raw := unhex(t, "c6fccbdcb8ec") // 日本語 in EUC-JP
	f := OpenAs(raw, Spec{EUCJP, false})
	if f.Lossy || string(f.Text) != "日本語" {
		t.Fatalf("%+v %q", f.Spec, f.Text)
	}
	// The same bytes as UTF-8 are not valid.
	f = OpenAs(raw, Spec{UTF8, false})
	if !f.Lossy {
		t.Fatal("invalid UTF-8 is not Lossy")
	}
}
