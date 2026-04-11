BINARY_NAME=cloadex
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X github.com/Ahmedlag/cloadex/cmd.version=$(VERSION)"
ZIP ?= zip -j
TAR ?= tar -czf

.PHONY: build install clean test lint release checksums formula

build:
	go build $(LDFLAGS) -o $(BINARY_NAME) .

install:
	go install $(LDFLAGS) .

clean:
	rm -f $(BINARY_NAME)
	rm -rf dist/

test:
	go test ./...

lint:
	go vet ./...

# Build for all platforms and package as tar.gz archives for release.
release: clean
	@mkdir -p dist
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o dist/$(BINARY_NAME) . && \
		$(TAR) dist/$(BINARY_NAME)-darwin-arm64.tar.gz -C dist $(BINARY_NAME) && \
		rm dist/$(BINARY_NAME)
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY_NAME) . && \
		$(TAR) dist/$(BINARY_NAME)-darwin-amd64.tar.gz -C dist $(BINARY_NAME) && \
		rm dist/$(BINARY_NAME)
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY_NAME) . && \
		$(TAR) dist/$(BINARY_NAME)-linux-amd64.tar.gz -C dist $(BINARY_NAME) && \
		rm dist/$(BINARY_NAME)
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o dist/$(BINARY_NAME) . && \
		$(TAR) dist/$(BINARY_NAME)-linux-arm64.tar.gz -C dist $(BINARY_NAME) && \
		rm dist/$(BINARY_NAME)
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY_NAME).exe . && \
		$(ZIP) dist/$(BINARY_NAME)-windows-amd64.zip dist/$(BINARY_NAME).exe && \
		rm dist/$(BINARY_NAME).exe
	GOOS=windows GOARCH=arm64 go build $(LDFLAGS) -o dist/$(BINARY_NAME).exe . && \
		$(ZIP) dist/$(BINARY_NAME)-windows-arm64.zip dist/$(BINARY_NAME).exe && \
		rm dist/$(BINARY_NAME).exe

# Print SHA256 checksums for release archives (use to fill Formula/cloadex.rb).
checksums: release
	@echo "SHA256 checksums for Formula/cloadex.rb:"
	@cd dist && shasum -a 256 *.tar.gz *.zip

# Update Formula/cloadex.rb with real checksums from the release build.
formula: release
	@echo "Updating Formula/cloadex.rb checksums..."
	@for pair in "darwin-arm64:on_macos.*on_arm" "darwin-amd64:on_macos.*on_intel" \
	             "linux-arm64:on_linux.*on_arm" "linux-amd64:on_linux.*on_intel"; do \
		archive=$$(echo "$$pair" | cut -d: -f1); \
		hash=$$(shasum -a 256 dist/$(BINARY_NAME)-$$archive.tar.gz | cut -d' ' -f1); \
		pattern=$$(echo "$$pair" | cut -d: -f2); \
		sed -i '' "/$$pattern/{n;n;s/sha256 \".*\"/sha256 \"$$hash\"/;}" Formula/cloadex.rb; \
	done
	@sed -i '' 's/version ".*"/version "$(VERSION)"/' Formula/cloadex.rb
	@echo "Done. Verify with: git diff Formula/cloadex.rb"
