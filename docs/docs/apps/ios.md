---
sidebar_position: 6
sidebar_label: iOS app
---

# Hisar for iOS

SwiftUI app and AutoFill Credential Provider extension on top of `HisarKit.xcframework`, the Go backend from
`mobile/hisarkit` (see [Design §9.3](../design.md#93-mobile--gomobile--native-ui)). Deployment target iOS 26.

```
project.yml              XcodeGen spec; Hisar.xcodeproj is generated from it and committed
Configuration/           Signing.xcconfig (team and bundle identifiers), Hisar.xcconfig, entitlements
App/                     the app: AppModel (state, calls Go off the main thread) and the SwiftUI screens
AutoFill/                CredentialProviderViewController, the extension's entry point
Shared/                  compiled into both: Go wrapper (VaultBackend, HisarKitBackend), Keychain, Face ID,
                         pasteboard, AutoFill model and screens, theme
Tests/Unit/              XCTest with a fake backend, plus Keychain, pasteboard and Go adapter tests
Tests/UI/                XCUITests against a real hisar-server (scripts/test-ios-ui.sh)
```

Item icons ([Design](../design.md) §4.5): the editor's icon opens `IconPickerView` (Automatic, presets, colors, Choose photo); `ItemIcon`
draws the chosen icon, else a login's website icon, which `AppModel.loadWebsiteIcon` loads once per domain through HisarKit.
Presets map to SF Symbols in `ChosenIcon.symbolByPreset`; `mobile/hisarkit/iconpresets_test.go` keeps the list on core's.
At accessibility text sizes, item-detail headers stack a capped decorative icon above the full-width title.
The icon picker's preset grid adapts its column count to the scaled tiles instead of fixing six columns.
Color swatches, the editor's **Change icon**, and removed-vault dismiss buttons each have at least a 44-point target.
Dismiss buttons name their vault so VoiceOver distinguishes notices. Each notice keeps its dismiss action separate from
the message. The empty Approvals screen likewise keeps its setup link separate from its explanation.
The Mac approvals page shows both the iPhone's approval explanation and the Mac settings needed to turn requests on.

At accessibility text sizes, browsing rows wrap the full title and username, put the vault name on its own line,
and cap the decorative tile at 44 points. Ordinary text sizes keep the compact row. Android browsing rows also wrap
titles and subtitles at large font scales without reducing the selected text size.

New items have a **Vault** picker that lists writable vaults and starts with the vault the editor opened from.
Saving stores the item in the chosen vault. Canceling or swiping down with unsaved edits asks whether to discard them.
Text and secret fields retain visible labels above their values, including populated identity, address, credential,
license, server and database editors. Labels and inputs stay vertically stacked at larger text sizes.
Organization vault role pickers announce the team, vault, and current role to VoiceOver.
The Search tab keeps its search field visible in compact and wide layouts without a pull-down gesture.
Nonempty item passwords show their strength below the field in detail, the editor, and the generator,
without revealing the password.
Account settings group the master password, Secret Key, Emergency Kit, imports, exports, account deletion, and sign out.
**Sign out** has its own final section below **Delete account** and still asks for confirmation.
Help and About remain informational destinations on the Settings root.
Security settings hold biometric unlock, auto-lock, and **Lock Hisar now**.
Section headings and explanations use the palette's solid secondary text color, including recovery, export and
account-change instructions. The color keeps at least 4.5:1 contrast on grouped backgrounds in Light and Dark appearance.

At accessibility text sizes, the lock screen keeps the account email on one line, shrinking it or truncating its middle.
VoiceOver reads the full address.
Sign-in and create-account failures appear immediately above their submit button, without clearing the entered fields.
At accessibility text sizes, sign-in modes use stacked buttons and a wrapping heading instead of a segmented control.
The Emergency Kit scanner explains denied or restricted camera access. **Open Settings** is available for denied access.
Returning from Settings rechecks permission before starting the camera.
If creating the first Emergency Kit fails, its section offers **Try again** and keeps the Secret Key and Copy available.
Retrying uses a fresh temporary folder. Only a completed non-copy handoff enables the explicit saved confirmation.
Devices distinguishes a failed first fetch from a successful empty list and offers **Retry**.
A failed refresh keeps the last loaded list with a warning that it has not refreshed; pull-to-refresh remains available.
Vaults and item lists show failed favorite, rename, and invitation actions on the screen where they began.
A failed refresh keeps the last readable list and offers **Retry**. A failed rename keeps the entered name when
**Rename** is opened again. Background successes do not dismiss these failures.
At accessibility text sizes, tag counts move below their names and the tag symbol stays at most 32 points wide.
Ordinary text sizes keep the native count badge.

Account deletion carries the `planVersion` of the preview the user reviewed into `DeleteAccountWithPlan`.
A failed deletion clears the password and email confirmation, reloads the preview, and requires another confirmation.
An absent plan version never enables the delete button.

`RootView` presents pending account recovery independently of the signed-in, locked, and signed-out screens.
`ListPendingRecovery` supplies account metadata, not the new Secret Key. The user chooses a record and enters its
master password to resume setup or prepare an Emergency Kit. The kit and Secret Key remain visible until the user
closes recovery, changes accounts, or locks the app. Closing removes the temporary PDF but keeps the protected journal.
Only an explicit saved confirmation calls `AcknowledgePendingRecovery`. Without the OS keychain, setup stops before
the remote change until the user saves and confirms the new kit. After a restart, the user supplies the saved kit's
Secret Key through `ResumePendingRecoveryWithSecretKey`. Metadata-only manual-unlock records remain visible for
future sign-ins. `DiscardPreparedRecovery` removes only a setup for which no remote change was sent.
The Android root screen and `PendingRecoveryModel` use the same protocol and test names.
Choosing a protected sign-up that only needs its kit opens **Save your Secret Key**. Enter its master password once,
save or print the kit, then confirm **I saved my Emergency Kit**. This does not switch the signed-in account.

## iPhone Duo

The open iPhone Duo is regular in both size classes, like an iPad; closed, it is a compact iPhone, and the app resizes as
the phone opens and closes. The layout follows the size class, never the device or the orientation: Vaults, Search and
Settings are `NavigationSplitView`s, which show the list beside what it opens on the open phone and collapse into the one
stack every other iPhone shows. The system does the rest: sheets center on the inner display, toolbars and tabs move to
the side next to the camera on the outer display, and sheets, alerts and menus keep away from the fold. Nothing reads
`UIScreen.main`, which is ambiguous on two displays.

Vaults keeps one **Tags** row. Its tag list and each tag's items open in the detail column. Choosing another vault,
item type, favorite, or **Tags** replaces that column's navigation stack.
Saving a new item opens it in the vault chosen in the editor.

![The open iPhone Duo in landscape: the Vaults list on the left, the GitHub login on the right, the toolbar buttons and the tabs in a column beside the camera.](/img/iphone-duo/vaults-and-item.png)

![Settings beside its Security screen on the open iPhone Duo.](/img/iphone-duo/settings-and-security.png)

The whole inner display is only used by a build with the iOS 27.1 SDK (Xcode 27.1); an older build runs letterboxed.
CI builds and tests the app on the `xcode-27` runner image for that reason, while the Mac app stays on `macos-26`. To try
a layout, run the app on the iPhone Duo simulator (Xcode 27.1 with the iOS 27.1 runtime: `xcodebuild -downloadPlatform iOS`)
and check every pose from Device Hub's buttons: open and closed, both orientations, half folded, and Split View on
either side.

## Build and test

Requirements: Xcode 27.1 with an iOS simulator runtime, Go, and for regenerating the project XcodeGen 2.46.0
(`brew install xcodegen`).

```sh
make build-ios-framework    # gomobile → mobile/build/HisarKit.xcframework (not committed)
make build-ios              # the app with the extension and the tests, and the free personal team variant, for the simulator
make test-ios               # unit tests in a simulator (IOS_TESTS_PREBUILT=1 skips building what build-ios built)
make build                  # hisar-server, for the UI tests
HISAR_DATABASE_URL=postgres://…/hisar_ios_empty make test-ios-ui   # UI tests against a fresh server
```

`IOS_SIMULATOR_ID=<udid>` picks the simulator; by default a numbered iPhone of the newest runtime is used
(`scripts/find-simulator.sh`), never the iPhone Duo while another iPhone exists, so the tests run in the compact layout. The UI test script starts `hisar-server` on port 18082, creates invites, and passes them
to the tests as `TEST_RUNNER_HISAR_UI_TEST_*` variables. The test classes run one after another; `IOS_UI_TEST_WORKERS=2`
runs them on two clones of the simulator at once, which is quicker on a Mac with cores to spare but a loss on the CI runners.
Set `HISAR_UI_TEST_SCREENSHOT_DIRECTORY` to save a PNG of each screen.

The sign-up tests save each Emergency Kit under a unique name because the simulator's Files folder survives between tests.
The helper waits for the Files picker to become tappable and for saving to finish. Formatted fields wait for each character's
update before typing the next one. Custom-field clipboard checks paste inside Hisar because iOS does not grant the test
runner access to the app's clipboard.

After editing `project.yml` (adding files is enough, sources are folders), run `make generate-ios-project` and commit
the project; CI fails if the committed project differs.

What the UI tests cover: create an account from an invite and read the new Secret Key, add a login, Favorites (the star in an item,
the swipe actions and the Favorites section at the top of Vaults), creating every item type through the + menu and its More… submenu, item detail
(tap a field anywhere to copy, including a concealed field, reveal independently with the eye, and long-press for
Copy or Copy secret reference from the field's context menu, independently of the page's secret-reference button), editing with the password generator, search, lock and unlock with the master password, Settings (the empty Approvals tab, auto-lock, devices, the Secret Key behind the master
password with the Emergency Kit share), turning Mac approvals off in Settings and back on from Approvals, sign out and sign in with the Secret Key, creating a shared vault and seeing its members, inviting someone from Settings → People, the privacy cover, Face ID (the test enrolls a face in the simulator and posts the
simulator's "matching face" notification), the AutoFill screens, and passkeys: register a passkey into a login, sign in
with it, check the attestation and the ES256 signature with CryptoKit, then see and delete it in the item.
`WideLayoutUITests` checks the split views: beside their list on a wide screen, pushed over it on a compact one, so it
means something on both the iPhone Duo simulator and the compact one CI uses.

The icon picker keeps the last completed icon while a photo is preparing. **Done** stays disabled until it finishes.
**Cancel preparation**, selecting another icon, or dismissing the sheet cancels the pending photo.
An older photo or its error cannot replace a newer choice.

`FaceIDUITests` first launches `com.apple.LocalAuthenticationUIService`, waits for it to reach the foreground, and returns Home. A cold service can start after `coreauthd`'s five-second UI activation deadline on a CI runner. Warming it before the real account flow avoids that startup condition without retrying a failed authentication or changing the app.

`FaceIDUITests` posts matching-face notifications every 500 ms on a background timer, starting before the action that requests authentication. Slow XCTest permission and accessibility queries must not delay the simulated face. The timer stops when that step finishes, and authentication errors still fail the test.

UI tests wait for the Files picker's **Save** button to become tappable before renaming an Emergency Kit.
They wait for the filename field to disappear after saving, before continuing in Hisar.
Visible formatted input and live search are entered one character at a time, with each field update observed before the next character.
Replacing a search query uses the native clear control, waits for it to disappear, and enters the new query through the same synchronized helper.

`FaceIDUITests` checks that cancelling authentication leaves Hisar locked, focuses the master password field, and shows no error banner.
`FavoritesUITests` injects its one-shot refresh failure only when the item list requests a manual refresh.
Background item loads cannot consume that failure. The test checks the retained list, the Retry action, and recovery after retrying.

Notification removal runs off the main thread because the system call can wait for the notification service during a scene transition.
Returning from another app keeps the interface responsive while Hisar clears its approval reminder and refreshes setup status.

**Approving the Mac's requests.** The approval sheet (a Mac's SSH or command-line request, an app such as Raycast
asking to use Hisar until it locks, or that app creating one share link, approved with Face ID or the master password,
or denied) is driven through a debug-only preview with a made-up request: launch with `-HisarApprovalPreview ssh` (or
`cli`, `forwarded`, `signing`, `launcher`, `launcher-share`). Add `-HisarApprovalPreviewTouchID` for Touch ID button
copy and its symbol, or `-HisarApprovalPreviewPassword` for the master password fallback.
`-HisarApprovalBannerPreview` shows the tabs with the waiting requests as a card on Vaults and in the Approvals tab;
followed by a kind (`-HisarApprovalBannerPreview launcher-share`), it shows only that request. The simulator cannot run the Mac side; `TestIntegrationPhoneApproval` in `mobile/hisarkit` runs the
whole flow between a desktop backend and a HisarKit backend in `make test-end-to-end`.

The waiting-request card has a remember switch when the Mac allows remembering. Approve sends the switch's value.
The card shows the app's warning, such as an unsigned app's or a share link's, in full between the request and the
switch, so approving from the card never skips it. The sheet puts the request summary first, shows app warnings
separately from details, and names the iPhone's biometric check on its Approve button.

**AutoFill in the simulator.** A simulator build without a development team ("Sign to Run Locally") is registered
(`pluginkit` lists the extension), but iOS 26.4 does not list it in Settings → General → AutoFill & Passwords, and
`ASSettingsHelper.requestToTurnOnCredentialProviderExtension()` fails without showing its prompt. Turning it on with
`pluginkit -e use` is not enough either: `ASCredentialIdentityStore` still reports it as off. So the UI test drives the
extension's screens and model through a debug-only preview in the app: launch with
`-HisarAutoFillPreview https://example.com/login` (passwords), `-HisarAutoFillOneTimeCodePreview <site>`,
`-HisarAutoFillPasskeyPreview example.com` (passkey sign-in) or `-HisarAutoFillPasskeyRegistrationPreview example.com`
(save passkey, with `-HisarAutoFillPasskeyUserName`, `-HisarAutoFillPasskeyUserHandle` and
`-HisarAutoFillPasskeyClientDataHash`, base64url). `-HisarAutoFillPasskeyUserVerification` accepts `required`,
`preferred` or `discouraged`, with `preferred` as the default. The simulator smoke uses `discouraged` and verifies UV=0.
The preview shows the credential ID, attestation object, authenticator data, signature and user handle for verification.
The full flow (enable Hisar in Settings or with the "Turn on Hisar
AutoFill" button in the app's Settings, open a login page in Safari, tap the password field) should work once a team
is set in `Configuration/Local.xcconfig`. That has not been verified yet, and neither has the system passkey flow
(Safari offering Hisar for a passkey sign-in or "Save passkey"): both need the paid team.

When saving a passkey, a failed destination load stops the spinner and offers **Try again** for the same request. If Hisar locked or signed out, the extension shows its authentication screen instead.

Manual run: open `Hisar.xcodeproj`, choose the **Hisar** scheme and an iPhone simulator, and Run. The simulator
reaches a server on your Mac at `http://127.0.0.1:<port>`; plain `http` is allowed for local addresses only
(`NSAllowsLocalNetworking`).

## Running on your own iPhone with a free Apple ID

A free Apple ID gives a "personal team": apps run for 7 days and then have to be installed again from Xcode. A
personal team is not expected to get app groups or the AutoFill Credential Provider capability (not verified yet), so the
**Hisar Personal Team** scheme builds the app
without the extension, without an app group and without a shared keychain group. It keeps its data in its own
Application Support folder and uses its own bundle identifier (`com.tryhisar.hisar.personal`). Using a separate
identifier means the free team never registers `com.tryhisar.hisar`, which the paid team needs later.

1. Build the framework once: `make build-ios-framework`.
2. Open `mobile/ios/Hisar.xcodeproj` in Xcode.
3. Xcode → Settings → Accounts → **+** → Apple Account, and sign in with your Apple ID.
4. Select the **HisarPersonal** target → Signing & Capabilities → Team: *Your Name (Personal Team)*. To keep this
   out of git, put `DEVELOPMENT_TEAM = <your team ID>` in `mobile/ios/Configuration/Local.xcconfig` instead (the file
   is ignored; the team ID is in the build log or in Keychain Access under "Apple Development: …"). If Xcode says
   the bundle identifier is not available, change `HISAR_PERSONAL_BUNDLE_IDENTIFIER` in `Configuration/Signing.xcconfig`
   (for example to `com.yourname.hisar`).
5. Connect the iPhone with a cable and unlock it. On the iPhone, turn on Settings → Privacy & Security →
   **Developer Mode** and restart when asked (the option appears after the phone has been connected to Xcode once).
6. In Xcode choose the scheme **Hisar Personal Team** and your iPhone as the destination, then Run (⌘R).
7. The first launch fails with "Untrusted Developer". On the iPhone open Settings → General → **VPN & Device
   Management**, tap your Apple ID under "Developer App", then **Trust**. Run again.
8. Sign in with your server, email, Secret Key and master password (or scan the Emergency Kit QR code). The server
   must be reachable from the phone over `https`; `http` only works for local addresses.
9. After 7 days the app no longer opens: connect the phone and Run again. Your data stays.

Face ID should work with a personal team, because the biometric key is a normal Keychain item with an access
control and needs no entitlement. It is tested in the simulator, not yet on a device.

## The paid team (589NVZWDWZ)

The team is an individual Apple Developer account, so other members cannot get its certificates through Xcode's
automatic signing. The **Hisar** app and its AutoFill extension therefore sign manually (`TeamSigning` in
`project.yml`); the personal team build keeps automatic signing. Simulator builds sign to run locally and need none of
this.

1. The team ID is in `Configuration/Signing.xcconfig` (`DEVELOPMENT_TEAM`).
2. The developer portal has the app `com.tryhisar.hisar`, the extension `com.tryhisar.hisar.autofill`, the app group
   `group.com.tryhisar.hisar` and the AutoFill Credential Provider capability on both the app and the extension (App
   Store Connect rejects an upload whose app lacks its extension's entitlement), and four profiles named after the
   target:

   | Target | Debug (a device from Xcode) | Release (archive) |
   |---|---|---|
   | Hisar | `Hisar Development` | `Hisar App Store` |
   | HisarAutoFill | `HisarAutoFill Development` | `HisarAutoFill App Store` |

3. To build for an iPhone, install the team's Apple Development certificate (with its private key) in your keychain and
   the two Development profiles (double-click them, or Xcode → Settings → Accounts → Download Manual Profiles). Your
   iPhone's UDID must be in the Development profiles. Archiving needs the Apple Distribution certificate and the App
   Store profiles.
4. Use the **Hisar** scheme on the iPhone. AutoFill then works on the device: Settings → General → AutoFill &
   Passwords → turn on Hisar, or the button in Hisar's Settings.
5. Data from the personal team build does not carry over (different bundle identifier and keychain). Sign in again
   and delete the old app.
6. The app has the Push Notifications capability (`aps-environment` in `Hisar.entitlements`): the App ID needs Push
   Notifications enabled, and both Hisar profiles must be generated again after that, or device builds and archives
   fail to sign. Debug builds register with APNs's development environment, release builds with production
   ([Design](../design.md) §9.12). The personal team build has no push.
6. A Release build signs with the App Store profiles, so it reaches a device through TestFlight or the App Store, not
   by running it from Xcode.

To use another bundle identifier prefix (for example a fork), change `HISAR_BUNDLE_IDENTIFIER_PREFIX` and
`DEVELOPMENT_TEAM` in `Configuration/Signing.xcconfig`, and create the four profiles above in your team under the same
names. The extension, app group and keychain group are derived from the prefix.
