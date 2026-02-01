# Dockerfile for building and testing gopass-secret-service
#
# Build:   docker build -t gopass-secret-service-test .
# Run:     docker run --rm gopass-secret-service-test
# Shell:   docker run --rm -it gopass-secret-service-test /bin/bash

FROM golang:1.25.6-bookworm

# Install dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    # D-Bus
    dbus \
    dbus-x11 \
    # secret-tool (libsecret)
    libsecret-tools \
    # Python with secretstorage
    python3 \
    python3-secretstorage \
    # GPG for gopass
    gnupg \
    pinentry-tty \
    # Utilities
    procps \
    && rm -rf /var/lib/apt/lists/*

# Install gopass
ARG GOPASS_VERSION=1.15.14
RUN curl -fsSL "https://github.com/gopasspw/gopass/releases/download/v${GOPASS_VERSION}/gopass_${GOPASS_VERSION}_linux_amd64.deb" -o /tmp/gopass.deb \
    && dpkg -i /tmp/gopass.deb \
    && rm /tmp/gopass.deb

# Set up working directory
WORKDIR /app

# Copy go.mod and go.sum first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source
COPY . .

# Build the binary
RUN go build -o gopass-secret-service ./cmd/gopass-secret-service

# Default command runs the tests
CMD ["./test.sh"]
