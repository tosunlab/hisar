package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/cliprotocol"
)

func answerOrganizationRequest(request cliprotocol.Request) (any, *cliprotocol.ResponseError) {
	switch request.Type {
	case cliprotocol.TypeListOrganization:
		return cliprotocol.OrganizationListResult{
			Organizations: []cliprotocol.OrganizationSummary{
				{ID: "acme-id", Name: "Acme", Role: "admin", MemberCount: 2},
				{ID: "twin-1", Name: "Twin", Role: "member", MemberCount: 4},
				{ID: "twin-2", Name: "twin", Role: "member", MemberCount: 5},
			},
			Invitations: []cliprotocol.OrganizationInvitation{{
				ID: "f0e1d2c3-0000-4000-8000-000000000003", OrganizationID: "globex-id", OrganizationName: "Globex", Role: "member",
				InvitedByName: "Hank", InvitedByEmail: "hank@example.com", ExpiresAt: "2026-10-20T09:00:00Z",
			}},
		}, nil
	case cliprotocol.TypeGetOrganization:
		return cliprotocol.OrganizationResult{Organization: cliprotocol.Organization{
			ID: request.OrganizationID, Name: "Acme", Role: "admin",
			Members: []cliprotocol.OrganizationMember{
				{AccountID: "ABC123", Name: "Alice", Email: "alice@example.com", Role: "admin", SecurityCode: "3F2A 91C0 7B44 E1D8", IsCurrentUser: true},
				{AccountID: "BOB111", Name: "Bob", Email: "bob@example.com", Role: "member", SecurityCode: "0000 1111 2222 3333", IsKeyChanged: true},
			},
			Teams: []cliprotocol.Team{{
				ID: "eng-id", Name: "Eng",
				Members: []cliprotocol.TeamMember{{AccountID: "ABC123", Name: "Alice", Email: "alice@example.com"}, {AccountID: "BOB111", Name: "Bob", Email: "bob@example.com"}},
				Vaults:  []cliprotocol.TeamVault{{VaultID: "eng-vault-id", Name: "EngVault", Role: "owner", CanOpen: true}, {VaultID: "locked-vault-id", Role: "reader"}},
			}},
			Invitations: []cliprotocol.SentOrganizationInvitation{{ID: "sent-1", Email: "carol@example.com", Role: "member", ExpiresAt: "2026-10-20T09:00:00Z"}},
		}}, nil
	case cliprotocol.TypeCompleteOrganizationKeys:
		return cliprotocol.OrganizationKeysResult{
			DeliveredCount: 1, RotatedCount: 1, DeliveryError: "dave@example.com: their signing key changed",
			Waiting: []cliprotocol.WaitingVaultKey{
				{VaultID: "v1", OrganizationID: "acme-id", OrganizationName: "Acme", Role: "editor"},
				{VaultID: "v2", OrganizationID: "acme-id", OrganizationName: "Acme", Role: "reader"},
				{VaultID: "v3", OrganizationID: "globex-id", OrganizationName: "Globex", Role: "reader"},
			},
		}, nil
	case cliprotocol.TypeCreateOrganization:
		return cliprotocol.OrganizationSummaryResult{Organization: cliprotocol.OrganizationSummary{ID: "new-id", Name: request.Name, Role: "admin", MemberCount: 1}}, nil
	case cliprotocol.TypeCreateTeam:
		return cliprotocol.TeamResult{Team: cliprotocol.Team{ID: "team-id", Name: request.Name}}, nil
	case cliprotocol.TypeCreateOrganizationVault:
		return cliprotocol.VaultResult{Vault: cliprotocol.Vault{ID: "new-vault-id", Name: request.Name, Type: "organization", Role: "owner", OrganizationID: request.OrganizationID}}, nil
	case cliprotocol.TypeListVault:
		return cliprotocol.VaultListResult{Vaults: []cliprotocol.Vault{
			{ID: "personal-id", Name: "Personal", Type: "personal", Role: "owner"},
			{ID: "eng-vault-id", Name: "EngVault", Type: "organization", Role: "owner", OrganizationID: "acme-id"},
			{ID: "other-id", Name: "Other", Type: "organization", Role: "owner", OrganizationID: "twin-1"},
		}}, nil
	case cliprotocol.TypeRenameOrganization, cliprotocol.TypeDeleteOrganization, cliprotocol.TypeInviteToOrganization, cliprotocol.TypeCancelOrganizationInvitation,
		cliprotocol.TypeAcceptOrganizationInvitation, cliprotocol.TypeDeclineOrganizationInvitation, cliprotocol.TypeSetOrganizationRole,
		cliprotocol.TypeRemoveOrganizationMember, cliprotocol.TypeLeaveOrganization, cliprotocol.TypeRenameTeam, cliprotocol.TypeDeleteTeam,
		cliprotocol.TypeAddTeamMember, cliprotocol.TypeRemoveTeamMember, cliprotocol.TypeGiveVaultToTeam, cliprotocol.TypeTakeVaultFromTeam:
		return struct{}{}, nil
	}
	return answerFakeVaultRequest(request)
}

func startOrganizationApp(t *testing.T) *fakeApp {
	t.Helper()
	app := startFakeApp(t)
	app.mutex.Lock()
	app.answer = answerOrganizationRequest
	app.mutex.Unlock()
	return app
}

func TestOrganizationAndTeamNamesResolve(t *testing.T) {
	app := startOrganizationApp(t)
	if _, _, err := runCapturingOutput(t, false, "", "org", "rename", "ACME", "Acme Inc"); err != nil {
		t.Fatalf("rename by a name in another case: %v", err)
	}
	if renames := app.findRequest(cliprotocol.TypeRenameOrganization); len(renames) != 1 || renames[0].OrganizationID != "acme-id" || renames[0].Name != "Acme Inc" {
		t.Errorf("rename requests: %+v", renames)
	}
	if _, _, err := runCapturingOutput(t, false, "", "org", "rename", "twin-2", "Twin Two"); err != nil {
		t.Errorf("an ID picks one of two organizations with the same name: %v", err)
	}
	cases := map[string][]string{
		`2 organizations are named "Twin"; use an ID instead: twin-1, twin-2`: {"org", "show", "Twin"},
		`organization "Initech": not found; you are in Acme, Twin, twin`:      {"org", "leave", "Initech", "-y"},
		`team "Ops": not found in Acme; its teams are Eng`:                    {"team", "add", "Acme", "Ops", "bob@example.com"},
		"zed@example.com is not a member of Acme":                             {"team", "add", "Acme", "eng", "zed@example.com"},
	}
	for expected, args := range cases {
		if _, _, err := runCapturingOutput(t, false, "", args...); err == nil || !strings.Contains(err.Error(), expected) {
			t.Errorf("%v: %v, want %q", args, err, expected)
		}
	}
	if _, stderr, err := runCapturingOutput(t, false, "", "team", "add", "acme", "ENG", "BOB@example.com"); err != nil || !strings.Contains(stderr, `Added bob@example.com to team "Eng".`) {
		t.Fatalf("team add: %v %s", err, stderr)
	}
	if additions := app.findRequest(cliprotocol.TypeAddTeamMember); len(additions) != 1 || additions[0].OrganizationID != "acme-id" || additions[0].TeamID != "eng-id" || additions[0].AccountID != "BOB111" {
		t.Errorf("add requests: %+v", additions)
	}
}

func TestOrganizationRemovalsNeedConfirmationWithoutATerminal(t *testing.T) {
	app := startOrganizationApp(t)
	setInteractiveTerminal(t, false)
	removals := []struct {
		args        []string
		requestType string
	}{
		{[]string{"org", "delete", "Acme"}, cliprotocol.TypeDeleteOrganization},
		{[]string{"org", "remove", "Acme", "bob@example.com"}, cliprotocol.TypeRemoveOrganizationMember},
		{[]string{"org", "leave", "Acme"}, cliprotocol.TypeLeaveOrganization},
		{[]string{"team", "delete", "Acme", "Eng"}, cliprotocol.TypeDeleteTeam},
	}
	for _, removal := range removals {
		if _, _, err := runCapturingOutput(t, false, "", removal.args...); err != errNeedsConfirmation {
			t.Errorf("%v without -y: %v", removal.args, err)
		}
		if sent := app.findRequest(removal.requestType); len(sent) != 0 {
			t.Errorf("%v sent %+v without -y", removal.args, sent)
		}
		if _, _, err := runCapturingOutput(t, false, "", append(removal.args, "-y")...); err != nil {
			t.Errorf("%v -y: %v", removal.args, err)
		}
		if sent := app.findRequest(removal.requestType); len(sent) != 1 || sent[0].OrganizationID != "acme-id" {
			t.Errorf("%v -y sent %+v", removal.args, sent)
		}
	}
	if sent := app.findRequest(cliprotocol.TypeRemoveOrganizationMember); len(sent) != 1 || sent[0].AccountID != "BOB111" {
		t.Errorf("remove requests: %+v", sent)
	}
	if sent := app.findRequest(cliprotocol.TypeDeleteTeam); len(sent) != 1 || sent[0].TeamID != "eng-id" {
		t.Errorf("team delete requests: %+v", sent)
	}
}

func TestOrganizationInviteSaysTheSameForAnyEmail(t *testing.T) {
	app := startOrganizationApp(t)
	_, existingOutput, err := runCapturingOutput(t, false, "", "org", "invite", "Acme", "bob@example.com")
	if err != nil || !strings.Contains(existingOutput, "Invitation sent to bob@example.com to join Acme as member.") {
		t.Fatalf("invite: %v %s", err, existingOutput)
	}
	_, unknownOutput, err := runCapturingOutput(t, false, "", "org", "invite", "Acme", " Nobody@Example.com ")
	if err != nil {
		t.Fatal(err)
	}
	if strings.ReplaceAll(unknownOutput, "nobody@example.com", "someone") != strings.ReplaceAll(existingOutput, "bob@example.com", "someone") {
		t.Errorf("an unknown email gets another answer:\n%s\n%s", unknownOutput, existingOutput)
	}
	if _, _, err := runCapturingOutput(t, false, "", "org", "invite", "Acme", "ada@example.com", "-r", "admin"); err != nil {
		t.Fatal(err)
	}
	invitations := app.findRequest(cliprotocol.TypeInviteToOrganization)
	if len(invitations) != 3 || invitations[0].Role != "member" || invitations[1].Email != "nobody@example.com" || invitations[2].Role != "admin" {
		t.Errorf("invite requests: %+v", invitations)
	}
	if _, _, err := runCapturingOutput(t, false, "", "org", "invite", "Acme", "ada@example.com", "-r", "owner"); err == nil || !strings.Contains(err.Error(), "use admin or member") {
		t.Errorf("an invalid role: %v", err)
	}
}

func TestOrgsDeliversKeysAndListsOrganizations(t *testing.T) {
	app := startOrganizationApp(t)
	stdout, stderr, err := runCapturingOutput(t, false, "", "orgs")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"Gave 1 vault key to a team member.\n",
		"Rotated the key of 1 vault someone lost access to.\n",
		"Could not give every vault key: dave@example.com: their signing key changed\n",
		"Waiting for the owners of 2 vaults in Acme to give you their keys.\n",
		"Waiting for an owner of a vault in Globex to give you its key.\n",
		"Answer an invitation with `hisar org accept <invitation>`",
	} {
		if !strings.Contains(stderr, expected) {
			t.Errorf("orgs misses %q:\n%s", expected, stderr)
		}
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 7 || !strings.HasPrefix(lines[0], "NAME") || !strings.HasPrefix(lines[1], "Acme ") || !strings.Contains(lines[1], "admin") ||
		!strings.HasPrefix(lines[5], "INVITATION") || !strings.HasPrefix(lines[6], "f0e1d2c3 ") || !strings.Contains(lines[6], "Hank <hank@example.com>") {
		t.Errorf("orgs:\n%s", stdout)
	}
	types := app.listRequestTypes()
	if completion, listing := slices.Index(types, cliprotocol.TypeCompleteOrganizationKeys), slices.Index(types, cliprotocol.TypeListOrganization); completion < 0 || completion > listing {
		t.Errorf("orgs did not deliver keys before listing: %v", types)
	}
}

func TestOrgShowListsMembersTeamsAndInvitations(t *testing.T) {
	startOrganizationApp(t)
	stdout, stderr, err := runCapturingOutput(t, false, "", "org", "show", "acme")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr, "Gave 1 vault key to a team member.") {
		t.Errorf("org show did not deliver keys:\n%s", stderr)
	}
	for _, expected := range []string{
		"Acme (you are an admin)",
		"Alice <alice@example.com> (you)",
		"0000 1111 2222 3333 (key changed)",
		"Team Eng\n  Members: Alice <alice@example.com>, Bob <bob@example.com>\n",
		`Vaults:  "EngVault" (owner), a vault you can't open (reader, ID locked-vault-id)`,
		"carol@example.com",
	} {
		if !strings.Contains(stdout, expected) {
			t.Errorf("org show misses %q:\n%s", expected, stdout)
		}
	}
}

func TestOrgAcceptAsksForTheOnlyInvitation(t *testing.T) {
	app := startOrganizationApp(t)
	if _, stderr, err := runCapturingOutput(t, false, "\n", "org", "accept"); err != nil || !strings.Contains(stderr, "You joined Globex as a member.") {
		t.Fatalf("accept: %v %s", err, stderr)
	}
	if accepts := app.findRequest(cliprotocol.TypeAcceptOrganizationInvitation); len(accepts) != 1 || accepts[0].InvitationID != "f0e1d2c3-0000-4000-8000-000000000003" {
		t.Errorf("accept requests: %+v", accepts)
	}
	if _, stderr, err := runCapturingOutput(t, false, "", "org", "decline", "f0e1"); err != nil || !strings.Contains(stderr, "Declined the invitation to Globex.") {
		t.Errorf("decline: %v %s", err, stderr)
	}
	if _, _, err := runCapturingOutput(t, false, "", "org", "accept", "zz"); err == nil || !strings.Contains(err.Error(), "run `hisar orgs` to list them") {
		t.Errorf("an unknown invitation: %v", err)
	}
}

func TestOrganizationVaultCommandsThroughApp(t *testing.T) {
	app := startOrganizationApp(t)
	if _, stderr, err := runCapturingOutput(t, false, "", "vault", "create", "Deploys", "--org", "acme", "--team", "eng"); err != nil || !strings.Contains(stderr, `Created vault "Deploys" in Acme for team "Eng".`) {
		t.Fatalf("vault create --org --team: %v %s", err, stderr)
	}
	if created := app.findRequest(cliprotocol.TypeCreateOrganizationVault); len(created) != 1 || created[0].OrganizationID != "acme-id" || created[0].TeamID != "eng-id" || created[0].Name != "Deploys" {
		t.Errorf("create requests: %+v", created)
	}
	if _, _, err := runCapturingOutput(t, false, "", "vault", "create", "Deploys", "--org", "Acme"); err != errOrganizationVaultNeedsTeam {
		t.Errorf("--org without --team: %v", err)
	}
	if _, _, err := runCapturingOutput(t, false, "", "vault", "create", "Trip"); err != nil || len(app.findRequest(cliprotocol.TypeCreateVault)) != 1 {
		t.Errorf("a plain vault create: %v %v", err, app.listRequestTypes())
	}

	if _, stderr, err := runCapturingOutput(t, false, "", "vault", "give", "engvault", "--org", "Acme", "--team", "Eng", "-r", "reader"); err != nil || !strings.Contains(stderr, `Gave "EngVault" to team "Eng" as reader.`) {
		t.Errorf("give: %v %s", err, stderr)
	}
	if _, stderr, err := runCapturingOutput(t, false, "", "vault", "take", "locked-vault-id", "--org", "Acme", "--team", "Eng"); err != nil || !strings.Contains(stderr, `Took the vault locked-vault-id from team "Eng".`) {
		t.Errorf("take a vault this user cannot open: %v %s", err, stderr)
	}
	if gives := app.findRequest(cliprotocol.TypeGiveVaultToTeam); len(gives) != 1 || gives[0].VaultID != "eng-vault-id" || gives[0].TeamID != "eng-id" || gives[0].Role != "reader" {
		t.Errorf("give requests: %+v", gives)
	}
	if takes := app.findRequest(cliprotocol.TypeTakeVaultFromTeam); len(takes) != 1 || takes[0].VaultID != "locked-vault-id" {
		t.Errorf("take requests: %+v", takes)
	}
	if _, _, err := runCapturingOutput(t, false, "", "vault", "give", "Other", "--org", "Acme", "--team", "Eng"); err == nil || !strings.Contains(err.Error(), `vault "Other": not found in Acme`) {
		t.Errorf("a vault of another organization: %v", err)
	}
}

func TestOrganizationsNeedAnAppThatKnowsThem(t *testing.T) {
	app := startOrganizationApp(t)
	app.mutex.Lock()
	app.hello.Features = []string{cliprotocol.FeatureSecureNotes, cliprotocol.FeatureFavorites, cliprotocol.FeatureTags}
	app.mutex.Unlock()
	for _, args := range [][]string{{"orgs"}, {"org", "create", "Acme"}, {"team", "create", "Acme", "Ops"}, {"vault", "create", "Deploys", "--org", "Acme", "--team", "Eng"}} {
		if _, _, err := runCapturingOutput(t, false, "", args...); err != errOrganizationsNeedNewerApp {
			t.Errorf("%v with an older app: %v", args, err)
		}
	}
	for _, requestType := range app.listRequestTypes() {
		if strings.HasPrefix(requestType, "organizations.") || strings.HasPrefix(requestType, "teams.") || requestType == cliprotocol.TypeCreateOrganizationVault {
			t.Errorf("an older app was sent %s", requestType)
		}
	}
}
