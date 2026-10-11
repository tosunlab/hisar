import { describe, expect, it } from "vitest";
import { isErrorCode } from "./protocol";

describe("error codes", () => {
  it("knows update-required, so the popup keeps the desktop's code", () => {
    expect(isErrorCode("update-required")).toBe(true);
    expect(isErrorCode("upgrade-required")).toBe(false);
  });
});
