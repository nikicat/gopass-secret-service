# gopass-secret-service

A D-Bus Secret Service provider that uses [GoPass](https://www.gopass.pw/) as the backend storage.

This enables desktop applications (browsers, email clients, etc.) to store and retrieve secrets via the standard [freedesktop.org Secret Service API](https://specifications.freedesktop.org/secret-service/latest/) while GoPass handles the actual encryption and storage with GPG.

## Features

- Full implementation of the Secret Service D-Bus API
- Secure storage using GoPass with GPG encryption
- DH key exchange with AES-128-CBC for encrypted D-Bus transport
- Multiple collections with attribute-based searching
- Collection aliases (including "default")
- D-Bus signals for real-time updates
- CLI with config file support

## Prerequisites

- [GoPass](https://www.gopass.pw/) installed and configured with a GPG key
- Go 1.21+ (for building from source)

## Installation

### Build from Source

```bash
git clone https://github.com/nblogist/gopass-secret-service.git
cd gopass-secret-service
make build
make install   # Installs to ~/.local/bin (no root required)
```

The default installation goes to `~/.local/bin`, which doesn't require root permissions. Make sure `~/.local/bin` is in your `PATH`:

```bash
export PATH="$HOME/.local/bin:$PATH"  # Add to your .bashrc/.zshrc
```

### System-wide Installation

```bash
sudo make install-system   # Installs to /usr/local/bin
```

### Installation Options

```bash
make install                     # User-local: ~/.local/bin (default)
make install PREFIX=/opt/myapps  # Custom location
make install-system              # System-wide: /usr/local/bin (requires root)
make help                        # Show all available targets
```

## Quick Start

```bash
# Start the service (or let D-Bus auto-start it)
gopass-secret-service &

# Store a secret
echo "mypassword" | secret-tool store --label='Email' service smtp server mail.example.com

# Retrieve a secret
secret-tool lookup service smtp server mail.example.com

# Search secrets
secret-tool search service smtp
```

## Usage

```
gopass-secret-service [options]

Options:
  -c, --config PATH     Config file path (default: ~/.config/gopass-secret-service/config.yaml)
  -p, --prefix PREFIX   GoPass prefix for secrets (default: "secret-service")
  -r, --replace         Replace existing secret service provider
  -d, --debug           Enable debug logging
  -v, --verbose         Enable verbose logging
      --version         Print version and exit
  -h, --help            Show help
```

## Configuration

Create `~/.config/gopass-secret-service/config.yaml`:

```yaml
# Prefix in gopass for Secret Service entries
prefix: secret-service

# Default collection name
default_collection: default

# Logging level: debug, info, warn, error
log_level: info

# Replace existing secret-service provider on startup
replace: false
```

## How It Works

Secrets are stored in GoPass under a configurable prefix:

```
~/.password-store/
└── secret-service/
    ├── default/           # Default collection
    │   ├── i<uuid>.gpg    # Secret items
    │   └── _meta.gpg      # Collection metadata
    └── _aliases.gpg       # Collection aliases
```

Each secret is stored with its value on the first line, followed by metadata and attributes:

```
the-secret-value
---
_ss_label: My Secret
_ss_created: 2024-01-15T10:30:00Z
_ss_modified: 2024-01-15T10:30:00Z
username: user@example.com
```

## Replacing GNOME Keyring

To use gopass-secret-service instead of GNOME Keyring:

```bash
# Stop GNOME Keyring's secret service component
# (Method varies by distribution)

# Start gopass-secret-service
gopass-secret-service -r  # -r to replace if keyring is still running
```

For permanent replacement, disable the GNOME Keyring secret service component in your session startup.

## Compatibility

Tested with:
- `secret-tool` (libsecret)
- Python `secretstorage` library
- Applications using libsecret (Firefox, Chrome, GNOME apps)

## Documentation

- [Detailed Usage Guide](docs/README.md)
- [Architecture](docs/ARCHITECTURE.md)

## Development

```bash
# Run tests
make test

# Run integration tests
./test.sh

# Build
make build
```

## License

MIT License
