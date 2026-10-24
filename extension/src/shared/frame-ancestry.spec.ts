import { describe, expect, it } from "vitest";
import { readAncestorOrigins } from "./frame-ancestry";

interface FakeFrame {
  origin: string;
  isReadable?: boolean;
  ancestorOrigins?: string[];
}

// createFrameChain builds windows from the top frame down and returns the innermost one. A frame that is not readable throws
// on location.origin, as a frame of another origin does in Firefox.
function createFrameChain(allFrame: FakeFrame[]): Window {
  let parent: Window | undefined;
  let top: Window | undefined;
  for (const frame of allFrame) {
    const location = {
      get origin() {
        if (frame.isReadable === false) throw new DOMException("Permission denied", "SecurityError");
        return frame.origin;
      },
      ancestorOrigins: frame.ancestorOrigins,
    };
    const window = { location } as unknown as Window & { parent: Window; top: Window };
    top ??= window;
    window.top = top;
    window.parent = parent ?? window;
    parent = window;
  }
  return parent!;
}

describe("readAncestorOrigins", () => {
  it("uses location.ancestorOrigins where the browser has it", () => {
    const frame = createFrameChain([{ origin: "https://top.example" }, { origin: "https://shop.example", ancestorOrigins: ["https://top.example"] }]);
    expect(readAncestorOrigins(frame)).toEqual(["https://top.example"]);
  });

  it("has no ancestors in the top frame", () => {
    expect(readAncestorOrigins(createFrameChain([{ origin: "https://shop.example" }]))).toEqual([]);
  });

  it("reads each parent's origin in Firefox, the parent first", () => {
    const frame = createFrameChain([{ origin: "https://shop.example" }, { origin: "https://shop.example" }, { origin: "https://shop.example" }]);
    expect(readAncestorOrigins(frame)).toEqual(["https://shop.example", "https://shop.example"]);
  });

  it("counts a parent of another origin as null, so a frame inside another site never passes the frame rule", () => {
    const frame = createFrameChain([{ origin: "https://evil.example", isReadable: false }, { origin: "https://shop.example" }]);
    const allAncestorOrigin = readAncestorOrigins(frame);
    expect(allAncestorOrigin).toEqual(["null"]);
    expect(allAncestorOrigin.every((origin) => origin === "https://shop.example")).toBe(false);
  });
});
