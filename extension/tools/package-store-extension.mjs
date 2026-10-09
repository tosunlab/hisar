#!/usr/bin/env node
// Builds the Chrome Web Store upload: the store rejects a manifest "key", so it is removed here.
// The first upload must carry the private key as key.pem so the store keeps the ID mmipiffoogblblocokimpemdopmdjghf:
//   npm run package:store -- --key-file ~/.hisar/chrome-web-store-key.pem
// Later uploads must not include it.
import { execFileSync } from "node:child_process";
import { copyFileSync, cpSync, existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { homedir, tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const extensionDirectory = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const distDirectory = join(extensionDirectory, "dist");
if (!existsSync(join(distDirectory, "manifest.json"))) {
  console.error("dist/manifest.json is missing: run npm run build first");
  process.exit(1);
}
const keyFileIndex = process.argv.indexOf("--key-file");
const keyFile = keyFileIndex > 0 ? process.argv[keyFileIndex + 1]?.replace(/^~(?=\/)/, homedir()) : undefined;
if (keyFileIndex > 0 && (!keyFile || !existsSync(keyFile))) {
  console.error(`--key-file ${keyFile ?? ""} does not exist`);
  process.exit(1);
}

const stagingDirectory = mkdtempSync(join(tmpdir(), "hisar-store-"));
try {
  cpSync(distDirectory, stagingDirectory, { recursive: true });
  const manifestPath = join(stagingDirectory, "manifest.json");
  const manifest = JSON.parse(readFileSync(manifestPath, "utf8"));
  delete manifest.key;
  writeFileSync(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);
  if (keyFile) {
    copyFileSync(keyFile, join(stagingDirectory, "key.pem"));
  }
  const suffix = keyFile ? "store-first-upload" : "store";
  const zipPath = join(extensionDirectory, `hisar-extension-${manifest.version}-${suffix}.zip`);
  rmSync(zipPath, { force: true });
  execFileSync("zip", ["-r", "-X", "-q", zipPath, "."], { cwd: stagingDirectory, stdio: "inherit" });
  console.log(zipPath);
  if (keyFile) {
    console.log("This zip contains the private key: upload it once, then delete it.");
  }
} finally {
  rmSync(stagingDirectory, { recursive: true, force: true });
}
