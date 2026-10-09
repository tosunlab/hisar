# Hisar Browser Extension

MV3 extension for Chromium browsers that fills and saves logins through the Hisar desktop app over native messaging. The protocol, matching and frame rules are in [docs/DESIGN.md §9.2](../docs/DESIGN.md#92-browser-extension--typescript-mv3). The extension holds no keys and does no crypto.

The manifest's `key` gives the fixed ID `mmipiffoogblblocokimpemdopmdjghf`, both unpacked and from the store. The native host `com.tryhisar.hisar` allows only that ID.

## Layout

```
public/manifest.json      MV3 manifest (version must equal package.json)
popup.html, offscreen.html
src/background/           service worker: native port (native-client.ts), popup/content handlers (controller.ts, router.ts),
                          frame rule (frames.ts), save offers (save.ts), chrome.storage.local (storage.ts)
src/content/              content script (all frames): form detection, fill, submit capture, save bar (closed shadow root)
src/popup/                Vue popup: pairing, desktop unavailable, signed out, locked, logins, generator
src/offscreen/            offscreen document that copies to the clipboard and clears it after 90 s
tools/                    fake native host, its registration script, packaging
end-to-end/               Playwright tests and screenshot capture
```

The build runs Vite twice: once for the popup, the offscreen page and the service worker (an ES module), and once for the content script as a single IIFE file, because content scripts cannot be modules. Icons come from `assets/logo/png` (rendered by `scripts/render-logo.sh`).

## Develop and test

```sh
npm ci
npm run typecheck
npm test                    # Vitest: detection fixtures, fill, frame rule, capture, save bar, service worker, popup, fake host
npm run build               # dist/
npm run package             # hisar-extension-<version>.zip (not committed)
npx playwright install chromium
npm run test:end-to-end     # builds, then runs Playwright with the fake host
```

`npm run test:end-to-end` starts Playwright's Chromium in headless mode. It uses a temporary profile, with the fake host registered in `<profile>/NativeMessagingHosts`, and a local test login page. The test loads the unpacked build, then:

- pairs with the code;
- fills a login (a cross-origin iframe stays empty);
- copies a password;
- updates a changed password and saves a new login from the save bar;
- checks that "Never for this site" works.

To watch it run, use `HISAR_HEADED=1 npm run test:end-to-end`.

`HISAR_SCREENSHOT_DIRECTORY=/tmp/shots npm run screenshots` saves the popup states and the save bar as PNGs. Add `HISAR_CHROME_PATH="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"` to use the installed Chrome. Branded Chrome ignores `--load-extension`, so the script loads the build over the DevTools protocol (`Extensions.loadUnpacked`) instead.

## Load unpacked

1. `npm run build`
2. Open `chrome://extensions`, turn on Developer mode, click **Load unpacked** and choose `extension/dist`.
3. Check that the ID is `mmipiffoogblblocokimpemdopmdjghf`.

With Hisar.app installed and launched once, the extension talks to the real app: Hisar registers its native host in every installed Chromium browser at launch.

## Fake native host

`tools/fake-native-host.mjs` implements the protocol with data from a JSON state file. Edit the file while the host runs to change its state:

- `state`: `unlocked`, `locked` or `signed-out`;
- `isOffline`, `isDesktopUnavailable`;
- `pairingCode`: a fixed code, or `null` for a random one written to `pendingPairing`;
- `allLogin`: the stored logins.

After an `unlock` request, `locked` changes to `unlocked` after `unlockDelayMilliseconds`, as if the user had unlocked Hisar. URL matching follows §9.2, but approximates the registrable domain as the last two host labels.

Register it for a separate Chrome profile:

```sh
npm run register-fake-host -- --user-data-dir /tmp/hisar-chrome-profile
open -na "Google Chrome" --args --user-data-dir=/tmp/hisar-chrome-profile
```

Then load `dist` unpacked in that profile. The host's log goes to Chrome's stderr. The state file and the launcher live in `extension/.fake-host/`, or wherever `--state` and `--host-dir` point.

**Warning:** registering the fake host in your normal browser profile (with no `--user-data-dir`) overwrites the `com.tryhisar.hisar.json` that the Hisar app installed. The script refuses unless you also pass `--replace-hisar-registration`. Hisar writes its own manifest back the next time it starts.

## Clipboard in MV3

MV3 service workers have no DOM, so they cannot use the clipboard. Copying goes through an offscreen document (reason `CLIPBOARD`), which writes with `document.execCommand("copy")`.

After 90 s, the offscreen document reads the clipboard back with `execCommand("paste")` and clears it only if it still holds the copied value. It clears anyway when the clipboard cannot be read. Then it closes.

This needs the `offscreen`, `clipboardWrite` and `clipboardRead` permissions. Clearing is best effort: it does not happen if the browser quits within the 90 s, and clipboard managers keep their own history.
