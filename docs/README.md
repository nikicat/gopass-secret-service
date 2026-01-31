# gopass-secret-service

A D-Bus Secret Service provider that uses [GoPass](https://www.gopass.pw/) as the backend storage.

This enables desktop applications (browsers, email clients, etc.) to store and retrieve secrets via the standard [freedesktop.org Secret Service API](https://specifications.freedesktop.org/secret-service/latest/) while GoPass handles the actual encryption and storage.

## Features

- Full implementation of the Secret Service D-Bus API
- Uses GoPass for secure, GPG-encrypted secret storage
- Support for multiple collections
- Attribute-based secret searching
- Collection and item signals (created, deleted, changed)
- Alias support (including the "default" alias)

## Prerequisites

- [GoPass](https://www.gopass.pw/) installed and configured
- GPG key set up for GoPass
- Go 1.21+ (for building from source)

## Installation

### From Source

```bash
git clone https://github.com/yourusername/gopass-secret-service.git
cd gopass-secret-service
make build
make install
```

### D-Bus Activation

The installation creates a D-Bus service file that allows the service to be auto-started when an application needs it:

```bash
# The service file is installed to:
~/.local/share/dbus-1/services/org.freedesktop.secrets.service
```

## Usage

### Starting the Service

```bash
# Run in foreground
gopass-secret-service

# Run with debug logging
gopass-secret-service -d

# Run with custom prefix
gopass-secret-service -p my-secrets
```

### CLI Options

```
gopass-secret-service [options]

Options:
  -c, --config PATH        Path to config file (default: ~/.config/gopass-secret-service/config.yaml)
  -s, --store-path PATH    GoPass store path (default: ~/.local/share/gopass/stores/root)
  -p, --prefix PREFIX      Prefix for secret-service entries in gopass (default: "secret-service")
  -v, --verbose            Enable verbose logging
  -d, --debug              Enable debug logging
      --version            Print version and exit
  -h, --help               Show help message
```

### Configuration File

Create `~/.config/gopass-secret-service/config.yaml`:

```yaml
# GoPass store path
store_path: ~/.local/share/gopass/stores/root

# Prefix in gopass for Secret Service entries
prefix: secret-service

# Default collection name
default_collection: default

# Logging level: debug, info, warn, error
log_level: info

# Log file path (empty for stderr)
log_file: ""

# Replace existing secret-service provider
replace: false
```

### Testing with secret-tool

```bash
# Store a secret
secret-tool store --label='Test Secret' service smtp server mail.example.com

# Look up a secret
secret-tool lookup service smtp server mail.example.com

# Search for secrets
secret-tool search service smtp

# Clear a secret
secret-tool clear service smtp server mail.example.com
```

### Testing with Python

```python
import secretstorage

# Connect to the Secret Service
conn = secretstorage.dbus_init()

# Get the default collection
collection = secretstorage.get_default_collection(conn)

# Create a new secret
item = collection.create_item(
    'My Secret',
    {'application': 'myapp', 'type': 'password'},
    b'supersecret123'
)

# Retrieve the secret
print(item.get_secret())

# Search for secrets
for item in collection.search_items({'application': 'myapp'}):
    print(item.get_label(), item.get_secret())
```

## How It Works

### Storage Structure

Secrets are stored in GoPass under a configurable prefix (default: `secret-service`):

```
~/.password-store/
└── secret-service/
    ├── default/              # Default collection
    │   ├── {uuid}.gpg        # Items identified by UUID
    │   └── _meta.gpg         # Collection metadata
    ├── login/                # Another collection
    │   ├── {uuid}.gpg
    │   └── _meta.gpg
    └── _aliases.gpg          # Collection alias mappings
```

### Secret Format

Each secret is stored in GoPass with the following format:

```
the-actual-secret-value
---
_ss_label: My Secret Label
_ss_created: 2024-01-15T10:30:00Z
_ss_modified: 2024-01-15T10:30:00Z
_ss_content_type: text/plain
username: john@example.com
xdg:schema: org.gnome.keyring.NetworkPassword
```

The first line is the secret value, followed by metadata (prefixed with `_ss_`) and user-defined attributes.

## Troubleshooting

### Another secret service is already running

If you get an error about the D-Bus name being taken:

```bash
# Check what's running
dbus-send --session --print-reply --dest=org.freedesktop.DBus /org/freedesktop/DBus \
  org.freedesktop.DBus.GetNameOwner string:org.freedesktop.secrets

# Kill the existing service or use --replace flag
gopass-secret-service --replace
```

### GPG agent issues

Make sure your GPG agent is running and configured:

```bash
# Start the agent
gpg-agent --daemon

# Test GPG
echo "test" | gpg --encrypt -r your@email.com | gpg --decrypt
```

### Debug logging

Enable debug logging to see detailed information:

```bash
gopass-secret-service -d 2>&1 | tee /tmp/secret-service.log
```

## License

MIT License - see LICENSE file for details.
