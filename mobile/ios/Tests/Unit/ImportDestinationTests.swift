import XCTest
@testable import Hisar

final class ImportDestinationTests: XCTestCase {
    private func vault(_ id: String, organization: String, name: String = "Production") -> VaultSummary {
        VaultSummary(id: id, name: name, type: "organization", role: "editor", itemCount: 0, organizationID: organization)
    }

    private func organization(_ id: String, _ name: String) -> OrganizationSummary {
        OrganizationSummary(id: id, name: name, role: "member", memberCount: 1)
    }

    func testImportDestinationsDistinguishOrganizations() {
        let groups = OrganizationLogic.importVaultGroups([
            vault("one", organization: "acme"), vault("two", organization: "beta"),
            VaultSummary(id: "personal", name: "Personal", type: "personal", role: "owner", itemCount: 0),
            VaultSummary(id: "shared", name: "Production", type: "shared", role: "owner", itemCount: 0),
        ], allOrganization: [organization("acme", "Acme"), organization("beta", "Beta")])
        XCTAssertEqual(groups.map(\.name), ["Personal", "Shared vaults", "Acme", "Beta"])
        XCTAssertEqual(groups.flatMap(\.allChoice).map(\.label), ["Personal", "Shared vaults › Production", "Acme › Production", "Beta › Production"])
    }

    func testImportDestinationsKeepMissingOrganizationIdentity() {
        let groups = OrganizationLogic.importVaultGroups([vault("one", organization: "missing-organization")], allOrganization: [])
        XCTAssertEqual(groups.first?.allChoice.first?.label, "missing-organization › Production")
    }

    func testImportDestinationsDisambiguateRepeatedNamesWithStableIDs() {
        let vaults = [vault("12345678-a", organization: "org-one"), vault("12345678-b", organization: "org-one"), vault("third", organization: "org-two")]
        let organizations = [organization("org-one", "Acme"), organization("org-two", "Acme")]
        let groups = OrganizationLogic.importVaultGroups(vaults, allOrganization: organizations)
        let labels = groups.flatMap(\.allChoice).map(\.label)
        XCTAssertEqual(Set(labels).count, 3)
        XCTAssertTrue(labels.contains("Acme (org-one) › Production (12345678-a)"))
        XCTAssertTrue(labels.contains("Acme (org-one) › Production (12345678-b)"))
        let reordered = OrganizationLogic.importVaultGroups(Array(vaults.reversed()), allOrganization: Array(organizations.reversed()))
        XCTAssertEqual(Set(reordered.flatMap(\.allChoice).map(\.label)), Set(labels))
    }
}
