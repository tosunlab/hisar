package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/approval"
	"github.com/tryhisar/hisar/core/session"
)

type testClock struct {
	mutex  sync.Mutex
	offset time.Duration
}

func (clock *testClock) now() time.Time {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()
	return time.Now().Add(clock.offset)
}

func (clock *testClock) advance(duration time.Duration) {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()
	clock.offset += duration
}

type approvalAccounts struct {
	mac   *session.Session
	phone *session.Session
	other *session.Session
}

// setupApprovalAccounts signs up one account on a "Mac" and an "iPhone", and a second account.
func setupApprovalAccounts(t *testing.T) (*Server, approvalAccounts) {
	t.Helper()
	baseURL, server, _ := setupServer(t)
	server.config.EventHeartbeatInterval = 100 * time.Millisecond
	ctx := context.Background()
	invite := requireValue[*apitypes.InviteResponse](t)(server.CreateInvite(ctx, "dev@example.com", apitypes.RoleAdmin, nil))
	password := []byte("dev password")
	mac, secretKey, err := session.SignUp(ctx, apiclient.New(baseURL), invite.Token, "dev@example.com", "Dev", password, apitypes.DeviceInfo{Name: "MacBook Pro", Platform: "darwin/desktop"})
	if err != nil {
		t.Fatal(err)
	}
	phone, err := session.LogIn(ctx, apiclient.New(baseURL), "dev@example.com", password, secretKey, apitypes.DeviceInfo{Name: "Hisar on iPhone", Platform: "ios/mobile"})
	if err != nil {
		t.Fatal(err)
	}
	other := signUpTestAccount(t, ctx, server, baseURL, "other@example.com", apitypes.RoleMember)
	return server, approvalAccounts{mac: mac, phone: phone, other: other}
}

func findOwnDeviceID(t *testing.T, signedIn *session.Session) string {
	t.Helper()
	for _, device := range requireValue[[]apitypes.Device](t)(signedIn.Client.ListDevices(context.Background())) {
		if device.Current {
			return device.ID
		}
	}
	t.Fatal("no current device")
	return ""
}

func expectStatus(t *testing.T, description string, err error, status int) {
	t.Helper()
	if !apiclient.IsStatus(err, status) {
		t.Errorf("%s: got %v, want %d", description, err, status)
	}
}

func decodeApprovalEvent(t *testing.T, event apiclient.Event) string {
	t.Helper()
	var approvalEvent apitypes.ApprovalEvent
	if err := json.Unmarshal(event.Data, &approvalEvent); err != nil {
		t.Fatal(err)
	}
	return approvalEvent.RequestID
}

func TestApprovalRelayFlow(t *testing.T) {
	server, accounts := setupApprovalAccounts(t)
	ctx := context.Background()
	macDeviceID, phoneDeviceID := findOwnDeviceID(t, accounts.mac), findOwnDeviceID(t, accounts.phone)
	macStream := openEventStream(t, accounts.mac.Client)
	phoneStream := openEventStream(t, accounts.phone.Client)
	otherStream := openEventStream(t, accounts.other.Client)
	waitForSubscriber(t, server, accounts.mac.AccountID, 2)
	waitForSubscriber(t, server, accounts.other.AccountID, 1)

	request := approval.NewRequest(macDeviceID, "MacBook Pro", approval.Description{Kind: approval.KindSSH, AppName: "Terminal", Purpose: "push to git.tryhisar.com"}, time.Now(), approval.MaximumLifetime)
	sealedRequest := requireValue[[]byte](t)(approval.SealRequest(accounts.mac.Keyset, accounts.mac.AccountID, request))
	if err := accounts.mac.Client.CreateApproval(ctx, &apitypes.CreateApprovalRequest{RequestID: request.RequestID, SealedRequest: sealedRequest, ExpiresAt: request.ExpiresAt}); err != nil {
		t.Fatal(err)
	}
	expectStatus(t, "the same ID again", accounts.mac.Client.CreateApproval(ctx, &apitypes.CreateApprovalRequest{RequestID: request.RequestID, SealedRequest: sealedRequest, ExpiresAt: request.ExpiresAt}), http.StatusConflict)
	if requestID := decodeApprovalEvent(t, phoneStream.waitForEvent(t, apitypes.EventApprovalRequested)); requestID != request.RequestID {
		t.Errorf("approval-requested for %q", requestID)
	}
	otherStream.expectNoEvent(t)

	pendingRequests := requireValue[[]apitypes.PendingApproval](t)(accounts.phone.Client.ListPendingApprovals(ctx))
	if len(pendingRequests) != 1 || pendingRequests[0].RequestID != request.RequestID || pendingRequests[0].RequesterDeviceID != macDeviceID || string(pendingRequests[0].SealedRequest) != string(sealedRequest) {
		t.Fatalf("pending = %+v", pendingRequests)
	}
	if otherPending := requireValue[[]apitypes.PendingApproval](t)(accounts.other.Client.ListPendingApprovals(ctx)); len(otherPending) != 0 {
		t.Errorf("another account sees %+v", otherPending)
	}
	_, err := accounts.other.Client.GetApproval(ctx, request.RequestID)
	expectStatus(t, "another account reads it", err, http.StatusNotFound)
	expectStatus(t, "another account answers it", accounts.other.Client.AnswerApproval(ctx, request.RequestID, []byte("answer")), http.StatusNotFound)
	expectStatus(t, "another account resolves it", accounts.other.Client.ResolveApproval(ctx, request.RequestID), http.StatusNotFound)
	expectStatus(t, "the Mac answers its own request", accounts.mac.Client.AnswerApproval(ctx, request.RequestID, []byte("answer")), http.StatusForbidden)

	opened := requireValue[approval.Request](t)(approval.OpenRequest(accounts.phone.Keyset, accounts.phone.AccountID, pendingRequests[0].RequestID, pendingRequests[0].SealedRequest, phoneDeviceID, time.Now()))
	sealedAnswer := requireValue[[]byte](t)(approval.CreateAnswer(accounts.phone.Keyset, accounts.phone.AccountID, pendingRequests[0].SealedRequest, opened, approval.DecisionApprove, phoneDeviceID, "Hisar on iPhone", time.Now()))
	if err := accounts.phone.Client.AnswerApproval(ctx, request.RequestID, sealedAnswer); err != nil {
		t.Fatal(err)
	}
	expectStatus(t, "a second answer", accounts.phone.Client.AnswerApproval(ctx, request.RequestID, sealedAnswer), http.StatusConflict)
	if requestID := decodeApprovalEvent(t, macStream.waitForEvent(t, apitypes.EventApprovalAnswered)); requestID != request.RequestID {
		t.Errorf("approval-answered for %q", requestID)
	}
	otherStream.expectNoEvent(t)
	if pendingRequests := requireValue[[]apitypes.PendingApproval](t)(accounts.phone.Client.ListPendingApprovals(ctx)); len(pendingRequests) != 0 {
		t.Errorf("an answered approval is still pending: %+v", pendingRequests)
	}

	stored := requireValue[*apitypes.Approval](t)(accounts.mac.Client.GetApproval(ctx, request.RequestID))
	tracker := approval.NewTracker()
	tracker.Add(request.RequestID, sealedRequest, request.ExpiresAt)
	if decision, err := tracker.Accept(accounts.mac.Keyset, accounts.mac.AccountID, request.RequestID, stored.SealedAnswer, macDeviceID, time.Now()); err != nil || decision != approval.DecisionApprove {
		t.Errorf("the Mac accepts the relayed answer: %q %v", decision, err)
	}

	expectStatus(t, "the phone resolves the Mac's request", accounts.phone.Client.ResolveApproval(ctx, request.RequestID), http.StatusForbidden)
	if err := accounts.mac.Client.ResolveApproval(ctx, request.RequestID); err != nil {
		t.Fatal(err)
	}
	if requestID := decodeApprovalEvent(t, phoneStream.waitForEvent(t, apitypes.EventApprovalResolved)); requestID != request.RequestID {
		t.Errorf("approval-resolved for %q", requestID)
	}
	if stored := requireValue[*apitypes.Approval](t)(accounts.mac.Client.GetApproval(ctx, request.RequestID)); !stored.IsResolved {
		t.Error("not marked resolved")
	}

	withdrawn := approval.NewRequest(macDeviceID, "MacBook Pro", approval.Description{Kind: approval.KindCommandLine, AppName: "hisar"}, time.Now(), approval.MaximumLifetime)
	if err := accounts.mac.Client.CreateApproval(ctx, &apitypes.CreateApprovalRequest{RequestID: withdrawn.RequestID, SealedRequest: sealedRequest, ExpiresAt: withdrawn.ExpiresAt}); err != nil {
		t.Fatal(err)
	}
	if err := accounts.mac.Client.ResolveApproval(ctx, withdrawn.RequestID); err != nil {
		t.Fatal(err)
	}
	expectStatus(t, "an answer after the Mac withdrew the request", accounts.phone.Client.AnswerApproval(ctx, withdrawn.RequestID, sealedAnswer), http.StatusGone)
}

func TestApprovalRelayValidation(t *testing.T) {
	server, accounts := setupApprovalAccounts(t)
	ctx := context.Background()
	now := time.Now()
	validID := approval.GenerateRequestID()
	cases := map[string]apitypes.CreateApprovalRequest{
		"malformed ID":         {RequestID: "not-an-id", SealedRequest: []byte("sealed"), ExpiresAt: now.Add(time.Minute)},
		"upper-case ID":        {RequestID: "00112233445566778899AABBCCDDEEFF", SealedRequest: []byte("sealed"), ExpiresAt: now.Add(time.Minute)},
		"empty":                {RequestID: validID, ExpiresAt: now.Add(time.Minute)},
		"too large":            {RequestID: validID, SealedRequest: make([]byte, approvalMaximumSealedSize+1), ExpiresAt: now.Add(time.Minute)},
		"expires in 4 minutes": {RequestID: validID, SealedRequest: []byte("sealed"), ExpiresAt: now.Add(4 * time.Minute)},
		"already expired":      {RequestID: validID, SealedRequest: []byte("sealed"), ExpiresAt: now.Add(-time.Second)},
	}
	for name, request := range cases {
		expectStatus(t, name, accounts.mac.Client.CreateApproval(ctx, &request), http.StatusBadRequest)
	}
	if err := accounts.mac.Client.CreateApproval(ctx, &apitypes.CreateApprovalRequest{RequestID: validID, SealedRequest: []byte("sealed"), ExpiresAt: now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	expectStatus(t, "an empty answer", accounts.phone.Client.AnswerApproval(ctx, validID, nil), http.StatusBadRequest)
	expectStatus(t, "an answer over 16 KiB", accounts.phone.Client.AnswerApproval(ctx, validID, make([]byte, approvalMaximumSealedSize+1)), http.StatusBadRequest)

	for range maxOpenApprovalPerAccount - 1 {
		if err := accounts.mac.Client.CreateApproval(ctx, &apitypes.CreateApprovalRequest{RequestID: approval.GenerateRequestID(), SealedRequest: []byte("sealed"), ExpiresAt: now.Add(time.Minute)}); err != nil {
			t.Fatal(err)
		}
	}
	expectStatus(t, "the 17th open approval", accounts.mac.Client.CreateApproval(ctx, &apitypes.CreateApprovalRequest{RequestID: approval.GenerateRequestID(), SealedRequest: []byte("sealed"), ExpiresAt: now.Add(time.Minute)}), http.StatusTooManyRequests)
	if err := accounts.other.Client.CreateApproval(ctx, &apitypes.CreateApprovalRequest{RequestID: approval.GenerateRequestID(), SealedRequest: []byte("sealed"), ExpiresAt: now.Add(time.Minute)}); err != nil {
		t.Errorf("another account is limited by this one's approvals: %v", err)
	}

	server.approvals.mutex.Lock()
	server.approvals.limitersByAccount = map[string]*rate.Limiter{}
	server.approvals.mutex.Unlock()
	limitedCount := 0
	for range approvalRateBurst + 10 {
		if _, err := accounts.phone.Client.ListPendingApprovals(ctx); apiclient.IsStatus(err, http.StatusTooManyRequests) {
			limitedCount++
		}
	}
	if limitedCount == 0 {
		t.Error("the per-account rate limit never applied")
	}
	if _, err := accounts.other.Client.ListPendingApprovals(ctx); err != nil {
		t.Errorf("another account was rate limited: %v", err)
	}
}

func TestApprovalRelayExpiry(t *testing.T) {
	server, accounts := setupApprovalAccounts(t)
	ctx := context.Background()
	clock := &testClock{}
	server.approvals.mutex.Lock()
	server.approvals.now = clock.now
	server.approvals.mutex.Unlock()

	requestID := approval.GenerateRequestID()
	if err := accounts.mac.Client.CreateApproval(ctx, &apitypes.CreateApprovalRequest{RequestID: requestID, SealedRequest: []byte("sealed"), ExpiresAt: time.Now().Add(approvalMaximumLifetime + 3*time.Second)}); err != nil {
		t.Fatal(err)
	}
	stored := requireValue[*apitypes.Approval](t)(accounts.phone.Client.GetApproval(ctx, requestID))
	if lifetime := time.Until(stored.ExpiresAt); lifetime > approvalMaximumLifetime {
		t.Errorf("the server keeps a request for %v", lifetime)
	}

	clock.advance(approvalMaximumLifetime + time.Second)
	if pendingRequests := requireValue[[]apitypes.PendingApproval](t)(accounts.phone.Client.ListPendingApprovals(ctx)); len(pendingRequests) != 0 {
		t.Errorf("an expired approval is pending: %+v", pendingRequests)
	}
	expectStatus(t, "an answer after expiry", accounts.phone.Client.AnswerApproval(ctx, requestID, []byte("answer")), http.StatusGone)
	if _, err := accounts.mac.Client.GetApproval(ctx, requestID); err != nil {
		t.Errorf("the Mac cannot read its expired request during the retention: %v", err)
	}

	clock.advance(approvalRetention)
	_, err := accounts.mac.Client.GetApproval(ctx, requestID)
	expectStatus(t, "a request after 2 minutes", err, http.StatusNotFound)
	if err := accounts.mac.Client.CreateApproval(ctx, &apitypes.CreateApprovalRequest{RequestID: approval.GenerateRequestID(), SealedRequest: []byte("sealed"), ExpiresAt: clock.now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	server.approvals.mutex.Lock()
	_, isKept := server.approvals.approvalsByAccount[accounts.mac.AccountID][requestID]
	server.approvals.mutex.Unlock()
	if isKept {
		t.Error("the expired approval is still in memory after a write")
	}
}

func TestApprovalRelayNeedsSession(t *testing.T) {
	baseURL, _, _ := setupServer(t)
	anonymous := apiclient.New(baseURL)
	_, err := anonymous.ListPendingApprovals(context.Background())
	expectStatus(t, "without a session", err, http.StatusUnauthorized)
	expectStatus(t, "posting without a session", anonymous.CreateApproval(context.Background(), &apitypes.CreateApprovalRequest{RequestID: approval.GenerateRequestID(), SealedRequest: []byte("x"), ExpiresAt: time.Now().Add(time.Minute)}), http.StatusUnauthorized)
}

func TestApprovalRelayKeepsALongerDialogsRequest(t *testing.T) {
	server, accounts := setupApprovalAccounts(t)
	ctx := context.Background()
	clock := &testClock{}
	server.approvals.mutex.Lock()
	server.approvals.now = clock.now
	server.approvals.mutex.Unlock()

	requestID := approval.GenerateRequestID()
	if err := accounts.mac.Client.CreateApproval(ctx, &apitypes.CreateApprovalRequest{RequestID: requestID, SealedRequest: []byte("sealed"), ExpiresAt: time.Now().Add(2 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	clock.advance(90 * time.Second)
	pendingRequests := requireValue[[]apitypes.PendingApproval](t)(accounts.phone.Client.ListPendingApprovals(ctx))
	if len(pendingRequests) != 1 || pendingRequests[0].RequestID != requestID {
		t.Fatalf("a phone that connects 90 s later sees %+v", pendingRequests)
	}
	if err := accounts.phone.Client.AnswerApproval(ctx, requestID, []byte("answer")); err != nil {
		t.Errorf("answering 90 s later: %v", err)
	}
}
