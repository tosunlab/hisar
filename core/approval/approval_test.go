package approval

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/crypt"
)

const (
	testAccountID = "ABC123"
	macDeviceID   = "mac-device"
	phoneDeviceID = "phone-device"
)

var testNow = time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)

type testFlow struct {
	keyset        *crypt.Keyset
	request       Request
	sealedRequest []byte
	tracker       *Tracker
}

func createTestKeyset(t *testing.T) *crypt.Keyset {
	t.Helper()
	keyset, err := crypt.GenerateKeyset()
	if err != nil {
		t.Fatal(err)
	}
	return keyset
}

func createTestDescription() Description {
	return Description{
		Kind:           KindSSH,
		AppName:        "Terminal",
		Purpose:        "push to git.tryhisar.com",
		Host:           "git.tryhisar.com:22",
		CommandLine:    "git push origin main",
		RepositoryPath: "~/Documents/hisar/core",
		KeyName:        "hisar gitlab",
	}
}

// startTestFlow plays the Mac: it seals a request and waits for its answer.
func startTestFlow(t *testing.T) *testFlow {
	t.Helper()
	keyset := createTestKeyset(t)
	request := CreateRequest(macDeviceID, "Alper's MacBook Pro", createTestDescription(), testNow, MaximumLifetime)
	sealedRequest, err := SealRequest(keyset, testAccountID, request)
	if err != nil {
		t.Fatal(err)
	}
	tracker := CreateTracker()
	tracker.Add(request.RequestID, sealedRequest, request.ExpiresAt)
	return &testFlow{keyset: keyset, request: request, sealedRequest: sealedRequest, tracker: tracker}
}

func (flow *testFlow) answerFromPhone(t *testing.T, decision string, now time.Time) []byte {
	t.Helper()
	opened, err := OpenRequest(flow.keyset, testAccountID, flow.request.RequestID, flow.sealedRequest, phoneDeviceID, now)
	if err != nil {
		t.Fatal(err)
	}
	sealedAnswer, err := CreateAnswer(flow.keyset, testAccountID, flow.sealedRequest, opened, decision, phoneDeviceID, "Hisar on iPhone", now)
	if err != nil {
		t.Fatal(err)
	}
	return sealedAnswer
}

func (flow *testFlow) accept(sealedAnswer []byte, now time.Time) (string, error) {
	return flow.tracker.Accept(flow.keyset, testAccountID, flow.request.RequestID, sealedAnswer, macDeviceID, now)
}

// forgeAnswer seals an answer with only the account's public KEM key, as the server could, signed by signingKeyset.
func (flow *testFlow) forgeAnswer(t *testing.T, signingKeyset *crypt.Keyset, change func(*crypt.ApprovalAnswerFields)) []byte {
	t.Helper()
	fields := crypt.ApprovalAnswerFields{
		AccountID:           testAccountID,
		RequestID:           flow.request.RequestID,
		RequestHash:         crypt.HashApprovalRequest(flow.sealedRequest),
		Decision:            DecisionApprove,
		AnsweringDeviceID:   phoneDeviceID,
		AnsweringDeviceName: "Hisar on iPhone",
		AnsweredAt:          FormatTime(testNow),
	}
	signedFields := fields
	if change != nil {
		change(&signedFields)
	}
	plaintext, _ := json.Marshal(Answer{
		RequestID: fields.RequestID, Decision: fields.Decision, AnsweringDeviceID: fields.AnsweringDeviceID,
		AnsweringDeviceName: fields.AnsweringDeviceName, AnsweredAt: fields.AnsweredAt, RequestHash: fields.RequestHash,
		Signature: signingKeyset.SignApprovalAnswer(signedFields),
	})
	sealedAnswer, err := crypt.SealApprovalAnswer(flow.keyset.GetKEMPublicKey(), testAccountID, flow.request.RequestID, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	return sealedAnswer
}

func TestApproveAndDeny(t *testing.T) {
	for _, decision := range []string{DecisionApprove, DecisionDeny} {
		flow := startTestFlow(t)
		opened, err := OpenRequest(flow.keyset, testAccountID, flow.request.RequestID, flow.sealedRequest, phoneDeviceID, testNow.Add(time.Second))
		if err != nil {
			t.Fatal(err)
		}
		if opened.Description != createTestDescription() || opened.RequesterDeviceName != "Alper's MacBook Pro" || !opened.ExpiresAt.Equal(testNow.Add(MaximumLifetime)) {
			t.Errorf("opened request = %+v", opened)
		}
		got, err := flow.accept(flow.answerFromPhone(t, decision, testNow.Add(2*time.Second)), testNow.Add(3*time.Second))
		if err != nil || got != decision {
			t.Errorf("%s: got %q %v", decision, got, err)
		}
	}
}

func TestReplayIsRefused(t *testing.T) {
	flow := startTestFlow(t)
	sealedAnswer := flow.answerFromPhone(t, DecisionApprove, testNow)
	if _, err := flow.accept(sealedAnswer, testNow); err != nil {
		t.Fatal(err)
	}
	if _, err := flow.accept(sealedAnswer, testNow); !errors.Is(err, ErrAlreadyAnswered) {
		t.Errorf("the same answer twice: %v", err)
	}
	if _, err := flow.accept(flow.answerFromPhone(t, DecisionDeny, testNow), testNow); !errors.Is(err, ErrAlreadyAnswered) {
		t.Errorf("a second answer: %v", err)
	}
	flow.tracker.Remove(flow.request.RequestID)
	if _, err := flow.accept(sealedAnswer, testNow); !errors.Is(err, ErrUnknownRequest) {
		t.Errorf("an answer for a finished request: %v", err)
	}

	other := startTestFlow(t)
	other.keyset = flow.keyset
	other.sealedRequest, _ = SealRequest(flow.keyset, testAccountID, other.request)
	other.tracker.Add(other.request.RequestID, other.sealedRequest, other.request.ExpiresAt)
	if _, err := other.tracker.Accept(flow.keyset, testAccountID, other.request.RequestID, sealedAnswer, macDeviceID, testNow); err == nil {
		t.Error("an answer was accepted for another request")
	}
}

func TestExpiry(t *testing.T) {
	flow := startTestFlow(t)
	expired := flow.request.ExpiresAt
	if _, err := OpenRequest(flow.keyset, testAccountID, flow.request.RequestID, flow.sealedRequest, phoneDeviceID, expired); !errors.Is(err, ErrExpired) {
		t.Errorf("the phone opened an expired request: %v", err)
	}
	if _, err := CreateAnswer(flow.keyset, testAccountID, flow.sealedRequest, flow.request, DecisionApprove, phoneDeviceID, "Hisar on iPhone", expired); !errors.Is(err, ErrExpired) {
		t.Errorf("the phone answered an expired request: %v", err)
	}
	sealedAnswer := flow.answerFromPhone(t, DecisionApprove, expired.Add(-time.Second))
	if _, err := flow.accept(sealedAnswer, expired); !errors.Is(err, ErrExpired) {
		t.Errorf("the Mac accepted after expiry: %v", err)
	}
	if _, err := flow.accept(sealedAnswer, expired.Add(-time.Millisecond)); err != nil {
		t.Errorf("an answer just in time: %v", err)
	}
}

func TestSelfAnswerIsRefused(t *testing.T) {
	flow := startTestFlow(t)
	if _, err := OpenRequest(flow.keyset, testAccountID, flow.request.RequestID, flow.sealedRequest, macDeviceID, testNow); !errors.Is(err, ErrOwnRequest) {
		t.Errorf("the Mac opened its own request as a phone: %v", err)
	}
	if _, err := CreateAnswer(flow.keyset, testAccountID, flow.sealedRequest, flow.request, DecisionApprove, macDeviceID, "Mac", testNow); !errors.Is(err, ErrSelfAnswer) {
		t.Errorf("the Mac answered its own request: %v", err)
	}
	selfAnswer := flow.forgeAnswer(t, flow.keyset, nil)
	if _, err := flow.tracker.Accept(flow.keyset, testAccountID, flow.request.RequestID, selfAnswer, phoneDeviceID, testNow); !errors.Is(err, ErrSelfAnswer) {
		t.Errorf("an answer from the requesting device: %v", err)
	}
	if _, err := flow.tracker.Accept(flow.keyset, testAccountID, flow.request.RequestID, selfAnswer, "", testNow); !errors.Is(err, ErrMissingDeviceID) {
		t.Errorf("accepted without knowing this device: %v", err)
	}
}

func TestForgedAndTamperedAnswersAreRefused(t *testing.T) {
	flow := startTestFlow(t)
	otherKeyset := createTestKeyset(t)
	genuine := flow.answerFromPhone(t, DecisionApprove, testNow)
	tampered := append([]byte(nil), genuine...)
	tampered[len(tampered)/2] ^= 1
	sealedToOtherAccount, _ := crypt.SealApprovalAnswer(otherKeyset.GetKEMPublicKey(), testAccountID, flow.request.RequestID, []byte("{}"))
	allCase := map[string]struct {
		sealedAnswer []byte
		want         error
	}{
		"tampered":                    {tampered, crypt.ErrDecrypt},
		"sealed to another account":   {sealedToOtherAccount, crypt.ErrDecrypt},
		"signed by another account":   {flow.forgeAnswer(t, otherKeyset, nil), crypt.ErrBadSignature},
		"signature for deny":          {flow.forgeAnswer(t, flow.keyset, func(fields *crypt.ApprovalAnswerFields) { fields.Decision = DecisionDeny }), crypt.ErrBadSignature},
		"signature for another hash":  {flow.forgeAnswer(t, flow.keyset, func(fields *crypt.ApprovalAnswerFields) { fields.RequestHash = make([]byte, 32) }), crypt.ErrBadSignature},
		"signature for another phone": {flow.forgeAnswer(t, flow.keyset, func(fields *crypt.ApprovalAnswerFields) { fields.AnsweringDeviceID = "other" }), crypt.ErrBadSignature},
		"garbage":                     {[]byte("not sealed"), crypt.ErrBadVersion},
		"too large":                   {make([]byte, MaximumSealedSize+1), ErrSealedTooLarge},
	}
	for name, testCase := range allCase {
		if _, err := flow.accept(testCase.sealedAnswer, testNow); !errors.Is(err, testCase.want) {
			t.Errorf("%s: got %v, want %v", name, err, testCase.want)
		}
	}
	if got, err := flow.accept(genuine, testNow); err != nil || got != DecisionApprove {
		t.Errorf("the genuine answer after refused forgeries: %q %v", got, err)
	}
}

func TestAnswerBindsTheExactSealedRequest(t *testing.T) {
	flow := startTestFlow(t)
	resealed, err := SealRequest(flow.keyset, testAccountID, flow.request)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := OpenRequest(flow.keyset, testAccountID, flow.request.RequestID, resealed, phoneDeviceID, testNow)
	if err != nil {
		t.Fatal(err)
	}
	sealedAnswer, err := CreateAnswer(flow.keyset, testAccountID, resealed, opened, DecisionApprove, phoneDeviceID, "Hisar on iPhone", testNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := flow.accept(sealedAnswer, testNow); !errors.Is(err, ErrRequestMismatch) {
		t.Errorf("an answer to a different sealing of the request: %v", err)
	}
}

func TestOpenRequestChecks(t *testing.T) {
	flow := startTestFlow(t)
	otherKeyset := createTestKeyset(t)
	if _, err := OpenRequest(otherKeyset, testAccountID, flow.request.RequestID, flow.sealedRequest, phoneDeviceID, testNow); !errors.Is(err, crypt.ErrDecrypt) {
		t.Errorf("another account opened the request: %v", err)
	}
	movedID := GenerateRequestID()
	if _, err := OpenRequest(flow.keyset, testAccountID, movedID, flow.sealedRequest, phoneDeviceID, testNow); !errors.Is(err, crypt.ErrDecrypt) {
		t.Errorf("the request opened under another ID: %v", err)
	}
	if _, err := OpenRequest(flow.keyset, testAccountID, "not-hex", flow.sealedRequest, phoneDeviceID, testNow); !errors.Is(err, ErrMissingRequestID) {
		t.Errorf("a malformed ID: %v", err)
	}
	if _, err := OpenRequest(flow.keyset, testAccountID, flow.request.RequestID, flow.sealedRequest, "", testNow); !errors.Is(err, ErrMissingDeviceID) {
		t.Errorf("opened without knowing this device: %v", err)
	}

	tooLong := flow.request
	tooLong.ExpiresAt = tooLong.CreatedAt.Add(MaximumLifetime + time.Second)
	if _, err := SealRequest(flow.keyset, testAccountID, tooLong); !errors.Is(err, ErrLifetimeTooLong) {
		t.Errorf("sealed a request over 60 seconds: %v", err)
	}
	plaintext, _ := json.Marshal(tooLong)
	sealedTooLong, _ := crypt.SealApprovalRequest(flow.keyset.GetKEMPublicKey(), testAccountID, tooLong.RequestID, plaintext)
	if _, err := OpenRequest(flow.keyset, testAccountID, tooLong.RequestID, sealedTooLong, phoneDeviceID, testNow); !errors.Is(err, ErrLifetimeTooLong) {
		t.Errorf("opened a request over 60 seconds: %v", err)
	}

	mismatched := flow.request
	mismatched.RequestID = GenerateRequestID()
	plaintext, _ = json.Marshal(mismatched)
	sealedMismatched, _ := crypt.SealApprovalRequest(flow.keyset.GetKEMPublicKey(), testAccountID, flow.request.RequestID, plaintext)
	if _, err := OpenRequest(flow.keyset, testAccountID, flow.request.RequestID, sealedMismatched, phoneDeviceID, testNow); !errors.Is(err, ErrRequestMismatch) {
		t.Errorf("a request whose inner ID differs: %v", err)
	}

	unknownKind := flow.request
	unknownKind.Kind = "unlock"
	if _, err := SealRequest(flow.keyset, testAccountID, unknownKind); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("sealed an unknown kind: %v", err)
	}
	if _, err := CreateAnswer(flow.keyset, testAccountID, flow.sealedRequest, flow.request, "unlock", phoneDeviceID, "Hisar on iPhone", testNow); !errors.Is(err, ErrUnknownDecision) {
		t.Errorf("answered with an unknown decision: %v", err)
	}
}

func TestRequestIDAndLifetime(t *testing.T) {
	first, second := GenerateRequestID(), GenerateRequestID()
	if first == second || !IsValidRequestID(first) || len(first) != 32 {
		t.Errorf("request IDs %q %q", first, second)
	}
	for _, invalid := range []string{"", "00112233445566778899AABBCCDDEEFF", "0011", "00112233445566778899aabbccddeeff00", "zz112233445566778899aabbccddeeff"} {
		if IsValidRequestID(invalid) {
			t.Errorf("%q counted as valid", invalid)
		}
	}
	request := CreateRequest(macDeviceID, "Mac", createTestDescription(), testNow, 5*time.Minute)
	if request.ExpiresAt.Sub(request.CreatedAt) != MaximumLifetime {
		t.Errorf("lifetime = %v", request.ExpiresAt.Sub(request.CreatedAt))
	}
	if FormatTime(time.Date(2026, 10, 11, 12, 0, 0, 123456789, time.FixedZone("CEST", 7200))) != "2026-10-11T10:00:00.123Z" {
		t.Error("answer times are not UTC with milliseconds")
	}
}
