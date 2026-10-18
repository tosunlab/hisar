//go:build darwin && !ios && cgo

package pasteboard

import (
	"fmt"
	"testing"
	"time"
)

const (
	stringType    = "public.utf8-plain-text"
	concealedType = "org.nspasteboard.ConcealedType"
	transientType = "org.nspasteboard.TransientType"
)

func newPasteboardName(t *testing.T) string {
	return fmt.Sprintf("com.tryhisar.hisar.test.%s.%d", t.Name(), time.Now().UnixNano())
}

func openPrivateClipboard(t *testing.T, pasteboardName string) *Clipboard {
	clipboard := newNamed(pasteboardName)
	t.Cleanup(clipboard.release)
	return clipboard
}

func TestCopyIsConcealedAndTransient(t *testing.T) {
	clipboard := openPrivateClipboard(t, newPasteboardName(t))
	if !clipboard.SetText("pässwörd 🔑 one") {
		t.Fatal("copy failed")
	}
	for _, pasteboardType := range []string{stringType, concealedType, transientType} {
		if !clipboard.hasType(pasteboardType) {
			t.Errorf("the pasteboard has no %s", pasteboardType)
		}
	}
	if text, ok := clipboard.GetText(); !ok || text != "pässwörd 🔑 one" {
		t.Errorf("read back %q %v", text, ok)
	}
}

func TestTextCopiedByAnotherAppIsNeverRead(t *testing.T) {
	pasteboardName := newPasteboardName(t)
	clipboard := openPrivateClipboard(t, pasteboardName)
	otherApp := openPrivateClipboard(t, pasteboardName)

	if !otherApp.SetText("copied in another app") {
		t.Fatal("other app's copy failed")
	}
	if text, ok := clipboard.GetText(); ok {
		t.Errorf("read %q before copying anything", text)
	}
	if !clipboard.SetText("hisar secret") {
		t.Fatal("copy failed")
	}
	if text, ok := clipboard.GetText(); !ok || text != "hisar secret" {
		t.Fatalf("read back %q %v", text, ok)
	}
	if !otherApp.SetText("copied in another app") {
		t.Fatal("other app's copy failed")
	}
	if text, ok := clipboard.GetText(); ok {
		t.Errorf("read %q after another app copied", text)
	}
}

func TestEmptyTextClearsTheClipboard(t *testing.T) {
	clipboard := openPrivateClipboard(t, newPasteboardName(t))
	if !clipboard.SetText("hisar secret") {
		t.Fatal("copy failed")
	}
	if !clipboard.SetText("") {
		t.Fatal("clear failed")
	}
	for _, pasteboardType := range []string{stringType, concealedType, transientType} {
		if clipboard.hasType(pasteboardType) {
			t.Errorf("the cleared pasteboard still has %s", pasteboardType)
		}
	}
	if text, _ := clipboard.GetText(); text != "" {
		t.Errorf("read %q after clearing", text)
	}
}
