VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
PLATFORMS := linux/amd64 linux/arm64 linux/arm darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

.PHONY: build test install dist clean

build:
	CGO_ENABLED=0 go build -ldflags '$(LDFLAGS)' -o bin/agent-bus ./cmd/agent-bus

test:
	go vet ./...
	go test -race ./...

install: build
	install -Dm755 bin/agent-bus $(HOME)/.local/bin/agent-bus

dist:
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; ext=; [ $$os = windows ] && ext=.exe; \
		echo "dist/agent-bus-$$os-$$arch$$ext"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -ldflags '$(LDFLAGS)' -o dist/agent-bus-$$os-$$arch$$ext ./cmd/agent-bus || exit 1; \
	done

clean:
	rm -rf bin dist
