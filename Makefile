BINARY_NAME=horta_kit

GOOS=linux
GOARCH=arm64

.PHONY: all build clean help update

all: clean build

build:
	env CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) \
	go build -ldflags="-s -w -extldflags '-static'" \
	-o build/$(BINARY_NAME) main.go history.go

clean:
	rm -rf build/

update:
	@echo "Buscando atualizações..."
	git fetch --all
	git reset --hard origin/main
	@echo "Executando programa..."
	chmod +x build/$(BINARY_NAME)
	./build/$(BINARY_NAME)