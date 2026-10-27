import XCTest
@testable import Hisar

@MainActor
final class VaultManagementTests: XCTestCase {
    private var backend: FakeBackend!

    override func setUp() async throws {
        backend = FakeBackend()
    }

    private func createUnlockedModel() async -> AppModel {
        backend.seedSignedIn()
        backend.allVault.append(VaultSummary(id: "work", name: "Work", type: "shared", role: Member.vaultRoleOwner, itemCount: 2))
        backend.allItemByVaultID["work"] = [
            ItemDetail(id: "aws", revision: "r1", title: "AWS", password: "aws-secret"),
            ItemDetail(id: "stripe", revision: "r1", title: "Stripe", password: "stripe-secret"),
        ]
        backend.allVaultMemberByVaultID["work"] = [
            Member(accountID: "alice-id", name: "Alice", email: "alice@example.com", role: Member.vaultRoleOwner, isCurrentUser: true),
            Member(accountID: "bob-id", name: "Bob", email: "bob@example.com", role: Member.vaultRoleEditor),
        ]
        let model = AppModel(credentialIdentityStore: FakeCredentialIdentityStore())
        await model.start { self.backend }?.value
        return model
    }

    func testVaultActionsFollowTheRole() async {
        let model = await createUnlockedModel()
        let personal = await model.loadVaultActions(vaultID: "personal")
        XCTAssertEqual(personal?.canDelete, false)
        XCTAssertEqual(personal?.canLeave, false)
        XCTAssertEqual(personal?.deleteBlockedReason.contains("Personal vault"), true)
        let family = await model.loadVaultActions(vaultID: "family")
        XCTAssertEqual(family?.canDelete, false)
        XCTAssertEqual(family?.canLeave, true)
        XCTAssertEqual(family?.canRename, false)
        let work = await model.loadVaultActions(vaultID: "work")
        XCTAssertEqual(work?.canDelete, true)
        XCTAssertEqual(work?.canRename, true)
        XCTAssertEqual(work?.allMemberLosingAccess.map(\.email), ["bob@example.com"])
        XCTAssertEqual(work?.allTargetVault.map(\.id), ["family", "personal"])
    }

    func testDeletionRequestDefaultsToMovingAndNeedsTheTypedName() {
        let target = VaultSummary(id: "personal", name: "Personal", type: "personal", role: "owner", itemCount: 0)
        let actions = VaultActions(vault: VaultSummary(id: "work", name: "Work", type: "shared", role: "owner", itemCount: 3), canDelete: true, allTargetVault: [target])
        let defaultTarget = VaultDeletionLogic.findDefaultTargetVaultID(actions)
        XCTAssertEqual(defaultTarget, "personal")
        XCTAssertEqual(VaultDeletionLogic.buildRequest(actions: actions, isDeletingItems: false, targetVaultID: defaultTarget, typedName: ""), VaultDeletionRequest(moveToVaultID: "personal"))
        XCTAssertNil(VaultDeletionLogic.buildRequest(actions: actions, isDeletingItems: false, targetVaultID: "elsewhere", typedName: ""))
        XCTAssertNil(VaultDeletionLogic.buildRequest(actions: actions, isDeletingItems: true, targetVaultID: defaultTarget, typedName: "work"))
        XCTAssertNil(VaultDeletionLogic.buildRequest(actions: actions, isDeletingItems: true, targetVaultID: defaultTarget, typedName: "Wor"))
        XCTAssertEqual(VaultDeletionLogic.buildRequest(actions: actions, isDeletingItems: true, targetVaultID: defaultTarget, typedName: " Work "), VaultDeletionRequest(isDeletingItems: true, confirmedName: "Work"))

        var withoutTarget = actions
        withoutTarget.allTargetVault = []
        XCTAssertNil(VaultDeletionLogic.buildRequest(actions: withoutTarget, isDeletingItems: false, targetVaultID: "", typedName: ""))
        XCTAssertNotNil(VaultDeletionLogic.buildRequest(actions: withoutTarget, isDeletingItems: false, targetVaultID: "", typedName: "Work"))

        var empty = actions
        empty.vault.itemCount = 0
        XCTAssertEqual(VaultDeletionLogic.buildRequest(actions: empty, isDeletingItems: false, targetVaultID: "", typedName: ""), VaultDeletionRequest())
        var notOwner = actions
        notOwner.canDelete = false
        XCTAssertNil(VaultDeletionLogic.buildRequest(actions: notOwner, isDeletingItems: false, targetVaultID: defaultTarget, typedName: ""))
    }

    func testDeleteVaultMovingItemsUpdatesTheVaults() async {
        let model = await createUnlockedModel()
        let isDeleted = await model.deleteVault(vaultID: "work", request: VaultDeletionRequest(moveToVaultID: "personal"))
        XCTAssertTrue(isDeleted)
        XCTAssertEqual(backend.allDeletionRequest, [VaultDeletionRequest(moveToVaultID: "personal")])
        XCTAssertNil(model.findVault(vaultID: "work"))
        XCTAssertEqual(model.countItem(vaultID: "personal"), 4)
        XCTAssertEqual(model.noticeMessage, "Moved 2 items to Personal and deleted Work.")
    }

    func testDeleteVaultWithItemsRefusesTheWrongName() async {
        let model = await createUnlockedModel()
        let isDeleted = await model.deleteVault(vaultID: "work", request: VaultDeletionRequest(isDeletingItems: true, confirmedName: "work"))
        XCTAssertFalse(isDeleted)
        XCTAssertNotNil(model.findVault(vaultID: "work"))
        XCTAssertNotNil(model.errorMessage)
        let isDeletedWithName = await model.deleteVault(vaultID: "work", request: VaultDeletionRequest(isDeletingItems: true, confirmedName: "Work"))
        XCTAssertTrue(isDeletedWithName)
        XCTAssertNil(model.findVault(vaultID: "work"))
        XCTAssertEqual(model.noticeMessage, "Deleted Work.")
    }

    func testLeaveAndRenameVault() async {
        let model = await createUnlockedModel()
        let isRenamed = await model.renameVault(vaultID: "work", name: "  Work 2026 ")
        XCTAssertTrue(isRenamed)
        XCTAssertEqual(model.findVault(vaultID: "work")?.name, "Work 2026")
        let isRenamedByEditor = await model.renameVault(vaultID: "family", name: "Mine")
        XCTAssertFalse(isRenamedByEditor)
        XCTAssertEqual(model.findVault(vaultID: "family")?.name, "Family")

        let isLeft = await model.leaveVault(vaultID: "family")
        XCTAssertTrue(isLeft)
        XCTAssertNil(model.findVault(vaultID: "family"))
        XCTAssertEqual(model.noticeMessage, "You left Family.")
        XCTAssertTrue(model.listAllItemReference().allSatisfy { $0.vaultID != "family" })
    }

    func testRemovedVaultNoticesAreShownOnceAndDismissed() async {
        let model = await createUnlockedModel()
        XCTAssertTrue(model.allRemovedVaultNotice.isEmpty)
        let notice = RemovedVaultNotice(id: "family", name: "Family", message: "“Family” is no longer available: it was deleted, or you no longer have access to it.")
        backend.allPendingRemovedVault = [notice]
        backend.allVault.removeAll { $0.id == "family" }
        await model.refreshAll()
        XCTAssertEqual(model.allRemovedVaultNotice, [notice])
        XCTAssertNil(model.findVault(vaultID: "family"))
        await model.refreshAll()
        XCTAssertEqual(model.allRemovedVaultNotice, [notice])
        model.dismissRemovedVaultNotice(notice)
        XCTAssertTrue(model.allRemovedVaultNotice.isEmpty)
        await model.refreshAll()
        XCTAssertTrue(model.allRemovedVaultNotice.isEmpty)
    }

    func testExportInputChecks() {
        XCTAssertNil(ExportLogic.checkInput(format: .plain, passphrase: "", repeatedPassphrase: "", minPassphraseLength: 10))
        XCTAssertEqual(ExportLogic.checkInput(format: .encrypted, passphrase: "short", repeatedPassphrase: "short", minPassphraseLength: 10), "Use at least 10 characters for the export passphrase.")
        XCTAssertEqual(ExportLogic.checkInput(format: .encrypted, passphrase: "a long passphrase", repeatedPassphrase: "a long passphrasf", minPassphraseLength: 10), "The passphrases don't match.")
        XCTAssertNil(ExportLogic.checkInput(format: .encrypted, passphrase: "a long passphrase", repeatedPassphrase: "a long passphrase", minPassphraseLength: 10))
    }

    func testExportWritesIntoItsOwnFolderAndIsRemoved() async throws {
        let model = await createUnlockedModel()
        let options = await model.loadExportOptions()
        XCTAssertEqual(options?.minPassphraseLength, 10)

        let exported = await model.exportAllData(format: .encrypted, passphrase: "a long passphrase", password: "")
        let result = try XCTUnwrap(exported)
        let fileURL = URL(fileURLWithPath: result.path)
        let folderURL = fileURL.deletingLastPathComponent()
        XCTAssertTrue(FileManager.default.fileExists(atPath: result.path))
        XCTAssertTrue(folderURL.lastPathComponent.hasPrefix("Export-"))
        XCTAssertEqual(result.vaultCount, 3)
        model.removeExport(result)
        XCTAssertFalse(FileManager.default.fileExists(atPath: folderURL.path))

        backend.isBiometricSucceeding = false
        let cancelled = await model.exportAllData(format: .plain, passphrase: "", password: "")
        XCTAssertNil(cancelled)
        XCTAssertEqual(model.errorMessage, "Face ID was cancelled.")
        let exportedWithPassword = await model.exportAllData(format: .plain, passphrase: "ignored", password: FakeBackend.password)
        let withPassword = try XCTUnwrap(exportedWithPassword)
        XCTAssertTrue(withPassword.path.hasSuffix(".json"))
        model.removeExport(withPassword)
    }

    func testRecoveryCountsOnlyKnownItemsAndAffectedVaults() {
        let item = ReadIssue(vaultId: "personal", itemId: "bad", kind: "item", canRename: false, canDelete: true)
        let name = ReadIssue(vaultId: "personal", kind: "vault-name", canRename: true, canDelete: false)
        let key = ReadIssue(vaultId: "work", kind: "vault-key", canRename: false, canDelete: false)
        XCTAssertEqual(ReadIssueSummary(issues: [item]).text, "1 known unreadable item in 1 affected vault.")
        XCTAssertEqual(ReadIssueSummary(issues: [item, item, name, key]).knownItemCount, 1)
        XCTAssertEqual(ReadIssueSummary(issues: [item, name, key]).affectedVaultCount, 2)
        XCTAssertEqual(ReadIssueSummary(issues: [key]).text, "1 affected vault. Items behind unopened vault keys are not counted.")
        XCTAssertEqual(name.explanation, "Vault name could not be opened. Its readable items remain available.")
        XCTAssertEqual(key.explanation, "Vault key could not be opened. Items in this vault cannot be displayed.")
        XCTAssertEqual(item.explanation, "Item could not be opened. It remains saved.")
    }
    func testUnreadableRecordsRemainExplicitBesideHealthyItems() throws {
        let vault = try JSONDecoder().decode(VaultSummary.self, from: Data(#"{"id":"v","name":"Private","type":"personal","role":"owner","itemCount":1,"readIssues":[{"vaultId":"v","itemId":"bad","revision":"r1","kind":"item","canDelete":true,"canRename":false}]}"#.utf8))
        let items = try JSONDecoder().decode([ItemDetail].self, from: Data(#"[{"id":"healthy","title":"Still available"},{"id":"bad","revision":"r1","title":"Unreadable item bad","readIssue":{"vaultId":"v","itemId":"bad","revision":"r1","kind":"item","canDelete":true,"canRename":false}}]"#.utf8))
        XCTAssertEqual(vault.itemCount, 1)
        XCTAssertEqual(vault.readIssues.first?.revision, "r1")
        XCTAssertEqual(vault.readIssues.first?.canDelete, true)
        XCTAssertEqual(items.filter { $0.readIssue == nil }.map(\.id), ["healthy"])
        XCTAssertEqual(items.first { $0.readIssue != nil }?.readIssue?.itemId, "bad")
    }

    func testNewlyUnreadableItemAfterVaultRefreshRemainsDisclosed() async throws {
        let model = await createUnlockedModel()
        let healthyCount = model.countItem(vaultID: "personal")
        let issue = ReadIssue(vaultId: "personal", itemId: "bad", revision: "r1", kind: "item", canRename: false, canDelete: true)
        var placeholder = ItemDetail(id: "bad", revision: "r1", title: "Unreadable item bad")
        placeholder.readIssue = issue
        backend.allItemByVaultID["personal", default: []].append(placeholder)
        await model.refreshVault(vaultID: "personal")
        let personal = try XCTUnwrap(model.findVault(vaultID: "personal"))
        XCTAssertEqual(personal.readIssues, [issue])
        XCTAssertEqual(personal.itemCount, healthyCount)
        XCTAssertFalse(model.listAllItemReference(vaultID: "personal").contains { $0.item.id == "bad" })
    }

    func testUnreadableVaultRecoveryNamesTheReviewedRevision() async throws {
        let model = await createUnlockedModel()
        let reviewed = ReadIssue(vaultId: "work", itemId: nil, revision: "review-1", kind: "vault-name", canRename: true, canDelete: true)
        let work = try XCTUnwrap(backend.allVault.firstIndex { $0.id == "work" })
        var current = reviewed
        current.revision = "review-2"
        backend.allVault[work].readIssues = [current]
        await model.refreshAll()
        await model.repairVaultName(reviewed, name: "Work")
        await model.deleteUnreadableRecord(reviewed)
        XCTAssertNotNil(model.errorMessage, "a stale review was not refused")
        XCTAssertNotNil(model.findVault(vaultID: "work"), "a stale review deleted the vault")
        let shown = try XCTUnwrap(model.findVault(vaultID: "work")?.readIssues.first)
        await model.repairVaultName(shown, name: "Work")
        XCTAssertEqual(model.findVault(vaultID: "work")?.name, "Work")
        XCTAssertEqual(model.findVault(vaultID: "work")?.readIssues, [])
    }
}
