# Hisar Browser Extension

MV3 extension for Chromium browsers that fills and saves logins, fills cards, identities and addresses, and browses every vault from its popup, through the Hisar desktop app over native messaging. The protocol, matching and frame rules are in [docs/DESIGN.md §9.2](../docs/DESIGN.md#92-browser-extension--typescript-mv3). The extension holds no keys and does no crypto.

The manifest's `key` gives the fixed ID `mmipiffoogblblocokimpemdopmdjghf`, both unpacked and from the store. The native host `com.tryhisar.hisar` allows only that ID.

## Layout

```
public/manifest.json      MV3 manifest (version must equal package.json)
popup.html, inline-menu.html, passkey-prompt.html
src/background/           service worker: native port (native-client.ts), popup/content handlers (controller.ts, router.ts),
                          frame rule (frames.ts), save offers (save.ts), chrome.storage.local (storage.ts)
src/content/              content script (all frames): form detection, fill, submit capture; no save confirmation UI;
                          payment forms (card-detection.ts, card-fill.ts), personal details (identity-detection.ts, identity-fill.ts)
src/content/inline-menu/  inline icon and menu frame placement, eligibility, keyboard, clickjacking checks
src/inline-menu/          the menu page shown in the iframe under a login field (inline-menu.html)
src/content/passkey/      passkeys (DESIGN §9.8): the MAIN-world navigator.credentials wrapper with PRF, the signal methods
                          and getClientCapabilities (passkey-main.js), the isolated-world bridge and prompt frame (passkey-content.js)
src/background/passkey*.ts  passkey requests: sender origin, option checks, prompt sessions over ports
src/passkey-prompt/       the passkey prompt page shown in the iframe (passkey-prompt.html)
src/popup/                Vue popup: pairing, desktop unavailable, signed out, locked (master password), the vault (VaultView: search,
                          categories, ItemDetail, NewItemView), cards, identities and addresses, generator, settings
tools/                    fake native host, its registration script, packaging
end-to-end/               Playwright tests and screenshot capture
```

The build runs Vite four times: once for the popup, the inline menu and passkey prompt pages and the service worker (an ES module), and once each for the content script and the two passkey content scripts as single IIFE files, because content scripts cannot be modules. Icons come from `assets/logo/png` (rendered by `scripts/render-logo.sh`).

## Develop and test

```sh
npm ci
npm run typecheck
npm test                    # Vitest: detection, fill, frame rule, capture, popup save confirmation, inline menu, service worker, fake host
npm run build               # dist/
npm run package             # hisar-extension-<version>.zip (not committed)
npx playwright install chromium
npm run test:end-to-end     # builds, then runs the Playwright tests with the fake host
```

`npm run test:end-to-end` starts Playwright's Chromium in headless mode. It uses a temporary profile, with the fake host registered in `<profile>/NativeMessagingHosts`, and a local test login page. The test loads the unpacked build, then:

- pairs with the code;
- fills a login (a cross-origin iframe stays empty);
- copies a password;
- updates a changed password and saves a new login through the extension-owned popup;
- checks that "Never for this site" works.

`end-to-end/inline-menu.spec.ts` covers the inline menu:

- the icon and the menu in the login field;
- choosing a login with the mouse and with the arrow keys;
- no icon in the cross-origin iframe;
- the not-paired, locked and not-running states;
- the suggested password on a sign-up page;
- the settings;
- clicks ignored while the page hides the menu (transparent page, or an overlay with `pointer-events: none`).

`end-to-end/passkey.spec.ts` runs a relying party on `http://localhost` that verifies with `@simplewebauthn/server`; the fake host signs like the desktop (`tools/fake-passkey-authenticator.mjs`):

- registers a passkey into an existing login, signs in with it, and gets `InvalidStateError` for an excluded credential;
- conditional mediation (passkey autofill) from a `webauthn` field;
- "Use another device or browser" and an unpaired browser fall back to the browser (a CDP virtual authenticator);
- unlocking from the prompt, and closing it while locked.

`end-to-end/passkey-extensions.spec.ts` uses the same site and fake host:

- creates a passkey with `prf.eval` and signs in twice with the same salt (same output) and once with another (a different one);
- `signalUnknownCredential` with an `rpId` of another site rejects with `SecurityError`; with the site's `rpId` it removes the passkey (the login stays), and signing in no longer offers it;
- `getClientCapabilities()` answers without starting the native host, and reports Hisar's capabilities once it is connected.

`end-to-end/vault.spec.ts`, `unlock.spec.ts`, `card.spec.ts`, `identity.spec.ts` and `save.spec.ts` cover the popup's vault (search, a card's details, reveal and copy, favorites), unlocking with the master password, filling a card (lists included, never into the cross-origin payment frame, and no login offered for its security code), filling an identity with the address beside it, and saving a new login under a chosen title and vault, or creating one from the popup.

`end-to-end/address.spec.ts` uses a checkout page with an address form, a same-origin frame and a cross-origin frame:

- the inline menu on an address field lists the address and fills the form, with the street split into its lines and the country list chosen;
- the popup's Addresses tab fills the focused same-origin frame, and the cross-origin frame stays empty;
- with an app that does not announce addresses, neither the tab nor the icon appears.

To watch it run, use `HISAR_HEADED=1 npm run test:end-to-end`.

`HISAR_SCREENSHOT_DIRECTORY=/tmp/shots npm run screenshots` saves the popup states, credential confirmations, and inline menu states as PNGs. Add `HISAR_CHROME_PATH="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"` to use installed Chrome. The screenshot script loads the build through `Extensions.loadUnpacked`, because branded Chrome ignores `--load-extension`.

## Load unpacked

1. `npm run build`
2. Open `chrome://extensions`, turn on Developer mode, click **Load unpacked** and choose `extension/dist`.
3. Check that the ID is `mmipiffoogblblocokimpemdopmdjghf`.

With Hisar.app installed and launched once, the extension talks to the real app: Hisar registers its native host in every installed Chromium browser at launch.

## Releases

A release is a run of Actions → Release with the app `extension` (the root [docs/RELEASING.md](../docs/RELEASING.md) has the steps). The workflow "Release extension" writes the version of the tag `extension-<version>` into `public/manifest.json` and `package.json` for the build, so the committed version is not bumped by hand; it builds `hisar-extension-<version>.zip` and checks that its manifest still gives the fixed ID and has that version, and the release puts it on the GitHub prerelease `extension-<version>`; approving the run's Publish job marks it as the latest release. To update an unpacked install, unzip it, remove the old Hisar extension in `chrome://extensions` and load the new folder: the `key` in the manifest keeps the ID, so pairing with the Mac app is kept. A custom build (`extension-0.2.0-test`) has the manifest version `0.2.0`, because Chrome only accepts numeric versions.

## Save and update submitted logins

After submitting a new or changed login, open Hisar from the toolbar's `+` badge.
The popup shows the site, login, username, destination vault, and access scope.
New logins default to Personal. You can edit the title and username or deliberately select another writable vault.
Choose **Save login** or **Update password** to write the reviewed candidate.
**Not now** discards it. **Never for this site** suppresses later candidates for that origin.

The page receives no password-dependent save UI or comparison result.
Page clicks cannot authorize credential updates. The popup is not web-accessible and cannot be embedded by the page.
Each new submission replaces the earlier candidate, including an unchanged password.
A candidate can follow one same-origin submission navigation within ten seconds, but a subsequent navigation discards it.
Candidates expire after two minutes and disappear when the worker restarts or the tab closes.
If the popup cannot reach Hisar while reviewing a candidate, the candidate waits until its two-minute deadline.
Lock, account changes, and pairing replacement invalidate the native confirmation.
A failed save needs a fresh submission and another explicit confirmation.

The app advertises `save-confirmation` for `logins.prepare`, `logins.review`, and context-bound `logins.save`.
`logins.prepare` names the submission's tab, and the app keeps one save context per tab, so a page that submits again and again retires its own earlier contexts.
Legacy `logins.check` and saves without a context are refused. Update both the app and extension for submitted-login saves.
Manual popup item creation and filling remain available independently of this feature.

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
- **Addresses:** in a field of an address form (by its `autocomplete` token, or two address fields found by their labels), the menu offers Hisar's addresses and identities with an address, and fills the empty fields of that form's shipping or billing group. Only with a Hisar app that announces addresses.
- **One-time codes:** in a two-factor field (`autocomplete="one-time-code"`, or a field named or labelled like otp, 2fa or verification code), and in a code split into one-character boxes, the menu offers the codes of the matching logins and fills the chosen one.
- **Settings:** the popup's settings (the sliders button in the header) turn the icon off, or open the menu only on a click on the icon or the down arrow.
- **Isolation:** the menu is an extension page in an iframe, so the page never sees login titles or usernames.
- **Clickjacking checks:** the menu fills only after it has been visible and uncovered for 300 ms. [docs/DESIGN.md §9.2](../docs/DESIGN.md#92-browser-extension--typescript-mv3) describes these checks.

## Copying

The popup copies usernames, passwords and generated passwords itself, with `navigator.clipboard.writeText`. It gets a password from the service worker, which asks the desktop with the tab's URL. A focused extension page may write to the clipboard without a clipboard permission.

After filling a login that has a one-time password, the service worker copies the current code through an offscreen document (it has no clipboard of its own), so the code is ready for the next page. The popup shows the code with a countdown and copies it too.

The extension does not clear the clipboard afterwards: a copied value stays until something else is copied. The desktop app still clears what it copies itself after 90 s.

## Chrome Web Store

`npm run package:store` builds the store upload without the manifest `key`. The first upload, listing text, permission justifications and screenshots are in [store/README.md](store/README.md).
