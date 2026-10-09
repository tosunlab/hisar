import { describe, expect, it } from "vitest";
import type { FrameProbe } from "../shared/messages";
import { chooseFocusedFrame, chooseLoginFrame, selectAllFillableFrame, type FrameInfo } from "./frames";

const emptyProbe: FrameProbe = { hasLoginForm: false, hasUsernameField: false, hasFocusedField: false, hasFocusedLoginField: false };

function createFrame(frameId: number, url: string, allAncestorOrigin: string[] = []): FrameInfo {
  return { frameId, url, origin: url.startsWith("about:") ? "null" : new URL(url).origin, allAncestorOrigin };
}

describe("selectAllFillableFrame", () => {
  const top = createFrame(0, "https://accounts.example.com/login");

  it("keeps the top frame and same-origin frames", () => {
    const sameOrigin = createFrame(3, "https://accounts.example.com/frame", ["https://accounts.example.com"]);
    expect(selectAllFillableFrame([top, sameOrigin]).map((frame) => frame.frameId)).toEqual([0, 3]);
  });

  it("drops cross-origin frames even on the same site", () => {
    const allFrame = [
      top,
      createFrame(1, "https://example.com/login", ["https://accounts.example.com"]),
      createFrame(2, "http://accounts.example.com/login", ["https://accounts.example.com"]),
      createFrame(3, "https://accounts.example.com:8443/login", ["https://accounts.example.com"]),
      createFrame(4, "https://evil.example.net/login", ["https://accounts.example.com"]),
    ];
    expect(selectAllFillableFrame(allFrame).map((frame) => frame.frameId)).toEqual([0]);
  });

  it("drops a same-origin frame nested in a cross-origin frame", () => {
    const nested = createFrame(5, "https://accounts.example.com/inner", ["https://ads.example.net", "https://accounts.example.com"]);
    expect(selectAllFillableFrame([top, nested]).map((frame) => frame.frameId)).toEqual([0]);
  });

  it("drops opaque-origin frames such as sandboxed iframes", () => {
    const sandboxed = { ...createFrame(6, "https://accounts.example.com/sandboxed", ["https://accounts.example.com"]), origin: "null" };
    expect(selectAllFillableFrame([top, sandboxed]).map((frame) => frame.frameId)).toEqual([0]);
  });

  it("returns nothing for non-web top frames or a missing top frame", () => {
    expect(selectAllFillableFrame([createFrame(0, "chrome-extension://abc/popup.html")])).toEqual([]);
    expect(selectAllFillableFrame([createFrame(2, "https://example.com/")])).toEqual([]);
  });
});

describe("chooseLoginFrame", () => {
  const top = createFrame(0, "https://example.com/");
  const child = createFrame(2, "https://example.com/child", ["https://example.com"]);

  it("prefers the frame with the focused login field", () => {
    const chosen = chooseLoginFrame([
      { ...top, probe: { ...emptyProbe, hasLoginForm: true } },
      { ...child, probe: { ...emptyProbe, hasLoginForm: true, hasFocusedLoginField: true } },
    ]);
    expect(chosen?.frameId).toBe(2);
  });

  it("prefers the top frame's login form, then any login form, then a username-only form", () => {
    expect(chooseLoginFrame([{ ...child, probe: { ...emptyProbe, hasLoginForm: true } }, { ...top, probe: { ...emptyProbe, hasLoginForm: true } }])?.frameId).toBe(0);
    expect(chooseLoginFrame([{ ...top, probe: emptyProbe }, { ...child, probe: { ...emptyProbe, hasLoginForm: true } }])?.frameId).toBe(2);
    expect(chooseLoginFrame([{ ...top, probe: { ...emptyProbe, hasUsernameField: true } }])?.frameId).toBe(0);
    expect(chooseLoginFrame([{ ...top, probe: emptyProbe }])).toBeUndefined();
  });
});

describe("chooseFocusedFrame", () => {
  it("returns the frame whose document has a focused field", () => {
    const top = { ...createFrame(0, "https://example.com/"), probe: emptyProbe };
    const child = { ...createFrame(1, "https://example.com/c", ["https://example.com"]), probe: { ...emptyProbe, hasFocusedField: true } };
    expect(chooseFocusedFrame([top, child])?.frameId).toBe(1);
    expect(chooseFocusedFrame([top])).toBeUndefined();
  });
});
