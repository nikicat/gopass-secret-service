BINARY := gopass-secret-service
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.Version=$(VERSION)"

# Installation directories
# Use 'make install' for user-local installation (default, no root required)
# Use 'make install PREFIX=/usr/local' for system-wide installation (requires root)
PREFIX := $(HOME)/.local
BINDIR := $(PREFIX)/bin
DBUS_SERVICE_DIR := $(HOME)/.local/share/dbus-1/services
SYSTEMD_USER_DIR := $(HOME)/.config/systemd/user
SERVICE_UNIT := gopass-secret-service.service

.PHONY: all build clean install install-user install-system install-service uninstall test lint fmt run config-dir

all: build

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/gopass-secret-service

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

# Install as a systemd user service
install-service: install
	@mkdir -p $(SYSTEMD_USER_DIR)
	@printf '[Unit]\nDescription=GoPass Secret Service - D-Bus Secret Service backed by GoPass\n\n[Service]\nType=simple\nExecStart=$(BINDIR)/$(BINARY)\nRestart=on-failure\nRestartSec=3\n\n[Install]\nWantedBy=default.target\n' > $(SYSTEMD_USER_DIR)/$(SERVICE_UNIT)
	systemctl --user daemon-reload
	systemctl --user enable $(SERVICE_UNIT)
	@echo ""
	@echo "Systemd service installed and enabled."
	@echo "  Unit: $(SYSTEMD_USER_DIR)/$(SERVICE_UNIT)"
	@echo "  Start with: systemctl --user start $(SERVICE_UNIT)"

uninstall:
	rm -f $(BINDIR)/$(BINARY)
	rm -f /usr/local/bin/$(BINARY)
	rm -f $(DBUS_SERVICE_DIR)/org.freedesktop.secrets.service
	@if [ -f $(SYSTEMD_USER_DIR)/$(SERVICE_UNIT) ]; then \
		systemctl --user disable --now $(SERVICE_UNIT) 2>/dev/null || true; \
		rm -f $(SYSTEMD_USER_DIR)/$(SERVICE_UNIT); \
		systemctl --user daemon-reload; \
	fi
	@echo "Uninstalled $(BINARY)"

test:
	go test -v ./...

# Run integration tests
test-integration: build
	./test.sh

lint:
	go vet ./...
	golangci-lint run

fmt:
	go fmt ./...

# Development helpers
run: build
	./$(BINARY) -d

# Create config directory
config-dir:
	mkdir -p $(HOME)/.config/gopass-secret-service

# Show help
help:
	@echo "gopass-secret-service Makefile"
	@echo ""
	@echo "Targets:"
	@echo "  build            Build the binary"
	@echo "  install          Install to ~/.local/bin (no root required)"
	@echo "  install-system   Install to /usr/local/bin (requires root)"
	@echo "  install-service  Install + enable as systemd user service"
	@echo "  uninstall        Remove installed files and service"
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
	@echo "Examples:"
	@echo "  make build                    # Build binary"
	@echo "  make install                  # Install to ~/.local/bin"
	@echo "  make install PREFIX=/opt/bin  # Install to custom location"
	@echo "  make install-system           # Install to /usr/local/bin (needs sudo)"
