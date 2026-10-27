import { describe, expect, it } from "vitest";
import { describeOrganizationAccessChange } from "./organizationAccess";
import { createOrganizationDetail, createOrganizationTeam } from "./testing";

const organization = createOrganizationDetail({ teams: [
  createOrganizationTeam({ id: "ops", name: "Ops" }),
  createOrganizationTeam({ id: "other", name: "Other", members: [{ accountId: "ALICE1", name: "Alice", email: "alice@example.com", isCurrentUser: true }], vaults: [{ vaultId: "deploy", name: "Deploy", role: "editor", canOpen: true }] }),
] });

describe("organization access consequences", () => {
  it("separates losses from reduced roles and preserves unopened vault identities", () => {
    const changed = createOrganizationDetail({ teams: [
      createOrganizationTeam({ id: "ops", vaults: [{ vaultId: "deploy", name: "Deploy", role: "owner", canOpen: true }, { vaultId: "secret-a", name: "", role: "reader", canOpen: false }, { vaultId: "secret-b", name: "", role: "reader", canOpen: false }] }),
      organization.teams[1]!,
    ] });
    const lines = describeOrganizationAccessChange(changed, { kind: "remove-team", teamID: "ops" });
    expect(lines).toContain("Deploy: Bob Stone loses access.");
    expect(lines).toContain("Deploy: Your role changes from Owner to Editor.");
    expect(lines.some((line) => line.startsWith("A vault you can't open (secret-a)"))).toBe(true);
    expect(lines.some((line) => line.startsWith("A vault you can't open (secret-b)"))).toBe(true);
    const leave = describeOrganizationAccessChange(changed, { kind: "remove-member", accountID: "ALICE1" });
    expect(leave.join(" ")).toContain("You lose access");
    expect(leave.join(" ")).not.toContain("Bob Stone");
  });

  it("compares each person's strongest remaining team role", () => {
    const lines = describeOrganizationAccessChange(organization, { kind: "team-role", teamID: "ops", vaultID: "deploy", role: "reader" });
    expect(lines.join(" ")).toContain("Your role changes from Owner to Editor");
    expect(lines.join(" ")).toContain("Bob Stone's role changes from Owner to Reader");
  });
});
