# Makefile for local-fx-host Chrome Native Messaging host.
# NOTE: use real TAB characters to indent recipes.

.PHONY: build build-win build-mac test clean fmt vet

build:
	go build -o bin/fx-host ./cmd/fx-host

build-win:
	GOOS=windows GOARCH=amd64 go build -o bin/fx-host.exe ./cmd/fx-host

build-mac:
	GOOS=darwin GOARCH=arm64 go build -o bin/fx-host-darwin-arm64 ./cmd/fx-host
	GOOS=darwin GOARCH=amd64 go build -o bin/fx-host-darwin-amd64 ./cmd/fx-host

test:
	go test ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

clean:
	rm -rf bin
