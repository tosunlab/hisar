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
  src/androidTest/ instrumented tests: on a FakeBackend, or against a real hisar-server (scripts/test-android-ui.sh)
  src/debug/       PreviewActivity, which shows AutoFill's screens to UI tests
```

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
test name (`ANDROID_UI_TESTS=HarnessServerTests,testAccountItemLockAndSignInFlow`), and `ANDROID_UI_TESTS=smoke` runs the
short list that the API 29 and 34 emulators get. Set `HISAR_UI_TEST_SCREENSHOT_DIRECTORY` to save each test's screenshots.
On an emulator, the biometric tests enroll fingerprints through the emulator console, with the token in
`~/.emulator_console_auth_token`.

`IosTestParityTest` fails when an iPhone test has no Android test of the same name, a rename or a reason. Port an iPhone
test under its Swift name and delete its line from
`android/model/src/test/resources/ios-test-parity-expected-missing.txt`, which lists the tests not ported yet.

CI's `android` job (`.github/workflows/ci.yml`) runs `make build-android-aar`, the `ThemePalettes.kt` freshness check,
`make build-android`, `make test-android` and `make test-android-ui` on an x86_64 API 37 emulator with Postgres.

### Push notifications

Firebase delivers the Android app's notifications ([Design](../design.md) §9.12). The app sets Firebase up from Gradle
properties instead of `google-services.json`; a build without them has no push notifications and works otherwise:

```properties
# ~/.gradle/gradle.properties or -P on the command line; these identify the app to Firebase and are not secrets
hisar.firebase.applicationId=1:000000000000:android:0000000000000000
hisar.firebase.projectId=hisar-example
hisar.firebase.apiKey=AIza...
hisar.firebase.senderId=000000000000
```

### Running the app

Install `android/app/build/outputs/apk/debug/app-debug.apk` with `adb install -r`. To reach a server on your computer,
run `adb reverse tcp:<port> tcp:<port>` and sign in to `http://127.0.0.1:<port>`. Debug builds accept `http://` servers;
release builds sign in only over `https://`. The release APK is unsigned.

To fill logins, turn on Hisar in Settings → AutoFill → **Turn on Hisar AutoFill**. Chrome fills with Google until you
choose **Autofill using another service** in Chrome's autofill settings and restart Chrome; the same screen opens them.
