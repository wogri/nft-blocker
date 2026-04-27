BINARY := nft-blocker
MODULE := github.com/wogri/nft-blocker

# Default: build for current platform
.PHONY: build
build:
	go build -o $(BINARY) .

# Static cross-compile for x86_64 GNU/Linux
.PHONY: linux
linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(BINARY)-linux-amd64 .

.PHONY: clean
clean:
	rm -f $(BINARY) $(BINARY)-linux-amd64

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: all
all: tidy build linux
