#!/usr/bin/env node
import { execFileSync } from "node:child_process";
import { existsSync, readFileSync, rmSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const extensionDirectory = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const distDirectory = join(extensionDirectory, "dist");
const manifestPath = join(distDirectory, "manifest.json");
if (!existsSync(manifestPath)) {
  console.error("dist/manifest.json is missing: run npm run build first");
  process.exit(1);
}
const { version } = JSON.parse(readFileSync(manifestPath, "utf8"));
const packageVersion = JSON.parse(readFileSync(join(extensionDirectory, "package.json"), "utf8")).version;
if (version !== packageVersion) {
  console.error(`manifest.json version ${version} differs from package.json version ${packageVersion}`);
  process.exit(1);
}
const zipPath = join(extensionDirectory, `hisar-extension-${version}.zip`);
rmSync(zipPath, { force: true });
execFileSync("zip", ["-r", "-X", "-q", zipPath, "."], { cwd: distDirectory, stdio: "inherit" });
console.log(zipPath);
