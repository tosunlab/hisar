import { describe, expect, it } from "vitest";
import { createMemoryArea } from "../background/testing";
import { VisibilityGuard } from "./click-guard";
import { parseInlineMenuSettings, readInlineMenuSettings, writeInlineMenuSetting } from "./settings";

describe("inline menu settings", () => {
  it("defaults to on and ignores values that are not booleans", () => {
    expect(parseInlineMenuSettings({})).toEqual({ isInlineMenuEnabled: true, isInlineMenuOpenedOnFocus: true });
    expect(parseInlineMenuSettings({ isInlineMenuEnabled: "no", isInlineMenuOpenedOnFocus: false })).toEqual({
      isInlineMenuEnabled: true,
      isInlineMenuOpenedOnFocus: false,
    });
  });

  it("writes and reads back each setting", async () => {
    const area = createMemoryArea();
    await writeInlineMenuSetting(area, "isInlineMenuEnabled", false);
    expect(area.data).toEqual({ isInlineMenuEnabled: false });
    expect(await readInlineMenuSettings(area)).toEqual({ isInlineMenuEnabled: false, isInlineMenuOpenedOnFocus: true });
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
