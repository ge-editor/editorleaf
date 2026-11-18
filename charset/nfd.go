package charset

import (
	"bytes"

	"golang.org/x/text/unicode/norm"
)

// UTF-8-mac: text in NFD, the way macOS makes file names (and some files).

// IsMacNFD reports whether the text is NFD as a whole: it is NFD, it has
// something that NFC would compose, and going to NFC and back gives the very
// same text. A file that mixes NFC and NFD (or that does not come back) is not
// converted, so that writing it never changes a line the user did not touch.
func IsMacNFD(text []byte) bool {
	ascii := true
	for _, b := range text {
		if b >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		return false
	}
	if !norm.NFD.IsNormal(text) || norm.NFC.IsNormal(text) {
		return false
	}
	return bytes.Equal(norm.NFD.Bytes(norm.NFC.Bytes(text)), text)
}

func ToNFC(text []byte) []byte { return norm.NFC.Bytes(text) }
func ToNFD(text []byte) []byte { return norm.NFD.Bytes(text) }
