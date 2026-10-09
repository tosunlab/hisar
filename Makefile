MODULES := core server cli mobile
CLI_TARGETS := linux/amd64 linux/arm64 darwin/arm64 darwin/amd64 windows/amd64
WAILS_VERSION := v3.0.0-beta.23

.PHONY: build build-cli-all test vet test-end-to-end docker install-wails test-desktop-frontend build-desktop package-desktop test-extension test-extension-end-to-end build-extension build-ios-framework generate-ios-project build-ios test-ios test-ios-ui

build:
	go build -o bin/hisar-server ./server/cmd/hisar-server
	go build -o bin/hisar ./cli/cmd/hisar

build-cli-all:
	@for t in $(CLI_TARGETS); do \
		echo "hisar $$t"; \
		GOOS=$${t%/*} GOARCH=$${t#*/} CGO_ENABLED=0 go build -o /dev/null ./cli/cmd/hisar || exit 1; \
	done

# -p 1: database tests in different packages share one schema.
# desktop: the vault, biometric and agent packages; the Wails main package needs cgo and the built frontend.
test:
	@for m in $(MODULES); do (cd $$m && go test -p 1 -count=1 $(TESTFLAGS) ./...) || exit 1; done
	cd desktop && go test -count=1 $(TESTFLAGS) ./vault/ ./biometric/ ./agent/ ./appinfo/ ./browser/ ./passkey/ ./cmd/...

vet:
	@for m in $(MODULES); do (cd $$m && go vet ./...) || exit 1; done
	cd desktop && go vet ./vault/ ./biometric/ ./agent/ ./appinfo/ ./browser/ ./passkey/ ./cmd/...

test-end-to-end: build
	./scripts/test-end-to-end.sh

docker:
	docker build -f deploy/Dockerfile -t hisar-server:dev .

install-wails:
	go install github.com/wailsapp/wails/v3/cmd/wails3@$(WAILS_VERSION)

test-desktop-frontend:
	cd desktop/frontend && npm ci --no-audit --no-fund && npm run typecheck && npm test && npm run check:bundle

build-desktop:
	cd desktop && wails3 build

package-desktop:
	cd desktop && wails3 task package

# The gomobile version follows golang.org/x/mobile in mobile/go.mod.
build-ios-framework:
	cd mobile && GOWORK=off go install golang.org/x/mobile/cmd/gomobile golang.org/x/mobile/cmd/gobind
	./mobile/build-framework.sh

# The generated Hisar.xcodeproj is committed; regenerate it after changing mobile/ios/project.yml (brew install xcodegen).
generate-ios-project:
	cd mobile/ios && xcodegen generate

IOS_PROJECT := mobile/ios/Hisar.xcodeproj
IOS_SIMULATOR_ID ?= $(shell mobile/ios/scripts/find-simulator.sh)

# Needs mobile/build/HisarKit.xcframework (make build-ios-framework).
build-ios:
	xcodebuild build -project $(IOS_PROJECT) -scheme Hisar -destination "generic/platform=iOS Simulator" -quiet
	xcodebuild build -project $(IOS_PROJECT) -scheme "Hisar Personal Team" -destination "generic/platform=iOS Simulator" -quiet

test-ios:
	xcodebuild test -project $(IOS_PROJECT) -scheme HisarUnitTests -destination "id=$(IOS_SIMULATOR_ID)"

# Needs bin/hisar-server (make build) and HISAR_DATABASE_URL pointing at an empty database.
test-ios-ui:
	IOS_SIMULATOR_ID=$(IOS_SIMULATOR_ID) ./scripts/test-ios-ui.sh

test-extension:
	cd extension && npm ci --no-audit --no-fund && npm run typecheck && npm test

# Needs Playwright's Chromium: cd extension && npx playwright install chromium
test-extension-end-to-end:
	cd extension && npm ci --no-audit --no-fund && npm run test:end-to-end

build-extension:
	cd extension && npm ci --no-audit --no-fund && npm run build && npm run package
