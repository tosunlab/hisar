---
sidebar_position: 5
sidebar_label: iPhone and Android
---

# Hisar Mobile

The iPhone and Android apps on top of the gomobile-bound `core`. See [Design](../design.md#93-mobile--gomobile--native-ui).

- `hisarkit/`: the Go backend, built into `build/HisarKit.xcframework` with `make build-ios-framework` and into
  `build/HisarKit.aar` with `make build-android-aar`.
- `ios/`: the iOS app and AutoFill extension; see [iOS app](./ios.md).
- `android/`: the Android app, below.
- `cmd/hisar-ui-test-mac`: plays the user's Mac in the Android phone approval UI test.

## Android

Kotlin and Jetpack Compose on top of `HisarKit.aar`, built with the Gradle wrapper in `android/`. Android 10 (API 29)
and later, on arm64-v8a and x86_64; 32-bit ARM is out because `github.com/bytemare/ksf`, which OPAQUE uses, does not
compile for 32-bit targets. The package is `com.tryhisar.hisar`, and `com.tryhisar.hisar.debug` for debug builds.

```
android/model/     :model, plain JVM: the HisarKit types, VaultBackend, the app model (com.tryhisar.hisar.app),
                   AutoFill's parser and matcher, ThemePalettes.kt; FakeBackend in src/testFixtures
android/app/       :app: HisarApplication, MainActivity, HisarKitBackend, the platform adapters (Keystore, biometrics,
                   clipboard, certificates), the Compose screens in ui/<area>, the AutoFill service in autofill/
  src/test/        JVM tests of :app
  src/androidTest/ instrumented tests: the core journeys against a real hisar-server (scripts/test-android-ui.sh), and
                   the security checks only a device can make
  src/debug/       PreviewActivity, which shows AutoFill's screens to UI tests
```

Website, admin-console and download links accept internationalized host names. The JVM model checks such a host in
its IDNA ASCII form, but Open, Share, labels and Copy keep the stored address as written and leave the conversion to
the browser: `java.net.IDN` is IDNA2003, which would turn `faß.de` into the different domain `fass.de`. Emergency Kit
server validation and invite app links share this URI parsing, and invite links keep the original server origin when
they prefill Create account.

### Build and test

You need Go, JDK 21 (the Gradle daemon asks for it in `android/gradle/gradle-daemon-jvm.properties`), and the Android
command-line tools in `$ANDROID_HOME/cmdline-tools/latest`. `make install-android-sdk` installs the pinned SDK
platform, build tools, NDK and emulator images, and creates the emulators `hisar-api29`, `hisar-api34` and `hisar-api37`
for this machine's architecture. Running it again changes nothing.

```sh
export ANDROID_HOME=<the Android SDK>
export ANDROID_NDK_HOME=$ANDROID_HOME/ndk/30.0.16248370
make install-android-sdk
make build-android-aar     # gomobile → mobile/build/HisarKit.aar (not committed); the Gradle build needs it
make build-android         # the debug and release APKs, the instrumented test APK, lint
make test-android          # the JVM tests of :model and :app, IosTestParityTest among them
$ANDROID_HOME/emulator/emulator -avd hisar-api37 &
HISAR_DATABASE_URL=postgres://…/hisar_android_empty make test-android-ui   # instrumented tests against a fresh server
```

`make test-android-ui` runs on the one connected emulator or device, or on `ANDROID_SERIAL`. The script builds and
starts `hisar-server` on port 18084 (`ANDROID_UI_TEST_PORT`), the SMTP sink, the sharing partner and
`hisar-ui-test-mac`, creates one invite per account the tests need, and reaches the server from the emulator through
`adb reverse`. The test orchestrator clears the app's data before each test. `ANDROID_UI_TESTS` picks tests by class or
test name (`ANDROID_UI_TESTS=HisarKitBackendTests,testAccountItemLockAndSignInFlow`), and `ANDROID_UI_TESTS=smoke` runs the
short list that the API 29 and 34 emulators get. Set `HISAR_UI_TEST_SCREENSHOT_DIRECTORY` to save each test's screenshots.
Instrumented taps and swipes scroll rows clear of both the floating tab bar and **New**, which can cover a row that is
still inside the list's visible bounds.
On an emulator, the biometric tests enroll fingerprints through the emulator console, with the token in
`~/.emulator_console_auth_token`.

The script builds both APKs before running instrumentation. It uses `--no-daemon` and
`-Pkotlin.compiler.execution.strategy=in-process` for each Gradle invocation so the build's Gradle and Kotlin JVMs
exit before tests start. Keeping those JVMs alive alongside the emulator can exhaust the CI runner's host memory.

The instrumented tests keep only the journeys a user depends on end to end and the security checks only a device can
make; the logic behind each screen is in the JVM tests of `:model`. The journeys: an account with a login and a card,
lock, unlock and signing in again (`AccountItemFlowUITests`, which also checks what the keyboard and autofill are told
about secret fields and that the invite link stays out of the saved state), changing the master password and Secret Key
across a process restart and deleting the account (`ui.AccountLifecycleUITests`), biometric unlock
(`ui.BiometricUITests`), sharing a vault (`SharingUITests`), organizations (`OrganizationUITests`), passkeys through
Credential Manager (`passkey.PasskeyUITests`), filling a native app and a WebView (`autofill.AutofillServiceUITests`),
answering a real request from `hisar-ui-test-mac` (`PhoneApprovalUITests`), and saving the Emergency Kit and a plain
export, whose copies go when their screens close (`FileHandoffUITests`). The device checks: the app switcher and the
provider windows follow Allow screenshots (`PrivacyUITests`), the Secret Key under a Keystore key and the state out of
every backup (`platform.KeystoreSecretKeyStoreTests`), the biometric key that a new fingerprint invalidates
(`platform.BiometricKeystoreTests`), sensitive copies that Hisar clears and never reads another app's copy
(`platform.ClipboardAdapterTests`), and HisarKit on the device with exactly the device's trusted certificates
(`backend.HisarKitBackendTests`).

`IosTestParityTest` fails when an iPhone test has no Android test of the same name, a rename or a reason. Port an iPhone
test under its Swift name and delete its line from
`android/model/src/test/resources/ios-test-parity-expected-missing.txt`, which lists the tests not ported yet.
Instrumented test method names start with `test`, so the runner can match every discovered test's logcat file to its XML result.

CI's `android-build` job (`.github/workflows/ci.yml`) builds and lints the app and runs its JVM tests.
The `android` job runs the instrumented tests in three parts on x86_64 API 36 emulators with Postgres.
These headless emulators use SwiftShader OpenGL ES with Vulkan disabled. The test script builds both APKs in a
separate Gradle invocation before running the tests. Every invocation uses `--no-daemon` and in-process Kotlin
compilation, so idle build JVMs do not retain their compiler heaps while the emulator's host memory grows with
each instrumented test. Host memory exhaustion can leave QEMU unresponsive with graphics-context errors.

### Push notifications

The Android app gets its notifications through UnifiedPush, with no Firebase or Google service
([Design](../design.md) §9.12). Install a UnifiedPush distributor on the phone or emulator, such as ntfy from F-Droid,
then turn notifications on in Settings → Notifications; Hisar registers with the distributor and gives its endpoint to
the server, which sends to it directly. Without a distributor, that screen links to UnifiedPush's list of them.
`scripts/check-android-no-firebase.sh`, which CI runs, fails if the release build depends on Firebase.

### Running the app

Install `android/app/build/outputs/apk/debug/app-debug.apk` with `adb install -r`. To reach a server on your computer,
run `adb reverse tcp:<port> tcp:<port>` and sign in to `http://127.0.0.1:<port>`. Debug builds accept `http://` servers;
release builds sign in only over `https://`. The release APK is unsigned.

If **Install the latest Hisar** cannot open a browser, the update screen shows the download URL and **Copy link**.
**Check again** remains available to ask the server whether the installed version is accepted.

To fill logins, turn on Hisar in Settings → AutoFill → **Turn on Hisar AutoFill**. Chrome fills with Google until you
choose **Autofill using another service** in Chrome's autofill settings and restart Chrome; the same screen opens them.

Focus a one-time-code field to fill only its current code, even on a form that also has a username and password.
Focus the username or password to fill the login without changing the code field.

On Android 14 and later, Credential Manager's password sheet also offers **Search Hisar** for logins that do not match
the requesting app or site. For an unlinked app, choose **Only this time** or **Always use for this app** before filling.
When the sheet has no matching credentials, Hisar returns your search choice to the sheet for you to confirm.

Copy and action notices use Material snackbars above the tab bar. Favorite changes offer **Undo** until the
snackbar is dismissed. TalkBack announces each notice. Swipe it away or use its dismiss button to close it without undoing the action.
If a snackbar covers another field or row, dismiss it before tapping that row. Notices stay clear of pinned primary actions.

The item detail's **Copy secret reference** footer says "For hisar run and hisar read on your Mac."
Touch and hold a field for that field's **Copy secret reference** action.

Touch and hold an Android app linked to a login, then choose **Copy certificate** to copy its full signing-certificate
SHA-256. The copy uses uppercase hex bytes separated by colons, as `apksigner` and Digital Asset Links show it.

### Account passwords and exports

The new master password and encrypted-export passphrase forms count extended grapheme clusters for their
ten-character minimum, like the iPhone app. Combining marks, flags and joined emoji each stay with their character.
This validation does not normalize the password or change the shared Go checks.

Credential Exchange imports keep Cancel and system Back available while batches are saving. Closing the import
does not roll back saved items or interrupt the current batch. The current batch can finish, but no later batch starts.

### Password generator

The item editor's generator sheet scrolls its options within the available height, including in landscape.
Close and Use stay above the scrolling content. The Settings generator remains a separate scrolling screen.
Both use one labelled length slider, with its current value at the end, for whole-number lengths from 8 through 64.
TalkBack and keyboard users can adjust the same slider.
