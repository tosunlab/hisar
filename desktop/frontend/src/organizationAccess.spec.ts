import { describe, expect, it } from "vitest";
import { describeOrganizationAccessChange } from "./organizationAccess";
import { createOrganizationDetail, createOrganizationTeam } from "./testing";

const organization = createOrganizationDetail({ teams: [
  createOrganizationTeam({ id: "ops", name: "Ops" }),
  createOrganizationTeam({ id: "other", name: "Other", members: [{ accountId: "ALICE1", name: "Alice", email: "alice@example.com", isCurrentUser: true }], vaults: [{ vaultId: "deploy", name: "Deploy", role: "editor", canOpen: true }] }),
] });

describe("organization access consequences", () => {
  it("compares each person's strongest remaining team role", () => {
    const lines = describeOrganizationAccessChange(organization, { kind: "team-role", teamID: "ops", vaultID: "deploy", role: "reader" });
    expect(lines.join(" ")).toContain("Your role changes from Owner to Editor");
    expect(lines.join(" ")).toContain("Bob Stone's role changes from Owner to Reader");
  });
});
