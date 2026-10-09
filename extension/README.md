# Hisar Browser Extension

MV3 extension for Chromium browsers that fills and saves logins through the Hisar desktop app over native messaging. The protocol, matching and frame rules are in [docs/DESIGN.md §9.2](../docs/DESIGN.md#92-browser-extension--typescript-mv3). The extension holds no keys and does no crypto.

The manifest's `key` gives the fixed ID `mmipiffoogblblocokimpemdopmdjghf`, both unpacked and from the store. The native host `com.tryhisar.hisar` allows only that ID.

## Layout

```
public/manifest.json      MV3 manifest (version must equal package.json)
popup.html, inline-menu.html
src/background/           service worker: native port (native-client.ts), popup/content handlers (controller.ts, router.ts),
                          frame rule (frames.ts), save offers (save.ts), chrome.storage.local (storage.ts)
src/content/              content script (all frames): form detection, fill, submit capture, save bar (closed shadow root)
src/content/inline-menu/  inline icon and menu frame placement, eligibility, keyboard, clickjacking checks
src/inline-menu/          the menu page shown in the iframe under a login field (inline-menu.html)
src/popup/                Vue popup: pairing, desktop unavailable, signed out, locked, logins, generator, settings
tools/                    fake native host, its registration script, packaging
end-to-end/               Playwright tests and screenshot capture
```

The build runs Vite twice: once for the popup, the inline menu page and the service worker (an ES module), and once for the content script as a single IIFE file, because content scripts cannot be modules. Icons come from `assets/logo/png` (rendered by `scripts/render-logo.sh`).

## Develop and test

```sh
npm ci
npm run typecheck
npm test                    # Vitest: detection fixtures, fill, frame rule, capture, save bar, inline menu, service worker, popup, fake host
npm run build               # dist/
npm run package             # hisar-extension-<version>.zip (not committed)
npx playwright install chromium
npm run test:end-to-end     # builds, then runs the Playwright tests with the fake host
```

`npm run test:end-to-end` starts Playwright's Chromium in headless mode. It uses a temporary profile, with the fake host registered in `<profile>/NativeMessagingHosts`, and a local test login page. The test loads the unpacked build, then:

- pairs with the code;
- fills a login (a cross-origin iframe stays empty);
- copies a password;
- updates a changed password and saves a new login from the save bar;
- checks that "Never for this site" works.

`end-to-end/inline-menu.spec.ts` covers the inline menu:

- the icon and the menu in the login field;
- choosing a login with the mouse and with the arrow keys;
- no icon in the cross-origin iframe;
- the not-paired, locked and not-running states;
- the suggested password on a sign-up page;
- the settings;
- clicks ignored while the page hides the menu (transparent page, or an overlay with `pointer-events: none`).

To watch it run, use `HISAR_HEADED=1 npm run test:end-to-end`.

`HISAR_SCREENSHOT_DIRECTORY=/tmp/shots npm run screenshots` saves the popup states, the save bar and the inline menu states as PNGs. Add `HISAR_CHROME_PATH="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"` to use the installed Chrome. Branded Chrome ignores `--load-extension`, so the script loads the build over the DevTools protocol (`Extensions.loadUnpacked`) instead.

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

## Inline menu

When a username or password field of a login form gets the focus, a Hisar icon appears inside the field's right edge, and a menu opens under the field.

- **Menu contents:** matching logins, a suggested password on sign-up forms, or what to do when Hisar is not paired, locked or not running.
- **Settings:** the popup's settings (the sliders button in the header) turn the icon off, or open the menu only on a click on the icon or the down arrow.
- **Isolation:** the menu is an extension page in an iframe, so the page never sees login titles or usernames.
- **Clickjacking checks:** the menu fills only after it has been visible and uncovered for 300 ms. [docs/DESIGN.md §9.2](../docs/DESIGN.md#92-browser-extension--typescript-mv3) describes these checks.

## Copying

The popup copies usernames, passwords and generated passwords itself, with `navigator.clipboard.writeText`. It gets a password from the service worker, which asks the desktop with the tab's URL. A focused extension page may write to the clipboard without a clipboard permission.

The extension does not clear the clipboard afterwards: a copied value stays until something else is copied. The desktop app still clears what it copies itself after 90 s.
