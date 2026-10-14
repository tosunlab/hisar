package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/approval"
	"github.com/tryhisar/hisar/core/crypt"
)

const (
	testMacDeviceID   = "mac-device"
	testPhoneDeviceID = "phone-device"
)

type fakeApproval struct {
	request      apitypes.CreateApprovalRequest
	sealedAnswer []byte
	isResolved   bool
}

func (server *fakeAPIServer) addApprovalRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/approvals", func(writer http.ResponseWriter, request *http.Request) {
		var createRequest apitypes.CreateApprovalRequest
		if err := json.NewDecoder(request.Body).Decode(&createRequest); err != nil {
			writeJSON(writer, http.StatusBadRequest, apitypes.Error{Error: err.Error()})
			return
		}
		server.mutex.Lock()
		defer server.mutex.Unlock()
		if server.failingApprovalPostCount > 0 {
			server.failingApprovalPostCount--
			writeJSON(writer, http.StatusServiceUnavailable, apitypes.Error{Error: "unavailable"})
			return
		}
		if server.approvalsByRequestID == nil {
			server.approvalsByRequestID = map[string]*fakeApproval{}
		}
		server.approvalsByRequestID[createRequest.RequestID] = &fakeApproval{request: createRequest}
		writer.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("GET /api/v1/approvals", func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		pendingRequests := []apitypes.PendingApproval{}
		for _, stored := range server.approvalsByRequestID {
			if stored.sealedAnswer == nil && !stored.isResolved {
				pendingRequests = append(pendingRequests, apitypes.PendingApproval{RequestID: stored.request.RequestID, SealedRequest: stored.request.SealedRequest, ExpiresAt: stored.request.ExpiresAt})
			}
		}
		writeJSON(writer, http.StatusOK, pendingRequests)
	})
	mux.HandleFunc("GET /api/v1/approvals/{requestID}", func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		stored, isFound := server.approvalsByRequestID[request.PathValue("requestID")]
		if !isFound {
			writeJSON(writer, http.StatusNotFound, apitypes.Error{Error: "not found"})
			return
		}
		writeJSON(writer, http.StatusOK, apitypes.Approval{RequestID: stored.request.RequestID, SealedRequest: stored.request.SealedRequest, SealedAnswer: stored.sealedAnswer, IsResolved: stored.isResolved, ExpiresAt: stored.request.ExpiresAt})
	})
	mux.HandleFunc("POST /api/v1/approvals/{requestID}/answer", func(writer http.ResponseWriter, request *http.Request) {
		var answerRequest apitypes.AnswerApprovalRequest
		if err := json.NewDecoder(request.Body).Decode(&answerRequest); err != nil {
			writeJSON(writer, http.StatusBadRequest, apitypes.Error{Error: err.Error()})
			return
		}
		server.mutex.Lock()
		defer server.mutex.Unlock()
		stored, isFound := server.approvalsByRequestID[request.PathValue("requestID")]
		switch {
		case !isFound:
			writeJSON(writer, http.StatusNotFound, apitypes.Error{Error: "not found"})
		case stored.sealedAnswer != nil:
			writeJSON(writer, http.StatusConflict, apitypes.Error{Error: "already answered"})
		default:
			stored.sealedAnswer = answerRequest.SealedAnswer
			writer.WriteHeader(http.StatusNoContent)
		}
	})
	mux.HandleFunc("DELETE /api/v1/approvals/{requestID}", func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		if stored, isFound := server.approvalsByRequestID[request.PathValue("requestID")]; isFound {
			stored.isResolved = true
		}
		writer.WriteHeader(http.StatusNoContent)
	})
}

func (server *fakeAPIServer) findApproval(requestID string) (fakeApproval, bool) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	stored, isFound := server.approvalsByRequestID[requestID]
	if !isFound {
		return fakeApproval{}, false
	}
	return *stored, true
}

func (server *fakeAPIServer) waitForApproval(t *testing.T) fakeApproval {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		server.mutex.Lock()
		for _, stored := range server.approvalsByRequestID {
			if !stored.isResolved && stored.sealedAnswer == nil {
				found := *stored
				server.mutex.Unlock()
				return found
			}
		}
		server.mutex.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the Mac never posted the request")
	return fakeApproval{}
}

func (server *fakeAPIServer) setAnswer(requestID string, sealedAnswer []byte) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	server.approvalsByRequestID[requestID].sealedAnswer = sealedAnswer
}

func sendApprovalEvent(t *testing.T, apiServer *fakeAPIServer, eventName, requestID string) {
	t.Helper()
	data, _ := json.Marshal(apitypes.ApprovalEvent{RequestID: requestID})
	select {
	case apiServer.approvalEventChannel <- apiclient.Event{Name: eventName, Data: data}:
	case <-time.After(5 * time.Second):
		t.Fatal("the service never read the event stream")
	}
}

type phoneApprovalTest struct {
	unlockedTestService
	phoneApprover   *PhoneApprover
	keyset          *crypt.Keyset
	decisionChannel chan bool
}

// createPhoneApprovalTest unlocks a Mac whose account also has an iPhone, with the event stream open.
func createPhoneApprovalTest(t *testing.T) *phoneApprovalTest {
	t.Helper()
	test := createUnlockedTestService(t)
	test.apiServer.mutex.Lock()
	test.apiServer.devices = []apitypes.Device{
		{ID: testMacDeviceID, Name: "MacBook Pro", Platform: "darwin/desktop", Current: true},
		{ID: testPhoneDeviceID, Name: "Hisar on iPhone", Platform: "ios/mobile"},
	}
	test.apiServer.eventChannel = make(chan string)
	test.apiServer.approvalEventChannel = make(chan apiclient.Event)
	test.apiServer.mutex.Unlock()
	phoneApprover := NewPhoneApprover(test.service)
	test.service.mutex.Lock()
	test.service.config.DeviceName = "MacBook-Pro.local"
	keyset := test.service.unlockedSession.Keyset
	test.service.phoneApprovals.deviceRefreshedAt = time.Time{}
	test.service.mutex.Unlock()
	deadline := time.Now().Add(5 * time.Second)
	for !phoneApprover.IsPhoneApprovalAvailable() {
		if time.Now().After(deadline) {
			t.Fatal("phone approvals never became available")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return &phoneApprovalTest{unlockedTestService: test, phoneApprover: phoneApprover, keyset: keyset, decisionChannel: make(chan bool, 4)}
}

func (test *phoneApprovalTest) start(t *testing.T) (fakeApproval, func()) {
	t.Helper()
	return test.startUntil(t, time.Now().Add(2*time.Minute))
}

func (test *phoneApprovalTest) startUntil(t *testing.T, dialogTimeoutAt time.Time) (fakeApproval, func()) {
	t.Helper()
	description := approval.Description{Kind: approval.KindSSH, AppName: "Terminal", Purpose: "push to git.tryhisar.com", Host: "git.tryhisar.com:22", KeyName: "hisar gitlab"}
	finish := test.phoneApprover.StartPhoneApproval(description, dialogTimeoutAt, func(answer approval.AcceptedAnswer) {
		test.decisionChannel <- answer.Decision == approval.DecisionApprove
	})
	return test.apiServer.waitForApproval(t), finish
}

// answerAsPhone opens the sealed request as the phone would and answers it.
func (test *phoneApprovalTest) answerAsPhone(t *testing.T, stored fakeApproval, decision, deviceID string) []byte {
	t.Helper()
	request, err := approval.OpenRequest(test.keyset, test.accountID, stored.request.RequestID, stored.request.SealedRequest, testPhoneDeviceID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if request.RequesterDeviceID != testMacDeviceID || request.RequesterDeviceName != "MacBook-Pro" || request.KeyName != "hisar gitlab" {
		t.Errorf("request = %+v", request)
	}
	sealedAnswer, err := approval.CreateAnswer(test.keyset, test.accountID, stored.request.SealedRequest, request, decision, deviceID, "Hisar on iPhone", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return sealedAnswer
}

// signAnswerFromDevice seals an answer from deviceID to the account, as anyone with its public KEM key could; CreateAnswer would refuse some of these.
func (test *phoneApprovalTest) signAnswerFromDevice(t *testing.T, stored fakeApproval, deviceID string, signingKeyset *crypt.Keyset) []byte {
	t.Helper()
	fields := crypt.ApprovalAnswerFields{
		AccountID:           test.accountID,
		RequestID:           stored.request.RequestID,
		RequestHash:         crypt.HashApprovalRequest(stored.request.SealedRequest),
		Decision:            approval.DecisionApprove,
		AnsweringDeviceID:   deviceID,
		AnsweringDeviceName: "MacBook Pro",
		AnsweredAt:          approval.FormatTime(time.Now()),
	}
	plaintext, _ := json.Marshal(approval.Answer{
		RequestID: fields.RequestID, Decision: fields.Decision, AnsweringDeviceID: fields.AnsweringDeviceID, AnsweringDeviceName: fields.AnsweringDeviceName,
		AnsweredAt: fields.AnsweredAt, RequestHash: fields.RequestHash, Signature: signingKeyset.SignApprovalAnswer(fields),
	})
	sealedAnswer, err := crypt.SealApprovalAnswer(test.keyset.GetKEMPublicKey(), test.accountID, stored.request.RequestID, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	return sealedAnswer
}

func (test *phoneApprovalTest) expectDecision(t *testing.T, want bool) {
	t.Helper()
	select {
	case isApproved := <-test.decisionChannel:
		if isApproved != want {
			t.Errorf("decision = %v, want %v", isApproved, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the dialog never got the phone's answer")
	}
}

func (test *phoneApprovalTest) expectNoDecision(t *testing.T) {
	t.Helper()
	select {
	case isApproved := <-test.decisionChannel:
		t.Errorf("unexpected decision %v", isApproved)
	case <-time.After(300 * time.Millisecond):
	}
}

func waitForResolved(t *testing.T, apiServer *fakeAPIServer, requestID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if stored, _ := apiServer.findApproval(requestID); stored.isResolved {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("request %s was not withdrawn", requestID)
}

func TestPhoneApprovalApprovedAndDenied(t *testing.T) {
	test := createPhoneApprovalTest(t)
	for _, decision := range []string{approval.DecisionApprove, approval.DecisionDeny} {
		stored, finish := test.start(t)
		test.apiServer.setAnswer(stored.request.RequestID, test.answerAsPhone(t, stored, decision, testPhoneDeviceID))
		sendApprovalEvent(t, test.apiServer, apitypes.EventApprovalAnswered, stored.request.RequestID)
		test.expectDecision(t, decision == approval.DecisionApprove)
		sendApprovalEvent(t, test.apiServer, apitypes.EventApprovalAnswered, stored.request.RequestID)
		test.expectNoDecision(t)
		finish()
		finish()
		waitForResolved(t, test.apiServer, stored.request.RequestID)
	}
}

func TestPhoneApprovalCarriesTheRememberChoice(t *testing.T) {
	test := createPhoneApprovalTest(t)
	description := approval.Description{Kind: approval.KindSSH, AppName: "Terminal", Purpose: "push to git.tryhisar.com", KeyName: "hisar gitlab", RememberLabel: "Also allows Terminal for 1 hour", IsRememberedByDefault: true}
	cases := []struct {
		name             string
		answer           func(stored fakeApproval, request approval.Request) []byte
		wantIsRemembered bool
	}{
		{"remembered on the phone", func(stored fakeApproval, request approval.Request) []byte {
			sealedAnswer, _ := approval.CreateRememberingAnswer(test.keyset, test.accountID, stored.request.SealedRequest, request, approval.DecisionApprove, true, testPhoneDeviceID, "Hisar on iPhone", time.Now())
			return sealedAnswer
		}, true},
		{"once on the phone", func(stored fakeApproval, request approval.Request) []byte {
			sealedAnswer, _ := approval.CreateRememberingAnswer(test.keyset, test.accountID, stored.request.SealedRequest, request, approval.DecisionApprove, false, testPhoneDeviceID, "Hisar on iPhone", time.Now())
			return sealedAnswer
		}, false},
		{"an older phone without the switch", func(stored fakeApproval, request approval.Request) []byte {
			request.RememberLabel = ""
			sealedAnswer, _ := approval.CreateRememberingAnswer(test.keyset, test.accountID, stored.request.SealedRequest, request, approval.DecisionApprove, true, testPhoneDeviceID, "Hisar on iPhone", time.Now())
			return sealedAnswer
		}, false},
	}
	for _, testCase := range cases {
		answerChannel := make(chan approval.AcceptedAnswer, 1)
		finish := test.phoneApprover.StartPhoneApproval(description, time.Now().Add(2*time.Minute), func(answer approval.AcceptedAnswer) { answerChannel <- answer })
		stored := test.apiServer.waitForApproval(t)
		request, err := approval.OpenRequest(test.keyset, test.accountID, stored.request.RequestID, stored.request.SealedRequest, testPhoneDeviceID, time.Now())
		if err != nil || request.RememberLabel != description.RememberLabel || !request.IsRememberedByDefault {
			t.Fatalf("%s: the phone sees %+v, %v", testCase.name, request.Description, err)
		}
		test.apiServer.setAnswer(stored.request.RequestID, testCase.answer(stored, request))
		sendApprovalEvent(t, test.apiServer, apitypes.EventApprovalAnswered, stored.request.RequestID)
		select {
		case answer := <-answerChannel:
			if answer.Decision != approval.DecisionApprove || answer.IsRemembered != testCase.wantIsRemembered {
				t.Errorf("%s: answer = %+v", testCase.name, answer)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: the dialog never got the phone's answer", testCase.name)
		}
		finish()
		waitForResolved(t, test.apiServer, stored.request.RequestID)
	}
}

func TestPhoneApprovalRefusesForgedAndSelfAnswers(t *testing.T) {
	test := createPhoneApprovalTest(t)
	stored, finish := test.start(t)
	defer finish()
	requestID := stored.request.RequestID

	otherKeyset, _ := crypt.GenerateKeyset()
	forgedRequest, _ := approval.OpenRequest(test.keyset, test.accountID, requestID, stored.request.SealedRequest, testPhoneDeviceID, time.Now())
	signedByOther, _ := approval.CreateAnswer(otherKeyset, test.accountID, stored.request.SealedRequest, forgedRequest, approval.DecisionApprove, testPhoneDeviceID, "Hisar on iPhone", time.Now())
	forged := map[string][]byte{
		"signed and sealed by another account":                           signedByOther,
		"from the Mac itself":                                            test.signAnswerFromDevice(t, stored, testMacDeviceID, test.keyset),
		"sealed to the account by the server, signed by another account": test.signAnswerFromDevice(t, stored, testPhoneDeviceID, otherKeyset),
		"garbage": []byte("approve"),
	}
	for name, sealedAnswer := range forged {
		test.apiServer.setAnswer(requestID, sealedAnswer)
		sendApprovalEvent(t, test.apiServer, apitypes.EventApprovalAnswered, requestID)
		select {
		case isApproved := <-test.decisionChannel:
			t.Errorf("%s: decided %v", name, isApproved)
		case <-time.After(300 * time.Millisecond):
		}
	}
	test.apiServer.setAnswer(requestID, test.answerAsPhone(t, stored, approval.DecisionApprove, testPhoneDeviceID))
	sendApprovalEvent(t, test.apiServer, apitypes.EventApprovalAnswered, requestID)
	test.expectDecision(t, true)
}

func TestPhoneApprovalEndsWhenHisarLocks(t *testing.T) {
	test := createPhoneApprovalTest(t)
	stored, finish := test.start(t)
	defer finish()
	sealedAnswer := test.answerAsPhone(t, stored, approval.DecisionApprove, testPhoneDeviceID)
	test.service.mutex.Lock()
	apiClient := test.service.unlockedSession.Client
	test.service.mutex.Unlock()
	test.service.Lock()
	waitForResolved(t, test.apiServer, stored.request.RequestID)
	test.apiServer.setAnswer(stored.request.RequestID, sealedAnswer)
	test.service.acceptPhoneApprovalAnswer(apiClient, stored.request.RequestID)
	test.expectNoDecision(t)
	if test.phoneApprover.IsPhoneApprovalAvailable() {
		t.Error("phone approvals are available while locked")
	}
}

func TestPhoneApprovalAvailability(t *testing.T) {
	test := createPhoneApprovalTest(t)
	preference := NewApprovalPreference(test.service)
	if err := preference.SetPhoneApproval(false); err != nil {
		t.Fatal(err)
	}
	if test.phoneApprover.IsPhoneApprovalAvailable() {
		t.Error("available with the switch off")
	}
	if finish := test.phoneApprover.StartPhoneApproval(approval.Description{Kind: approval.KindSSH}, time.Now().Add(time.Minute), func(approval.AcceptedAnswer) { t.Error("decided with the switch off") }); finish == nil {
		t.Error("no finish function")
	} else {
		finish()
	}
	preference.SetPhoneApproval(true)

	test.apiServer.mutex.Lock()
	test.apiServer.devices = test.apiServer.devices[:1]
	test.apiServer.mutex.Unlock()
	if _, err := test.service.refreshPhoneApprovalDevice(test.service.unlockedSession.Client); err != nil {
		t.Fatal(err)
	}
	if test.phoneApprover.IsPhoneApprovalAvailable() {
		t.Error("available without an iPhone on the account")
	}
	test.apiServer.mutex.Lock()
	if len(test.apiServer.approvalsByRequestID) != 0 {
		t.Errorf("requests were posted: %d", len(test.apiServer.approvalsByRequestID))
	}
	test.apiServer.mutex.Unlock()
}

func TestPhoneListsAndAnswersRequests(t *testing.T) {
	test := createUnlockedTestService(t)
	test.apiServer.mutex.Lock()
	test.apiServer.devices = []apitypes.Device{
		{ID: testMacDeviceID, Name: "MacBook Pro", Platform: "darwin/desktop"},
		{ID: testPhoneDeviceID, Name: "Hisar on iPhone", Platform: "ios/mobile", Current: true},
	}
	test.apiServer.mutex.Unlock()
	test.service.mutex.Lock()
	keyset := test.service.unlockedSession.Keyset
	test.service.config.DeviceName = "Hisar on iPhone"
	test.service.mutex.Unlock()

	postRequest := func(requesterDeviceID string, createdAt time.Time) (approval.Request, []byte) {
		request := approval.NewRequest(requesterDeviceID, "MacBook Pro", approval.Description{Kind: approval.KindCommandLine, AppName: "hisar in Terminal", Purpose: "use Hisar", TerminalName: "ttys003"}, createdAt, approval.MaximumLifetime)
		sealedRequest, err := approval.SealRequest(keyset, test.accountID, request)
		if err != nil {
			t.Fatal(err)
		}
		test.apiServer.mutex.Lock()
		if test.apiServer.approvalsByRequestID == nil {
			test.apiServer.approvalsByRequestID = map[string]*fakeApproval{}
		}
		test.apiServer.approvalsByRequestID[request.RequestID] = &fakeApproval{request: apitypes.CreateApprovalRequest{RequestID: request.RequestID, SealedRequest: sealedRequest, ExpiresAt: request.ExpiresAt}}
		test.apiServer.mutex.Unlock()
		return request, sealedRequest
	}
	open, sealedRequest := postRequest(testMacDeviceID, time.Now())
	postRequest(testMacDeviceID, time.Now().Add(-approval.MaximumLifetime-time.Second))
	postRequest(testPhoneDeviceID, time.Now())
	test.apiServer.mutex.Lock()
	test.apiServer.approvalsByRequestID[approval.GenerateRequestID()] = &fakeApproval{request: apitypes.CreateApprovalRequest{RequestID: "not-a-request", SealedRequest: []byte("garbage")}}
	test.apiServer.mutex.Unlock()

	requests, err := ListPendingPhoneApprovals(test.service)
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 1 || requests[0].RequestID != open.RequestID || requests[0].RequesterDeviceName != "MacBook Pro" || requests[0].TerminalName != "ttys003" || requests[0].Kind != approval.KindCommandLine {
		t.Fatalf("listed = %+v", requests)
	}
	if err := AnswerPhoneApproval(test.service, open.RequestID, true, false); err != nil {
		t.Fatal(err)
	}
	stored, _ := test.apiServer.findApproval(open.RequestID)
	tracker := approval.NewTracker()
	tracker.Add(open.RequestID, sealedRequest, open.ExpiresAt)
	test.service.mutex.Lock()
	decision, err := tracker.Accept(keyset, test.accountID, open.RequestID, stored.sealedAnswer, testMacDeviceID, time.Now())
	test.service.mutex.Unlock()
	if err != nil || decision != approval.DecisionApprove {
		t.Errorf("the Mac checks the phone's answer: %q %v", decision, err)
	}
	if err := AnswerPhoneApproval(test.service, open.RequestID, false, false); !errors.Is(err, errPhoneApprovalGone) {
		t.Errorf("answering twice: %v", err)
	}
	if err := AnswerPhoneApproval(test.service, approval.GenerateRequestID(), true, false); !errors.Is(err, errPhoneApprovalGone) {
		t.Errorf("answering an unknown request: %v", err)
	}
	if requests, _ := ListPendingPhoneApprovals(test.service); len(requests) != 0 {
		t.Errorf("an answered request is still listed: %+v", requests)
	}

	test.service.Lock()
	if _, err := ListPendingPhoneApprovals(test.service); !errors.Is(err, ErrLocked) {
		t.Errorf("listing while locked: %v", err)
	}
	if err := AnswerPhoneApproval(test.service, open.RequestID, true, false); !errors.Is(err, ErrLocked) {
		t.Errorf("answering while locked: %v", err)
	}
}

func TestPhoneApprovalLastsAsLongAsTheDialog(t *testing.T) {
	test := createPhoneApprovalTest(t)
	cases := []struct {
		dialogTimeout time.Duration
		want          time.Duration
	}{
		{2 * time.Minute, 2 * time.Minute},
		{30 * time.Second, 30 * time.Second},
		{10 * time.Minute, approval.MaximumLifetime},
	}
	for _, testCase := range cases {
		stored, finish := test.startUntil(t, time.Now().Add(testCase.dialogTimeout))
		request, err := approval.OpenRequest(test.keyset, test.accountID, stored.request.RequestID, stored.request.SealedRequest, testPhoneDeviceID, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if lifetime := request.ExpiresAt.Sub(request.CreatedAt); lifetime < testCase.want-time.Second || lifetime > testCase.want {
			t.Errorf("dialog waits %v: the request lasts %v", testCase.dialogTimeout, lifetime)
		}
		if !stored.request.ExpiresAt.Equal(request.ExpiresAt) {
			t.Errorf("the server was told %v, the sealed request says %v", stored.request.ExpiresAt, request.ExpiresAt)
		}
		finish()
		waitForResolved(t, test.apiServer, stored.request.RequestID)
	}
}

func (test *phoneApprovalTest) forgetDeviceList() {
	test.service.mutex.Lock()
	defer test.service.mutex.Unlock()
	relay := test.service.phoneApprovals
	relay.mutex.Lock()
	defer relay.mutex.Unlock()
	relay.ownDeviceID, relay.hasIOSDevice, relay.deviceRefreshedAt = "", false, time.Time{}
}

func TestPhoneApprovalChecksAStaleDeviceListBeforeAnswering(t *testing.T) {
	test := createPhoneApprovalTest(t)
	test.forgetDeviceList()
	if !test.phoneApprover.IsPhoneApprovalAvailable() {
		t.Fatal("the first request after the device list was lost skipped the phone")
	}
	stored, finish := test.start(t)
	defer finish()
	test.apiServer.setAnswer(stored.request.RequestID, test.answerAsPhone(t, stored, approval.DecisionApprove, testPhoneDeviceID))
	sendApprovalEvent(t, test.apiServer, apitypes.EventApprovalAnswered, stored.request.RequestID)
	test.expectDecision(t, true)
}

func TestPhoneApprovalKeepsTheLastDeviceListWhenTheRefreshFails(t *testing.T) {
	test := createPhoneApprovalTest(t)
	test.service.mutex.Lock()
	test.service.phoneApprovals.deviceRefreshedAt = time.Time{}
	test.service.mutex.Unlock()
	test.apiServer.setUnreachable(true)
	isAvailable := test.phoneApprover.IsPhoneApprovalAvailable()
	test.apiServer.setUnreachable(false)
	if !isAvailable {
		t.Error("a failed refresh hid the phone")
	}
}

func TestPhoneApprovalDoesNotDependOnTheLastSyncBeingOnline(t *testing.T) {
	test := createPhoneApprovalTest(t)
	test.service.mutex.Lock()
	test.service.config.CachePath = filepath.Join(t.TempDir(), "desktop-cache.db")
	test.service.unlockedSession.Cache = test.service.getCache()
	test.service.mutex.Unlock()
	if _, err := test.service.ListVaults(); err != nil {
		t.Fatal(err)
	}
	test.apiServer.setUnreachable(true)
	test.service.ListVaults()
	test.apiServer.setUnreachable(false)
	if !test.service.GetStatus().IsOffline {
		t.Fatal("the test did not leave the session offline")
	}
	if !test.phoneApprover.IsPhoneApprovalAvailable() {
		t.Error("an offline flag from an earlier sync hid the phone")
	}
}

func TestPhoneApprovalPostIsRetried(t *testing.T) {
	test := createPhoneApprovalTest(t)
	test.apiServer.mutex.Lock()
	test.apiServer.failingApprovalPostCount = 1
	test.apiServer.mutex.Unlock()
	stored, finish := test.start(t)
	defer finish()
	test.apiServer.setAnswer(stored.request.RequestID, test.answerAsPhone(t, stored, approval.DecisionDeny, testPhoneDeviceID))
	sendApprovalEvent(t, test.apiServer, apitypes.EventApprovalAnswered, stored.request.RequestID)
	test.expectDecision(t, false)
}

func TestPhoneApprovalOutlivesAHangingDeviceRefresh(t *testing.T) {
	test := createPhoneApprovalTest(t)
	stored, finish := test.start(t)
	test.apiServer.setAnswer(stored.request.RequestID, test.answerAsPhone(t, stored, approval.DecisionApprove, testPhoneDeviceID))
	sendApprovalEvent(t, test.apiServer, apitypes.EventApprovalAnswered, stored.request.RequestID)
	test.expectDecision(t, true)
	finish()

	hangingChannel := make(chan struct{})
	test.apiServer.mutex.Lock()
	test.apiServer.deviceListHangingChannel = hangingChannel
	test.apiServer.mutex.Unlock()
	defer close(hangingChannel)
	test.service.mutex.Lock()
	test.service.config.Now = func() time.Time { return time.Now().Add(time.Hour) }
	test.service.mutex.Unlock()
	for range 3 {
		startedAt := time.Now()
		if !test.phoneApprover.IsPhoneApprovalAvailable() {
			t.Fatal("a hanging device refresh hid the phone")
		}
		if elapsed := time.Since(startedAt); elapsed > time.Second {
			t.Errorf("the availability check waited %v for the hanging refresh", elapsed)
		}
	}
	description := approval.Description{Kind: approval.KindSSH, AppName: "Terminal", Purpose: "sign a commit"}
	later := test.phoneApprover.StartPhoneApproval(description, time.Now().Add(time.Hour+time.Minute), func(answer approval.AcceptedAnswer) {
		test.decisionChannel <- answer.Decision == approval.DecisionApprove
	})
	defer later()
	laterStored := test.apiServer.waitForApproval(t)
	if laterStored.request.RequestID == stored.request.RequestID {
		t.Fatal("the later request never reached the server")
	}
}
