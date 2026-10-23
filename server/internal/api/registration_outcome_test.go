package api

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/app"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/session"
)

type registrationManualStore struct{ *recoveryPlatformStore }

func (store registrationManualStore) StoreRecoveryKey(string, []byte) error {
	return errors.New("protected storage unavailable")
}

func TestRegistrationRefusalDistinguishesUnusedPreparedAccountFromCommittedRetry(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	email := "registration-race@example.com"
	invite := requireValue[*apitypes.InviteResponse](t)(server.CreateInvite(ctx, email, apitypes.RoleMember, nil))
	password := []byte("registration master password")
	var losing session.PendingFactor
	if _, _, err := session.SignUpWithRecovery(ctx, apiclient.New(baseURL), invite.Token, email, "Prepared loser", password, testDevice, func(pending session.PendingFactor) error {
		losing = pending
		return session.ErrRecoveryDeliveryRequired
	}); !errors.Is(err, session.ErrRecoveryDeliveryRequired) {
		t.Fatalf("registration did not stop before finish: %v", err)
	}
	directory := t.TempDir()
	manualStore := registrationManualStore{&recoveryPlatformStore{factors: map[string]crypt.SecretKey{}, wrapping: map[string][]byte{}}}
	native := requireValue[*app.Service](t)(app.NewService(app.Config{StatePath: filepath.Join(directory, "state.json"), SecretKeyStore: manualStore, IsEventStreamDisabled: true, FileSaver: recoveryKitPath(filepath.Join(directory, "prepared-kit.pdf"))}))
	prepared := requireValue[app.SignUpResult](t)(native.SignUp(baseURL+"/invite/"+invite.Token, email, "Native prepared loser", string(password)))
	if prepared.SetupState != "awaiting-kit" {
		t.Fatalf("unavailable protected storage did not offer precommit delivery: %s", prepared.SetupState)
	}
	if _, err := native.SavePendingRecoveryEmergencyKit(prepared.PendingOperationID, string(password)); err != nil {
		t.Fatal(err)
	}
	if err := native.AcknowledgePendingRecovery(prepared.PendingOperationID, string(password)); err != nil {
		t.Fatal(err)
	}
	var winning session.PendingFactor
	winner, _, err := session.SignUpWithRecovery(ctx, apiclient.New(baseURL), invite.Token, email, "Winner", password, testDevice, func(pending session.PendingFactor) error { winning = pending; return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer winner.Lock()
	if _, _, err := session.ResumePendingFactor(ctx, apiclient.New(baseURL), email, password, losing, testDevice); !session.IsRegistrationNotCommitted(err) {
		t.Fatalf("definite invitation race became an indefinite unknown outcome: %v", err)
	}
	if _, err := native.ResumePendingRecovery(prepared.PendingOperationID, string(password)); err == nil {
		t.Fatal("native setup hid a certified noncommit as an uncertain success")
	}
	if entries := requireValue[[]app.PendingRecoverySummary](t)(native.ListPendingRecovery()); len(entries) != 0 {
		t.Fatal("native setup retained an indefinitely pending never-created account")
	}
	if native.GetStatus().State != app.StateSignedOut {
		t.Fatal("a losing registration installed a native account")
	}
	if _, err := winner.Client.GetAccount(ctx); err != nil {
		t.Fatalf("a losing operation changed the winning account: %v", err)
	}
	if _, err := apiclient.New(baseURL).FinishRegistration(ctx, winning.Registration); err == nil || session.IsRegistrationNotCommitted(err) {
		t.Fatalf("a committed response retry was falsely certified uncommitted: %v", err)
	}
}
