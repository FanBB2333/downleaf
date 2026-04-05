# Downleaf

Mount Overleaf projects into a local workspace and edit LaTeX files with your favorite editor.

Downleaf runs a local WebDAV server that maps Overleaf project files to a standard filesystem. The repository currently contains:

- A desktop app built with Wails at the repo root (`main.go` + `frontend/`)
- A CLI tool in [`cmd/downleaf`](cmd/downleaf)

Native auto-mount is implemented for macOS and Linux. On other platforms, Downleaf still serves WebDAV and can be mounted manually.

## Current Features

- Mount all projects or a selected project into a local directory
- Zen mode (`--zen`) for local-first editing with explicit sync
- Download projects without mounting
- Inspect projects with `ls`, `tree`, and `cat`
- Desktop login UI with saved credentials
- Browser-based login in the desktop app on macOS

## Quick Start (CLI)

### 1. Build

```bash
git clone https://github.com/FanBB2333/downleaf.git
cd downleaf
go build -o downleaf ./cmd/downleaf
```

### 2. Configure authentication

Copy your Overleaf session cookie from the browser and create a `.env` file:

```
SITE=https://www.overleaf.com/
COOKIES=overleaf_session2=s%3Axxxxxxxxxx...
```

> How to get the cookie: Log in to Overleaf → F12 Developer Tools → Application → Cookies → copy the `overleaf_session2` value

If you use a self-hosted Overleaf instance, set `SITE` to its base URL.

### 3. Mount your projects

```bash
./downleaf mount
```

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

In normal mode, changes are uploaded automatically on write. Press `Ctrl+C` to stop.

## Desktop App

The desktop app exposes the same core mount workflow through a GUI and adds login conveniences that do not exist in the CLI:

- Saved credentials
- Browser login on macOS
- Manual cookie login fallback
- Project picker, mount status, and log viewer

The desktop backend lives in [`main.go`](main.go) and [`internal/gui/app.go`](internal/gui/app.go).

## Example: Editing a Paper with Claude Code

```bash
# Terminal 1: Mount in zen mode (keep changes local until you sync)
./downleaf mount -i --zen

# Interactive project selection:
#   Select a project to mount (52 projects):
#     0) [all projects]
#     1) My-Paper
#     2) Another-Project
#   Enter number (0 for all): 1
#   Selected: My-Paper (692fce31ee51890d4f6f14af)

# Terminal 2: Edit with Claude Code
cd ~/downleaf/My-Paper
claude

# Terminal 3: When done, push all changes to Overleaf at once
./downleaf sync
```

`--zen` is the current local-first mode. The old `--batch` flag no longer exists.

## Commands

| Command | Description |
|---------|-------------|
| `downleaf ls` | List all projects |
| `downleaf tree <project-id>` | Show a project's file tree |
| `downleaf cat <project-id> <doc-id>` | Print document content |
| `downleaf download <project-id> [dest-dir]` | Download a project locally without mounting |
| `downleaf mount` | Mount projects locally (interactive selection, default `~/downleaf`) |
| `downleaf mount --zen` | Mount in zen mode (interactive, changes stay local) |
| `downleaf mount --all` | Mount all projects without prompting |
| `downleaf sync` | Push local changes from zen mode |
| `downleaf umount` | Unmount and stop daemon |
| `downleaf version` | Print version |
| `downleaf help` | Show help |

Mount options:

- `--project <name|id>`: mount specific project(s), can be repeated
- `--all`: mount all projects (skip interactive selection)
- `--zen`: keep writes local and sync on `downleaf sync` or `downleaf umount`
- `--foreground`, `-f`: run in foreground (block terminal, Ctrl+C to stop)
- `--port <port>`: set the WebDAV server port (default: `9090`)

## Authentication Modes

CLI authentication currently requires `SITE` and `COOKIES` from `.env` or the environment.

The desktop app supports:

- Browser login on macOS
- Reusing saved credentials
- Manual cookie login

## Platform Notes

- macOS: native auto-mount via `mount_webdav`
- Linux: native auto-mount via `mount -t davfs`
- Other platforms: Downleaf still starts the local WebDAV server, but mounting is manual

If auto-mount fails, the CLI prints manual mounting instructions and the WebDAV URL.

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
