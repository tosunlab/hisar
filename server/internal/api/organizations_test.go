package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"
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

// apiCaller sends the organization requests, which the core client of this module does not have yet.
type apiCaller struct {
	t       *testing.T
	baseURL string
}

// call sends a JSON request as the account, fails the test unless the answer has wantStatus, and decodes the answer
// into output when it is not nil.
func (caller apiCaller) call(account *session.Session, method, path string, body any, wantStatus int, output any) {
	caller.t.Helper()
	status, responseBody := sendRawRequest(caller.t, caller.baseURL, account.Client.Token, method, path, body)
	if status != wantStatus {
		caller.t.Fatalf("%s %s as %s: %d %s, want %d", method, path, account.Email, status, responseBody, wantStatus)
	}
	if output != nil {
		if err := json.Unmarshal([]byte(responseBody), output); err != nil {
			caller.t.Fatalf("%s %s: %v in %s", method, path, err, responseBody)
		}
	}
}

func organizationPath(organizationID string, parts ...string) string {
	return strings.Join(append([]string{"/api/v1/organizations", organizationID}, parts...), "/")
}

func (caller apiCaller) createOrganization(admin *session.Session, name string) apitypes.OrganizationSummary {
	caller.t.Helper()
	var organization apitypes.OrganizationSummary
	caller.call(admin, "POST", "/api/v1/organizations", apitypes.CreateOrganizationRequest{Name: name}, http.StatusCreated, &organization)
	return organization
}

// joinOrganization runs the invitation flow: the admin invites the member's email, and the member accepts.
func (caller apiCaller) joinOrganization(admin, member *session.Session, organizationID, role string) {
	caller.t.Helper()
	caller.call(admin, "POST", organizationPath(organizationID, "invitations"), apitypes.OrganizationInvitationRequest{Email: member.Email, Role: role},
		http.StatusAccepted, nil)
	var organizations apitypes.OrganizationList
	caller.call(member, "GET", "/api/v1/organizations", nil, http.StatusOK, &organizations)
	invitationIndex := slices.IndexFunc(organizations.Invitations, func(invitation apitypes.ReceivedOrganizationInvitation) bool {
		return invitation.OrganizationID == organizationID
	})
	if invitationIndex < 0 {
		caller.t.Fatalf("%s received no invitation: %+v", member.Email, organizations)
	}
	caller.call(member, "POST", "/api/v1/organization-invitations/"+organizations.Invitations[invitationIndex].ID+"/accept", nil, http.StatusNoContent, nil)
}

func (caller apiCaller) createTeam(admin *session.Session, organizationID, name string, members ...*session.Session) apitypes.Team {
	caller.t.Helper()
	var team apitypes.Team
	caller.call(admin, "POST", organizationPath(organizationID, "teams"), apitypes.CreateTeamRequest{Name: name}, http.StatusCreated, &team)
	for _, member := range members {
		caller.call(admin, "PUT", organizationPath(organizationID, "teams", team.ID, "members", member.AccountID), nil, http.StatusNoContent, nil)
	}
	return team
}

func (caller apiCaller) listKeyRequests(account *session.Session) apitypes.VaultKeyRequestList {
	caller.t.Helper()
	var keyRequests apitypes.VaultKeyRequestList
	caller.call(account, "GET", "/api/v1/vault-key-requests", nil, http.StatusOK, &keyRequests)
	return keyRequests
}

// deliverVaultKeys does what an owner's app does for each key request of the vault: it checks the recipient's signed
// keys, wraps the vault key to them and sends it with its key generation. It returns the recipients.
func (caller apiCaller) deliverVaultKeys(owner *session.Session, vault *session.Vault) []string {
	caller.t.Helper()
	ctx := context.Background()
	var recipientIDs []string
	for _, keyRequest := range caller.listKeyRequests(owner).Pending {
		if keyRequest.VaultID != vault.ID {
			continue
		}
		if err := keyRequest.Recipient.PublicKeys.Verify(keyRequest.Recipient.AccountID); err != nil {
			caller.t.Fatalf("the key request of %s carries unsigned keys: %v", keyRequest.Recipient.Email, err)
		}
		wrappedKey := requireValue[[]byte](caller.t)(crypt.WrapVaultKey(keyRequest.Recipient.PublicKeys.KEM, vault.ID, keyRequest.Recipient.AccountID, vault.Key))
		request := &apitypes.SetAccessRequest{WrappedKey: wrappedKey, Role: keyRequest.Role, KeyGeneration: keyRequest.KeyGeneration}
		if err := owner.Client.SetAccess(ctx, vault.ID, keyRequest.Recipient.AccountID, request); err != nil {
			caller.t.Fatalf("delivering the key of %s to %s: %v", vault.Name, keyRequest.Recipient.Email, err)
		}
		recipientIDs = append(recipientIDs, keyRequest.Recipient.AccountID)
	}
	return recipientIDs
}

// createOrganizationVault creates a vault for the team as an app does: the key and the name are sealed on the device.
func createOrganizationVault(t *testing.T, creator *session.Session, organizationID, teamID, name string) *session.Vault {
	t.Helper()
	vault := &session.Vault{
		ID: uuid.NewString(), Type: apitypes.VaultOrganization, Name: name, Role: apitypes.VaultOwner, Key: crypt.GenerateKey(),
		OrganizationID: organizationID, KeyGeneration: 1,
	}
	encName := requireValue[[]byte](t)(crypt.SealVaultName(vault.Key, vault.ID, []byte(name)))
	wrappedKey := requireValue[[]byte](t)(crypt.WrapVaultKey(creator.Keyset.GetPublicKeys(creator.AccountID).KEM, vault.ID, creator.AccountID, vault.Key))
	request := &apitypes.CreateVaultRequest{
		ID: vault.ID, Type: apitypes.VaultOrganization, EncName: encName, WrappedKey: wrappedKey, OrganizationID: organizationID, TeamID: teamID,
	}
	if err := creator.Client.CreateVault(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	return vault
}

// findListedVault returns the vault as GET /vaults lists it to the account.
func findListedVault(t *testing.T, account *session.Session, vaultID string) (apitypes.Vault, bool) {
	t.Helper()
	vaults := requireValue[[]apitypes.Vault](t)(account.Client.ListVaults(context.Background()))
	index := slices.IndexFunc(vaults, func(vault apitypes.Vault) bool { return vault.ID == vaultID })
	if index < 0 {
		return apitypes.Vault{}, false
	}
	return vaults[index], true
}

func countDatabaseRows(t *testing.T, databaseURL, query string, arguments ...any) int {
	t.Helper()
	ctx := context.Background()
	connection := requireValue[*pgx.Conn](t)(pgx.Connect(ctx, databaseURL))
	defer connection.Close(ctx)
	var rowCount int
	if err := connection.QueryRow(ctx, query, arguments...).Scan(&rowCount); err != nil {
		t.Fatal(err)
	}
	return rowCount
}

func TestOrganizationInvitationAndKeyDelivery(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	server.config.EventHeartbeatInterval = 100 * time.Millisecond
	ctx := context.Background()
	recorder := attachMailRecorder(server)
	caller := apiCaller{t: t, baseURL: baseURL}
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", apitypes.RoleMember)
	bob := signUpTestAccount(t, ctx, server, baseURL, "bob@example.com", apitypes.RoleMember)
	bobEvents := openEventStream(t, bob.Client)
	waitForSubscriber(t, server, bob.AccountID, 1)

	acme := caller.createOrganization(alice, "  Acme  ")
	if acme.Name != "Acme" || acme.Role != apitypes.OrganizationRoleAdmin || acme.MemberCount != 1 || uuid.Validate(acme.ID) != nil {
		t.Fatalf("created organization: %+v", acme)
	}
	server.WaitForBackgroundWork()
	emailCountBefore := len(recorder.ListMessages())
	invitationPath := organizationPath(acme.ID, "invitations")
	existingStatus, existingBody := sendRawRequest(t, baseURL, alice.Client.Token, "POST", invitationPath,
		apitypes.OrganizationInvitationRequest{Email: "Bob@Example.com", Role: apitypes.OrganizationRoleMember})
	unknownStatus, unknownBody := sendRawRequest(t, baseURL, alice.Client.Token, "POST", invitationPath,
		apitypes.OrganizationInvitationRequest{Email: "ghost@example.com", Role: apitypes.OrganizationRoleMember})
	if existingStatus != http.StatusAccepted || unknownStatus != existingStatus || unknownBody != existingBody || !strings.Contains(existingBody, apitypes.InvitationSentMessage) {
		t.Fatalf("responses differ: %d %s / %d %s", existingStatus, existingBody, unknownStatus, unknownBody)
	}
	bobEvents.waitForEvent(t, apitypes.EventOrganizationsChanged)
	messages := recorder.WaitForMessageCount(emailCountBefore+2, 5*time.Second)[emailCountBefore:]
	toBob, isToBob := findEmail(messages, "bob@example.com")
	if !isToBob || toBob.Subject != "alice@example.com invited you to Acme in Hisar" || !strings.Contains(toBob.Body, "“Acme”") ||
		!strings.Contains(toBob.Body, "as a member") || !strings.Contains(toBob.Body, "hisar orgs") || strings.Contains(toBob.Body, "/invite/") {
		t.Errorf("email to Bob: %+v", toBob)
	}
	toGhost, isToGhost := findEmail(messages, "ghost@example.com")
	if !isToGhost || !strings.Contains(toGhost.Body, "https://hisar.test/invite/") || !strings.Contains(toGhost.Body, "hisar org accept") {
		t.Errorf("email to the unknown address: %+v", toGhost)
	}

	caller.call(bob, "GET", organizationPath(acme.ID), nil, http.StatusNotFound, nil)
	var bobList apitypes.OrganizationList
	caller.call(bob, "GET", "/api/v1/organizations", nil, http.StatusOK, &bobList)
	if len(bobList.Organizations) != 0 || len(bobList.Invitations) != 1 {
		t.Fatalf("Bob's organizations: %+v", bobList)
	}
	if invitation := bobList.Invitations[0]; invitation.OrganizationID != acme.ID || invitation.OrganizationName != "Acme" ||
		invitation.Role != apitypes.OrganizationRoleMember || invitation.InvitedByEmail != "alice@example.com" {
		t.Errorf("Bob's invitation: %+v", invitation)
	}
	acceptPath := "/api/v1/organization-invitations/" + bobList.Invitations[0].ID + "/accept"
	caller.call(bob, "POST", acceptPath, nil, http.StatusNoContent, nil)
	caller.call(bob, "POST", acceptPath, nil, http.StatusNotFound, nil)
	caller.call(bob, "GET", "/api/v1/organizations", nil, http.StatusOK, &bobList)
	if len(bobList.Invitations) != 0 || len(bobList.Organizations) != 1 || bobList.Organizations[0].MemberCount != 2 ||
		bobList.Organizations[0].Role != apitypes.OrganizationRoleMember {
		t.Fatalf("Bob's organizations after accepting: %+v", bobList)
	}

	platform := caller.createTeam(alice, acme.ID, "Platform", alice, bob)
	caller.call(alice, "POST", organizationPath(acme.ID, "teams"), apitypes.CreateTeamRequest{Name: "PLATFORM"}, http.StatusConflict, nil)
	production := createOrganizationVault(t, alice, acme.ID, platform.ID, "Production")
	requireValue[*item.Item](t)(alice.SaveItem(ctx, production, &item.Item{Overview: item.Overview{Title: "Database"}, Details: item.Details{Password: "db-pass"}}))

	aliceRequests := caller.listKeyRequests(alice)
	if len(aliceRequests.Pending) != 1 || len(aliceRequests.Waiting) != 0 {
		t.Fatalf("Alice's key requests: %+v", aliceRequests)
	}
	if pending := aliceRequests.Pending[0]; pending.VaultID != production.ID || pending.Recipient.AccountID != bob.AccountID ||
		pending.Role != apitypes.VaultOwner || pending.KeyGeneration != 1 || pending.Recipient.PublicKeys.Verify(bob.AccountID) != nil {
		t.Errorf("Alice's pending key request: %+v", pending)
	}
	bobRequests := caller.listKeyRequests(bob)
	if len(bobRequests.Pending) != 0 || len(bobRequests.Waiting) != 1 || bobRequests.Waiting[0] != (apitypes.WaitingVaultKey{
		VaultID: production.ID, OrganizationID: acme.ID, Role: apitypes.VaultOwner,
	}) {
		t.Errorf("Bob's key requests: %+v", bobRequests)
	}
	if _, err := bob.FindVault(ctx, production.ID); !errors.Is(err, session.ErrNotFound) {
		t.Errorf("Bob has the vault before an owner delivered its key: %v", err)
	}

	if delivered := caller.deliverVaultKeys(alice, production); !slices.Equal(delivered, []string{bob.AccountID}) {
		t.Fatalf("delivered to %v", delivered)
	}
	bobEvents.waitForEvent(t, apitypes.EventVaultsChanged)
	bobProduction := requireValue[*session.Vault](t)(bob.FindVault(ctx, production.ID))
	if items := requireValue[[]item.Item](t)(bob.ListItems(ctx, bobProduction)); bobProduction.Name != "Production" || len(items) != 1 || items[0].Details.Password != "db-pass" {
		t.Errorf("Bob's production vault: %+v %+v", bobProduction, items)
	}
	if listed, _ := findListedVault(t, bob, production.ID); listed.Type != apitypes.VaultOrganization || listed.OrganizationID != acme.ID ||
		listed.KeyGeneration != 1 || listed.NeedsRotation || listed.Role != apitypes.VaultOwner {
		t.Errorf("Bob's listed vault: %+v", listed)
	}
	if keyRequests := caller.listKeyRequests(alice); len(keyRequests.Pending) != 0 {
		t.Errorf("key requests after delivery: %+v", keyRequests)
	}

	var organization apitypes.Organization
	caller.call(alice, "GET", organizationPath(acme.ID), nil, http.StatusOK, &organization)
	if organization.Name != "Acme" || organization.Role != apitypes.OrganizationRoleAdmin || len(organization.Members) != 2 || len(organization.Teams) != 1 ||
		len(organization.Invitations) != 1 || organization.Invitations[0].Email != "ghost@example.com" {
		t.Fatalf("Acme as its admin sees it: %+v", organization)
	}
	if team := organization.Teams[0]; team.Name != "Platform" || !slices.Equal(team.MemberIDs, []string{alice.AccountID, bob.AccountID}) ||
		!slices.Equal(team.Vaults, []apitypes.TeamVault{{VaultID: production.ID, Role: apitypes.VaultOwner}}) {
		t.Errorf("Platform: %+v", team)
	}
	for _, member := range organization.Members {
		if member.PublicKeys.Verify(member.AccountID) != nil {
			t.Errorf("member %s without signed keys", member.Email)
		}
	}
	caller.call(bob, "GET", organizationPath(acme.ID), nil, http.StatusOK, &organization)
	if organization.Role != apitypes.OrganizationRoleMember || len(organization.Invitations) != 0 {
		t.Errorf("Acme as a member sees it: %+v", organization)
	}

	auditTypes := listAuditTypes(t, databaseURL)
	for _, auditType := range []string{"organization_created", "organization_invitation_sent", "organization_invitation_accepted", "team_created", "team_member_added", "vault_created"} {
		if !slices.Contains(auditTypes, auditType) {
			t.Errorf("the audit log lacks %s: %v", auditType, auditTypes)
		}
	}
	server.WaitForBackgroundWork()
}

func TestOrganizationTeamRemovalNeedsRotation(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	server.config.EventHeartbeatInterval = 100 * time.Millisecond
	ctx := context.Background()
	caller := apiCaller{t: t, baseURL: baseURL}
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", apitypes.RoleMember)
	bob := signUpTestAccount(t, ctx, server, baseURL, "bob@example.com", apitypes.RoleMember)
	carol := signUpTestAccount(t, ctx, server, baseURL, "carol@example.com", apitypes.RoleMember)
	acme := caller.createOrganization(alice, "Acme")
	caller.joinOrganization(alice, bob, acme.ID, apitypes.OrganizationRoleMember)
	caller.joinOrganization(alice, carol, acme.ID, apitypes.OrganizationRoleMember)
	platform := caller.createTeam(alice, acme.ID, "Platform", alice)
	developers := caller.createTeam(alice, acme.ID, "Developers", bob, carol)
	production := createOrganizationVault(t, alice, acme.ID, platform.ID, "Production")
	requireValue[*item.Item](t)(alice.SaveItem(ctx, production, &item.Item{Overview: item.Overview{Title: "Database"}, Details: item.Details{Password: "db-pass"}}))
	caller.call(alice, "PUT", organizationPath(acme.ID, "teams", developers.ID, "vaults", production.ID), apitypes.GrantTeamVaultRequest{Role: apitypes.VaultEditor},
		http.StatusNoContent, nil)
	if delivered := caller.deliverVaultKeys(alice, production); len(delivered) != 2 {
		t.Fatalf("delivered to %v", delivered)
	}
	if listed, _ := findListedVault(t, bob, production.ID); listed.Role != apitypes.VaultEditor {
		t.Fatalf("Bob's role: %+v", listed)
	}

	aliceEvents := openEventStream(t, alice.Client)
	waitForSubscriber(t, server, alice.AccountID, 1)
	caller.call(alice, "DELETE", organizationPath(acme.ID, "teams", developers.ID, "members", bob.AccountID), nil, http.StatusNoContent, nil)
	if _, isListed := findListedVault(t, bob, production.ID); isListed {
		t.Error("Bob still lists the vault after leaving the team")
	}
	if _, err := bob.Client.ListChanges(ctx, production.ID, 0, 0); !apiclient.IsStatus(err, http.StatusNotFound) {
		t.Errorf("Bob reads the vault after leaving the team: %v", err)
	}
	aliceEvents.waitForEvent(t, apitypes.EventVaultsChanged)
	if listed, _ := findListedVault(t, alice, production.ID); !listed.NeedsRotation || listed.KeyGeneration != 1 {
		t.Errorf("the vault after a removal: %+v", listed)
	}

	for name, grants := range map[string][]session.AccessGrant{
		"omitting Carol":        {{AccountID: alice.AccountID, Role: apitypes.VaultOwner}},
		"changing Carol's role": {{AccountID: alice.AccountID, Role: apitypes.VaultOwner}, {AccountID: carol.AccountID, Role: apitypes.VaultReader}},
	} {
		if err := alice.RotateVaultKey(ctx, production, grants); !apiclient.IsStatus(err, http.StatusBadRequest) {
			t.Errorf("rotation %s: %v", name, err)
		}
	}
	if err := alice.RotateVaultKey(ctx, production, []session.AccessGrant{
		{AccountID: alice.AccountID, Role: apitypes.VaultOwner}, {AccountID: carol.AccountID, Role: apitypes.VaultEditor},
	}); err != nil {
		t.Fatalf("rotation with every member: %v", err)
	}
	if listed, _ := findListedVault(t, alice, production.ID); listed.NeedsRotation || listed.KeyGeneration != 2 {
		t.Errorf("the vault after its rotation: %+v", listed)
	}
	carolProduction := requireValue[*session.Vault](t)(carol.FindVault(ctx, production.ID))
	if items := requireValue[[]item.Item](t)(carol.ListItems(ctx, carolProduction)); len(items) != 1 || items[0].Details.Password != "db-pass" {
		t.Errorf("Carol's items after the rotation: %+v", items)
	}
}

func TestOrganizationVaultKeepsAnOwner(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	ctx := context.Background()
	caller := apiCaller{t: t, baseURL: baseURL}
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", apitypes.RoleMember)
	bob := signUpTestAccount(t, ctx, server, baseURL, "bob@example.com", apitypes.RoleMember)
	acme := caller.createOrganization(alice, "Acme")
	caller.joinOrganization(alice, bob, acme.ID, apitypes.OrganizationRoleMember)
	platform := caller.createTeam(alice, acme.ID, "Platform", alice, bob)
	production := createOrganizationVault(t, alice, acme.ID, platform.ID, "Production")
	teamVaultPath := organizationPath(acme.ID, "teams", platform.ID, "vaults", production.ID)

	caller.call(alice, "DELETE", teamVaultPath, nil, http.StatusConflict, nil)
	caller.call(alice, "PUT", teamVaultPath, apitypes.GrantTeamVaultRequest{Role: apitypes.VaultEditor}, http.StatusConflict, nil)
	caller.call(alice, "DELETE", organizationPath(acme.ID, "teams", platform.ID), nil, http.StatusConflict, nil)
	status, body := sendRawRequest(t, baseURL, alice.Client.Token, "DELETE", organizationPath(acme.ID, "teams", platform.ID, "members", alice.AccountID), nil)
	if status != http.StatusConflict || !strings.Contains(body, "Nobody who can open this vault would be its owner") {
		t.Errorf("removing the only owner with a key, while Bob has none: %d %s", status, body)
	}
	if listed, isListed := findListedVault(t, alice, production.ID); !isListed || listed.Role != apitypes.VaultOwner || listed.NeedsRotation {
		t.Fatalf("a refused change touched Alice's access: %+v", listed)
	}
	var organization apitypes.Organization
	caller.call(alice, "GET", organizationPath(acme.ID), nil, http.StatusOK, &organization)
	if len(organization.Teams) != 1 || len(organization.Teams[0].MemberIDs) != 2 || len(organization.Teams[0].Vaults) != 1 {
		t.Fatalf("a refused change touched the team: %+v", organization.Teams)
	}

	caller.deliverVaultKeys(alice, production)
	caller.call(alice, "DELETE", organizationPath(acme.ID, "teams", platform.ID, "members", alice.AccountID), nil, http.StatusNoContent, nil)
	if _, isListed := findListedVault(t, alice, production.ID); isListed {
		t.Error("Alice still has the vault after leaving its only team")
	}
	if listed, _ := findListedVault(t, bob, production.ID); listed.Role != apitypes.VaultOwner || !listed.NeedsRotation {
		t.Errorf("Bob's vault after Alice left the team: %+v", listed)
	}
	if ownerCount := countDatabaseRows(t, databaseURL, `SELECT count(*) FROM vault_access WHERE vault_id = $1 AND role = 'owner'`, production.ID); ownerCount != 1 {
		t.Errorf("owners: %d", ownerCount)
	}
}

func TestOrganizationVaultAccessRules(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	caller := apiCaller{t: t, baseURL: baseURL}
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", apitypes.RoleMember)
	bob := signUpTestAccount(t, ctx, server, baseURL, "bob@example.com", apitypes.RoleMember)
	carol := signUpTestAccount(t, ctx, server, baseURL, "carol@example.com", apitypes.RoleMember)
	dave := signUpTestAccount(t, ctx, server, baseURL, "dave@example.com", apitypes.RoleMember)
	acme := caller.createOrganization(alice, "Acme")
	for _, member := range []*session.Session{bob, carol, dave} {
		caller.joinOrganization(alice, member, acme.ID, apitypes.OrganizationRoleMember)
	}
	platform := caller.createTeam(alice, acme.ID, "Platform", alice, bob)
	developers := caller.createTeam(alice, acme.ID, "Developers", carol)
	production := createOrganizationVault(t, alice, acme.ID, platform.ID, "Production")
	caller.call(alice, "PUT", organizationPath(acme.ID, "teams", developers.ID, "vaults", production.ID), apitypes.GrantTeamVaultRequest{Role: apitypes.VaultEditor},
		http.StatusNoContent, nil)

	wrapFor := func(recipient *session.Session) []byte {
		t.Helper()
		kemKey := recipient.Keyset.GetPublicKeys(recipient.AccountID).KEM
		return requireValue[[]byte](t)(crypt.WrapVaultKey(kemKey, production.ID, recipient.AccountID, production.Key))
	}
	setAccess := func(recipient *session.Session, role string, keyGeneration int64) error {
		return alice.Client.SetAccess(ctx, production.ID, recipient.AccountID,
			&apitypes.SetAccessRequest{WrappedKey: wrapFor(recipient), Role: role, KeyGeneration: keyGeneration})
	}
	expectStatus(t, "a key for a member in no team of the vault", setAccess(dave, apitypes.VaultReader, 1), http.StatusConflict)
	expectStatus(t, "a key with another role than the teams give", setAccess(carol, apitypes.VaultOwner, 1), http.StatusConflict)
	expectStatus(t, "a key without its generation", setAccess(carol, apitypes.VaultEditor, 0), http.StatusConflict)
	if err := setAccess(carol, apitypes.VaultEditor, 1); err != nil {
		t.Fatalf("a key with the entitled role: %v", err)
	}
	expectStatus(t, "changing a role the teams give", setAccess(carol, apitypes.VaultReader, 1), http.StatusBadRequest)
	if err := setAccess(carol, apitypes.VaultEditor, 1); err != nil {
		t.Errorf("a new key with the same role: %v", err)
	}

	if err := alice.RotateVaultKey(ctx, production, []session.AccessGrant{
		{AccountID: alice.AccountID, Role: apitypes.VaultOwner}, {AccountID: carol.AccountID, Role: apitypes.VaultEditor},
	}); err != nil {
		t.Fatal(err)
	}
	expectStatus(t, "a key wrapped before the rotation", setAccess(bob, apitypes.VaultOwner, 1), http.StatusConflict)
	if err := setAccess(bob, apitypes.VaultOwner, 2); err != nil {
		t.Errorf("a key of the current generation: %v", err)
	}
	personal := findPersonalVault(t, alice)
	if err := alice.Client.SetAccess(ctx, personal.ID, bob.AccountID, &apitypes.SetAccessRequest{WrappedKey: []byte{1}, Role: apitypes.VaultReader, KeyGeneration: 7}); !apiclient.IsStatus(err, http.StatusConflict) ||
		!strings.Contains(err.Error(), "vault key changed") {
		t.Errorf("a stale generation for a personal vault: %v", err)
	}

	expectStatus(t, "an owner removes someone from an organization vault", alice.Client.RemoveAccess(ctx, production.ID, carol.AccountID), http.StatusBadRequest)
	expectStatus(t, "a member leaves an organization vault", carol.Client.RemoveAccess(ctx, production.ID, carol.AccountID), http.StatusBadRequest)
	expectStatus(t, "a share invitation to an organization vault", alice.InviteToVault(ctx, production, "erin@example.com", apitypes.VaultReader), http.StatusBadRequest)
	if _, isListed := findListedVault(t, carol, production.ID); !isListed {
		t.Error("a refused removal took Carol's access")
	}
}

func TestOrganizationVaultRotationFromAStaleKey(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	caller := apiCaller{t: t, baseURL: baseURL}
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", apitypes.RoleMember)
	bob := signUpTestAccount(t, ctx, server, baseURL, "bob@example.com", apitypes.RoleMember)
	acme := caller.createOrganization(alice, "Acme")
	caller.joinOrganization(alice, bob, acme.ID, apitypes.OrganizationRoleMember)
	platform := caller.createTeam(alice, acme.ID, "Platform", alice, bob)
	production := createOrganizationVault(t, alice, acme.ID, platform.ID, "Production")
	caller.deliverVaultKeys(alice, production)
	bobProduction := requireValue[*session.Vault](t)(bob.FindVault(ctx, production.ID))
	if bobProduction.KeyGeneration != 1 {
		t.Fatalf("Bob's copy: generation %d", bobProduction.KeyGeneration)
	}
	grants := []session.AccessGrant{{AccountID: alice.AccountID, Role: apitypes.VaultOwner}, {AccountID: bob.AccountID, Role: apitypes.VaultOwner}}

	withoutGeneration := *production
	withoutGeneration.KeyGeneration = 0
	expectStatus(t, "a rotation without the key generation", alice.RotateVaultKey(ctx, &withoutGeneration, grants), http.StatusConflict)
	if err := alice.RotateVaultKey(ctx, production, grants); err != nil {
		t.Fatalf("the first rotation: %v", err)
	}
	expectStatus(t, "a second rotation from the same key", bob.RotateVaultKey(ctx, bobProduction, grants), http.StatusConflict)

	if listed, _ := findListedVault(t, alice, production.ID); listed.KeyGeneration != 2 {
		t.Errorf("the vault after the rotations: %+v", listed)
	}
	for _, member := range []*session.Session{alice, bob} {
		stored := requireValue[*session.Vault](t)(member.FindVault(ctx, production.ID))
		if !bytes.Equal(stored.Key, production.Key) || stored.KeyGeneration != 2 {
			t.Errorf("%s's stored key is not the first rotation's (generation %d)", member.Email, stored.KeyGeneration)
		}
	}
}

func TestRenameWithARotatedAwayKey(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	ctx := context.Background()
	caller := apiCaller{t: t, baseURL: baseURL}
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", apitypes.RoleMember)
	bob := signUpTestAccount(t, ctx, server, baseURL, "bob@example.com", apitypes.RoleMember)
	acme := caller.createOrganization(alice, "Acme")
	caller.joinOrganization(alice, bob, acme.ID, apitypes.OrganizationRoleMember)
	platform := caller.createTeam(alice, acme.ID, "Platform", alice, bob)
	production := createOrganizationVault(t, alice, acme.ID, platform.ID, "Production")
	caller.deliverVaultKeys(alice, production)
	bobProduction := requireValue[*session.Vault](t)(bob.FindVault(ctx, production.ID))
	grants := []session.AccessGrant{{AccountID: alice.AccountID, Role: apitypes.VaultOwner}, {AccountID: bob.AccountID, Role: apitypes.VaultOwner}}
	if err := alice.RotateVaultKey(ctx, production, grants); err != nil {
		t.Fatal(err)
	}
	rotated, _ := findListedVault(t, alice, production.ID)
	if rotated.KeyGeneration != 2 || production.KeyGeneration != 2 || bobProduction.KeyGeneration != 1 {
		t.Fatalf("generations after the rotation: listed %d, Alice's %d, Bob's %d", rotated.KeyGeneration, production.KeyGeneration, bobProduction.KeyGeneration)
	}

	expectStatus(t, "a rename sealed with the replaced key", bob.RenameVault(ctx, bobProduction, "Stale"), http.StatusConflict)
	staleName := requireValue[[]byte](t)(crypt.SealVaultName(bobProduction.Key, production.ID, []byte("Stale")))
	expectStatus(t, "a rename of an organization vault without its key generation",
		bob.Client.RenameVault(ctx, production.ID, &apitypes.RenameVaultRequest{EncName: staleName}), http.StatusConflict)
	if countDatabaseRows(t, databaseURL, `SELECT count(*) FROM vaults WHERE id = $1 AND enc_name = $2`, production.ID, rotated.EncName) != 1 {
		t.Error("a refused rename replaced the stored name")
	}
	if err := alice.RenameVault(ctx, production, "Production 2026"); err != nil {
		t.Fatalf("a rename with the current key: %v", err)
	}
	if listed, _ := findTestVault(t, bob, production.ID); listed == nil || listed.Name != "Production 2026" {
		t.Errorf("Bob's vault list after the rename: %+v", listed)
	}

	family := requireValue[*session.Vault](t)(alice.CreateVault(ctx, "Family", apitypes.VaultShared))
	familyName := requireValue[[]byte](t)(crypt.SealVaultName(family.Key, family.ID, []byte("Home")))
	expectStatus(t, "a shared vault renamed with another key generation",
		alice.Client.RenameVault(ctx, family.ID, &apitypes.RenameVaultRequest{EncName: familyName, KeyGeneration: 7}), http.StatusConflict)
	if err := alice.Client.RenameVault(ctx, family.ID, &apitypes.RenameVaultRequest{EncName: familyName}); err != nil {
		t.Errorf("a shared vault renamed without a key generation: %v", err)
	}
}

func TestOrganizationRights(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	caller := apiCaller{t: t, baseURL: baseURL}
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", apitypes.RoleMember)
	bob := signUpTestAccount(t, ctx, server, baseURL, "bob@example.com", apitypes.RoleMember)
	carol := signUpTestAccount(t, ctx, server, baseURL, "carol@example.com", apitypes.RoleMember)
	acme := caller.createOrganization(alice, "Acme")
	caller.joinOrganization(alice, bob, acme.ID, apitypes.OrganizationRoleMember)
	platform := caller.createTeam(alice, acme.ID, "Platform", alice)
	production := createOrganizationVault(t, alice, acme.ID, platform.ID, "Production")
	caller.call(alice, "POST", organizationPath(acme.ID, "invitations"), apitypes.OrganizationInvitationRequest{Email: "erin@example.com", Role: apitypes.OrganizationRoleMember},
		http.StatusAccepted, nil)
	var organization apitypes.Organization
	caller.call(alice, "GET", organizationPath(acme.ID), nil, http.StatusOK, &organization)
	invitationID := organization.Invitations[0].ID

	adminRequests := []struct {
		method, path string
		body         any
	}{
		{"PUT", organizationPath(acme.ID, "name"), apitypes.RenameOrganizationRequest{Name: "Evil"}},
		{"DELETE", organizationPath(acme.ID), nil},
		{"POST", organizationPath(acme.ID, "invitations"), apitypes.OrganizationInvitationRequest{Email: "mallory@example.com", Role: apitypes.OrganizationRoleAdmin}},
		{"DELETE", organizationPath(acme.ID, "invitations", invitationID), nil},
		{"PUT", organizationPath(acme.ID, "members", alice.AccountID), apitypes.SetOrganizationRoleRequest{Role: apitypes.OrganizationRoleMember}},
		{"DELETE", organizationPath(acme.ID, "members", alice.AccountID), nil},
		{"POST", organizationPath(acme.ID, "teams"), apitypes.CreateTeamRequest{Name: "Mine"}},
		{"PUT", organizationPath(acme.ID, "teams", platform.ID, "name"), apitypes.RenameTeamRequest{Name: "Mine"}},
		{"DELETE", organizationPath(acme.ID, "teams", platform.ID), nil},
		{"PUT", organizationPath(acme.ID, "teams", platform.ID, "members", bob.AccountID), nil},
		{"DELETE", organizationPath(acme.ID, "teams", platform.ID, "members", alice.AccountID), nil},
		{"PUT", organizationPath(acme.ID, "teams", platform.ID, "vaults", production.ID), apitypes.GrantTeamVaultRequest{Role: apitypes.VaultReader}},
		{"DELETE", organizationPath(acme.ID, "teams", platform.ID, "vaults", production.ID), nil},
	}
	for _, request := range adminRequests {
		caller.call(bob, request.method, request.path, request.body, http.StatusForbidden, nil)
		caller.call(carol, request.method, request.path, request.body, http.StatusNotFound, nil)
	}
	caller.call(carol, "GET", organizationPath(acme.ID), nil, http.StatusNotFound, nil)
	caller.call(alice, "GET", organizationPath("not-an-id"), nil, http.StatusNotFound, nil)
	caller.call(alice, "GET", organizationPath(uuid.NewString()), nil, http.StatusNotFound, nil)
	caller.call(bob, "GET", organizationPath(acme.ID), nil, http.StatusOK, nil)

	invitationPath := organizationPath(acme.ID, "invitations")
	caller.call(alice, "POST", invitationPath, apitypes.OrganizationInvitationRequest{Email: "Alice@example.com", Role: apitypes.OrganizationRoleMember}, http.StatusBadRequest, nil)
	caller.call(alice, "POST", invitationPath, apitypes.OrganizationInvitationRequest{Email: "bob@example.com", Role: apitypes.OrganizationRoleAdmin}, http.StatusConflict, nil)
	caller.call(alice, "POST", invitationPath, apitypes.OrganizationInvitationRequest{Email: "carol@example.com", Role: apitypes.VaultOwner}, http.StatusBadRequest, nil)
	caller.call(alice, "POST", invitationPath, apitypes.OrganizationInvitationRequest{Email: "not an email", Role: apitypes.OrganizationRoleMember}, http.StatusBadRequest, nil)
	for _, name := range []string{"", "   ", strings.Repeat("a", apitypes.MaxOrganizationNameLength+1), "Line\nbreak"} {
		caller.call(alice, "POST", "/api/v1/organizations", apitypes.CreateOrganizationRequest{Name: name}, http.StatusBadRequest, nil)
	}

	createVault := func(creator *session.Session, request apitypes.CreateVaultRequest) error {
		request.ID, request.EncName, request.WrappedKey = uuid.NewString(), []byte{1}, []byte{1}
		return creator.Client.CreateVault(ctx, &request)
	}
	globex := caller.createOrganization(carol, "Globex")
	globexTeam := caller.createTeam(carol, globex.ID, "Ops", carol)
	expectStatus(t, "a vault for a team the creator is not in", createVault(bob, apitypes.CreateVaultRequest{Type: apitypes.VaultOrganization, OrganizationID: acme.ID, TeamID: platform.ID}), http.StatusForbidden)
	expectStatus(t, "a vault in an organization the creator is not in", createVault(carol, apitypes.CreateVaultRequest{Type: apitypes.VaultOrganization, OrganizationID: acme.ID, TeamID: platform.ID}), http.StatusNotFound)
	expectStatus(t, "a vault for another organization's team", createVault(alice, apitypes.CreateVaultRequest{Type: apitypes.VaultOrganization, OrganizationID: acme.ID, TeamID: globexTeam.ID}), http.StatusNotFound)
	expectStatus(t, "an organization vault without a team", createVault(alice, apitypes.CreateVaultRequest{Type: apitypes.VaultOrganization, OrganizationID: acme.ID}), http.StatusBadRequest)
	expectStatus(t, "a shared vault in an organization", createVault(alice, apitypes.CreateVaultRequest{Type: apitypes.VaultShared, OrganizationID: acme.ID, TeamID: platform.ID}), http.StatusBadRequest)
	caller.call(alice, "PUT", organizationPath(acme.ID, "teams", platform.ID, "vaults", findPersonalVault(t, alice).ID), apitypes.GrantTeamVaultRequest{Role: apitypes.VaultReader},
		http.StatusNotFound, nil)
	caller.call(alice, "PUT", organizationPath(acme.ID, "teams", platform.ID, "members", carol.AccountID), nil, http.StatusNotFound, nil)
}

func TestOrganizationAdminRules(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	caller := apiCaller{t: t, baseURL: baseURL}
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", apitypes.RoleMember)
	bob := signUpTestAccount(t, ctx, server, baseURL, "bob@example.com", apitypes.RoleMember)
	acme := caller.createOrganization(alice, "Acme")
	caller.joinOrganization(alice, bob, acme.ID, apitypes.OrganizationRoleMember)
	alicePath, bobPath := organizationPath(acme.ID, "members", alice.AccountID), organizationPath(acme.ID, "members", bob.AccountID)
	toMember := apitypes.SetOrganizationRoleRequest{Role: apitypes.OrganizationRoleMember}

	caller.call(alice, "PUT", alicePath, toMember, http.StatusBadRequest, nil)
	caller.call(alice, "DELETE", alicePath, nil, http.StatusBadRequest, nil)
	caller.call(alice, "PUT", bobPath, apitypes.SetOrganizationRoleRequest{Role: apitypes.OrganizationRoleAdmin}, http.StatusNoContent, nil)
	caller.call(alice, "PUT", alicePath, toMember, http.StatusNoContent, nil)
	caller.call(bob, "PUT", bobPath, toMember, http.StatusBadRequest, nil)
	caller.call(alice, "DELETE", alicePath, nil, http.StatusNoContent, nil)
	caller.call(bob, "DELETE", bobPath, nil, http.StatusBadRequest, nil)

	ops := caller.createTeam(bob, acme.ID, "Ops", bob)
	scratch := createOrganizationVault(t, bob, acme.ID, ops.ID, "Scratch")
	caller.call(bob, "DELETE", organizationPath(acme.ID), nil, http.StatusBadRequest, nil)
	if _, err := bob.DeleteVault(ctx, scratch, session.VaultDeletion{IsDeletingItems: true}); err != nil {
		t.Fatal(err)
	}
	caller.call(bob, "DELETE", organizationPath(acme.ID), nil, http.StatusNoContent, nil)
	var organizations apitypes.OrganizationList
	caller.call(bob, "GET", "/api/v1/organizations", nil, http.StatusOK, &organizations)
	if len(organizations.Organizations) != 0 {
		t.Errorf("organizations after deleting Acme: %+v", organizations)
	}
}

func TestDeleteAccountInOrganizations(t *testing.T) {
	baseURL, server, databaseURL, _ := setupLifecycleServer(t)
	server.config.EventHeartbeatInterval = 100 * time.Millisecond
	ctx := context.Background()
	caller := apiCaller{t: t, baseURL: baseURL}
	alice := signUpLifecycleAccount(t, server, baseURL, "alice@example.com")
	carol := signUpLifecycleAccount(t, server, baseURL, "carol@example.com").session
	bob := signUpLifecycleAccount(t, server, baseURL, "bob@example.com").session
	acme := caller.createOrganization(alice.session, "Acme")
	caller.joinOrganization(alice.session, carol, acme.ID, apitypes.OrganizationRoleMember)
	caller.joinOrganization(alice.session, bob, acme.ID, apitypes.OrganizationRoleMember)
	owners := caller.createTeam(alice.session, acme.ID, "Owners", alice.session)
	platform := caller.createTeam(alice.session, acme.ID, "Platform", carol)
	production := createOrganizationVault(t, alice.session, acme.ID, owners.ID, "Production")
	caller.call(alice.session, "PUT", organizationPath(acme.ID, "teams", platform.ID, "vaults", production.ID), apitypes.GrantTeamVaultRequest{Role: apitypes.VaultEditor},
		http.StatusNoContent, nil)
	if delivered := caller.deliverVaultKeys(alice.session, production); !slices.Equal(delivered, []string{carol.AccountID}) {
		t.Fatalf("delivered to %v", delivered)
	}
	// Alice and Carol own Staging through Leads, so it outlives Alice; Carol's shared Family has Alice as an editor.
	leads := caller.createTeam(alice.session, acme.ID, "Leads", alice.session, carol)
	staging := createOrganizationVault(t, alice.session, acme.ID, leads.ID, "Staging")
	if delivered := caller.deliverVaultKeys(alice.session, staging); !slices.Equal(delivered, []string{carol.AccountID}) {
		t.Fatalf("delivered Staging to %v", delivered)
	}
	family := requireValue[*session.Vault](t)(carol.CreateVault(ctx, "Family", apitypes.VaultShared))
	shareThroughInvitation(t, carol, alice.session, family, apitypes.VaultEditor)
	caller.call(alice.session, "POST", organizationPath(acme.ID, "invitations"), apitypes.OrganizationInvitationRequest{Email: "erin@example.com", Role: apitypes.OrganizationRoleMember},
		http.StatusAccepted, nil)
	solo := caller.createOrganization(alice.session, "Solo")
	soloTeam := caller.createTeam(alice.session, solo.ID, "Me", alice.session)
	createOrganizationVault(t, alice.session, solo.ID, soloTeam.ID, "Solo vault")
	dave := signUpLifecycleAccount(t, server, baseURL, "dave@example.com").session
	initech := caller.createOrganization(dave, "Initech")
	caller.call(dave, "POST", organizationPath(initech.ID, "invitations"), apitypes.OrganizationInvitationRequest{Email: "alice@example.com", Role: apitypes.OrganizationRoleMember},
		http.StatusAccepted, nil)

	preview := requireValue[*apitypes.AccountDeletionPreview](t)(alice.session.Client.GetAccountDeletionPreview(ctx))
	slices.SortFunc(preview.Organizations, func(first, second apitypes.DeletionOrganization) int { return strings.Compare(first.Name, second.Name) })
	if !slices.Equal(preview.Organizations, []apitypes.DeletionOrganization{
		{OrganizationID: acme.ID, Name: "Acme", NewAdminName: "carol"},
		{OrganizationID: solo.ID, Name: "Solo", IsDeleted: true},
	}) {
		t.Errorf("organizations in the preview: %+v", preview.Organizations)
	}
	if preview.SentInvitationCount != 1 {
		t.Errorf("sent invitations in the preview: %d", preview.SentInvitationCount)
	}

	server.WaitForBackgroundWork()
	carolEvents := openEventStream(t, carol.Client)
	waitForSubscriber(t, server, carol.AccountID, 1)
	daveEvents := openEventStream(t, dave.Client)
	waitForSubscriber(t, server, dave.AccountID, 1)
	if err := alice.session.DeleteAccount(ctx, alice.password, alice.secretKey, "alice@example.com"); err != nil {
		t.Fatal(err)
	}
	carolEvents.waitForEvent(t, apitypes.EventOrganizationsChanged)
	// Dave only invited Alice, so the invitation he sees under Initech is all that changed for him.
	daveEvents.waitForEvent(t, apitypes.EventOrganizationsChanged)
	var organization apitypes.Organization
	caller.call(carol, "GET", organizationPath(acme.ID), nil, http.StatusOK, &organization)
	memberRoles := map[string]string{}
	for _, member := range organization.Members {
		memberRoles[member.Email] = member.Role
	}
	teamMemberIDs := map[string][]string{}
	for _, team := range organization.Teams {
		teamMemberIDs[team.Name] = team.MemberIDs
		if slices.ContainsFunc(team.Vaults, func(teamVault apitypes.TeamVault) bool { return teamVault.VaultID == production.ID }) {
			t.Errorf("%s keeps the deleted vault: %+v", team.Name, team.Vaults)
		}
	}
	if organization.Role != apitypes.OrganizationRoleAdmin || len(memberRoles) != 2 || memberRoles["bob@example.com"] != apitypes.OrganizationRoleMember ||
		len(organization.Invitations) != 0 || len(teamMemberIDs["Owners"]) != 0 || !slices.Equal(teamMemberIDs["Platform"], []string{carol.AccountID}) ||
		!slices.Equal(teamMemberIDs["Leads"], []string{carol.AccountID}) {
		t.Errorf("Acme after Alice's deletion: %+v", organization)
	}
	if _, isListed := findListedVault(t, carol, production.ID); isListed {
		t.Error("the vault Alice owned alone outlived her")
	}
	// Alice held Staging's key, so its remaining owner's app rotates it, as after a team removal; Family is not marked (§5.7.1).
	if listedStaging, isListed := findListedVault(t, carol, staging.ID); !isListed || !listedStaging.NeedsRotation {
		t.Errorf("the organization vault Alice had a key to: listed %v, needs rotation %v", isListed, listedStaging.NeedsRotation)
	}
	if listedFamily, isListed := findListedVault(t, carol, family.ID); !isListed || listedFamily.NeedsRotation {
		t.Errorf("the shared vault Alice was in: listed %v, needs rotation %v", isListed, listedFamily.NeedsRotation)
	}
	for name, query := range map[string]string{
		"the organization Alice was alone in": `SELECT count(*) FROM organizations WHERE id = '` + solo.ID + `'`,
		"Alice's organization invitations":    `SELECT count(*) FROM organization_invitations WHERE email = 'erin@example.com'`,
		"the invitation to Alice":             `SELECT count(*) FROM organization_invitations WHERE organization_id = '` + initech.ID + `'`,
		"Alice's memberships":                 `SELECT count(*) FROM organization_members WHERE account_id = '` + alice.session.AccountID + `'`,
	} {
		if rowCount := countDatabaseRows(t, databaseURL, query); rowCount != 0 {
			t.Errorf("%s: %d rows left", name, rowCount)
		}
	}
}

func TestConcurrentTeamRemovalsKeepAnOwner(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	ctx := context.Background()
	caller := apiCaller{t: t, baseURL: baseURL}
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", apitypes.RoleMember)
	bob := signUpTestAccount(t, ctx, server, baseURL, "bob@example.com", apitypes.RoleMember)
	acme := caller.createOrganization(alice, "Acme")
	caller.joinOrganization(alice, bob, acme.ID, apitypes.OrganizationRoleMember)
	platform := caller.createTeam(alice, acme.ID, "Platform", alice, bob)
	production := createOrganizationVault(t, alice, acme.ID, platform.ID, "Production")
	caller.deliverVaultKeys(alice, production)

	start := make(chan struct{})
	statuses := make([]int, 2)
	errs := make([]error, 2)
	var waitGroup sync.WaitGroup
	for index, member := range []*session.Session{alice, bob} {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			request, err := http.NewRequest("DELETE", baseURL+organizationPath(acme.ID, "teams", platform.ID, "members", member.AccountID), bytes.NewReader(nil))
			if err != nil {
				errs[index] = err
				return
			}
			request.Header.Set("Authorization", "Bearer "+alice.Client.Token)
			<-start
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				errs[index] = err
				return
			}
			response.Body.Close()
			statuses[index] = response.StatusCode
		}()
	}
	close(start)
	waitGroup.Wait()
	if err := errors.Join(errs...); err != nil {
		t.Fatal(err)
	}
	slices.Sort(statuses)
	if !slices.Equal(statuses, []int{http.StatusNoContent, http.StatusConflict}) {
		t.Errorf("two removals of the vault's two owners: %v", statuses)
	}
	if ownerCount := countDatabaseRows(t, databaseURL, `SELECT count(*) FROM vault_access WHERE vault_id = $1 AND role = 'owner'`, production.ID); ownerCount != 1 {
		t.Errorf("owners after the race: %d", ownerCount)
	}
	if memberCount := countDatabaseRows(t, databaseURL, `SELECT count(*) FROM team_members WHERE team_id = $1`, platform.ID); memberCount != 1 {
		t.Errorf("team members after the race: %d", memberCount)
	}
}
