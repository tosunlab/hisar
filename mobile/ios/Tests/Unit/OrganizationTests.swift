import XCTest
@testable import Hisar

@MainActor
final class OrganizationTests: XCTestCase {
    private var backend: FakeBackend!

    private static let alice = Member(accountID: "alice-id", name: "Alice", email: "alice@example.com", role: OrganizationRole.admin, securityCode: "1111 2222", isCurrentUser: true)
    private static let bob = Member(accountID: "bob-id", name: "Bob", email: "bob@example.com", role: OrganizationRole.member, securityCode: "3333 4444")
    private static let carol = Member(accountID: "carol-id", name: "Carol", email: "carol@example.com", role: OrganizationRole.admin, securityCode: "5555 6666")

    override func setUp() async throws {
        backend = FakeBackend()
    }

    private static func buildAcme(isAdmin: Bool) -> Organization {
        let platform = Team(
            id: "platform", name: "Platform",
            allMember: [TeamMember(accountID: "alice-id", name: "Alice", email: "alice@example.com", isCurrentUser: true), TeamMember(accountID: "bob-id", name: "Bob", email: "bob@example.com")],
            allVault: [TeamVault(vaultID: "deploy", name: "Deploy", role: Member.vaultRoleEditor, canOpen: true)]
        )
        let audit = Team(
            id: "audit", name: "Audit",
            allMember: [TeamMember(accountID: "carol-id", name: "Carol", email: "carol@example.com")],
            allVault: [TeamVault(vaultID: "deploy", name: "Deploy", role: Member.vaultRoleReader, canOpen: true), TeamVault(vaultID: "ledger", name: "", role: Member.vaultRoleOwner, canOpen: false)]
        )
        var alice = alice
        alice.role = isAdmin ? OrganizationRole.admin : OrganizationRole.member
        return Organization(
            id: "acme", name: "Acme", role: alice.role, isAdmin: isAdmin,
            allMember: [carol, alice, bob], allTeam: [audit, platform],
            allInvitation: isAdmin ? [SentOrganizationInvitation(id: "invitation-1", email: "dave@example.com", role: OrganizationRole.member, createdAt: "", expiresAt: "")] : []
        )
    }

    private func createUnlockedModel() async -> AppModel {
        backend.seedSignedIn()
        let model = AppModel(credentialIdentityStore: FakeCredentialIdentityStore())
        await model.start { self.backend }?.value
        return model
    }

    func testDecodesWhatCoreSends() throws {
        let organizationJSON = """
        {"id":"acme","name":"Acme","role":"member","isAdmin":false,
         "members":[{"accountId":"bob-id","name":"Bob","email":"bob@example.com","role":"admin","securityCode":"3333 4444","isCurrentUser":false,"isKeyChanged":true}],
         "teams":[{"id":"platform","name":"Platform","members":[{"accountId":"alice-id","name":"Alice","email":"alice@example.com","isCurrentUser":true}],
                   "vaults":[{"vaultId":"ledger","name":"","role":"owner","canOpen":false}]}],
         "invitations":[]}
        """
        let organization = try JSONDecoder().decode(Organization.self, from: Data(organizationJSON.utf8))
        XCTAssertEqual(organization.allMember.first?.isKeyChanged, true)
        XCTAssertEqual(organization.allMember.first?.securityCode, "3333 4444")
        XCTAssertEqual(organization.allTeam.first?.hasCurrentUser, true)
        XCTAssertEqual(organization.allTeam.first?.allVault.first?.displayName, "A vault you can't open")

        let vault = try JSONDecoder().decode(VaultSummary.self, from: Data(#"{"id":"ledger","name":"Ledger","type":"organization","role":"reader","itemCount":3,"organizationId":"acme"}"#.utf8))
        XCTAssertEqual(vault.organizationID, "acme")
        XCTAssertTrue(vault.isOrganization)

        let keys = try JSONDecoder().decode(WaitingVaultKeys.self, from: Data(#"{"waiting":[{"vaultId":"ledger","organizationId":"acme","organizationName":"Acme","role":"reader"}],"deliveryError":"","rotationError":"the signing key of b@example.com changed"}"#.utf8))
        XCTAssertEqual(keys.allWaiting.map(\.organizationName), ["Acme"])
        XCTAssertEqual(keys.rotationError, "the signing key of b@example.com changed")

        let organizations = try JSONDecoder().decode(Organizations.self, from: Data(#"{"organizations":[{"id":"acme","name":"Acme","role":"admin","memberCount":3}],"invitations":[{"id":"i","organizationId":"globex","organizationName":"Globex","role":"admin","invitedByName":"Ada","invitedByEmail":"ada@example.com","expiresAt":"2027-01-22T08:00:00Z"}]}"#.utf8))
        XCTAssertEqual(organizations.allOrganization.first?.memberCount, 3)
        XCTAssertEqual(organizations.allInvitation.first?.message, "Ada invited you to Globex as an admin")

        let preview = try JSONDecoder().decode(AccountDeletionPreview.self, from: Data(#"{"email":"a@example.com","ownedVaults":[],"memberVaults":[],"organizations":[{"id":"acme","name":"Acme","isDeleted":false,"newAdminName":"Bob"}]}"#.utf8))
        XCTAssertEqual(preview.allOrganization, [DeletionOrganization(id: "acme", name: "Acme", newAdminName: "Bob")])
    }

    func testOrganizationVaultsAreGroupedUnderTheirOrganizationWithWaitingKeys() {
        let personal = VaultSummary(id: "personal", name: "Personal", type: VaultSummary.typePersonal, role: Member.vaultRoleOwner, itemCount: 0)
        let family = VaultSummary(id: "family", name: "Family", type: VaultSummary.typeShared, role: Member.vaultRoleEditor, itemCount: 0)
        let deploy = VaultSummary(id: "deploy", name: "Deploy", type: VaultSummary.typeOrganization, role: Member.vaultRoleEditor, itemCount: 0, organizationID: "northwind")
        let audit = VaultSummary(id: "audit", name: "Audit", type: VaultSummary.typeOrganization, role: Member.vaultRoleReader, itemCount: 0, organizationID: "northwind")
        let ledger = VaultSummary(id: "ledger", name: "Ledger", type: VaultSummary.typeOrganization, role: Member.vaultRoleOwner, itemCount: 0, organizationID: "acme")
        let unlisted = VaultSummary(id: "unlisted", name: "Unlisted", type: VaultSummary.typeOrganization, role: Member.vaultRoleOwner, itemCount: 0, organizationID: "initech")
        let allOrganization = [
            OrganizationSummary(id: "northwind", name: "Northwind", role: OrganizationRole.member, memberCount: 2),
            OrganizationSummary(id: "acme", name: "acme", role: OrganizationRole.admin, memberCount: 4),
        ]
        let allWaiting = [
            WaitingVaultKey(vaultID: "ops", organizationID: "northwind", organizationName: "Northwind", role: Member.vaultRoleEditor),
            WaitingVaultKey(vaultID: "deploy", organizationID: "northwind", organizationName: "Northwind", role: Member.vaultRoleEditor),
            WaitingVaultKey(vaultID: "plans", organizationID: "globex", organizationName: "Globex", role: Member.vaultRoleReader),
            WaitingVaultKey(vaultID: "notes", organizationID: "globex", organizationName: "Globex", role: Member.vaultRoleReader),
        ]

        let grouping = OrganizationLogic.groupVaults([personal, family, audit, deploy, ledger, unlisted], allOrganization: allOrganization, allWaiting: allWaiting)

        XCTAssertEqual(grouping.allOtherVault.map(\.id), ["personal", "family", "unlisted"], "a vault of an organization not listed yet stays with the others")
        XCTAssertEqual(grouping.allGroup.map(\.name), ["acme", "Globex", "Northwind"])
        XCTAssertEqual(grouping.allGroup[0].allVault.map(\.id), ["ledger"])
        XCTAssertEqual(grouping.allGroup[0].waitingMessage, "")
        XCTAssertEqual(grouping.allGroup[1].allVault, [], "an organization whose only vaults wait for keys still gets its section")
        XCTAssertEqual(grouping.allGroup[1].waitingMessage, "Waiting for the owners of 2 vaults in Globex to give you their keys")
        XCTAssertEqual(grouping.allGroup[2].allVault.map(\.id), ["audit", "deploy"])
        XCTAssertEqual(grouping.allGroup[2].waitingMessage, "Waiting for an owner of a vault in Northwind to give you its key", "a vault that arrived no longer waits")
    }

    func testKeyWarningsSayWhatHisarCouldNotDo() {
        XCTAssertEqual(OrganizationLogic.describeKeyWarnings(WaitingVaultKeys()), [])
        XCTAssertEqual(
            OrganizationLogic.describeKeyWarnings(WaitingVaultKeys(deliveryError: "the signing key of bob@example.com changed", rotationError: "cannot reach the Hisar server")),
            [
                "Hisar couldn't give every vault key: the signing key of bob@example.com changed",
                "Hisar couldn't re-encrypt every vault that needs a new key: cannot reach the Hisar server",
            ]
        )
    }

    func testWhereOffersASharedVaultAndTheTeamsTheUserIsIn() {
        let northwind = Organization(id: "northwind", name: "Northwind", role: OrganizationRole.member, isAdmin: false, allTeam: [
            Team(id: "ops", name: "Ops", allMember: [TeamMember(accountID: "alice-id", name: "Alice", email: "alice@example.com", isCurrentUser: true)]),
        ])
        let allLocation = OrganizationLogic.listAllVaultLocation([northwind, Self.buildAcme(isAdmin: true)])
        XCTAssertEqual(allLocation.map(\.label), ["Shared vault", "Acme › Platform", "Northwind › Ops"], "Audit is left out: Alice is not in it")
        XCTAssertTrue(allLocation[0].isSharedVault)
        XCTAssertEqual(allLocation[1].organizationID, "acme")
        XCTAssertEqual(allLocation[1].teamID, "platform")
        XCTAssertEqual(OrganizationLogic.listAllVaultLocation([]), [.sharedVault])
    }

    func testOnlyAdminsChangeWhichTeamsHaveAnOrganizationVault() {
        let asMember = OrganizationLogic.describeVaultSharing(organization: Self.buildAcme(isAdmin: false), vaultID: "deploy")
        XCTAssertEqual(asMember.allTeamAccess.map(\.team.name), ["Audit", "Platform"])
        XCTAssertEqual(asMember.allTeamAccess.map(\.role), [Member.vaultRoleReader, Member.vaultRoleEditor])
        XCTAssertFalse(asMember.canChangeTeams)
        XCTAssertEqual(asMember.askAdminNote, "Ask an admin of Acme to change who has this vault.")
        XCTAssertEqual(asMember.allTeamWithoutVault, [])

        let asAdmin = OrganizationLogic.describeVaultSharing(organization: Self.buildAcme(isAdmin: true), vaultID: "ledger")
        XCTAssertTrue(asAdmin.canChangeTeams)
        XCTAssertNil(asAdmin.askAdminNote)
        XCTAssertEqual(asAdmin.allTeamAccess.map(\.team.name), ["Audit"])
        XCTAssertEqual(asAdmin.allTeamWithoutVault.map(\.name), ["Platform"], "Give to a team… offers the teams without the vault")
    }

    func testOnlyAdminsManageMembersAndTeams() {
        let asMember = Self.buildAcme(isAdmin: false)
        let platform = asMember.allTeam[1]
        XCTAssertEqual(OrganizationLogic.listAllMemberAction(organization: asMember, member: Self.bob), [])
        XCTAssertEqual(OrganizationLogic.listAllAddableMember(organization: asMember, team: platform), [])
        XCTAssertEqual(OrganizationLogic.listAllGivableVault(organization: asMember, team: platform, allVault: []), [])

        let asAdmin = Self.buildAcme(isAdmin: true)
        XCTAssertEqual(OrganizationLogic.listAllMemberAction(organization: asAdmin, member: Self.bob), [.makeAdmin, .remove])
        XCTAssertEqual(OrganizationLogic.listAllMemberAction(organization: asAdmin, member: Self.carol), [.makeMember, .remove])
        XCTAssertEqual(OrganizationLogic.listAllMemberAction(organization: asAdmin, member: asAdmin.allMember[1]), [.makeMember], "admins leave instead of removing themselves")
        XCTAssertEqual(OrganizationLogic.listAllAddableMember(organization: asAdmin, team: asAdmin.allTeam[1]).map(\.name), ["Carol"])

        let onlyOnThisDevice = VaultSummary(id: "runbooks", name: "Runbooks", type: VaultSummary.typeOrganization, role: Member.vaultRoleOwner, itemCount: 0, organizationID: "acme")
        let elsewhere = VaultSummary(id: "other", name: "Other", type: VaultSummary.typeOrganization, role: Member.vaultRoleOwner, itemCount: 0, organizationID: "globex")
        let allGivable = OrganizationLogic.listAllGivableVault(organization: asAdmin, team: asAdmin.allTeam[1], allVault: [onlyOnThisDevice, elsewhere])
        XCTAssertEqual(allGivable.map(\.displayName), ["Runbooks", "A vault you can't open"], "Deploy is already Platform's; another organization's vault is not offered")

        var sameNamedMembers = asAdmin
        sameNamedMembers.allMember[1].name = "Alex Morgan"
        sameNamedMembers.allMember[2].name = "Alex Morgan"
        var team = platform
        team.allMember.append(TeamMember(accountID: "unknown", name: "Alex Morgan", email: "unknown@example.com"))
        let allTeamMember = OrganizationLogic.listAllTeamMember(organization: sameNamedMembers, team: team)
        XCTAssertEqual(allTeamMember, Array(sameNamedMembers.allMember[1...2]))
        XCTAssertEqual(allTeamMember.map(\.email), ["alice@example.com", "bob@example.com"])
        XCTAssertEqual(allTeamMember.map(\.securityCode), ["1111 2222", "3333 4444"])
        XCTAssertEqual(allTeamMember.map(\.role), [OrganizationRole.admin, OrganizationRole.member])
        XCTAssertTrue(allTeamMember[0].isCurrentUser)
        XCTAssertFalse(allTeamMember[1].isCurrentUser)
    }

    func testRefreshListsOrganizationsInvitationsAndWaitingKeysBeforeTheVaults() async {
        backend.allOrganization = [Self.buildAcme(isAdmin: true)]
        backend.allOrganizationInvitation = [OrganizationInvitation(id: "invitation-9", organizationID: "globex", organizationName: "Globex", role: OrganizationRole.member, invitedByName: "Ada", invitedByEmail: "ada@example.com", expiresAt: "")]
        backend.waitingVaultKeys = WaitingVaultKeys(allWaiting: [WaitingVaultKey(vaultID: "ledger", organizationID: "acme", organizationName: "Acme", role: Member.vaultRoleOwner)])
        let model = await createUnlockedModel()

        XCTAssertEqual(model.allOrganization.map(\.name), ["Acme"])
        XCTAssertEqual(model.allOrganization.first?.memberCount, 3)
        XCTAssertEqual(model.organizationInvitationCount, 1)
        XCTAssertEqual(model.waitingVaultKeys.allWaiting.map(\.vaultID), ["ledger"])
        XCTAssertLessThan(backend.allCallName.firstIndex(of: "listAllWaitingVaultKey") ?? .max, backend.allCallName.firstIndex(of: "listAllVault") ?? 0, "keys are given and rotated before the vaults load")

        model.lock()
        XCTAssertEqual(model.allOrganization, [])
        XCTAssertEqual(model.organizationInvitationCount, 0)
        XCTAssertEqual(model.waitingVaultKeys, WaitingVaultKeys())
    }

    func testAcceptingAnInvitationJoinsTheOrganization() async {
        backend.allOrganizationInvitation = [
            OrganizationInvitation(id: "invitation-9", organizationID: "globex", organizationName: "Globex", role: OrganizationRole.member, invitedByName: "Ada", invitedByEmail: "ada@example.com", expiresAt: ""),
            OrganizationInvitation(id: "invitation-10", organizationID: "initech", organizationName: "Initech", role: OrganizationRole.admin, invitedByName: "", invitedByEmail: "bill@example.com", expiresAt: ""),
        ]
        let model = await createUnlockedModel()
        XCTAssertEqual(model.allOrganizationInvitation.map(\.message), ["Ada invited you to Globex as a member", "bill@example.com invited you to Initech as an admin"])

        let isAccepted = await model.acceptOrganizationInvitation(invitationID: "invitation-9")
        XCTAssertTrue(isAccepted)
        XCTAssertEqual(model.allOrganizationInvitation.map(\.id), ["invitation-10"])
        XCTAssertEqual(model.allOrganization.map(\.name), ["Globex"])

        let isDeclined = await model.declineOrganizationInvitation(invitationID: "invitation-10")
        XCTAssertTrue(isDeclined)
        XCTAssertEqual(model.organizationInvitationCount, 0)
        XCTAssertEqual(backend.allOrganization.map(\.name), ["Globex"])
    }

    func testCreatingAVaultForATeamMakesAnOrganizationVault() async {
        backend.allOrganization = [Self.buildAcme(isAdmin: false)]
        let model = await createUnlockedModel()
        let allLocation = await model.loadVaultLocations()
        XCTAssertEqual(allLocation?.map(\.label), ["Shared vault", "Acme › Platform"])

        let vault = await model.createOrganizationVault(organizationID: "acme", teamID: "platform", name: "Runbooks")
        XCTAssertEqual(vault?.organizationID, "acme")
        XCTAssertEqual(model.findVault(vaultID: vault?.id ?? "")?.isOrganization, true)
        let grouping = OrganizationLogic.groupVaults(model.allVault, allOrganization: model.allOrganization, allWaiting: [])
        XCTAssertEqual(grouping.allGroup.first?.allVault.map(\.name), ["Runbooks"], "the new vault shows under Acme")

        let refused = await model.createOrganizationVault(organizationID: "acme", teamID: "audit", name: "Elsewhere")
        XCTAssertNil(refused, "only members of a team create its vaults")
        XCTAssertEqual(model.errorMessage, "Only members of the team can create a vault for it")
    }

    func testANewVaultWaitsForWhereAndNeverTakesAFailedLookupForASharedVault() async {
        backend.allOrganization = [Self.buildAcme(isAdmin: false)]
        let model = await createUnlockedModel()
        XCTAssertNil(OrganizationLogic.findVaultLocation(id: VaultLocation.sharedVault.id, in: nil), "Create waits until Where is loaded")

        let sentence = "cannot reach the Hisar server"
        backend.getOrganizationErrorMessage = sentence
        let failed = await model.loadVaultLocations()
        XCTAssertNil(failed, "a failed team lookup offers no shared vault in its place")
        XCTAssertNil(OrganizationLogic.findVaultLocation(id: VaultLocation.sharedVault.id, in: failed), "Create stays disabled")
        XCTAssertEqual(model.errorMessage, sentence, "the new vault screen's banner says why")

        backend.getOrganizationErrorMessage = nil
        backend.listAllOrganizationErrorMessage = sentence
        let unlisted = await model.loadVaultLocations()
        XCTAssertNil(unlisted, "nor does a failed list of the organizations")
        XCTAssertEqual(model.errorMessage, sentence)

        backend.listAllOrganizationErrorMessage = nil
        let allLocation = await model.loadVaultLocations()
        XCTAssertNil(model.errorMessage)
        XCTAssertEqual(OrganizationLogic.findVaultLocation(id: VaultLocation.sharedVault.id, in: allLocation), .sharedVault)
        XCTAssertEqual(OrganizationLogic.findVaultLocation(id: "acme/platform", in: allLocation)?.label, "Acme › Platform")
        XCTAssertNil(OrganizationLogic.findVaultLocation(id: "acme/audit", in: allLocation), "Alice is not in Audit")
    }

    func testTheOrganizationScreenGoesBackOnlyWhenTheOrganizationIsGone() async {
        backend.allOrganization = [Self.buildAcme(isAdmin: true)]
        let model = await createUnlockedModel()
        let loaded = await model.loadOrganizationScreen(organizationID: "acme")
        guard case .loaded(let organization) = loaded else { return XCTFail("\(loaded)") }
        XCTAssertEqual(organization.name, "Acme")

        let sentence = "Hisar could not read a vault of Acme"
        backend.getOrganizationErrorMessage = sentence
        let unread = await model.loadOrganizationScreen(organizationID: "acme")
        XCTAssertEqual(unread, .failed, "Alice is still in Acme, so the screen stays")
        XCTAssertEqual(model.takeRefusal(title: "Acme")?.message, sentence, "the alert says why")
        XCTAssertNil(model.errorMessage)

        backend.listAllOrganizationErrorMessage = "cannot reach the Hisar server"
        let unlisted = await model.loadOrganizationScreen(organizationID: "acme")
        XCTAssertEqual(unlisted, .failed, "a failed list does not say Alice left")
        XCTAssertEqual(model.takeRefusal(title: "Acme")?.message, sentence, "the alert gives the organization's sentence, not the list's")
        XCTAssertEqual(model.allOrganization.map(\.id), ["acme"])

        backend.getOrganizationErrorMessage = nil
        backend.listAllOrganizationErrorMessage = nil
        backend.allOrganization = []
        let gone = await model.loadOrganizationScreen(organizationID: "acme")
        XCTAssertEqual(gone, .gone, "deleted, or Alice was removed: the screen goes back")
        XCTAssertNil(model.errorMessage)
        XCTAssertEqual(model.allOrganization, [])
    }

    func testARefusalKeepsTheServersSentenceForTheAlert() async {
        backend.allOrganization = [Self.buildAcme(isAdmin: true)]
        let model = await createUnlockedModel()
        let sentence = "Nobody who can open this vault would be its owner. Give owner access to a team whose members already have the vault first."
        backend.organizationRefusal = sentence

        let isTaken = await model.takeVaultFromTeam(organizationID: "acme", teamID: "audit", vaultID: "ledger")
        XCTAssertFalse(isTaken)
        let refusal = model.takeRefusal(title: "Take back")
        XCTAssertEqual(refusal?.title, "Take back")
        XCTAssertEqual(refusal?.message, sentence)
        XCTAssertNil(model.errorMessage, "the alert shows the sentence, not the screen's error banner")
        XCTAssertNil(model.takeRefusal(title: "Take back"))
        XCTAssertEqual(backend.allOrganization.first?.allTeam.first?.allVault.map(\.vaultID), ["deploy", "ledger"])
    }

    func testLeavingAnOrganizationDropsItsVaults() async {
        backend.allOrganization = [Self.buildAcme(isAdmin: false)]
        let model = await createUnlockedModel()
        _ = await model.createOrganizationVault(organizationID: "acme", teamID: "platform", name: "Runbooks")
        XCTAssertEqual(model.allVault.filter(\.isOrganization).map(\.name), ["Runbooks"])

        let isLeft = await model.leaveOrganization(organizationID: "acme")
        XCTAssertTrue(isLeft)
        XCTAssertEqual(model.allOrganization, [])
        XCTAssertEqual(model.allVault.filter(\.isOrganization), [])
        XCTAssertEqual(model.noticeMessage, "You left Acme.")
    }

    func testAccountDeletionSaysWhatHappensToEachOrganization() {
        XCTAssertEqual(AccountLifecycleLogic.describeOrganization(DeletionOrganization(id: "a", name: "Acme", isDeleted: true)), "Acme, its vaults, and their items are deleted with your account")
        XCTAssertEqual(AccountLifecycleLogic.describeOrganization(DeletionOrganization(id: "a", name: "Acme", newAdminName: "Ada")), "Ada becomes an admin of Acme")
        XCTAssertEqual(AccountLifecycleLogic.describeOrganization(DeletionOrganization(id: "a", name: "Acme")), "You leave Acme")
    }
}
