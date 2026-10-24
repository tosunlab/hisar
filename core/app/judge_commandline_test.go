package app

import (
	"context"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/clisocket"
	"testing"
)

func TestJudgeCommandContinuationDoesNotMutateReplacementAccount(t *testing.T) {
	s := createUnlockedTestService(t).service
	bridge := NewCommandLineBridge(s, clisocket.NewSessionApprovals(0, nil, nil))
	ctx := bridge.BindSession(context.Background())
	epoch := ctx.Value(sessionEpochKey{}).(uint64)
	if !s.isSessionEpoch(epoch) {
		t.Fatal("original epoch is not current")
	}
	signInOtherAccount(t, s, createUnlockedTestService(t))
	_, err := bridge.answer(ctx, epoch, &cliprotocol.Request{Type: cliprotocol.TypeCreateVault, Name: "stale approval wrote this"})
	if err != nil {
		t.Logf("continuation returned %v", err)
	}
	if s.isSessionEpoch(epoch) {
		t.Fatal("account switch did not move epoch")
	}
	vaults, err := s.ListVaults()
	if err != nil {
		t.Fatal(err)
	}
	for _, vault := range vaults {
		if vault.Name == "stale approval wrote this" {
			t.Error("request switched after HandleRequest's initial check and created a vault in the new account; final check only drops the response")
		}
	}
}
