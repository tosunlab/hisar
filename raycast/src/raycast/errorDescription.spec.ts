import { describe, expect, it } from "vitest";
import { HisarError } from "../hisar";
import { describeHisarError } from "./errorDescription";

describe("describeHisarError", () => {
  it("offers Update Hisar when Hisar needs an update, with the app's reason", () => {
    const reason = "Hisar needs an update: the server requires version 1.4.0 or later.";
    expect(describeHisarError(new HisarError("update-required", reason))).toEqual({
      title: "Hisar needs an update",
      message: reason,
      action: "update-hisar",
    });
  });

  it("offers Open Hisar when Hisar must be opened to go on", () => {
    for (const code of ["integration-disabled", "signed-out", "locked", "app-not-running"] as const) {
      expect(describeHisarError(new HisarError(code, "")).action, code).toBe("open-hisar");
    }
  });

  it("offers nothing to open when Hisar answered and refused", () => {
    const description = describeHisarError(new HisarError("not-found", "“Office Wi-Fi” has no password"));
    expect(description).toEqual({ title: "Not found in Hisar", message: "“Office Wi-Fi” has no password" });
  });
});
