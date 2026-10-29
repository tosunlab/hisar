MODULES := core server cli mobile
CLI_TARGETS := linux/amd64 linux/arm64 darwin/arm64 darwin/amd64 windows/amd64 windows/arm64
WAILS_VERSION := v3.0.0-beta.23

override GOTOOLCHAIN := $(shell bash scripts/go-toolchain.sh)
$(if $(strip $(GOTOOLCHAIN)),,$(error go.work must name the release toolchain))
export GOTOOLCHAIN

.PHONY: build build-admin test-admin test-admin-end-to-end build-cli-all package-cli-release test vet test-end-to-end test-agent-sign-in docker install-wails test-desktop-frontend build-desktop package-desktop package-desktop-dmg package-desktop-update package-desktop-linux package-desktop-flatpak test-extension test-extension-end-to-end test-extension-firefox-end-to-end build-extension test-raycast build-docs test-docs test-omp test-share-link-page build-ios-framework generate-ios-project build-ios test-ios test-ios-ui install-android-sdk build-android-aar build-android test-android test-android-ui generate-themes

build: build-admin
	go build -o bin/hisar-server ./server/cmd/hisar-server
	go build -o bin/hisar ./cli/cmd/hisar

build-admin:
	mkdir -p admin/wasm
	cd server && GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" -o ../admin/wasm/hisar.wasm ./cmd/hisar-admin-wasm
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" admin/wasm/
	cd admin && npm ci --no-audit --no-fund && npm run build

test-admin:
	cd admin && npm ci --no-audit --no-fund && npm run typecheck && npm test

# Needs Playwright's Chromium: cd admin && npx playwright install chromium
test-admin-end-to-end: build-admin
	go build -o bin/hisar-admin-page-fixture ./server/cmd/hisar-admin-page-fixture
	go build -o bin/hisar ./cli/cmd/hisar
	cd admin && npm run test:end-to-end

build-cli-all:
	@for t in $(CLI_TARGETS); do \
		echo "hisar $$t"; \
		GOOS=$${t%/*} GOARCH=$${t#*/} CGO_ENABLED=0 go build -o /dev/null ./cli/cmd/hisar || exit 1; \
	done

# Release archives and SHA256SUMS of the standalone CLI in bin/cli-release, as the cli-<version> release workflow builds them.
package-cli-release:
	./scripts/package-cli-release.sh $(VERSION) bin/cli-release

# server tests with -p 1: its database tests in different packages share one schema. The other modules have no database
# tests, so they test in parallel while server's tests run; server's output follows theirs.
# desktop: every package but the Wails main package, which needs cgo and the built frontend, and secretclipboard, which
# needs the X11 and Wayland headers and a display; the desktop-linux CI job tests it.
# hisar_fast_test_kdf: accounts the tests create get the minimum Argon2 parameters (core/crypt/newaccount_fast_test_kdf.go).
DESKTOP_PACKAGES = $(shell cd desktop && go list -e ./... | grep -v -e '^github.com/tryhisar/hisar/desktop$$' -e '/desktop/secretclipboard$$')
TEST_TAGS := -tags hisar_fast_test_kdf

test:
	@status=0; serverLog=$$(mktemp); \
	(cd server && go test $(TEST_TAGS) -p 1 -count=1 $(TESTFLAGS) ./...) > "$$serverLog" 2>&1 & serverTests=$$!; \
	for m in $(filter-out server,$(MODULES)); do (cd $$m && go test $(TEST_TAGS) -count=1 $(TESTFLAGS) ./...) || status=1; done; \
	(cd desktop && go test $(TEST_TAGS) -count=1 $(TESTFLAGS) $(DESKTOP_PACKAGES)) || status=1; \
	wait $$serverTests || status=1; \
	cat "$$serverLog"; rm -f "$$serverLog"; exit $$status

vet:
	@for m in $(MODULES); do (cd $$m && go vet ./...) || exit 1; done
	cd desktop && go vet $(DESKTOP_PACKAGES)

test-end-to-end: build
	go build -o bin/hisar-smtp-sink ./server/cmd/hisar-smtp-sink
	go build -o bin/hisar-fake-github ./server/cmd/hisar-fake-github
	./scripts/test-end-to-end.sh

test-agent-sign-in:
	./scripts/test-agent-sign-in.sh

# VERSION=1.2.3 builds hisar-server 1.2.3, as the server-<version> release workflow does; without it the version is dev.
docker:
	docker build -f deploy/Dockerfile --build-arg VERSION=$(or $(VERSION),dev) -t hisar-server:$(or $(VERSION),dev) .

install-wails:
	go install github.com/wailsapp/wails/v3/cmd/wails3@$(WAILS_VERSION)

# Writes desktop/frontend/src/themes.css, admin/src/themes.css, mobile/ios/Shared/Views/ThemePalettes.swift and
# mobile/android/model/src/main/kotlin/com/tryhisar/hisar/model/theme/ThemePalettes.kt from assets/themes/themes.json (Node 22.6 or later).
generate-themes:
	node scripts/themes/generate.ts

# Needs Playwright's Chromium: cd desktop/frontend && npx playwright install chromium
test-desktop-frontend:
	cd desktop/frontend && npm ci --no-audit --no-fund && npm run typecheck && npm test && npm run test:end-to-end && npm run check:bundle

build-desktop:
	cd desktop && wails3 build

package-desktop:
	cd desktop && wails3 task package

# Universal Hisar.app in desktop/bin/Hisar-<version>.dmg; needs dmgbuild (pip install dmgbuild==1.6.5).
package-desktop-dmg:
	./scripts/package-desktop-dmg.sh $(VERSION)

# The DMG plus the signed Sparkle update (Hisar-<version>.zip, update.json); SPARKLE_PRIVATE_KEY is the key file's path.
package-desktop-update: package-desktop-dmg
	./scripts/package-desktop-update.sh $(VERSION) "$(SPARKLE_PRIVATE_KEY)" $(RELEASE_NOTES)

# desktop/bin/Hisar-<version>-linux-<arch>.{deb,rpm,AppImage} for this machine's architecture; on Linux with GTK 4 and WebKitGTK 6.0 headers.
package-desktop-linux:
	./scripts/package-desktop-linux.sh $(VERSION)

# desktop/bin/Hisar-<version>-linux-<arch>.flatpak; needs flatpak-builder (as in Flathub's builder container) and network.
package-desktop-flatpak:
	./scripts/package-desktop-flatpak.sh $(VERSION)

# The gomobile version follows golang.org/x/mobile in mobile/go.mod.
build-ios-framework:
	cd mobile && GOWORK=off go install golang.org/x/mobile/cmd/gomobile golang.org/x/mobile/cmd/gobind
	./mobile/build-framework.sh

# The generated Hisar.xcodeproj is committed; regenerate it after changing mobile/ios/project.yml (brew install xcodegen).
generate-ios-project:
	cd mobile/ios && xcodegen generate

IOS_PROJECT := mobile/ios/Hisar.xcodeproj
IOS_SIMULATOR_ID ?= $(shell mobile/ios/scripts/find-simulator.sh)

IOS_DERIVED_DATA_FLAG = $(if $(IOS_DERIVED_DATA),-derivedDataPath "$(IOS_DERIVED_DATA)")

# The app with its extension and both test bundles, for the simulator the tests run on, and the personal team variant.
# Needs mobile/build/HisarKit.xcframework (make build-ios-framework).
build-ios:
	xcodebuild build-for-testing -project $(IOS_PROJECT) -scheme Hisar -destination "id=$(IOS_SIMULATOR_ID)" $(IOS_DERIVED_DATA_FLAG) -quiet
	xcodebuild build -project $(IOS_PROJECT) -scheme "Hisar Personal Team" -destination "id=$(IOS_SIMULATOR_ID)" $(IOS_DERIVED_DATA_FLAG) -quiet

# IOS_TESTS_PREBUILT=1 runs the tests make build-ios built, as CI does, instead of building them again. The tests start once the
# simulator has finished booting; bootstatus boots it if it is not running.
test-ios:
	xcrun simctl bootstatus "$(IOS_SIMULATOR_ID)" -b
	xcodebuild $(if $(IOS_TESTS_PREBUILT),test-without-building -scheme Hisar,test -scheme HisarUnitTests) -only-testing:HisarTests \
		-project $(IOS_PROJECT) -destination "id=$(IOS_SIMULATOR_ID)" $(IOS_DERIVED_DATA_FLAG)

# Needs HISAR_DATABASE_URL pointing at an empty database; the script builds and starts the server (scripts/ui-test-partner.sh).
test-ios-ui:
	IOS_SIMULATOR_ID=$(IOS_SIMULATOR_ID) ./scripts/test-ios-ui.sh

# The pinned Android SDK, NDK and emulator images in ANDROID_HOME, and the AVDs hisar-api29, hisar-api34 and hisar-api37.
install-android-sdk:
	./scripts/install-android-sdk.sh

# mobile/build/HisarKit.aar for arm64-v8a and x86_64; needs ANDROID_HOME, ANDROID_NDK_HOME and JDK 21. The gomobile version
# follows golang.org/x/mobile in mobile/go.mod.
build-android-aar:
	cd mobile && GOWORK=off go install golang.org/x/mobile/cmd/gomobile golang.org/x/mobile/cmd/gobind
	./mobile/build-android-aar.sh

ANDROID_GRADLE := cd mobile/android && ./gradlew --console=plain

# The debug and release APKs, the instrumented test APK and Android lint; needs mobile/build/HisarKit.aar.
build-android:
	$(ANDROID_GRADLE) assembleDebug assembleDebugAndroidTest assembleRelease lint

# The JVM tests, IosTestParityTest among them; needs mobile/build/HisarKit.aar.
test-android:
	$(ANDROID_GRADLE) :model:test :app:testDebugUnitTest

# The instrumented tests on the one booted emulator or device, or on ANDROID_SERIAL, against a server the script starts; needs
# HISAR_DATABASE_URL pointing at an empty database. ANDROID_UI_TESTS=<names> or ANDROID_UI_TESTS=smoke runs a subset.
test-android-ui: build-android-aar
	./scripts/test-android-ui.sh

test-extension:
	cd extension && npm ci --no-audit --no-fund && npm run typecheck && npm test

# Needs Playwright's Chromium: cd extension && npx playwright install chromium
test-extension-end-to-end:
	cd extension && npm ci --no-audit --no-fund && npm run test:end-to-end

# The Firefox end-to-end tests, through geckodriver; the first run downloads the pinned Firefox and geckodriver into
# extension/.test-browsers. HISAR_TEST_FIREFOX=esr_140.17.0esr runs the oldest Firefox the add-on supports.
test-extension-firefox-end-to-end:
	cd extension && npm ci --no-audit --no-fund && npm run test:end-to-end-firefox

# The share link recipient page in Chromium against the real server; needs HISAR_DATABASE_URL and Playwright's Chromium.
test-share-link-page:
	go build -o bin/hisar-share-link-page-fixture ./server/cmd/hisar-share-link-page-fixture
	cd extension && npm ci --no-audit --no-fund && npm run test:share-link-page

# The Chrome zip and the Firefox one; addons-linter checks the Firefox build as AMO's signing does.
build-extension:
	cd extension && npm ci --no-audit --no-fund && npm run build && npm run lint:firefox && npm run package

# The docs site (docs/, Docusaurus); test-docs typechecks and builds it, the image is docs/Dockerfile.
build-docs:
	cd docs && npm ci --no-audit --no-fund && npm run build

test-docs:
	cd docs && npm ci --no-audit --no-fund && npm run typecheck && npm run build

# The Raycast extension; its build writes raycast/dist and never installs into Raycast (npm run dev does, for development).
test-raycast:
	cd raycast && npm ci --no-audit --no-fund && npm run typecheck && npm test && npm run lint && npm run build

# The Hisar plugin for omp, typechecked against the omp version Tern ships (omp/package.json).
test-omp:
	cd omp && bun install --frozen-lockfile && bun run typecheck && bun test && bun run lint
