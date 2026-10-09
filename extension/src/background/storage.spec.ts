import { describe, expect, it } from "vitest";
import { ExtensionStorage } from "./storage";
import { createMemoryArea } from "./testing";

describe("ExtensionStorage", () => {
  it("creates the instance ID once and keeps it", async () => {
    const area = createMemoryArea();
    let counter = 0;
    const storage = new ExtensionStorage(area, () => `uuid-${(counter += 1)}`);
    expect(await storage.getInstanceID()).toBe("uuid-1");
    expect(await storage.getInstanceID()).toBe("uuid-1");
    expect(area.data).toEqual({ instanceID: "uuid-1" });
  });

  it("stores and removes the token", async () => {
    const area = createMemoryArea();
    const storage = new ExtensionStorage(area, () => "id");
    expect(await storage.getToken()).toBeUndefined();
    await storage.setToken("secret-token");
    expect(await storage.getToken()).toBe("secret-token");
    await storage.removeToken();
    expect(await storage.getToken()).toBeUndefined();
  });

  it("keeps the never list without duplicates", async () => {
    const area = createMemoryArea();
    const storage = new ExtensionStorage(area, () => "id");
    await storage.addNeverOrigin("https://example.com");
    await storage.addNeverOrigin("https://example.com");
    await storage.addNeverOrigin("https://example.org");
    expect(await storage.getAllNeverOrigin()).toEqual(["https://example.com", "https://example.org"]);
    expect(await storage.isNeverOrigin("https://example.org")).toBe(true);
    expect(await storage.isNeverOrigin("https://example.net")).toBe(false);
  });

  it("stores only the instance ID, the token and the never list", async () => {
    const area = createMemoryArea();
    const storage = new ExtensionStorage(area, () => "id");
    await storage.getInstanceID();
    await storage.setToken("t");
    await storage.addNeverOrigin("https://example.com");
    expect(Object.keys(area.data).sort()).toEqual(["allNeverOrigin", "instanceID", "token"]);
  });
});
