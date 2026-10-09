import { afterEach, describe, expect, it, vi } from "vitest";
import { computeSecondsLeft, formatOneTimeCode } from "../shared/one-time-code";
import { copyText, isOffscreenCopyMessage } from "./copy";

afterEach(() => {
  vi.restoreAllMocks();
  document.body.innerHTML = "";
});

describe("offscreen copy", () => {
  it("copies through a selected text area and removes it again", () => {
    let selectedText = "";
    const execCommand = vi.fn((command: string) => {
      selectedText = (document.activeElement as HTMLTextAreaElement | null)?.value ?? document.querySelector("textarea")?.value ?? "";
      return command === "copy";
    });
    Object.defineProperty(document, "execCommand", { configurable: true, value: execCommand });
    expect(copyText(document, "123456")).toBe(true);
    expect(execCommand).toHaveBeenCalledWith("copy");
    expect(selectedText).toBe("123456");
    expect(document.querySelector("textarea")).toBeNull();
  });

  it("accepts copy requests only from the extension's service worker", () => {
    const message = { target: "offscreen", type: "offscreen.copy", text: "123456" };
    expect(isOffscreenCopyMessage(message, { id: "extension" }, "extension")).toBe(true);
    expect(isOffscreenCopyMessage(message, { id: "extension", tab: { id: 1 } }, "extension")).toBe(false);
    expect(isOffscreenCopyMessage(message, { id: "other" }, "extension")).toBe(false);
    expect(isOffscreenCopyMessage({ ...message, text: 1 }, { id: "extension" }, "extension")).toBe(false);
    expect(isOffscreenCopyMessage({ target: "inline-menu", type: "inlineMenu.pressKey" }, { id: "extension" }, "extension")).toBe(false);
  });
});

describe("one-time code display", () => {
  it("splits the code in the middle and counts the seconds left", () => {
    expect(formatOneTimeCode("123456")).toBe("123 456");
    expect(formatOneTimeCode("12345678")).toBe("1234 5678");
    expect(computeSecondsLeft("2026-10-09T12:00:30Z", Date.parse("2026-10-09T12:00:20.5Z"))).toBe(10);
    expect(computeSecondsLeft("2026-10-09T12:00:30Z", Date.parse("2026-10-09T12:00:31Z"))).toBe(0);
    expect(computeSecondsLeft("not a date", 0)).toBe(0);
  });
});
