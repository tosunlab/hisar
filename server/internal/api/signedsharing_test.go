package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/session"
)

type shareCompletionRecorder struct {
	calls int
	last  apitypes.CompleteShareRequest
}

func (recorder *shareCompletionRecorder) RoundTrip(request *http.Request) (*http.Response, error) {
	if strings.HasSuffix(request.URL.Path, "/complete") {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		request.Body = io.NopCloser(bytes.NewReader(body))
		if err := json.Unmarshal(body, &recorder.last); err != nil {
			return nil, err
		}
		recorder.calls++
	}
	return http.DefaultTransport.RoundTrip(request)
}

func signedShareRequest(t *testing.T, owner *session.Session, vault *session.Vault, email, role string, recipient *session.Session) apitypes.ShareInvitationRequest {
	t.Helper()
	current := requireValue[*session.Vault](t)(owner.FindVault(context.Background(), vault.ID))
	intent := crypt.ShareIntent{Version: 1, Origin: owner.Client.BaseURL, ID: uuid.NewString(), VaultID: vault.ID, InvitedBy: owner.AccountID, Email: apitypes.NormalizeEmail(email), Role: role, KeyGeneration: current.KeyGeneration, AccessRevision: current.AccessRevision, IssuedAt: time.Now().Unix(), ExpiresAt: time.Now().Add(time.Hour).Unix()}
	if recipient != nil {
		keys := recipient.Keyset.GetPublicKeys(recipient.AccountID)
		intent.RecipientID = recipient.AccountID
		intent.RecipientKeyHash = crypt.ShareRecipientKeyHash(keys.Sign, keys.KEM)
	}
	return apitypes.ShareInvitationRequest{Email: email, Role: role, Intent: &apitypes.SignedShareIntent{Context: intent, Signature: owner.Keyset.SignShareIntent(intent)}}
}

func secondSharingDevice(t *testing.T, owner *session.Session) *session.Session {
	t.Helper()
	key := crypt.GenerateKey()
	defer clear(key)
	sealed := requireValue[[]byte](t)(owner.Keyset.Encrypt(key, owner.AccountID))
	keyset := requireValue[*crypt.Keyset](t)(crypt.DecryptKeyset(key, owner.AccountID, sealed))
	t.Cleanup(keyset.Wipe)
	client := apiclient.New(owner.Client.BaseURL)
	client.Token = owner.Client.Token
	return &session.Session{Client: client, AccountID: owner.AccountID, Email: owner.Email, Keyset: keyset, MemberVerifier: owner.MemberVerifier}
}

func completeReviewedShares(t *testing.T, owner *session.Session) []session.CompletedShare {
	t.Helper()
	ctx := context.Background()
	completed := requireValue[[]session.CompletedShare](t)(owner.CompleteAcceptedShares(ctx))
	invitations := requireValue[*apitypes.ShareInvitationList](t)(owner.Client.ListShareInvitations(ctx))
	for _, invitation := range invitations.Sent {
		if invitation.State != apitypes.ShareInvitationAccepted || invitation.Recipient == nil {
			continue
		}
		if err := owner.ConfirmShareRecipient(ctx, invitation.ID, session.ShareInvitationReviewHash(invitation)); err != nil {
			t.Fatal(err)
		}
		vault := requireValue[*session.Vault](t)(owner.FindVault(ctx, invitation.VaultID))
		completed = append(completed, session.CompletedShare{VaultID: vault.ID, VaultName: vault.Name, Role: invitation.Role, Recipient: *invitation.Recipient})
	}
	return completed
}

func TestSignedSharingRejectsStolenBearerAndRetiredIntent(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	owner := signUpTestAccount(t, ctx, server, baseURL, "owner@example.com", apitypes.RoleMember)
	attacker := signUpTestAccount(t, ctx, server, baseURL, "attacker@example.com", apitypes.RoleMember)
	vault := requireValue[*session.Vault](t)(owner.CreateVault(ctx, "Secrets", apitypes.VaultShared))
	requireValue[*item.Item](t)(owner.SaveItem(ctx, vault, &item.Item{Overview: item.Overview{Title: "Private"}, Details: item.Details{Password: "owner-only"}}))
	stolen := apiclient.New(baseURL)
	stolen.Token = owner.Client.Token
	if _, err := stolen.CreateShareInvitation(ctx, vault.ID, &apitypes.ShareInvitationRequest{Email: attacker.Email, Role: apitypes.VaultOwner}); !apiclient.IsStatus(err, http.StatusBadRequest) {
		t.Fatalf("unsigned bearer invitation accepted: %v", err)
	}
	if completed, err := owner.CompleteAcceptedShares(ctx); err != nil || len(completed) != 0 {
		t.Fatalf("owner sync: %v %v", completed, err)
	}
	if _, err := attacker.FindVault(ctx, vault.ID); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("stolen bearer delivered a key: %v", err)
	}
	request := signedShareRequest(t, owner, vault, attacker.Email, apitypes.VaultReader, attacker)
	if err := owner.Client.CreateSignedShareInvitation(ctx, vault.ID, &request); err != nil {
		t.Fatal(err)
	}
	if err := attacker.Client.AcceptShareInvitation(ctx, request.Intent.Context.ID); err != nil {
		t.Fatal(err)
	}
	// Another unlocked owner device has no creating-device ledger.
	otherDevice := secondSharingDevice(t, owner)
	completed := requireValue[[]session.CompletedShare](t)(otherDevice.CompleteAcceptedShares(ctx))
	if len(completed) != 1 {
		t.Fatalf("cross-device completion: %+v", completed)
	}
	received := requireValue[*session.Vault](t)(attacker.FindVault(ctx, vault.ID))
	if !bytes.Equal(received.Key, vault.Key) {
		t.Fatal("legitimate recipient cannot decrypt")
	}
	if err := owner.Client.RemoveAccess(ctx, vault.ID, attacker.AccountID); err != nil {
		t.Fatal(err)
	}
	if err := stolen.CreateSignedShareInvitation(ctx, vault.ID, &request); !apiclient.IsStatus(err, http.StatusConflict) {
		t.Fatalf("retired operation recreated: %v", err)
	}
	wrapped := requireValue[[]byte](t)(crypt.WrapVaultKey(attacker.Keyset.GetPublicKeys(attacker.AccountID).KEM, vault.ID, attacker.AccountID, vault.Key))
	if err := stolen.SetAccess(ctx, vault.ID, attacker.AccountID, &apitypes.SetAccessRequest{WrappedKey: wrapped, Role: apitypes.VaultReader, KeyGeneration: vault.KeyGeneration}); !apiclient.IsStatus(err, http.StatusConflict) {
		t.Fatalf("generic grant bypass: %v", err)
	}
	if err := stolen.CompleteSignedShare(ctx, &apitypes.CompleteShareRequest{Intent: *request.Intent, WrappedKey: wrapped}); !apiclient.IsStatus(err, http.StatusConflict) {
		t.Fatalf("consumed intent replay: %v", err)
	}
	if _, err := attacker.FindVault(ctx, vault.ID); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("replay restored access: %v", err)
	}
}

func TestNewRecipientRequiresExactNativeReview(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	owner := signUpTestAccount(t, ctx, server, baseURL, "owner@example.com", apitypes.RoleMember)
	vault := requireValue[*session.Vault](t)(owner.CreateVault(ctx, "Team", apitypes.VaultShared))
	coOwner := signUpTestAccount(t, ctx, server, baseURL, "coowner@example.com", apitypes.RoleMember)
	shareThroughInvitation(t, owner, coOwner, vault, apitypes.VaultOwner)
	if err := owner.InviteToVault(ctx, vault, "new@example.com", apitypes.VaultEditor); err != nil {
		t.Fatal(err)
	}
	recipient := signUpTestAccount(t, ctx, server, baseURL, "new@example.com", apitypes.RoleMember)
	received := requireValue[*apitypes.ShareInvitationList](t)(recipient.Client.ListShareInvitations(ctx))
	if len(received.Received) != 1 {
		t.Fatal(received)
	}
	if err := recipient.Client.AcceptShareInvitation(ctx, received.Received[0].ID); err != nil {
		t.Fatal(err)
	}
	if completed := requireValue[[]session.CompletedShare](t)(owner.CompleteAcceptedShares(ctx)); len(completed) != 0 {
		t.Fatal("unknown identity received an automatic key")
	}
	invitations := requireValue[*apitypes.ShareInvitationList](t)(owner.Client.ListShareInvitations(ctx))
	reviewed := invitations.Sent[0]
	digest := session.ShareInvitationReviewHash(reviewed)
	altered := reviewed
	altered.Role = apitypes.VaultOwner
	if err := owner.ConfirmShareRecipient(ctx, reviewed.ID, session.ShareInvitationReviewHash(altered)); err == nil {
		t.Fatal("changed reviewed role authorized")
	}
	otherDevice := secondSharingDevice(t, coOwner)
	if err := otherDevice.ConfirmShareRecipient(ctx, reviewed.ID, digest); err != nil {
		t.Fatal(err)
	}
	actual := requireValue[*session.Vault](t)(recipient.FindVault(ctx, vault.ID))
	if !bytes.Equal(actual.Key, vault.Key) || actual.Role != apitypes.VaultEditor {
		t.Fatal("confirmed new recipient cannot open exact grant")
	}
}

func TestSharingMultiplePendingRecipientsAndIssuerRestore(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	owner := signUpTestAccount(t, ctx, server, baseURL, "owner@example.com", apitypes.RoleMember)
	bob := signUpTestAccount(t, ctx, server, baseURL, "bob@example.com", apitypes.RoleMember)
	carol := signUpTestAccount(t, ctx, server, baseURL, "carol@example.com", apitypes.RoleMember)
	dave := signUpTestAccount(t, ctx, server, baseURL, "dave@example.com", apitypes.RoleMember)
	vault := requireValue[*session.Vault](t)(owner.CreateVault(ctx, "Team", apitypes.VaultShared))
	for _, recipient := range []*session.Session{bob, carol} {
		request := signedShareRequest(t, owner, vault, recipient.Email, apitypes.VaultOwner, recipient)
		if err := owner.Client.CreateSignedShareInvitation(ctx, vault.ID, &request); err != nil {
			t.Fatal(err)
		}
		if err := recipient.Client.AcceptShareInvitation(ctx, request.Intent.Context.ID); err != nil {
			t.Fatal(err)
		}
	}
	if completed := requireValue[[]session.CompletedShare](t)(secondSharingDevice(t, owner).CompleteAcceptedShares(ctx)); len(completed) != 2 {
		t.Fatalf("unrelated completion invalidated another recipient: %+v", completed)
	}
	request := signedShareRequest(t, owner, vault, dave.Email, apitypes.VaultReader, dave)
	if err := owner.Client.CreateSignedShareInvitation(ctx, vault.ID, &request); err != nil {
		t.Fatal(err)
	}
	if err := dave.Client.AcceptShareInvitation(ctx, request.Intent.Context.ID); err != nil {
		t.Fatal(err)
	}
	bobVault := requireValue[*session.Vault](t)(bob.FindVault(ctx, vault.ID))
	ownerMember := requireValue[*apitypes.Member](t)(bob.GetVerifiedMember(ctx, owner.AccountID))
	if err := bob.ShareVault(ctx, bobVault, ownerMember, apitypes.VaultEditor); err != nil {
		t.Fatal(err)
	}
	if err := bob.ShareVault(ctx, bobVault, ownerMember, apitypes.VaultOwner); err != nil {
		t.Fatal(err)
	}
	if completed := requireValue[[]session.CompletedShare](t)(owner.CompleteAcceptedShares(ctx)); len(completed) != 0 {
		t.Fatal("restoring issuer resurrected an old invitation")
	}
	if err := owner.Client.CreateSignedShareInvitation(ctx, vault.ID, &request); !apiclient.IsStatus(err, http.StatusConflict) {
		t.Fatalf("restored owner replayed retired authorization: %v", err)
	}
	if _, err := dave.FindVault(ctx, vault.ID); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("revoked inviter delivered a key: %v", err)
	}
}

func TestLegacyAndAlteredSharingNeverAutoDeliver(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	ctx := context.Background()
	owner := signUpTestAccount(t, ctx, server, baseURL, "owner@example.com", apitypes.RoleMember)
	recipient := signUpTestAccount(t, ctx, server, baseURL, "recipient@example.com", apitypes.RoleMember)
	vault := requireValue[*session.Vault](t)(owner.CreateVault(ctx, "Team", apitypes.VaultShared))
	recorder := &shareCompletionRecorder{}
	httpClient := *owner.Client.HTTP
	httpClient.Transport = recorder
	owner.Client.HTTP = &httpClient
	connection := requireValue[*pgx.Conn](t)(pgx.Connect(ctx, databaseURL))
	defer connection.Close(ctx)
	request := signedShareRequest(t, owner, vault, recipient.Email, apitypes.VaultReader, recipient)
	if err := owner.Client.CreateSignedShareInvitation(ctx, vault.ID, &request); err != nil {
		t.Fatal(err)
	}
	if err := recipient.Client.AcceptShareInvitation(ctx, request.Intent.Context.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(ctx, `UPDATE share_invitations SET expires_at=expires_at+interval '1 hour' WHERE id=$1`, request.Intent.Context.ID); err != nil {
		t.Fatal(err)
	}
	if completed, err := owner.CompleteAcceptedShares(ctx); err == nil || len(completed) != 0 || recorder.calls != 0 {
		t.Fatalf("altered unsigned deadline caused a wrap: %+v %v", completed, err)
	}
	if _, err := connection.Exec(ctx, `UPDATE share_invitations SET expires_at=to_timestamp($2) WHERE id=$1`, request.Intent.Context.ID, request.Intent.Context.ExpiresAt); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(ctx, `UPDATE share_invitations SET role='owner', intent=jsonb_set(intent,'{context,role}',to_jsonb('owner'::text)) WHERE id=$1`, request.Intent.Context.ID); err != nil {
		t.Fatal(err)
	}
	if completed, err := owner.CompleteAcceptedShares(ctx); err == nil || len(completed) != 0 {
		t.Fatalf("altered signed payload completed: %+v %v", completed, err)
	}
	if err := owner.Client.CancelShareInvitation(ctx, request.Intent.Context.ID); err != nil {
		t.Fatal(err)
	}
	legacyID := uuid.NewString()
	if _, err := connection.Exec(ctx, `INSERT INTO share_invitations(id,vault_id,email,role,invited_by,state,accepted_by,accepted_at,expires_at) VALUES($1,$2,$3,'reader',$4,'accepted',$5,now(),now()+interval '1 hour')`, legacyID, vault.ID, recipient.Email, owner.AccountID, recipient.AccountID); err != nil {
		t.Fatal(err)
	}
	if completed := requireValue[[]session.CompletedShare](t)(owner.CompleteAcceptedShares(ctx)); len(completed) != 0 {
		t.Fatal("unsigned legacy invitation delivered a key")
	}
	if _, err := recipient.FindVault(ctx, vault.ID); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("unreviewed legacy recipient got access: %v", err)
	}
	if recorder.calls != 0 {
		t.Fatalf("an unapproved recipient got an attacker-decryptable wrap in %d completion requests", recorder.calls)
	}
	invitation := requireValue[*apitypes.ShareInvitationList](t)(owner.Client.ListShareInvitations(ctx)).Sent[0]
	if err := owner.ConfirmShareRecipient(ctx, legacyID, session.ShareInvitationReviewHash(invitation)); err != nil {
		t.Fatal(err)
	}
	actual := requireValue[*session.Vault](t)(recipient.FindVault(ctx, vault.ID))
	if !bytes.Equal(actual.Key, vault.Key) || actual.Role != apitypes.VaultReader {
		t.Fatal("explicit legacy review did not deliver exact grant")
	}
	if recorder.calls != 1 {
		t.Fatalf("the recorder did not observe the legitimate wrap: %d", recorder.calls)
	}
	wrappedKey := requireValue[[]byte](t)(recipient.Keyset.UnwrapVaultKey(vault.ID, recipient.AccountID, recorder.last.WrappedKey))
	if !bytes.Equal(wrappedKey, vault.Key) {
		t.Fatal("recorded legitimate key delivery did not decrypt")
	}
}
