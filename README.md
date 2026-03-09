# gopass-secret-service

[![CI](https://github.com/nikicat/gopass-secret-service/actions/workflows/ci.yml/badge.svg)](https://github.com/nikicat/gopass-secret-service/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Use [GoPass](https://www.gopass.pw/) as the backend for the [freedesktop.org Secret Service API](https://specifications.freedesktop.org/secret-service/latest/).

```
 Desktop Apps (Firefox, Chrome, VS Code, Electron apps, ...)
                    ↕ D-Bus Secret Service API
        gopass-secret-service (this project)
                    ↕
            GoPass → GPG-encrypted files
                    ↕
          ~/.password-store/ (git-syncable)
```

## Why

Linux desktop apps store secrets (passwords, tokens, API keys) via the Secret Service API, which typically means GNOME Keyring or KDE Wallet. If you use GoPass, your secrets end up in two places — GoPass for the terminal, keyring for GUI apps.

**gopass-secret-service** bridges this gap. Desktop apps write secrets into GoPass transparently, giving you:

- **One store for everything** — CLI and GUI apps use the same GoPass entries
- **GPG encryption you control** — audit with `gopass ls`, inspect with `gopass show`
- **Git sync across machines** — GoPass's built-in git support works for all secrets
- **No GNOME/KDE dependency** — single binary, works with any window manager or DE
- **Drop-in replacement** for GNOME Keyring's Secret Service component

## Compatibility

**Works with** any application that uses libsecret or the Secret Service D-Bus API:

Firefox, Chromium/Chrome, VS Code, Electron apps (Slack, Discord, etc.), NetworkManager, GNOME apps, `secret-tool`, Python `secretstorage`, and more.

**Replaces** the Secret Service component of GNOME Keyring or KDE Wallet. Other keyring functions (SSH agent, GPG agent) are unaffected.

## Quick Start

**Prerequisites:** [GoPass](https://www.gopass.pw/) installed and configured with a GPG key.

```bash
# Install
git clone https://github.com/nikicat/gopass-secret-service.git
cd gopass-secret-service
make build && make install   # installs to ~/.local/bin

# Start (replace GNOME Keyring if running)
gopass-secret-service -r &

# Verify — store and retrieve a secret
echo "test123" | secret-tool store --label='Test' app test
secret-tool lookup app test   # → test123

# Install as a systemd user service (auto-start on login)
gopass-secret-service install
```

## Replacing GNOME Keyring

The most common setup — use gopass-secret-service instead of GNOME Keyring for secret storage:

```bash
# One-time: start with --replace to take over the D-Bus name
gopass-secret-service -r

# Permanent: install as a systemd user service
gopass-secret-service install
systemctl --user start gopass-secret-service
```

To prevent GNOME Keyring from grabbing the Secret Service bus name at login, disable its secret service component:

```bash
# Copy the desktop file to override it
cp /etc/xdg/autostart/gnome-keyring-secrets.desktop ~/.config/autostart/
echo "Hidden=true" >> ~/.config/autostart/gnome-keyring-secrets.desktop
```

Or simply use the `-r` / `--replace` flag — gopass-secret-service will take over from whatever is currently running.

## Installation

### Build from Source

```bash
git clone https://github.com/nikicat/gopass-secret-service.git
cd gopass-secret-service
make build
make install   # Installs to ~/.local/bin (no root required)
```

Make sure `~/.local/bin` is in your `PATH`:

```bash
export PATH="$HOME/.local/bin:$PATH"  # Add to your .bashrc/.zshrc
```

### System-wide Installation

```bash
sudo make install-system   # Installs to /usr/local/bin
```

### Other Options

```bash
make install PREFIX=/opt/myapps  # Custom location
make help                        # Show all available targets
```

## Usage

```bash
gopass-secret-service              # start in foreground
gopass-secret-service -r           # replace existing provider (e.g., GNOME Keyring)
gopass-secret-service -d           # debug logging
gopass-secret-service install      # install systemd user service
gopass-secret-service uninstall    # remove systemd user service
```

See the [full CLI, configuration, and environment variable reference](docs/README.md) for all options.

## How It Works

Secrets are stored in GoPass under a configurable prefix (default: `secret-service`):

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

## Troubleshooting

### Startup Hangs with GPG Passphrase Prompt

If gopass-secret-service hangs on startup waiting for a GPG passphrase (with a ~1 minute timeout), you may have a circular dependency:

1. gopass-secret-service starts and initializes GoPass
2. GoPass needs to decrypt the store, triggering GPG
3. GPG uses `pinentry-gnome3` to prompt for the passphrase
4. `pinentry-gnome3` tries to check libsecret (Secret Service) for cached passphrases
5. But gopass-secret-service hasn't finished starting yet → deadlock

**Solution:** Disable external password cache in gpg-agent while keeping pinentry-gnome3:

```bash
echo "no-allow-external-cache" >> ~/.gnupg/gpg-agent.conf
gpgconf --kill gpg-agent
```

This only disables libsecret integration. GPG-agent's internal passphrase cache still works (controlled by `default-cache-ttl`, default 10 minutes), so you won't be prompted repeatedly.

To increase the cache duration:

```bash
cat >> ~/.gnupg/gpg-agent.conf << 'EOF'
no-allow-external-cache
default-cache-ttl 28800
max-cache-ttl 28800
EOF
gpgconf --kill gpg-agent
```

**Alternative:** Use a pinentry that doesn't use libsecret:

```bash
echo "pinentry-program /usr/bin/pinentry-qt" >> ~/.gnupg/gpg-agent.conf
gpgconf --kill gpg-agent
```

Other options: `pinentry-gtk`, `pinentry-curses`, `pinentry-tty`.

## Documentation

- [Full CLI & Configuration Reference](docs/README.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Target Audience & User Personas](docs/TARGET-AUDIENCE.md)

## Development

```bash
make test       # Run tests
./test.sh       # Run integration tests (Docker)
make build      # Build
make lint       # Lint
```

## License

MIT License
