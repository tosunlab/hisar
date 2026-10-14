package crypt

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"
)

const (
	testApprovalAccountID = "ABC123"
	testApprovalRequestID = "00112233445566778899aabbccddeeff"
)

func createVectorKeyset(t *testing.T) *Keyset {
	t.Helper()
	seeds := make([]byte, 3*seedSize)
	for index := range seeds {
		seeds[index] = byte(index)
	}
	keyset, err := decodeKeyset(seeds)
	if err != nil {
		t.Fatal(err)
	}
	return keyset
}

func createVectorAnswerFields() ApprovalAnswerFields {
	return ApprovalAnswerFields{
		AccountID:           testApprovalAccountID,
		RequestID:           testApprovalRequestID,
		RequestHash:         HashApprovalRequest([]byte("sealed request")),
		Decision:            "approve",
		AnsweringDeviceID:   "phone-1",
		AnsweringDeviceName: "Hisar on iPhone",
		AnsweredAt:          "2026-10-11T12:00:00.000Z",
	}
}

func TestApprovalRequestVector(t *testing.T) {
	keyset := createVectorKeyset(t)
	plaintext, err := keyset.OpenApprovalRequest(testApprovalAccountID, testApprovalRequestID, decodeHex(t, vectorApprovalSealedRequest))
	if err != nil {
		t.Fatal(err)
	}
	if string(plaintext) != `{"requestId":"00112233445566778899aabbccddeeff"}` {
		t.Errorf("plaintext = %s", plaintext)
	}
}

func TestApprovalAnswerVector(t *testing.T) {
	keyset := createVectorKeyset(t)
	fields := createVectorAnswerFields()
	if got := hex.EncodeToString(encodeApprovalAnswer(fields)); got != vectorApprovalAnswerMessage {
		t.Errorf("signed message = %s", got)
	}
	signature := keyset.SignApprovalAnswer(fields)
	if got := hex.EncodeToString(signature); got != vectorApprovalAnswerSignature {
		t.Errorf("signature = %s", got)
	}
	if err := VerifyApprovalAnswer(keyset.GetSigningPublicKey(), fields, decodeHex(t, vectorApprovalAnswerSignature)); err != nil {
		t.Error(err)
	}
}

func TestApprovalAnswerRememberChoiceIsSigned(t *testing.T) {
	keyset := createVectorKeyset(t)
	fields := createVectorAnswerFields()
	fields.RememberChoice = ApprovalRememberChoice
	if got, want := hex.EncodeToString(encodeApprovalAnswer(fields)), vectorApprovalAnswerMessage+"0000000872656d656d626572"; got != want {
		t.Errorf("signed message = %s", got)
	}
	signature := keyset.SignApprovalAnswer(fields)
	if err := VerifyApprovalAnswer(keyset.GetSigningPublicKey(), fields, signature); err != nil {
		t.Fatal(err)
	}
	for _, changedChoice := range []string{"", ApprovalOnceChoice} {
		changed := fields
		changed.RememberChoice = changedChoice
		if err := VerifyApprovalAnswer(keyset.GetSigningPublicKey(), changed, signature); !errors.Is(err, ErrBadSignature) {
			t.Errorf("choice %q: %v", changedChoice, err)
		}
	}
}

func TestApprovalRequestSealOpen(t *testing.T) {
	keyset, _ := GenerateKeyset()
	otherKeyset, _ := GenerateKeyset()
	sealed, err := SealApprovalRequest(keyset.GetKEMPublicKey(), testApprovalAccountID, testApprovalRequestID, []byte("push to git.tryhisar.com"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("git.tryhisar.com")) {
		t.Fatal("the sealed request contains its plaintext")
	}
	if plaintext, err := keyset.OpenApprovalRequest(testApprovalAccountID, testApprovalRequestID, sealed); err != nil || string(plaintext) != "push to git.tryhisar.com" {
		t.Fatalf("open: %q %v", plaintext, err)
	}
	tampered := bytes.Clone(sealed)
	tampered[len(tampered)-1] ^= 1
	cases := map[string]struct {
		keyset               *Keyset
		accountID, requestID string
		sealed               []byte
		want                 error
	}{
		"wrong account key":    {otherKeyset, testApprovalAccountID, testApprovalRequestID, sealed, ErrDecrypt},
		"other account ID":     {keyset, "XYZ789", testApprovalRequestID, sealed, ErrDecrypt},
		"other request ID":     {keyset, testApprovalAccountID, "ffeeddccbbaa99887766554433221100", sealed, ErrDecrypt},
		"tampered":             {keyset, testApprovalAccountID, testApprovalRequestID, tampered, ErrDecrypt},
		"bad version":          {keyset, testApprovalAccountID, testApprovalRequestID, append([]byte{2}, sealed[1:]...), ErrBadVersion},
		"empty":                {keyset, testApprovalAccountID, testApprovalRequestID, nil, ErrShortCipher},
		"answer is no request": {keyset, testApprovalAccountID, testApprovalRequestID, mustSealAnswer(t, keyset), ErrDecrypt},
	}
	for name, testCase := range cases {
		if _, err := testCase.keyset.OpenApprovalRequest(testCase.accountID, testCase.requestID, testCase.sealed); !errors.Is(err, testCase.want) {
			t.Errorf("%s: got %v, want %v", name, err, testCase.want)
		}
	}
}

func mustSealAnswer(t *testing.T, keyset *Keyset) []byte {
	t.Helper()
	sealed, err := SealApprovalAnswer(keyset.GetKEMPublicKey(), testApprovalAccountID, testApprovalRequestID, []byte("answer"))
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func TestApprovalAnswerSealOpen(t *testing.T) {
	keyset, _ := GenerateKeyset()
	otherKeyset, _ := GenerateKeyset()
	sealed := mustSealAnswer(t, keyset)
	if plaintext, err := keyset.OpenApprovalAnswer(testApprovalAccountID, testApprovalRequestID, sealed); err != nil || string(plaintext) != "answer" {
		t.Fatalf("open: %q %v", plaintext, err)
	}
	if _, err := otherKeyset.OpenApprovalAnswer(testApprovalAccountID, testApprovalRequestID, sealed); !errors.Is(err, ErrDecrypt) {
		t.Errorf("wrong account key: %v", err)
	}
	if _, err := keyset.OpenApprovalAnswer(testApprovalAccountID, "ffeeddccbbaa99887766554433221100", sealed); !errors.Is(err, ErrDecrypt) {
		t.Errorf("other request ID: %v", err)
	}
}

func TestApprovalAnswerSignatureBindsEveryField(t *testing.T) {
	keyset, _ := GenerateKeyset()
	otherKeyset, _ := GenerateKeyset()
	fields := createVectorAnswerFields()
	signature := keyset.SignApprovalAnswer(fields)
	if err := VerifyApprovalAnswer(keyset.GetSigningPublicKey(), fields, signature); err != nil {
		t.Fatal(err)
	}
	changes := map[string]func(*ApprovalAnswerFields){
		"account": func(changed *ApprovalAnswerFields) { changed.AccountID = "XYZ789" },
		"request": func(changed *ApprovalAnswerFields) { changed.RequestID = "ffeeddccbbaa99887766554433221100" },
		"request hash": func(changed *ApprovalAnswerFields) {
			changed.RequestHash = HashApprovalRequest([]byte("other request"))
		},
		"decision":    func(changed *ApprovalAnswerFields) { changed.Decision = "deny" },
		"device":      func(changed *ApprovalAnswerFields) { changed.AnsweringDeviceID = "phone-2" },
		"device name": func(changed *ApprovalAnswerFields) { changed.AnsweringDeviceName = "Other iPhone" },
		"answered at": func(changed *ApprovalAnswerFields) { changed.AnsweredAt = "2026-10-11T12:00:01.000Z" },
		"remember choice added": func(changed *ApprovalAnswerFields) {
			changed.RememberChoice = ApprovalRememberChoice
		},
	}
	for name, change := range changes {
		changed := fields
		change(&changed)
		if err := VerifyApprovalAnswer(keyset.GetSigningPublicKey(), changed, signature); !errors.Is(err, ErrBadSignature) {
			t.Errorf("changed %s: %v", name, err)
		}
	}
	if err := VerifyApprovalAnswer(otherKeyset.GetSigningPublicKey(), fields, signature); !errors.Is(err, ErrBadSignature) {
		t.Errorf("other account's key: %v", err)
	}
	if err := VerifyApprovalAnswer(nil, fields, signature); !errors.Is(err, ErrBadSignature) {
		t.Errorf("no key: %v", err)
	}
	if err := VerifyApprovalAnswer(keyset.GetSigningPublicKey(), fields, otherKeyset.SignApprovalAnswer(fields)); !errors.Is(err, ErrBadSignature) {
		t.Errorf("signed by another account: %v", err)
	}
	kemSignature := keyset.GetPublicKeys(testApprovalAccountID).KEMSig
	if err := VerifyApprovalAnswer(keyset.GetSigningPublicKey(), fields, kemSignature); !errors.Is(err, ErrBadSignature) {
		t.Errorf("a signature from another domain: %v", err)
	}
}
