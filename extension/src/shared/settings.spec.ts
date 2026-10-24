import { describe, expect, it } from "vitest";
import { createMemoryArea } from "../background/testing";
import { VisibilityGuard } from "./click-guard";
import { parsePageSettings, readPageSettings, writePageSetting } from "./settings";

describe("page settings", () => {
  it("defaults to the menu on and diagnostics off, and ignores values that are not booleans", () => {
    expect(parsePageSettings({})).toEqual({ isInlineMenuEnabled: true, isInlineMenuOpenedOnFocus: true, isFillDiagnosticsEnabled: false });
    expect(parsePageSettings({ isInlineMenuEnabled: "no", isInlineMenuOpenedOnFocus: false, isFillDiagnosticsEnabled: 1 })).toEqual({
      isInlineMenuEnabled: true,
      isInlineMenuOpenedOnFocus: false,
      isFillDiagnosticsEnabled: false,
    });
  });

  it("writes and reads back each setting", async () => {
    const area = createMemoryArea();
    await writePageSetting(area, "isInlineMenuEnabled", false);
    expect(area.data).toEqual({ isInlineMenuEnabled: false });
    expect(await readPageSettings(area)).toEqual({ isInlineMenuEnabled: false, isInlineMenuOpenedOnFocus: true, isFillDiagnosticsEnabled: false });
  });
});

describe("VisibilityGuard", () => {
  it("allows interaction only after 300 ms of continuous visibility", () => {
    const guard = new VisibilityGuard();
    expect(guard.isInteractionAllowed(0)).toBe(false);
    guard.recordVisibility(true, 100);
    guard.recordVisibility(true, 200);
    expect(guard.isInteractionAllowed(399)).toBe(false);
    expect(guard.isInteractionAllowed(400)).toBe(true);
  });

  it("starts over when the menu is hidden or covered, and after a reset", () => {
    const guard = new VisibilityGuard();
    guard.recordVisibility(true, 0);
    guard.recordVisibility(false, 250);
    guard.recordVisibility(true, 260);
    expect(guard.isInteractionAllowed(500)).toBe(false);
    expect(guard.isInteractionAllowed(560)).toBe(true);
    guard.reset();
    expect(guard.isInteractionAllowed(1000)).toBe(false);
  });
});
