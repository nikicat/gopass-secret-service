# Promotion Plan

Checklist for increasing project visibility and adoption.

## Social Promotion

### High-Impact, Low-Effort

- [ ] **GoPass ecosystem listing** — Ask GoPass maintainers to list this on gopass.pw and/or the GoPass README as a community integration. Open a PR or issue on their repo. This is where most GoPass users would discover the project.

- [ ] **Arch Wiki mentions** — Add mentions to relevant Arch Wiki pages:
  - [GNOME/Keyring](https://wiki.archlinux.org/title/GNOME/Keyring) — as an alternative Secret Service provider
  - [Pass](https://wiki.archlinux.org/title/Pass) — in the "extensions/integrations" section
  - [Desktop notifications / D-Bus](https://wiki.archlinux.org/title/D-Bus) — if relevant section exists
  - Arch Wiki gets enormous search traffic from exactly the target audience.

- [ ] **Reddit posts** — One well-written post each on:
  - [ ] r/linux — "I replaced GNOME Keyring with GoPass for all my desktop apps"
  - [ ] r/unixporn — pair with a clean setup screenshot if possible
  - [ ] r/golang — technical angle: D-Bus protocol, crypto, Go API design
  - [ ] r/privacy — GPG encryption, auditability, no cloud dependency

- [ ] **Blog post** — Write "Why I replaced GNOME Keyring with GoPass" covering:
  - The problem (secrets split across keyring and GoPass)
  - The circular GPG dependency trap (good technical story)
  - How the project solves it
  - Publish on personal blog, dev.to, or lobste.rs
  - Evergreen content that ranks in search for years.

- [ ] **Hacker News** — "Show HN" post, ideally timed with a milestone (v1.0, blog post, or notable feature). HN loves niche Linux tools that solve real problems cleanly.

### Medium-Term Community Building

- [ ] **GoPass community channels** — Engage in GoPass GitHub Discussions / Discord / Matrix. Answer Secret Service integration questions, link the project naturally when relevant.

- [ ] **Linux distro forums** — Answer existing "how do I replace GNOME Keyring?" threads on:
  - [ ] Fedora Discussion
  - [ ] Ubuntu Discourse
  - [ ] Void Linux forums
  - [ ] NixOS Discourse

- [ ] **Conference talk** — Submit a lightning talk (5 min) to:
  - [ ] FOSDEM (Desktop / Security devroom)
  - [ ] All Systems Go
  - [ ] NixCon
  - [ ] Any Linux desktop / security conference
  - Pitch: "Replacing GNOME Keyring with 2000 lines of Go"

## Technical Promotion

### Packaging — Meet Users Where They Install

- [x] **AUR** — `gopass-secret-service` (done)
- [ ] **Nixpkgs** — High priority. Massive overlap with target audience (privacy-conscious, minimal, tiling WM users).
- [ ] **Fedora COPR** — Fedora users are a core persona.
- [ ] **Homebrew (linuxbrew)** — Easy to set up, captures devs who use Homebrew on Linux.
- [ ] **Alpine APK** — Container and minimal distro users.
- [ ] **Debian/Ubuntu PPA** — Broadest reach but most packaging work.
- [ ] **openSUSE OBS** — Covers openSUSE and can cross-build for Debian/Fedora.

### Project Credibility Signals

- [ ] **CONTRIBUTING.md** — Short guide covering: how to build, how to test, how to submit PRs. Signals the project is open to contributions.

- [ ] **Issue templates** — Bug report + feature request templates in `.github/ISSUE_TEMPLATE/`. Low effort, signals maturity.

- [ ] **Additional badges in README**:
  - [ ] Code coverage (codecov — already in CI, just need the badge)
  - [ ] Go Report Card
  - [ ] Latest release version
  - [ ] AUR package version

- [ ] **Curated release notes** — Write human-readable changelogs for GitHub Releases instead of relying solely on auto-generated notes. Highlight what users care about.

- [ ] **CHANGELOG.md** — Keep a running changelog in the repo (follows [keepachangelog.com](https://keepachangelog.com/) format).

- [ ] **Security policy** — `SECURITY.md` with instructions for reporting vulnerabilities. Standard for any project handling secrets.

### Discoverability

- [ ] **GitHub topics** — Add to the repo: `gopass`, `secret-service`, `dbus`, `gnome-keyring`, `password-manager`, `freedesktop`, `gpg`, `linux`, `libsecret`.

- [ ] **GitHub repo description** — Ensure it matches the README one-liner: "Use GoPass as the backend for the freedesktop.org Secret Service API"

- [ ] **Search-relevant phrases in README** — The terms people actually Google. The current README already covers most of these:
  - "replace gnome keyring"
  - "gopass dbus"
  - "secret service alternative linux"
  - "pass secret service"
  - "libsecret gopass"

### Features That Drive Adoption

- [ ] **Migration tool** — `gopass-secret-service migrate` command (or a documented script) that imports existing GNOME Keyring secrets into GoPass. The #1 friction point for new users who already have secrets in GNOME Keyring.

- [ ] **Status command** — `gopass-secret-service status` showing: is it running, which D-Bus name it owns, how many collections/items, last activity. Useful for debugging and confidence-building.

- [ ] **Shell completions** — Bash, Zsh, Fish completions. Low effort, improves polish, and completion scripts often get packaged by distros (increasing visibility).

- [ ] **Man page** — `gopass-secret-service(1)`. Some distro packagers expect this, and it shows up in `apropos` searches.

## Priority Order

Top 5 highest-ROI actions to do first:

| # | Action | Type | Rationale |
|---|--------|------|-----------|
| 1 | GoPass ecosystem listing | Social | Direct access to every GoPass user |
| 2 | Arch Wiki mentions | Social | Evergreen search traffic from exact audience |
| 3 | GitHub topics + repo metadata | Technical | 5 minutes, permanent discoverability |
| 4 | Nixpkgs package | Technical | Second-largest target audience after Arch |
| 5 | Blog post + Show HN | Social | One-time effort, long-tail results |

Next 5:

| # | Action | Type | Rationale |
|---|--------|------|-----------|
| 6 | CONTRIBUTING.md + issue templates | Technical | Lowers contribution barrier |
| 7 | Reddit posts | Social | Broad reach across multiple communities |
| 8 | Migration tool | Technical | Removes #1 adoption friction |
| 9 | Additional badges | Technical | Instant credibility signals |
| 10 | Fedora COPR | Technical | Expands distro coverage |
