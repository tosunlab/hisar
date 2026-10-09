import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";
import packageJson from "../package.json";
import manifest from "../public/manifest.json";

function computeExtensionID(publicKey: string): string {
  const digest = createHash("sha256").update(Buffer.from(publicKey, "base64")).digest("hex").slice(0, 32);
  return Array.from(digest, (character) => String.fromCharCode("a".charCodeAt(0) + parseInt(character, 16))).join("");
}

describe("manifest.json", () => {
  it("has the key that gives the extension ID from the design", () => {
    expect(computeExtensionID(manifest.key)).toBe("mmipiffoogblblocokimpemdopmdjghf");
  });

  it("matches the native host's allowed origin in the fake host registration", () => {
    const registerScript = readFileSync(resolve(process.cwd(), "tools/register-fake-host.mjs"), "utf8");
    expect(registerScript).toContain(`"mmipiffoogblblocokimpemdopmdjghf"`);
  });

  it("asks for exactly the permissions in the design", () => {
    expect(manifest.permissions).toEqual([
      "nativeMessaging",
      "storage",
      "scripting",
      "activeTab",
      "tabs",
    ]);
    expect(manifest.host_permissions).toEqual(["<all_urls>"]);
  });

  it("runs the content script in all frames and offers the fill-login shortcut", () => {
    expect(manifest.content_scripts[0]).toMatchObject({ matches: ["<all_urls>"], all_frames: true, js: ["content.js"] });
    expect(manifest.commands["fill-login"].suggested_key).toEqual({ default: "Ctrl+Shift+L", mac: "Command+Shift+L" });
  });

  it("exposes only the inline menu page to web pages, through a dynamic URL", () => {
    expect(manifest.web_accessible_resources).toEqual([{ resources: ["inline-menu.html"], matches: ["<all_urls>"], use_dynamic_url: true }]);
  });

  it("has the same version as package.json", () => {
    expect(manifest.version).toBe(packageJson.version);
  });
});
