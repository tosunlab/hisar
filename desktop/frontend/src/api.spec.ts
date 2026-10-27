import { describe, expect, it } from "vitest";
import { getErrorMessage } from "./api";

describe("getErrorMessage", () => {
  it.each([
    [new Error("incorrect master password"), "Incorrect master password."],
    ["the Secret Key is not valid", "The Secret Key is not valid."],
    ["Try again.", "Try again."],
    ["try again?", "Try again?"],
    ["stop!", "Stop!"],
    [undefined, "Something went wrong."],
    ["  ", "Something went wrong."],
  ])("formats prose without changing its source", (error, expected) => {
    expect(getErrorMessage(error)).toBe(expected);
  });

  it("preserves the original error and its structured code", () => {
    const error = Object.assign(new Error("incorrect master password"), { code: "authentication_failed" });
    getErrorMessage(error);
    expect(error.message).toBe("incorrect master password");
    expect(error.code).toBe("authentication_failed");
  });
});
