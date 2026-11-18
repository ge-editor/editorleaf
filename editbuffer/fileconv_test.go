package editbuffer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ge-editor/editorleaf/charset"
)

func newLoaded(t *testing.T, content []byte) (*EditBuffer, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	eb := NewEditBuffer(nil)
	eb.UndoAction = NewUndoStack()
	eb.SetPath(path)
	if err := eb.Load(); err != nil {
		t.Fatal(err)
	}
	return eb, path
}

func rowsText(eb *EditBuffer) string {
	text, _, _ := eb.Bytes()
	return string(text)
}

func TestLoadConvertsAndRemembersTheFile(t *testing.T) {
	sjis := []byte{0x93, 0xFA, 0x96, 0x7B, '\r', '\n', 0x8C, 0xEA, '\r', '\n'} // 日本\r\n語\r\n
	eb, _ := newLoaded(t, sjis)

	if got := rowsText(eb); got != "日本\n語\n" {
		t.Fatalf("rows = %q", got)
	}
	if eb.GetEncoding() != "CP932" || eb.GetNewLine() != NewlineTypeCRLF {
		t.Fatalf("%s %v", eb.GetEncoding(), eb.GetNewLine())
	}
	if eb.IsDirtyFlag() || eb.IsReadonly() {
		t.Fatal("a file that was just read is not modified or read-only")
	}
	if eb.LoadResult()&ResultFromShiftJIS == 0 {
		t.Fatalf("result = %v", eb.LoadResult())
	}

	// What Save writes (see Save) is the file again.
	text, _, _ := eb.Bytes()
	out, err := charset.Encode(text, eb.EncodingSpec(), eb.codec.macNFD, eb.NewlineType.Bytes())
	if err != nil || string(out) != string(sjis) {
		t.Fatalf("written %x, %v", out, err)
	}
}

func TestLoadFromStdinConvertsToo(t *testing.T) {
	eb := NewEditBuffer(nil)
	if err := eb.LoadFrom(strings.NewReader("\xEF\xBB\xBFabc\n")); err != nil {
		t.Fatal(err)
	}
	if eb.GetEncoding() != "UTF-8-BOM" || rowsText(eb) != "abc\n" {
		t.Fatalf("%s %q", eb.GetEncoding(), rowsText(eb))
	}
}

func TestMixedNewlinesAreReported(t *testing.T) {
	eb, _ := newLoaded(t, []byte("a\r\nb\r\nc\nd"))
	if eb.GetNewLine() != NewlineTypeCRLF {
		t.Fatalf("newline = %v", eb.GetNewLine())
	}
	if eb.LoadResult()&ResultMixedNewline == 0 {
		t.Fatalf("result = %v", eb.LoadResult())
	}
}

func TestLossyFileIsReadOnlyAndNotSaved(t *testing.T) {
	eb, path := newLoaded(t, []byte{'a', 0x81, ' ', 'b'}) // not valid in any encoding
	if !eb.IsLossy() || !eb.IsReadonly() {
		t.Fatal("a file that was not read without loss must be read-only")
	}
	if !strings.HasSuffix(eb.GetEncoding(), "!") {
		t.Fatalf("mode line name = %q", eb.GetEncoding())
	}
	if _, err := eb.Save(); err != ErrLossy {
		t.Fatalf("Save err = %v", err)
	}
	if b, _ := os.ReadFile(path); string(b) != "a\x81 b" {
		t.Fatalf("the file was changed: %q", b)
	}
}

func TestReloadAsReadsAnotherEncoding(t *testing.T) {
	euc := []byte{0xC6, 0xFC, 0xCB, 0xDC, '\n'} // 日本\n
	eb, _ := newLoaded(t, euc)
	if eb.GetEncoding() != "EUC-JP" {
		t.Fatalf("detected %s", eb.GetEncoding())
	}
	if err := eb.ReloadAs(charset.Spec{Enc: charset.UTF8}); err != nil {
		t.Fatal(err)
	}
	if !eb.IsLossy() {
		t.Fatal("EUC-JP bytes are not UTF-8")
	}
	if err := eb.ReloadAs(charset.Spec{Enc: charset.EUCJP}); err != nil {
		t.Fatal(err)
	}
	if eb.IsLossy() || rowsText(eb) != "日本\n" {
		t.Fatalf("lossy %v rows %q", eb.IsLossy(), rowsText(eb))
	}
}

func TestChangingEncodingOrNewlineModifiesTheBuffer(t *testing.T) {
	eb, _ := newLoaded(t, []byte("abc\n"))
	if eb.IsDirtyFlag() {
		t.Fatal("modified before the change")
	}

	eb.SetNewline(NewlineTypeCRLF)
	if !eb.IsDirtyFlag() || eb.GetNewLine() != NewlineTypeCRLF {
		t.Fatal("a newline change must modify the buffer")
	}
	eb.metaDirty = false

	eb.SetEncoding(charset.Spec{Enc: charset.UTF8, BOM: true})
	if !eb.IsDirtyFlag() || eb.GetEncoding() != "UTF-8-BOM" {
		t.Fatalf("%s dirty=%v", eb.GetEncoding(), eb.IsDirtyFlag())
	}

	// The same value is not a change.
	eb.metaDirty = false
	eb.SetEncoding(charset.Spec{Enc: charset.UTF8, BOM: true})
	eb.SetNewline(NewlineTypeCRLF)
	if eb.IsDirtyFlag() {
		t.Fatal("setting the same value must not modify the buffer")
	}

	// A character that the encoding cannot hold is found before saving.
	eb2 := NewEditBuffer(nil)
	if err := eb2.LoadFrom(strings.NewReader("a\n😀\n")); err != nil {
		t.Fatal(err)
	}
	if err := eb2.CheckEncoding(charset.Spec{Enc: charset.CP932}); err == nil {
		t.Fatal("an emoji in CP932 was accepted")
	}
}
