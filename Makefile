MODULES := core server cli
CLI_TARGETS := linux/amd64 linux/arm64 darwin/arm64 darwin/amd64 windows/amd64
WAILS_VERSION := v3.0.0-beta.23

.PHONY: build build-cli-all test vet test-end-to-end docker install-wails test-desktop-frontend build-desktop package-desktop

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
	cd desktop && go test -count=1 $(TESTFLAGS) ./vault/ ./biometric/ ./agent/

vet:
	@for m in $(MODULES); do (cd $$m && go vet ./...) || exit 1; done
	cd desktop && go vet ./vault/ ./biometric/ ./agent/

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
