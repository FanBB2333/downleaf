# Downleaf

Mount your [Overleaf](https://www.overleaf.com/) projects as a local filesystem, so you can edit `.tex` files with any editor you like.

## Features

- **WebDAV mount** — access Overleaf projects as regular files via Finder, VS Code, vim, etc.
- **Zen mode** — defer all syncs until you're ready; stay focused on writing.
- **Interactive project selection** — pick which project to mount from a list.
- **Full read/write** — create, edit, rename, move, and delete files and folders.
- **GUI app** — a Wails v2 desktop app with login, mount, sync, and log viewing.

## Quick Start

```bash
# Build
go build -o downleaf ./cmd/downleaf

# Configure (get cookies from your browser DevTools)
cat > .env <<EOF
SITE=https://your-overleaf-instance.com
COOKIES="your_session_cookie"
EOF

# Mount all projects
./downleaf mount

# Or mount a single project in zen mode
./downleaf mount --project "My Thesis" --zen
```

See [Getting Started](getting-started.md) for full setup instructions.
