MAKEFLAGS += -j

BINARY := gopass-secret
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.Version=$(VERSION)"

# Installation directories
# Use 'make install' for user-local installation (default, no root required)
# Use 'make install PREFIX=/usr/local' for system-wide installation (requires root)
PREFIX := $(HOME)/.local
BINDIR := $(PREFIX)/bin
DBUS_SERVICE_DIR := $(HOME)/.local/share/dbus-1/services

.PHONY: all build clean install install-user install-system uninstall test lint fmt run \
	check check-go-fmt check-go-vet check-go-staticcheck pre-commit

all: build

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/gopass-secret

clean:
	rm -f $(BINARY)

# Default install: user-local (no root required)
install: build
	@echo "Installing to $(BINDIR) (user-local)"
	@mkdir -p $(BINDIR)
	@mkdir -p $(DBUS_SERVICE_DIR)
	install -m 755 $(BINARY) $(BINDIR)/
	sed 's|@BINDIR@|$(BINDIR)|g' org.freedesktop.secrets.service.in > $(DBUS_SERVICE_DIR)/org.freedesktop.secrets.service
	@echo ""
	@echo "Installation complete!"
	@echo "  Binary: $(BINDIR)/$(BINARY)"
	@echo "  D-Bus service: $(DBUS_SERVICE_DIR)/org.freedesktop.secrets.service"
	@echo ""
	@echo "To install as a systemd service, run:"
	@echo "  $(BINDIR)/$(BINARY) service install"
	@echo ""
	@if ! echo "$$PATH" | grep -q "$(BINDIR)"; then \
		echo "NOTE: $(BINDIR) is not in your PATH."; \
		echo "Add it with: export PATH=\"$(BINDIR):\$$PATH\""; \
		echo ""; \
	fi

# Alias for user-local install
install-user: install

# System-wide install (requires root)
install-system: build
	@echo "Installing to /usr/local/bin (system-wide, requires root)"
	install -d /usr/local/bin
	install -m 755 $(BINARY) /usr/local/bin/
	@mkdir -p $(DBUS_SERVICE_DIR)
	sed 's|@BINDIR@|/usr/local/bin|g' org.freedesktop.secrets.service.in > $(DBUS_SERVICE_DIR)/org.freedesktop.secrets.service
	@echo ""
	@echo "Installation complete!"
	@echo "  Binary: /usr/local/bin/$(BINARY)"
	@echo "  D-Bus service: $(DBUS_SERVICE_DIR)/org.freedesktop.secrets.service"

uninstall:
	rm -f $(BINDIR)/$(BINARY)
	rm -f /usr/local/bin/$(BINARY)
	rm -f $(DBUS_SERVICE_DIR)/org.freedesktop.secrets.service
	@echo "Uninstalled $(BINARY)"
	@echo "To remove the systemd service, run: gopass-secret service uninstall"

# Run checks and tests in parallel
pre-commit: check test

# --- Checks (linters, formatters, static analysis) ---

check: lint check-go-fmt check-go-vet check-go-staticcheck

check-go-fmt:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "Go files need formatting:"; \
		echo "$$unformatted"; \
		echo "Run 'make fmt' to fix."; \
		exit 1; \
	fi

check-go-vet:
	go vet ./...

check-go-staticcheck:
	@out=$$(staticcheck ./... 2>&1 | grep -v '^-:'); \
	if [ -n "$$out" ]; then \
		echo "$$out"; \
		exit 1; \
	fi

test:
	go test -v ./...

# Run integration tests
test-integration: build
	./test.sh

lint:
	golangci-lint run

fmt:
	go fmt ./...

# Development helpers
run: build
	./$(BINARY) service -d

# Show help
help:
	@echo "gopass-secret Makefile"
	@echo ""
	@echo "Targets:"
	@echo "  build            Build the binary"
	@echo "  install          Install to ~/.local/bin (no root required)"
	@echo "  install-system   Install to /usr/local/bin (requires root)"
	@echo "  uninstall        Remove installed files"
	@echo "  check            Run all checks (fmt, vet, staticcheck)"
	@echo "  test             Run unit tests"
	@echo "  test-integration Run integration tests"
	@echo "  run              Build and run with debug logging"
	@echo "  clean            Remove built binary"
	@echo "  fmt              Format code"
	@echo "  lint             Run linters"
	@echo "  help             Show this help"
	@echo ""
	@echo "Variables:"
	@echo "  PREFIX           Installation prefix (default: ~/.local)"
	@echo ""
	@echo "Systemd service:"
	@echo "  gopass-secret service install    # Install systemd user service"
	@echo "  gopass-secret service uninstall  # Remove systemd user service"
