import { describe, expect, it } from "vitest";
import { afterApprovalContext, isReopeningAfterApproval } from "./afterApproval";

describe("isReopeningAfterApproval", () => {
  it("brings the command back when it loaded after Hisar's approval window took focus", () => {
    expect(isReopeningAfterApproval({ wasApprovalNeeded: true, isLoaded: true })).toBe(true);
    expect(isReopeningAfterApproval({ wasApprovalNeeded: true, isLoaded: true, launchContext: {} })).toBe(true);
  });

  it("stays put when no approval window opened, or the load failed", () => {
    expect(isReopeningAfterApproval({ wasApprovalNeeded: false, isLoaded: true })).toBe(false);
    expect(isReopeningAfterApproval({ wasApprovalNeeded: true, isLoaded: false })).toBe(false);
    expect(isReopeningAfterApproval({ wasApprovalNeeded: false, isLoaded: false })).toBe(false);
  });

  it("never reopens the launch that is the reopening, so it cannot loop", () => {
    expect(
      isReopeningAfterApproval({ wasApprovalNeeded: true, isLoaded: true, launchContext: afterApprovalContext }),
    ).toBe(false);
  });
});
