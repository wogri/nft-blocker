BINARY := nft-blocker
MODULE := github.com/wogri/nft-blocker

# Run unit tests
.PHONY: test
test:
	go test -v -count=1 ./...

# Default: test then build for current platform
.PHONY: build
build: test
	go build -o $(BINARY) .

# Static cross-compile for x86_64 GNU/Linux
.PHONY: linux
linux: test
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(BINARY)-linux-amd64 .

.PHONY: clean
clean:
	rm -f $(BINARY) $(BINARY)-linux-amd64

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: all
all: tidy build linux
