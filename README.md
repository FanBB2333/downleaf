# Downleaf

Mount Overleaf projects into a local workspace and edit LaTeX files with your favorite editor.

Downleaf runs a local WebDAV server that maps Overleaf project files to a standard filesystem. The repository currently contains:

- A desktop app built with Wails at the repo root (`main.go` + `frontend/`)
- A CLI tool in [`cmd/downleaf`](cmd/downleaf)

Native auto-mount is implemented for macOS and Linux. On other platforms, Downleaf still serves WebDAV and can be mounted manually.

## Current Features

- Mount all projects or a selected project into a local directory
- Interactive project selection by default
- Zen mode (`--zen`) for local-first editing with explicit sync
- Background daemon mode (mount returns to terminal immediately)
- Pluggable mount backends (`--backend webdav`, FUSE planned)
- Download projects without mounting
- Inspect projects with `ls`, `tree`, and `cat`
- Multi-account credential management (`login`, `logout`, `accounts`)
- Browser-based login on all platforms (native WKWebView on macOS, system browser elsewhere)
- Desktop app with GUI login, project picker, and log viewer

## Quick Start (CLI)

### 1. Build

```bash
git clone https://github.com/FanBB2333/downleaf.git
cd downleaf
go build -o downleaf ./cmd/downleaf
```

### 2. Log in

```bash
./downleaf login https://www.overleaf.com
```

This opens a browser window for login (macOS uses a native window; other platforms open the system browser with a helper page). After login, your credential is saved to `~/.downleaf/credentials/`.

Alternatively, create a `.env` file with your session cookie:

```
SITE=https://www.overleaf.com/
COOKIES=overleaf_session2=s%3Axxxxxxxxxx...
```

> How to get the cookie: Log in to Overleaf → F12 Developer Tools → Application → Cookies → copy the `overleaf_session2` value

If you use a self-hosted Overleaf instance, pass its URL to `login` or set `SITE`.

### 3. Mount your projects

```bash
./downleaf mount
```

You'll be prompted to select a project interactively. The mount runs as a background daemon — your terminal is free immediately.

All projects appear under `~/downleaf/`:

```
~/downleaf/
├── My-Paper/
│   ├── main.tex
│   ├── refs.bib
│   └── figures/
│       └── fig1.png
├── Another-Project/
│   └── ...
```

Open with any editor:

```bash
code ~/downleaf/My-Paper          # VS Code
vim ~/downleaf/My-Paper/main.tex  # Vim
cd ~/downleaf/My-Paper && claude  # Claude Code
```

In normal mode, changes are uploaded automatically on write. To stop:

```bash
./downleaf umount
```

This syncs any pending changes and stops the daemon. If running in foreground mode (`-f`), press `Ctrl+C` instead.

## Desktop App

The desktop app provides the same functionality through a GUI:

- Saved credentials with browser login
- Project picker, mount status, and log viewer
- Manual cookie login fallback

The desktop backend lives in [`main.go`](main.go) and [`internal/gui/app.go`](internal/gui/app.go).

## Example: Editing a Paper with Claude Code

```bash
# Mount in zen mode (runs as background daemon)
./downleaf mount --zen
# Interactive project selection:
#   Select a project to mount (52 projects):
#     0) [all projects]
#     1) My-Paper
#     2) Another-Project
#   Enter number (0 for all): 1
#   Downleaf daemon started (PID 12345)

# Edit with Claude Code
cd ~/downleaf/My-Paper
claude

# When done, push all changes to Overleaf at once
./downleaf sync

# Or unmount (also syncs automatically)
./downleaf umount
```

## Commands

| Command | Description |
|---------|-------------|
| `downleaf ls` | List all projects |
| `downleaf tree <project-id>` | Show a project's file tree |
| `downleaf cat <project-id> <doc-id>` | Print document content |
| `downleaf download <project-id> [dest-dir]` | Download a project locally without mounting |
| `downleaf mount` | Mount projects locally (interactive, daemon, default `~/downleaf`) |
| `downleaf mount --zen` | Mount in zen mode (changes stay local until sync) |
| `downleaf mount --all` | Mount all projects without prompting |
| `downleaf sync` | Push local changes from zen mode |
| `downleaf umount` | Sync, stop daemon, and unmount |
| `downleaf login [site-url]` | Log in (browser or cookie) and save credential |
| `downleaf logout [email]` | Remove a saved credential |
| `downleaf accounts` | List saved credentials |
| `downleaf version` | Print version |
| `downleaf help` | Show help |

Mount options:

- `--project <name|id>`: mount specific project(s), can be repeated
- `--all`: mount all projects (skip interactive selection)
- `--zen`: keep writes local and sync on `downleaf sync` or `downleaf umount`
- `--foreground`, `-f`: run in foreground (block terminal, Ctrl+C to stop)
- `--port <port>`: set the WebDAV server port (default: `9090`)
- `--backend <name>`: mount backend (default: `webdav`)

## Authentication

The CLI uses saved credentials by default (stored in `~/.downleaf/credentials/`). Run `downleaf login` to add an account. Environment variables `SITE` + `COOKIES` override stored credentials if set.

Multiple accounts are supported — the most recently used credential is selected automatically. Use `downleaf accounts` to list them, `downleaf logout` to remove one.

Browser login is available on all platforms:
- **macOS**: native WKWebView window (seamless, no manual steps)
- **Linux/Windows**: opens system browser with a helper page for cookie capture

## Platform Notes

- **macOS**: native auto-mount via `mount_webdav`, native browser login via WKWebView
- **Linux**: native auto-mount via `mount -t davfs`, browser login via system browser
- **Windows**: WebDAV server starts but mounting is manual, browser login via system browser

If auto-mount fails, the CLI prints manual mounting instructions and the WebDAV URL.

### macOS metadata files on mounted projects

Finder may create metadata files such as `.DS_Store` and `._*` while browsing mounted projects.

- The recommended fix is Downleaf's `Ignore macOS Dotfiles` setting, which prevents those files from syncing to Overleaf.
- As an optional system-wide mitigation, you can try:

```bash
defaults write com.apple.desktopservices DSDontWriteNetworkStores -bool TRUE
```

Log out and log back in after changing it. Apple documents this for SMB shares; it may reduce `.DS_Store` creation on mounted network volumes, but it is not a complete replacement for Downleaf's ignore setting.

- `DSDontWriteUSBStores` is not relevant for Downleaf mounts:

```bash
defaults write com.apple.desktopservices DSDontWriteUSBStores -bool TRUE
```

This only targets removable USB storage, while Downleaf mounts projects through WebDAV.
- Neither setting reliably prevents `._*` AppleDouble files, so keep the Downleaf ignore option enabled if you want to avoid syncing macOS metadata files.

## How It Works

1. Authenticates with Overleaf using session cookies
2. Fetches project trees and document content via Overleaf APIs and Socket.IO
3. Starts a local WebDAV server that mirrors the Overleaf file structure
4. Mounts that WebDAV server into a local directory when native mounting is available
5. Writes local file changes back to Overleaf immediately, or later in zen mode

## Documentation

- [CLI Reference](docs/cli-reference.md)
- [Getting Started](docs/getting-started.md)
- [Architecture](docs/architecture.md)
- [Troubleshooting](docs/troubleshooting.md)

## License

MIT
