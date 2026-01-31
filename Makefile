BINARY := gopass-secret-service
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.Version=$(VERSION)"
PREFIX := /usr/local
DBUS_SERVICE_DIR := $(HOME)/.local/share/dbus-1/services

.PHONY: all build clean install uninstall test lint fmt

all: build

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/gopass-secret-service

clean:
	rm -f $(BINARY)

install: build
	install -d $(DESTDIR)$(PREFIX)/bin
	install -m 755 $(BINARY) $(DESTDIR)$(PREFIX)/bin/
	install -d $(DBUS_SERVICE_DIR)
	sed 's|@BINDIR@|$(PREFIX)/bin|g' org.freedesktop.secrets.service.in > $(DBUS_SERVICE_DIR)/org.freedesktop.secrets.service

uninstall:
	rm -f $(DESTDIR)$(PREFIX)/bin/$(BINARY)
	rm -f $(DBUS_SERVICE_DIR)/org.freedesktop.secrets.service

test:
	go test -v ./...

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
