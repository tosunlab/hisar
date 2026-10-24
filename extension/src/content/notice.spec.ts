import { afterEach, describe, expect, it, vi } from "vitest";
import { noticeMilliseconds } from "../shared/messages";
import { showNotice } from "./notice";

afterEach(() => {
  vi.useRealTimers();
  document.querySelector("hisar-notice")?.remove();
});

describe("showNotice", () => {
  it("shows one notice at a time and removes it after a few seconds", () => {
    vi.useFakeTimers();
    showNotice(document, "No login form found on this page");
    showNotice(document, "No saved login for this site");
    expect(document.querySelectorAll("hisar-notice")).toHaveLength(1);
    vi.advanceTimersByTime(noticeMilliseconds);
    expect(document.querySelector("hisar-notice")).toBeNull();
  });

  it("keeps its text out of the page's reach", () => {
    showNotice(document, "No login form found on this page");
    const host = document.querySelector("hisar-notice")!;
    expect(host.shadowRoot).toBeNull();
    expect(document.documentElement.textContent).not.toContain("No login form found");
  });
});
