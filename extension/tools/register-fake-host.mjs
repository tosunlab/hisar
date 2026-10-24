#!/usr/bin/env node
import { chmodSync, existsSync, mkdirSync, writeFileSync } from "node:fs";
import { homedir, platform } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { createInitialState, writeState } from "./fake-native-host.mjs";

export const extensionID = "mmipiffoogblblocokimpemdopmdjghf";
export const firefoxExtensionID = "extension@tryhisar.com";
export const hostName = "com.tryhisar.hisar";
const toolsDirectory = dirname(fileURLToPath(import.meta.url));

export function getDefaultUserDataDirectory(browser = "chrome") {
  const allMacDirectory = { chrome: "Google/Chrome", chromium: "Chromium", brave: "BraveSoftware/Brave-Browser", edge: "Microsoft Edge" };
  const allLinuxDirectory = { chrome: "google-chrome", chromium: "chromium", brave: "BraveSoftware/Brave-Browser", edge: "microsoft-edge" };
  if (platform() === "darwin") return join(homedir(), "Library/Application Support", allMacDirectory[browser]);
  if (platform() === "linux") return join(homedir(), ".config", allLinuxDirectory[browser]);
  throw new Error("Registering the fake host is supported on macOS and Linux only");
}

function quoteForShell(value) {
  return `'${value.replaceAll("'", "'\\''")}'`;
}

function writeLauncher(hostDirectory, stateFile) {
  mkdirSync(hostDirectory, { recursive: true });
  const launcherPath = join(hostDirectory, "hisar-fake-host.sh");
  const scriptPath = join(toolsDirectory, "fake-native-host.mjs");
  writeFileSync(
    launcherPath,
    [
      "#!/bin/sh",
      `HISAR_FAKE_HOST_STATE=${quoteForShell(stateFile)}`,
      "export HISAR_FAKE_HOST_STATE",
      `exec ${quoteForShell(process.execPath)} ${quoteForShell(scriptPath)} "$@"`,
      "",
    ].join("\n"),
  );
  chmodSync(launcherPath, 0o755);
  return launcherPath;
}

function writeManifest(manifestDirectory, manifest) {
  mkdirSync(manifestDirectory, { recursive: true });
  const manifestPath = join(manifestDirectory, `${hostName}.json`);
  writeFileSync(manifestPath, `${JSON.stringify({ name: hostName, description: "Hisar fake native host for development", type: "stdio", ...manifest }, null, 2)}\n`);
  return manifestPath;
}

export function registerFakeHost({ userDataDirectory, stateFile, hostDirectory, shouldResetState = false }) {
  const launcherPath = writeLauncher(hostDirectory, stateFile);
  const manifestPath = writeManifest(join(userDataDirectory, "NativeMessagingHosts"), { path: launcherPath, allowed_origins: [`chrome-extension://${extensionID}/`] });
  if (shouldResetState || !existsSync(stateFile)) writeState(stateFile, createInitialState());
  return { manifestPath, launcherPath, stateFile };
}

// getFirefoxManifestsDirectory is where Firefox looks for the user's native host manifests (its XREUserNativeManifests),
// which holds NativeMessagingHosts on macOS and native-messaging-hosts on Linux.
export function getFirefoxManifestsDirectory() {
  if (platform() === "darwin") return join(homedir(), "Library/Application Support/Mozilla");
  if (platform() === "linux") return join(homedir(), ".mozilla");
  throw new Error("Registering the fake host is supported on macOS and Linux only");
}

// registerFakeFirefoxHost writes the manifest Firefox reads, which names the add-on ID instead of an origin. Firefox has no
// per-profile manifests, so tests pass their own manifestsDirectory and point Firefox at it.
export function registerFakeFirefoxHost({ manifestsDirectory, stateFile, hostDirectory, shouldResetState = false }) {
  const launcherPath = writeLauncher(hostDirectory, stateFile);
  const directoryName = platform() === "linux" ? "native-messaging-hosts" : "NativeMessagingHosts";
  const manifestPath = writeManifest(join(manifestsDirectory, directoryName), { path: launcherPath, allowed_extensions: [firefoxExtensionID] });
  if (shouldResetState || !existsSync(stateFile)) writeState(stateFile, createInitialState());
  return { manifestPath, launcherPath, stateFile };
}

function parseArguments(allArgument) {
  const options = {};
  for (let index = 0; index < allArgument.length; index += 1) {
    const argument = allArgument[index];
    if (argument === "--reset") options.shouldResetState = true;
    else if (argument === "--replace-hisar-registration") options.shouldReplaceHisarRegistration = true;
    else if (argument.startsWith("--")) options[argument.slice(2)] = allArgument[++index];
  }
  return options;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const options = parseArguments(process.argv.slice(2));
  // Firefox reads one manifest directory for every profile, so it always replaces the Hisar app's registration there.
  if ((!options["user-data-dir"] || options.browser === "firefox") && !options.shouldReplaceHisarRegistration) {
    console.error("Pass --user-data-dir <directory> to register the fake host for a separate Chrome profile.");
    console.error("Registering it for your normal browser profile replaces the Hisar app's com.tryhisar.hisar.json;");
    console.error("add --replace-hisar-registration if you mean that (Hisar writes its own manifest back at its next launch).");
    process.exit(1);
  }
  const extensionDirectory = resolve(toolsDirectory, "..");
  const common = {
    stateFile: resolve(options.state ?? join(extensionDirectory, ".fake-host/state.json")),
    hostDirectory: resolve(options["host-dir"] ?? join(extensionDirectory, ".fake-host")),
    shouldResetState: options.shouldResetState === true,
  };
  const result =
    options.browser === "firefox"
      ? registerFakeFirefoxHost({ ...common, manifestsDirectory: getFirefoxManifestsDirectory() })
      : registerFakeHost({ ...common, userDataDirectory: options["user-data-dir"] ? resolve(options["user-data-dir"]) : getDefaultUserDataDirectory(options.browser) });
  console.log(`Host manifest: ${result.manifestPath}`);
  console.log(`Launcher:      ${result.launcherPath}`);
  console.log(`State file:    ${result.stateFile}`);
}
