# Downleaf

Mount Overleaf projects as local directories and edit LaTeX files with your favorite editor.

Downleaf runs a local WebDAV server that maps Overleaf project files to a standard filesystem. No kernel extensions required — works on macOS, Linux, and Windows.

## Quick Start

### 1. Build

```bash
git clone https://github.com/FanBB2333/downleaf.git
cd downleaf
go build -o downleaf ./cmd/downleaf
```

### 2. Configure

Copy your Overleaf session cookie from the browser and create a `.env` file:

```
SITE=https://www.overleaf.com/
COOKIES=overleaf_session2=s%3Axxxxxxxxxx...
```

> How to get the cookie: Log in to Overleaf → F12 Developer Tools → Application → Cookies → copy the `overleaf_session2` value

### 3. Mount

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

Changes are automatically synced to Overleaf on save. Press `Ctrl+C` to stop.

## Example: Editing a Paper with Claude Code

```bash
# Terminal 1: Mount in batch mode (defers sync to avoid frequent uploads)
./downleaf mount -i --batch

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

## Commands

| Command | Description |
|---------|-------------|
| `downleaf ls` | List all projects |
| `downleaf mount` | Mount projects locally (default `~/downleaf`) |
| `downleaf mount -i --batch` | Interactive project selection, batch mode |
| `downleaf sync` | Push local changes from batch mode |
| `downleaf download <id>` | Download a project to a local directory |
| `downleaf umount` | Unmount |
| `downleaf help` | Show help |

Mount options: `--project <name|id>`, `--batch`, `-i`, `--port <port>`

## Documentation

- [CLI Reference](docs/cli-reference.md)
- [Getting Started](docs/getting-started.md)
- [Architecture](docs/architecture.md)
- [Troubleshooting](docs/troubleshooting.md)

## How It Works

1. Authenticates with Overleaf using session cookies
2. Fetches project file trees and document content via Socket.IO v0
3. Starts a local WebDAV server mapping Overleaf's file structure to a standard filesystem
4. Mounts to a local directory via `mount_webdav` (macOS) or `davfs` (Linux)
5. File changes are written back through the Overleaf REST API

## License

MIT
