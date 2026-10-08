MODULES := core server cli
CLI_TARGETS := linux/amd64 linux/arm64 darwin/arm64 darwin/amd64 windows/amd64

.PHONY: build build-cli-all test vet test-end-to-end docker

build:
	go build -o bin/hisar-server ./server/cmd/hisar-server
	go build -o bin/hisar ./cli/cmd/hisar

build-cli-all:
	@for t in $(CLI_TARGETS); do \
		echo "hisar $$t"; \
		GOOS=$${t%/*} GOARCH=$${t#*/} CGO_ENABLED=0 go build -o /dev/null ./cli/cmd/hisar || exit 1; \
	done

# -p 1: database tests in different packages share one schema.
test:
	@for m in $(MODULES); do (cd $$m && go test -p 1 -count=1 $(TESTFLAGS) ./...) || exit 1; done

vet:
	@for m in $(MODULES); do (cd $$m && go vet ./...) || exit 1; done

test-end-to-end: build
	./scripts/test-end-to-end.sh

docker:
	docker build -f deploy/Dockerfile -t hisar-server:dev .
