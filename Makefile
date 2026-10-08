MODULES := core server cli

.PHONY: build test vet

build:
	go build -o bin/hisar-server ./server/cmd/hisar-server
	go build -o bin/hisar ./cli/cmd/hisar

test:
	@for m in $(MODULES); do (cd $$m && go test ./...) || exit 1; done

vet:
	@for m in $(MODULES); do (cd $$m && go vet ./...) || exit 1; done
