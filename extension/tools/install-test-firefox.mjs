#!/usr/bin/env node
// Installs the Firefox and geckodriver the Firefox end-to-end tests run, pinned so every run uses the same browser, into
// .test-browsers. A second run finds them there.
import { Browser, detectBrowserPlatform, install } from "@puppeteer/browsers";
import { download } from "geckodriver";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

export const testFirefoxBuildID = "esr_153.5.1esr";
export const testGeckodriverVersion = "0.37.1";
const cacheDirectory = join(resolve(dirname(fileURLToPath(import.meta.url)), ".."), ".test-browsers");

// HISAR_TEST_FIREFOX names another build, such as esr_140.17.0esr, the oldest the add-on supports.
export async function installTestFirefox(buildId = process.env.HISAR_TEST_FIREFOX || testFirefoxBuildID) {
  const firefox = await install({ browser: Browser.FIREFOX, buildId, cacheDir: cacheDirectory, platform: detectBrowserPlatform() });
  const geckodriverPath = await download(testGeckodriverVersion, join(cacheDirectory, "geckodriver"));
  return { firefoxPath: firefox.executablePath, geckodriverPath };
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  console.log(await installTestFirefox());
}
