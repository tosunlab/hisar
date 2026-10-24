package app

import (
	"testing"
	"time"
)

func TestClearCopiedTextClearsOnlyHisarsPendingCopy(t *testing.T) {
	testService := createUnlockedTestService(t)
	service, clipboard := testService.service, testService.clipboard
	if err := service.CopyText("copied-secret"); err != nil {
		t.Fatal(err)
	}
	ClearCopiedText(service)
	if text, _ := clipboard.GetText(); text != "" {
		t.Errorf("quitting left %q on the clipboard", text)
	}
	if err := service.CopyText("copied-secret"); err != nil {
		t.Fatal(err)
	}
	clipboard.SetText("another app's text")
	ClearCopiedText(service)
	if text, _ := clipboard.GetText(); text != "another app's text" {
		t.Errorf("quitting cleared another app's copy: %q", text)
	}
}

func TestLockClearsOnlyHisarsPendingCopy(t *testing.T) {
	lockAfterCopy := func(otherAppText string) *fakeClipboard {
		testService := createUnlockedTestService(t)
		testService.service.config.ClipboardClearAfter = time.Hour
		if err := testService.service.CopyText("copied-secret"); err != nil {
			t.Fatal(err)
		}
		if otherAppText != "" {
			testService.clipboard.SetText(otherAppText)
		}
		testService.service.Lock()
		return testService.clipboard
	}

	clipboard := lockAfterCopy("")
	for deadline := time.Now().Add(2 * time.Second); ; time.Sleep(time.Millisecond) {
		text, _ := clipboard.GetText()
		if text == "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("locking left %q on the clipboard", text)
		}
	}

	clipboard = lockAfterCopy("another app's text")
	time.Sleep(50 * time.Millisecond)
	if text, _ := clipboard.GetText(); text != "another app's text" {
		t.Errorf("locking cleared another app's copy: %q", text)
	}
}
