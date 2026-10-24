#!/usr/bin/env node
// Builds dist-firefox from the Chrome build in dist: the same files, with the Firefox manifest. Run after vite build.
import { cpSync, existsSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

export const firefoxExtensionID = "extension@tryhisar.com";
// Firefox 140, the extended support release, is the first to read data_collection_permissions; the passkey wrapper's MAIN
// world content script needs 128.
export const firefoxMinimumVersion = "140.0";
// Unlisted add-ons update from a manifest Hisar hosts: publishing a Firefox release rewrites this file.
export const firefoxUpdateURL = "https://github.com/tosunlab/hisar/releases/download/extension-firefox-updates/updates.json";

// toFirefoxManifest derives the Firefox manifest from Chrome's. Firefox runs the background as an event page, has no
// offscreen documents or dynamic resource URLs (its moz-extension origin is already random per install), and names the
// add-on by browser_specific_settings instead of a key.
export function toFirefoxManifest(chromeManifest) {
  const { key: _key, minimum_chrome_version: _minimumChromeVersion, background: _background, ...manifest } = chromeManifest;
  return {
    ...manifest,
    background: { scripts: ["service-worker.js"], type: "module" },
    permissions: manifest.permissions.filter((permission) => permission !== "offscreen"),
    web_accessible_resources: manifest.web_accessible_resources.map(({ use_dynamic_url: _useDynamicURL, ...resource }) => resource),
    browser_specific_settings: {
      gecko: {
        id: firefoxExtensionID,
        strict_min_version: firefoxMinimumVersion,
        update_url: firefoxUpdateURL,
        // Logins and the rest go only to the Hisar app on the same computer, over native messaging; nothing is sent to a server.
        data_collection_permissions: { required: ["none"] },
      },
    },
  };
}

export function buildFirefox(extensionDirectory) {
  const distDirectory = join(extensionDirectory, "dist");
  const firefoxDirectory = join(extensionDirectory, "dist-firefox");
  if (!existsSync(join(distDirectory, "manifest.json"))) throw new Error("dist/manifest.json is missing: run vite build first");
  rmSync(firefoxDirectory, { recursive: true, force: true });
  cpSync(distDirectory, firefoxDirectory, { recursive: true });
  rmSync(join(firefoxDirectory, "offscreen.html"));
  const chromeManifest = JSON.parse(readFileSync(join(distDirectory, "manifest.json"), "utf8"));
  writeFileSync(join(firefoxDirectory, "manifest.json"), `${JSON.stringify(toFirefoxManifest(chromeManifest), null, 2)}\n`);
  return firefoxDirectory;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  console.log(buildFirefox(resolve(dirname(fileURLToPath(import.meta.url)), "..")));
}
