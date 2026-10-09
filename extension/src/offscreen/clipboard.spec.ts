import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ClipboardClearer, type ClipboardAccess } from "./clipboard";

function createFakeClipboard(): ClipboardAccess & { content: string | undefined; isReadable: boolean } {
  const clipboard = {
    content: undefined as string | undefined,
    isReadable: true,
    writeText(text: string) {
      clipboard.content = text;
    },
    readText() {
      return clipboard.isReadable ? clipboard.content : undefined;
    },
  };
  return clipboard;
}

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("ClipboardClearer", () => {
  it("clears the clipboard after the delay when it still holds the copied value", () => {
    const clipboard = createFakeClipboard();
    const onIdle = vi.fn();
    const clearer = new ClipboardClearer(clipboard, onIdle);
    clearer.copy("secret", 90000);
    expect(clipboard.content).toBe("secret");
    vi.advanceTimersByTime(89999);
    expect(clipboard.content).toBe("secret");
    vi.advanceTimersByTime(1);
    expect(clipboard.content).toBe("");
    expect(onIdle).toHaveBeenCalledTimes(1);
    expect(clearer.hasPending()).toBe(false);
  });

  it("leaves the clipboard alone when the user copied something else", () => {
    const clipboard = createFakeClipboard();
    new ClipboardClearer(clipboard).copy("secret", 90000);
    clipboard.content = "a recipe";
    vi.advanceTimersByTime(90000);
    expect(clipboard.content).toBe("a recipe");
  });

  it("clears when the clipboard cannot be read back", () => {
    const clipboard = createFakeClipboard();
    new ClipboardClearer(clipboard).copy("secret", 90000);
    clipboard.isReadable = false;
    vi.advanceTimersByTime(90000);
    expect(clipboard.content).toBe("");
  });

  it("restarts the timer for a newer copy and checks against the newest value", () => {
    const clipboard = createFakeClipboard();
    const clearer = new ClipboardClearer(clipboard);
    clearer.copy("first", 90000);
    vi.advanceTimersByTime(60000);
    clearer.copy("second", 90000);
    vi.advanceTimersByTime(60000);
    expect(clipboard.content).toBe("second");
    vi.advanceTimersByTime(30000);
    expect(clipboard.content).toBe("");
  });
});
