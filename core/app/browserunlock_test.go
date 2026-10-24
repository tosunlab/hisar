package app

import (
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/browserprotocol"
)

func TestBrowserUnlockWithPassword(t *testing.T) {
	testBridge := createBrowserTestBridge(t)
	service := testBridge.testService.service
	prepareTestApprovalUnlock(t, testBridge.testService, "master password")
	token := testBridge.pair(t, "chrome-1")
	otherToken := testBridge.pair(t, "chrome-2")
	unlock := func(token, password string) (any, string) {
		return testBridge.sendRequest(browserprotocol.Request{Type: "unlock.password", Token: token, Password: password})
	}

	// Already unlocked: nothing to do.
	if result, code := unlock(token, "anything"); code != "" || result.(browserprotocol.StatusResult).State != StateUnlocked {
		t.Fatalf("unlock.password while unlocked = %+v %s", result, code)
	}
	service.Lock()
	if _, code := unlock(token, ""); code != browserprotocol.ErrorInvalidRequest {
		t.Errorf("unlock.password without a password = %s", code)
	}
	for attempt := range browserUnlockMaxFailures {
		if _, code := unlock(token, "wrong password"); code != browserprotocol.ErrorWrongPassword {
			t.Fatalf("wrong password %d = %s", attempt, code)
		}
	}
	// The browser now has to wait, even with the right password; another paired browser does not.
	result, code := unlock(token, "master password")
	if code != browserprotocol.ErrorWrongPassword || service.GetStatus().State != StateLocked {
		t.Fatalf("unlock.password after too many wrong ones = %+v %s, state %s", result, code, service.GetStatus().State)
	}
	if result, code := unlock(otherToken, "master password"); code != "" || result.(browserprotocol.StatusResult).State != StateUnlocked {
		t.Fatalf("unlock.password from another browser = %+v %s", result, code)
	}

	service.Lock()
	testBridge.currentTime = testBridge.currentTime.Add(browserUnlockFailureWindow)
	if result, code := unlock(token, "master password"); code != "" || result.(browserprotocol.StatusResult).State != StateUnlocked {
		t.Fatalf("unlock.password once the wait is over = %+v %s", result, code)
	}
	if _, code := testBridge.sendRequest(browserprotocol.Request{Type: "unlock.password", Password: "master password"}); code != browserprotocol.ErrorNotPaired {
		t.Errorf("unlock.password without a token = %s", code)
	}
}

func TestBrowserUnlockNotifiesMainWindow(t *testing.T) {
	testBridge := createBrowserTestBridge(t)
	service := testBridge.testService.service
	prepareTestApprovalUnlock(t, testBridge.testService, "master password")
	token := testBridge.pair(t, "chrome-1")
	notified := 0
	testBridge.bridge.config.OnUnlock = func() {
		notified++
		if service.GetStatus().State != StateUnlocked {
			t.Error("the main window was notified before Hisar unlocked")
		}
	}
	service.Lock()
	unlock := func(password string) {
		testBridge.sendRequest(browserprotocol.Request{Type: "unlock.password", Token: token, Password: password})
	}
	unlock("wrong password")
	if notified != 0 {
		t.Fatal("a wrong password notified the main window")
	}
	unlock("master password")
	if notified != 1 {
		t.Fatalf("successful unlock notified the main window %d times", notified)
	}
	unlock("anything")
	if notified != 1 {
		t.Fatalf("an already-unlocked request notified the main window again, %d calls", notified)
	}
}

func TestBrowserUnlockWaitMessage(t *testing.T) {
	testBridge := createBrowserTestBridge(t)
	prepareTestApprovalUnlock(t, testBridge.testService, "master password")
	token := testBridge.pair(t, "chrome-1")
	testBridge.testService.service.Lock()
	for range browserUnlockMaxFailures {
		testBridge.sendRequest(browserprotocol.Request{Type: "unlock.password", Token: token, Password: "wrong"})
		testBridge.currentTime = testBridge.currentTime.Add(10 * time.Second)
	}
	_, err := NewBrowserRequestHandler(testBridge.bridge).HandleRequest(t.Context(), &browserprotocol.Request{Type: "unlock.password", Token: token, Password: "wrong"})
	if err == nil || !strings.Contains(err.Error(), "Try again in 5 minutes") {
		t.Errorf("wait message = %v", err)
	}
	result, _ := testBridge.sendRequest(browserprotocol.Request{Type: "hello", InstanceID: "chrome-1"})
	if !slices.Contains(result.(browserprotocol.HelloResult).Features, browserprotocol.FeatureUnlockPassword) {
		t.Errorf("hello = %+v", result)
	}
}

// Attempts sent at once over several native connections all count: none of them sees a stale list of failures.
func TestBrowserUnlockCountsConcurrentAttempts(t *testing.T) {
	testBridge := createBrowserTestBridge(t)
	service := testBridge.testService.service
	prepareTestApprovalUnlock(t, testBridge.testService, "master password")
	token := testBridge.pair(t, "chrome-1")
	service.Lock()
	var group sync.WaitGroup
	for range 2 * browserUnlockMaxFailures {
		group.Go(func() {
			if _, code := testBridge.sendRequest(browserprotocol.Request{Type: "unlock.password", Token: token, Password: "wrong password"}); code != browserprotocol.ErrorWrongPassword {
				t.Errorf("concurrent wrong password = %s", code)
			}
		})
	}
	group.Wait()
	if result, code := testBridge.sendRequest(browserprotocol.Request{Type: "unlock.password", Token: token, Password: "master password"}); code != browserprotocol.ErrorWrongPassword || service.GetStatus().State != StateLocked {
		t.Fatalf("unlock.password after concurrent wrong ones = %+v %s, state %s", result, code, service.GetStatus().State)
	}
}
