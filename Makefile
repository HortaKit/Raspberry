BINARY_NAME=horta_kit

GOOS=linux
GOARCH=arm64

.PHONY: all build clean help

all: clean build

build:
	env CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) \
	go build -ldflags="-s -w -extldflags '-static'" \
	-o build/$(BINARY_NAME) main.go history.go

clean:
	rm -rf build/