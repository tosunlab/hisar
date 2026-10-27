import { describe, expect, it } from "vitest";
import { groupVaultDestinations } from "./vaultDestinations";

describe("Vault destinations", () => {
  it("distinguishes shared vaults, multiple organizations and duplicate names in one group", () => {
    const groups = groupVaultDestinations([
      { id: "shared-a", name: "Production", type: "shared", role: "owner", itemCount: 0 },
      { id: "shared-b", name: "Production", type: "shared", role: "owner", itemCount: 0 },
      { id: "north", name: "Production", type: "organization", organizationId: "northwind", role: "owner", itemCount: 0 },
      { id: "south", name: "Production", type: "organization", organizationId: "southwind", role: "owner", itemCount: 0 },
    ], [{ id: "northwind", name: "Northwind" }, { id: "southwind", name: "Southwind" }]);
    expect(groups.map((group) => group.name)).toEqual(["Northwind", "Shared vault", "Southwind"]);
    expect(groups.flatMap((group) => group.destinations.map((destination) => destination.label))).toEqual([
      "Northwind / Production", "Shared vault / Production (shared-a)", "Shared vault / Production (shared-b)", "Southwind / Production",
    ]);
    expect(groups[0].destinations[0].id).toBe("north");
    expect(groups[0].destinations[0].context).toBe("Shared through teams in Northwind.");
  });
});
