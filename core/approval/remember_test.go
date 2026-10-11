package approval

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/crypt"
)

const testRememberLabel = "Also allows Terminal for 1 hour"

func startRememberingTestFlow(t *testing.T, isRememberedByDefault bool) *testFlow {
	t.Helper()
	keyset := createTestKeyset(t)
	description := createTestDescription()
	description.RememberLabel, description.IsRememberedByDefault = testRememberLabel, isRememberedByDefault
	request := CreateRequest(macDeviceID, "Alper's MacBook Pro", description, testNow, MaximumLifetime)
	sealedRequest, err := SealRequest(keyset, testAccountID, request)
	if err != nil {
		t.Fatal(err)
	}
	tracker := CreateTracker()
	tracker.Add(request.RequestID, sealedRequest, request.ExpiresAt)
	return &testFlow{keyset: keyset, request: request, sealedRequest: sealedRequest, tracker: tracker}
}

func (flow *testFlow) answerFromPhoneRemembering(t *testing.T, decision string, isRemembered bool) []byte {
	t.Helper()
	opened, err := OpenRequest(flow.keyset, testAccountID, flow.request.RequestID, flow.sealedRequest, phoneDeviceID, testNow)
	if err != nil {
		t.Fatal(err)
	}
	sealedAnswer, err := CreateRememberingAnswer(flow.keyset, testAccountID, flow.sealedRequest, opened, decision, isRemembered, phoneDeviceID, "Hisar on iPhone", testNow)
	if err != nil {
		t.Fatal(err)
	}
	return sealedAnswer
}

func (flow *testFlow) openAnswer(t *testing.T, sealedAnswer []byte) Answer {
	t.Helper()
	plaintext, err := flow.keyset.OpenApprovalAnswer(testAccountID, flow.request.RequestID, sealedAnswer)
	if err != nil {
		t.Fatal(err)
	}
	var answer Answer
	if err := json.Unmarshal(plaintext, &answer); err != nil {
		t.Fatal(err)
	}
	return answer
}

func (flow *testFlow) resealAnswer(t *testing.T, answer Answer) []byte {
	t.Helper()
	plaintext, _ := json.Marshal(answer)
	sealedAnswer, err := crypt.SealApprovalAnswer(flow.keyset.GetKEMPublicKey(), testAccountID, flow.request.RequestID, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	return sealedAnswer
}

func TestRememberChoiceTravelsWithTheRequest(t *testing.T) {
	flow := startRememberingTestFlow(t, true)
	opened, err := OpenRequest(flow.keyset, testAccountID, flow.request.RequestID, flow.sealedRequest, phoneDeviceID, testNow)
	if err != nil || opened.RememberLabel != testRememberLabel || !opened.IsRememberedByDefault {
		t.Fatalf("opened = %+v, %v", opened.Description, err)
	}
}

func TestPhoneRememberChoiceIsAccepted(t *testing.T) {
	allCase := []struct {
		decision         string
		isRemembered     bool
		wantIsRemembered bool
	}{
		{DecisionApprove, true, true},
		{DecisionApprove, false, false},
		{DecisionDeny, true, false},
	}
	for _, testCase := range allCase {
		flow := startRememberingTestFlow(t, true)
		accepted, err := flow.tracker.AcceptAnswer(flow.keyset, testAccountID, flow.request.RequestID, flow.answerFromPhoneRemembering(t, testCase.decision, testCase.isRemembered), macDeviceID, testNow)
		if err != nil || accepted.Decision != testCase.decision || accepted.IsRemembered != testCase.wantIsRemembered {
			t.Errorf("%s remembered %v: got %+v, %v", testCase.decision, testCase.isRemembered, accepted, err)
		}
	}
}

func TestOldPhoneAnswerIsNotRemembered(t *testing.T) {
	flow := startRememberingTestFlow(t, true)
	sealedAnswer := flow.forgeAnswer(t, flow.keyset, nil)
	if strings.Contains(string(flow.openAnswerPlaintext(t, sealedAnswer)), "isRemembered") {
		t.Fatal("the old answer has the remember field")
	}
	accepted, err := flow.tracker.AcceptAnswer(flow.keyset, testAccountID, flow.request.RequestID, sealedAnswer, macDeviceID, testNow)
	if err != nil || accepted.Decision != DecisionApprove || accepted.IsRemembered {
		t.Errorf("an answer without the remember choice: %+v, %v", accepted, err)
	}
}

func (flow *testFlow) openAnswerPlaintext(t *testing.T, sealedAnswer []byte) []byte {
	t.Helper()
	plaintext, err := flow.keyset.OpenApprovalAnswer(testAccountID, flow.request.RequestID, sealedAnswer)
	if err != nil {
		t.Fatal(err)
	}
	return plaintext
}

func TestRememberChoiceIsBoundBySignature(t *testing.T) {
	isRemembered, isOnce := true, false
	allChange := map[string]struct {
		isRemembered bool
		change       func(*Answer)
	}{
		"once flipped to remember": {false, func(answer *Answer) { answer.IsRemembered = &isRemembered }},
		"remember flipped to once": {true, func(answer *Answer) { answer.IsRemembered = &isOnce }},
		"remember choice removed":  {true, func(answer *Answer) { answer.IsRemembered = nil }},
	}
	for name, testCase := range allChange {
		flow := startRememberingTestFlow(t, true)
		answer := flow.openAnswer(t, flow.answerFromPhoneRemembering(t, DecisionApprove, testCase.isRemembered))
		testCase.change(&answer)
		if _, err := flow.tracker.AcceptAnswer(flow.keyset, testAccountID, flow.request.RequestID, flow.resealAnswer(t, answer), macDeviceID, testNow); !errors.Is(err, crypt.ErrBadSignature) {
			t.Errorf("%s: %v", name, err)
		}
	}

	flow := startRememberingTestFlow(t, true)
	forged := flow.openAnswer(t, flow.forgeAnswer(t, flow.keyset, nil))
	forged.IsRemembered = &isRemembered
	if _, err := flow.tracker.AcceptAnswer(flow.keyset, testAccountID, flow.request.RequestID, flow.resealAnswer(t, forged), macDeviceID, testNow); !errors.Is(err, crypt.ErrBadSignature) {
		t.Errorf("remember choice added to an old answer: %v", err)
	}
}

func TestNoRememberChoiceWithoutOffer(t *testing.T) {
	flow := startTestFlow(t)
	sealedAnswer := flow.answerFromPhoneRemembering(t, DecisionApprove, true)
	if answer := flow.openAnswer(t, sealedAnswer); answer.IsRemembered != nil {
		t.Fatalf("the answer to a request without a remember label carries %v", *answer.IsRemembered)
	}
	fields := crypt.ApprovalAnswerFields{
		AccountID: testAccountID, RequestID: flow.request.RequestID, RequestHash: crypt.HashApprovalRequest(flow.sealedRequest),
		Decision: DecisionApprove, AnsweringDeviceID: phoneDeviceID, AnsweringDeviceName: "Hisar on iPhone", AnsweredAt: FormatTime(testNow),
	}
	if err := crypt.VerifyApprovalAnswer(flow.keyset.GetSigningPublicKey(), fields, flow.openAnswer(t, sealedAnswer).Signature); err != nil {
		t.Errorf("a Mac without the remember choice cannot verify the answer: %v", err)
	}
	accepted, err := flow.tracker.AcceptAnswer(flow.keyset, testAccountID, flow.request.RequestID, sealedAnswer, macDeviceID, testNow)
	if err != nil || accepted.IsRemembered {
		t.Errorf("accepted = %+v, %v", accepted, err)
	}
}
