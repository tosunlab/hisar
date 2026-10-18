import { describe, expect, it } from "vitest";
import { computeIconRect, computeMenuRect, iconSize, isFieldLargeEnough, isRectInViewport } from "./placement";

const viewport = { width: 1000, height: 800 };

describe("placement", () => {
  it("puts the icon inside the right edge of the field, centred vertically", () => {
    expect(computeIconRect({ left: 100, top: 200, width: 300, height: 40 })).toEqual({
      left: 100 + 300 - iconSize - 8,
      top: 210,
      width: iconSize,
      height: iconSize,
    });
  });

  it("puts the menu under the field, at least 280 px wide", () => {
    expect(computeMenuRect({ left: 100, top: 200, width: 200, height: 40 }, 150, viewport)).toEqual({ left: 100, top: 244, width: 280, height: 150 });
    expect(computeMenuRect({ left: 100, top: 200, width: 320, height: 40 }, 150, viewport).width).toBe(320);
    expect(computeMenuRect({ left: 100, top: 200, width: 900, height: 40 }, 150, viewport).width).toBe(360);
  });

  it("flips the menu above the field when there is no room below", () => {
    expect(computeMenuRect({ left: 100, top: 700, width: 300, height: 40 }, 150, viewport).top).toBe(700 - 4 - 150);
  });

  it("takes the side with more room when it fits neither, cut to that room so it stays in the window", () => {
    expect(computeMenuRect({ left: 100, top: 100, width: 300, height: 40 }, 700, viewport)).toMatchObject({ top: 144, height: 800 - 8 - 144 });
    expect(computeMenuRect({ left: 100, top: 600, width: 300, height: 40 }, 700, viewport)).toMatchObject({ top: 8, height: 600 - 4 - 8 });
  });

  it("keeps the menu inside the viewport horizontally", () => {
    expect(computeMenuRect({ left: 900, top: 100, width: 90, height: 30 }, 100, viewport).left).toBe(1000 - 280 - 8);
    expect(computeMenuRect({ left: -50, top: 100, width: 300, height: 30 }, 100, viewport).left).toBe(8);
    expect(computeMenuRect({ left: 0, top: 100, width: 300, height: 30 }, 100, { width: 300, height: 800 }).width).toBe(284);
  });

  it("rejects tiny fields and detects fields outside the viewport", () => {
    expect(isFieldLargeEnough({ left: 0, top: 0, width: 59, height: 30 })).toBe(false);
    expect(isFieldLargeEnough({ left: 0, top: 0, width: 200, height: 15 })).toBe(false);
    expect(isFieldLargeEnough({ left: 0, top: 0, width: 200, height: 30 })).toBe(true);
    expect(isRectInViewport({ left: 0, top: -50, width: 200, height: 40 }, viewport)).toBe(false);
    expect(isRectInViewport({ left: 0, top: 810, width: 200, height: 40 }, viewport)).toBe(false);
    expect(isRectInViewport({ left: 0, top: -20, width: 200, height: 40 }, viewport)).toBe(true);
  });
});
