import { describe, expect, it } from "vitest";
import { getBrowserName, getHost, getOrigin, isWebUrl } from "./url";

describe("url helpers", () => {
  it("accepts only http and https pages", () => {
    expect(isWebUrl("https://example.com/")).toBe(true);
    expect(isWebUrl("http://localhost:8080/")).toBe(true);
    expect(isWebUrl("chrome://settings")).toBe(false);
    expect(isWebUrl("file:///tmp/a.html")).toBe(false);
    expect(isWebUrl("not a url")).toBe(false);
    expect(isWebUrl(undefined)).toBe(false);
  });

  it("returns origins and hosts", () => {
    expect(getOrigin("https://Example.com:443/a?b")).toBe("https://example.com");
    expect(getOrigin("nonsense")).toBe("null");
    expect(getHost("http://127.0.0.1:8080/x")).toBe("127.0.0.1:8080");
  });
});

describe("getBrowserName", () => {
  it.each([
    [[{ brand: "Chromium" }, { brand: "Google Chrome" }], "", false, "Chrome"],
    [[{ brand: "Chromium" }, { brand: "Microsoft Edge" }], "", false, "Edge"],
    [[{ brand: "Chromium" }], "Mozilla/5.0 Chrome/140 Safari/537.36", false, "Chromium"],
    [[{ brand: "Chromium" }], "", true, "Brave"],
    [[], "Mozilla/5.0 Chrome/140 Vivaldi/7.0", false, "Vivaldi"],
  ])("brands %j user agent %s brave %s gives %s", (brands, userAgent, isBrave, expected) => {
    expect(getBrowserName({ brands }, userAgent, isBrave)).toBe(expected);
  });
});
