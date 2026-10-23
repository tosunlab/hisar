# Hisar for iOS

SwiftUI app and AutoFill Credential Provider extension on top of `HisarKit.xcframework`, the Go backend from
`mobile/hisarkit` (see [DESIGN.md §9.3](../../docs/DESIGN.md#93-mobile--gomobile--native-ui)). Deployment target iOS 26.

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

Item icons (DESIGN.md §4.5): the editor's icon opens `IconPickerView` (Automatic, presets, colors, Choose photo); `ItemIcon`
draws the chosen icon, else a login's website icon, which `AppModel.loadWebsiteIcon` loads once per domain through HisarKit.
Presets map to SF Symbols in `ChosenIcon.symbolByPreset`; `mobile/hisarkit/iconpresets_test.go` keeps the list on core's.

Account deletion carries the `planVersion` of the preview the user reviewed into `DeleteAccountWithPlan`.
A failed deletion clears the password and email confirmation, reloads the preview, and requires another confirmation.
An absent plan version never enables the delete button.

`RootView` presents pending account recovery independently of the signed-in, locked, and signed-out screens.
`ListPendingRecovery` supplies account metadata, not the new Secret Key. The user chooses a record and enters its
master password to resume setup or prepare an Emergency Kit. The kit and Secret Key remain visible until the user
closes recovery, changes accounts, or locks the app. Closing removes the temporary PDF but keeps the protected journal.
Only **I saved the new Emergency Kit** calls `AcknowledgePendingRecovery`. Without the OS keychain, setup stops before
the remote change until the user saves and confirms the new kit. After a restart, the user supplies the saved kit's
Secret Key through `ResumePendingRecoveryWithSecretKey`. Metadata-only manual-unlock records remain visible for
future sign-ins. `DiscardPreparedRecovery` removes only a setup for which no remote change was sent.
The Android root screen and `PendingRecoveryModel` use the same protocol and test names.

## Build and test

Requirements: Xcode with an iOS simulator runtime, Go, and for regenerating the project XcodeGen 2.46.0
(`brew install xcodegen`).

```sh
make build-ios-framework    # gomobile → mobile/build/HisarKit.xcframework (not committed)
make build-ios              # the app with the extension and the tests, and the free personal team variant, for the simulator
make test-ios               # unit tests in a simulator (IOS_TESTS_PREBUILT=1 skips building what build-ios built)
make build                  # hisar-server, for the UI tests
HISAR_DATABASE_URL=postgres://…/hisar_ios_empty make test-ios-ui   # UI tests against a fresh server
```

`IOS_SIMULATOR_ID=<udid>` picks the simulator; by default the first iPhone of the newest runtime is used
(`scripts/find-simulator.sh`). The UI test script starts `hisar-server` on port 18082, creates invites, and passes them
to the tests as `TEST_RUNNER_HISAR_UI_TEST_*` variables. The test classes run one after another; `IOS_UI_TEST_WORKERS=2`
runs them on two clones of the simulator at once, which is quicker on a Mac with cores to spare but a loss on the CI runners.
Set `HISAR_UI_TEST_SCREENSHOT_DIRECTORY` to save a PNG of each screen.

After editing `project.yml` (adding files is enough, sources are folders), run `make generate-ios-project` and commit
the project; CI fails if the committed project differs.

What the UI tests cover: create an account from an invite and read the new Secret Key, add a login, Favorites (the star in an item,
the swipe actions and the Favorites section at the top of Vaults), item detail
(reveal, copy), editing with the password generator, search, lock and unlock with the master password, Settings (the empty Approvals tab, auto-lock, devices, the Secret Key behind the master
password with the Emergency Kit share), sign out and sign in with the Secret Key, creating a shared vault and seeing its members, inviting someone from Settings → People, the privacy cover, Face ID (the test enrolls a face in the simulator and posts the
simulator's "matching face" notification), the AutoFill screens, and passkeys: register a passkey into a login, sign in
with it, check the attestation and the ES256 signature with CryptoKit, then see and delete it in the item.

**Approving the Mac's requests.** The approval sheet (a Mac's SSH or command-line request, an app such as Raycast
asking to use Hisar until it locks, or that app creating one share link, approved with Face ID or the master password,
or denied) is driven through a debug-only preview with a made-up request: launch with `-HisarApprovalPreview ssh` (or
`cli`, `forwarded`, `signing`, `launcher`, `launcher-share`), plus `-HisarApprovalPreviewPassword` to act as if Face ID
were unavailable, and `-HisarApprovalBannerPreview` shows the tabs with the waiting requests as a card on Vaults and in the Approvals tab. The simulator cannot run the Mac side; `TestIntegrationPhoneApproval` in `mobile/hisarkit` runs the
whole flow between a desktop backend and a HisarKit backend in `make test-end-to-end`.

**AutoFill in the simulator.** A simulator build without a development team ("Sign to Run Locally") is registered
(`pluginkit` lists the extension), but iOS 26.4 does not list it in Settings → General → AutoFill & Passwords, and
`ASSettingsHelper.requestToTurnOnCredentialProviderExtension()` fails without showing its prompt. Turning it on with
`pluginkit -e use` is not enough either: `ASCredentialIdentityStore` still reports it as off. So the UI test drives the
extension's screens and model through a debug-only preview in the app: launch with
`-HisarAutoFillPreview https://example.com/login` (passwords), `-HisarAutoFillOneTimeCodePreview <site>`,
`-HisarAutoFillPasskeyPreview example.com` (passkey sign-in) or `-HisarAutoFillPasskeyRegistrationPreview example.com`
(save passkey, with `-HisarAutoFillPasskeyUserName`, `-HisarAutoFillPasskeyUserHandle` and
`-HisarAutoFillPasskeyClientDataHash`, base64url). The preview shows the produced credential ID, attestation object,
authenticator data, signature and user handle so the UI test can verify them. The full flow (enable Hisar in Settings or with the "Turn on Hisar
AutoFill" button in the app's Settings, open a login page in Safari, tap the password field) should work once a team
is set in `Configuration/Local.xcconfig`. That has not been verified yet, and neither has the system passkey flow
(Safari offering Hisar for a passkey sign-in or "Save passkey"): both need the paid team.

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
6. A Release build signs with the App Store profiles, so it reaches a device through TestFlight or the App Store, not
   by running it from Xcode.

To use another bundle identifier prefix (for example a fork), change `HISAR_BUNDLE_IDENTIFIER_PREFIX` and
`DEVELOPMENT_TEAM` in `Configuration/Signing.xcconfig`, and create the four profiles above in your team under the same
names. The extension, app group and keychain group are derived from the prefix.
