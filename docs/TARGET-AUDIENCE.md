# Target Audience & User Personas

## Overview

gopass-secret-service occupies a niche at the intersection of three communities: GoPass users, Linux desktop power users, and security-conscious professionals. The common thread is a desire for **control over secret storage** without sacrificing desktop integration.

## Market Context

The Linux desktop secret management landscape has a gap: applications that use the freedesktop.org Secret Service API (browsers, email clients, GUI password managers, CLI tools like `secret-tool`) default to GNOME Keyring or KDE Wallet. These work fine for most users, but offer limited visibility into what's stored, no GPG-based encryption, and no easy way to sync secrets across machines via git.

GoPass solves the storage side beautifully — GPG encryption, git sync, team sharing, multi-store support — but has no D-Bus interface. Applications can't use it transparently.

**gopass-secret-service bridges this gap.**

---

## Persona 1: The GoPass Convert

**Name:** Marcus, 34, Backend Developer (Berlin)

**Background:** Switched from LastPass to `pass`, then to GoPass about two years ago. Manages ~200 entries across personal and work stores. Uses `git` to sync his password store between a workstation and a laptop. Runs Arch Linux with Sway (Wayland).

**Pain point:** Every time he installs a fresh system or logs into a new session, GNOME Keyring grabs the Secret Service bus name. Applications like VS Code, Slack (Electron), and NetworkManager store WiFi passwords and tokens in GNOME Keyring — a completely separate silo from his GoPass store. He ends up with secrets scattered across two systems.

**What he wants:**
- A single source of truth for all secrets (GoPass)
- Desktop apps storing secrets into GoPass transparently
- No manual copy-paste between `gopass show` and application prompts

**How he found the project:** Searched "gopass dbus secret service" after getting frustrated that `secret-tool lookup` couldn't find his GoPass entries.

**Technical comfort:** High. Comfortable with systemd, D-Bus debugging (`dbus-monitor`), GPG key management, and reading Go source code.

**Usage pattern:**
- Installs via AUR (`yay -S gopass-secret-service`)
- Runs as a systemd user service, auto-starting on login
- Uses `--replace` flag to take over from GNOME Keyring
- Occasionally checks `journalctl --user -u gopass-secret-service` when something misbehaves

---

## Persona 2: The Privacy-First Developer

**Name:** Anya, 29, Security Engineer (Remote, EU)

**Background:** Works on application security for a mid-size SaaS company. Uses GoPass with a hardware security key (YubiKey) for GPG operations. Runs Fedora with GNOME, but has replaced most default components with more transparent alternatives. Audits her own toolchain.

**Pain point:** She doesn't trust GNOME Keyring's storage model — it's opaque, hard to audit, and she can't verify what's stored or who has access. She wants her secrets encrypted with her own GPG key, stored in a git repo she controls, and accessible to desktop applications without changing her workflow.

**What she wants:**
- Full auditability of stored secrets (`gopass ls`, `gopass show`)
- GPG-at-rest encryption she controls (not kernel keyring)
- Ability to review what each application stores via attributes
- No secrets leaking to a keyring she can't inspect

**How she found the project:** Mentioned in the GoPass community chat when she asked about replacing GNOME Keyring.

**Technical comfort:** Very high. Reads D-Bus specifications for fun. Has contributed to GoPass.

**Usage pattern:**
- Builds from source after reading the code
- Runs with debug logging initially to audit what applications are storing
- Creates separate GoPass stores/collections for work vs personal
- Has custom GPG agent config to avoid the pinentry circular dependency

---

## Persona 3: The Tiling WM Minimalist

**Name:** Jake, 26, DevOps Engineer (Portland, OR)

**Background:** Runs a minimal Linux setup — Void Linux, i3wm, no full desktop environment. Doesn't have GNOME Keyring or KDE Wallet installed. Uses GoPass for passwords and `pass-otp` for TOTP codes. His dotfiles are a curated git repo.

**Pain point:** Without a Secret Service provider, many applications either crash, show confusing errors, or fall back to storing credentials in plaintext config files. Python tools using `keyring` or `secretstorage` fail outright. He's been working around this by setting `DBUS_SESSION_BUS_ADDRESS` and running `gnome-keyring-daemon` standalone, which feels wrong.

**What he wants:**
- A lightweight Secret Service provider that doesn't pull in GNOME/KDE dependencies
- Something that fits his minimal stack — single binary, systemd unit, done
- GoPass integration so he doesn't need a separate credential store

**How he found the project:** Reddit thread in r/unixporn about secret management without a full DE.

**Technical comfort:** High. Writes his own systemd units, builds packages from source, uses Makefiles daily.

**Usage pattern:**
- `make build && make install` into `~/.local/bin`
- Writes a custom systemd user unit that starts before his WM
- Config via environment variables in his shell profile
- Rarely interacts with it directly — it just works in the background

---

## Persona 4: The Team Lead Standardizing Tooling

**Name:** Priya, 38, Platform Engineering Lead (London)

**Background:** Manages a team of 12 developers. The team uses GoPass with a shared git repo for service credentials (API keys, database passwords, shared secrets). They've standardized on Ubuntu with GNOME for developer workstations. Currently, developers manually copy secrets from GoPass into their IDEs or tools.

**Pain point:** Developers context-switch between GoPass CLI and their applications constantly. Some have started storing secrets in GNOME Keyring "temporarily" which means they're now in two places. She wants a unified workflow where the team's GoPass store is the single source of truth, accessible to both CLI and GUI applications.

**What she wants:**
- A standard setup she can document and roll out to 12 machines
- Systemd integration that "just works" on Ubuntu
- Compatibility with the applications her team uses (VS Code, Postman, browsers)
- Reduction in "where is this secret stored?" confusion

**How she found the project:** GoPass documentation / ecosystem page.

**Technical comfort:** Moderate-to-high. Comfortable with Linux admin but won't debug D-Bus issues. Expects things to work after following setup docs.

**Usage pattern:**
- Packages it internally (or uses the Makefile install)
- Writes a team wiki page with setup instructions
- Runs `gopass-secret-service install` on each machine
- Files issues when applications don't behave as expected

---

## Common Traits Across All Personas

| Trait | Detail |
|-------|--------|
| **OS** | Linux (no macOS/Windows — D-Bus is Linux/freedesktop only) |
| **Existing GoPass user** | Already invested in GoPass or `pass` ecosystem |
| **Values transparency** | Wants to know where secrets are stored and how |
| **CLI-comfortable** | At minimum comfortable with terminal; most are power users |
| **Privacy-conscious** | Chose GoPass over cloud-based password managers deliberately |
| **Minimal dependencies** | Prefers lean tools over heavy frameworks |

## Who This Is NOT For

- **Users who don't use GoPass** — this is a GoPass integration, not a standalone secret manager
- **Non-Linux users** — D-Bus and the Secret Service API are Linux/freedesktop-specific
- **Users happy with GNOME Keyring/KDE Wallet** — if the default works, there's no reason to switch
- **Non-technical users** — setup requires understanding of systemd, D-Bus, and GPG; there's no GUI
- **Enterprise SSO/vault users** — organizations using HashiCorp Vault, AWS Secrets Manager, or 1Password have different workflows entirely
