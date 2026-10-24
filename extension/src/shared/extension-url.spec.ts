import { describe, expect, it } from "vitest";
import { isExtensionPageUrl } from "./extension-url";

describe("isExtensionPageUrl", () => {
  it("accepts this browser's extension scheme with any ID, at the given path", () => {
    expect(isExtensionPageUrl("chrome-extension://dynamic-id/inline-menu.html#s", "chrome-extension://static-id/", "/inline-menu.html")).toBe(true);
    expect(isExtensionPageUrl("moz-extension://uuid/passkey-prompt.html#s", "moz-extension://uuid/", "/passkey-prompt.html")).toBe(true);
    expect(isExtensionPageUrl("moz-extension://uuid/inline-menu.html", "moz-extension://uuid/")).toBe(true);
  });

  it("refuses another scheme, another path, a web page and a malformed URL", () => {
    expect(isExtensionPageUrl("moz-extension://uuid/inline-menu.html", "chrome-extension://static-id/", "/inline-menu.html")).toBe(false);
    expect(isExtensionPageUrl("chrome-extension://id/popup.html", "chrome-extension://id/", "/inline-menu.html")).toBe(false);
    expect(isExtensionPageUrl("https://chrome-extension.example/inline-menu.html", "chrome-extension://id/", "/inline-menu.html")).toBe(false);
    expect(isExtensionPageUrl(undefined, "chrome-extension://id/")).toBe(false);
    expect(isExtensionPageUrl("not a url", "chrome-extension://id/")).toBe(false);
  });
});
