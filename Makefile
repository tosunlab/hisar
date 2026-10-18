MODULES := core server cli mobile
CLI_TARGETS := linux/amd64 linux/arm64 darwin/arm64 darwin/amd64 windows/amd64 windows/arm64
WAILS_VERSION := v3.0.0-beta.23

.PHONY: build build-admin test-admin test-admin-end-to-end build-cli-all package-cli-release test vet test-end-to-end docker install-wails test-desktop-frontend build-desktop package-desktop package-desktop-dmg package-desktop-update test-extension test-extension-end-to-end build-extension test-share-link-page build-ios-framework generate-ios-project build-ios test-ios test-ios-ui generate-themes

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
	cd admin && npm run test:end-to-end

build-cli-all:
	@for t in $(CLI_TARGETS); do \
		echo "hisar $$t"; \
		GOOS=$${t%/*} GOARCH=$${t#*/} CGO_ENABLED=0 go build -o /dev/null ./cli/cmd/hisar || exit 1; \
	done

# Release archives and SHA256SUMS of the standalone CLI in bin/cli-release, as the cli-<version> release workflow builds them.
package-cli-release:
	./scripts/package-cli-release.sh $(VERSION) bin/cli-release

# -p 1: database tests in different packages share one schema.
# desktop: every package but the Wails main package, which needs cgo and the built frontend.
DESKTOP_PACKAGES = $(shell cd desktop && go list -e ./... | grep -v '^github.com/tryhisar/hisar/desktop$$')

test:
	@for m in $(MODULES); do (cd $$m && go test -p 1 -count=1 $(TESTFLAGS) ./...) || exit 1; done
	cd desktop && go test -count=1 $(TESTFLAGS) $(DESKTOP_PACKAGES)

vet:
	@for m in $(MODULES); do (cd $$m && go vet ./...) || exit 1; done
	cd desktop && go vet $(DESKTOP_PACKAGES)

test-end-to-end: build
	go build -o bin/hisar-smtp-sink ./server/cmd/hisar-smtp-sink
	go build -o bin/hisar-fake-github ./server/cmd/hisar-fake-github
	./scripts/test-end-to-end.sh

# VERSION=1.2.3 builds hisar-server 1.2.3, as the server-<version> release workflow does; without it the version is dev.
docker:
	docker build -f deploy/Dockerfile --build-arg VERSION=$(or $(VERSION),dev) -t hisar-server:$(or $(VERSION),dev) .

install-wails:
	go install github.com/wailsapp/wails/v3/cmd/wails3@$(WAILS_VERSION)

# Writes desktop/frontend/src/themes.css, admin/src/themes.css and mobile/ios/Shared/Views/ThemePalettes.swift from assets/themes/themes.json (Node 22.6 or later).
generate-themes:
	node scripts/themes/generate.ts

test-desktop-frontend:
	cd desktop/frontend && npm ci --no-audit --no-fund && npm run typecheck && npm test && npm run check:bundle

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

# IOS_TESTS_PREBUILT=1 runs the tests make build-ios built, as CI does, instead of building them again.
test-ios:
	xcodebuild $(if $(IOS_TESTS_PREBUILT),test-without-building -scheme Hisar,test -scheme HisarUnitTests) -only-testing:HisarTests \
		-project $(IOS_PROJECT) -destination "id=$(IOS_SIMULATOR_ID)" $(IOS_DERIVED_DATA_FLAG)

# Needs bin/hisar-server and bin/hisar (make build) and HISAR_DATABASE_URL pointing at an empty database.
test-ios-ui:
	IOS_SIMULATOR_ID=$(IOS_SIMULATOR_ID) ./scripts/test-ios-ui.sh

test-extension:
	cd extension && npm ci --no-audit --no-fund && npm run typecheck && npm test

# Needs Playwright's Chromium: cd extension && npx playwright install chromium
test-extension-end-to-end:
	cd extension && npm ci --no-audit --no-fund && npm run test:end-to-end

# The share link recipient page in Chromium against the real server; needs HISAR_DATABASE_URL and Playwright's Chromium.
test-share-link-page:
	go build -o bin/hisar-share-link-page-fixture ./server/cmd/hisar-share-link-page-fixture
	cd extension && npm ci --no-audit --no-fund && npm run test:share-link-page

build-extension:
	cd extension && npm ci --no-audit --no-fund && npm run build && npm run package
