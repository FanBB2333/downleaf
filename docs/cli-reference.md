# Downleaf CLI Reference

## Synopsis

```
downleaf <command> [arguments] [flags]
```

Downleaf mounts Overleaf projects as a local filesystem via WebDAV, so you can edit them with any local editor (VS Code, vim, Claude Code, etc.).

---

## Commands

### `ls`

List all projects in your account.

```
downleaf ls
```

Example output:
```
52 projects:
  692fce31ee51890d4f6f14af  test-thesis
  67cd600ad849d892c3f220f1  Resume
  ...
```

---

### `tree`

Show the file tree of a project (including entity IDs).

```
downleaf tree <project-id>
```

Example:
```
downleaf tree 692fce31ee51890d4f6f14af
```

Output:
```
Project: test-thesis
main.tex (doc: 692fce31ee51890d4f6f14b4)
```

---

### `cat`

Print document content. Uses Socket.IO joinDoc for text files (`.tex`, etc.) and falls back to REST API download for binary files.

```
downleaf cat <project-id> <doc-id>
```

Example:
```
downleaf cat 692fce31ee51890d4f6f14af 692fce31ee51890d4f6f14b4
```

---

### `download`

Download an entire project to a local directory.

```
downleaf download <project-id> [dest-dir]
```

- `dest-dir` defaults to the current directory `.`
- Creates a subdirectory named after the project

Example:
```
downleaf download 692fce31ee51890d4f6f14af ~/projects
# Result: ~/projects/test-thesis/main.tex
```

---

### `mount`

Start a WebDAV server and mount it to a local directory.

```
downleaf mount [mountpoint] [flags]
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--project <name\|id>` | Mount only the specified project |
| `--zen` | Zen mode — writes are cached locally, synced on exit or via `downleaf sync` |
| `-i`, `--interactive` | Interactively select a project to mount |
| `--port <port>` | WebDAV server port (default: 9090) |

**Defaults:**
- Mountpoint: `~/downleaf`
- Port: `9090`

Examples:
```bash
# Mount all projects
downleaf mount

# Interactive project selection + zen mode
downleaf mount -i --zen

# Custom mountpoint and port
downleaf mount /mnt/overleaf --port 8080

# Mount a specific project
downleaf mount --project test-thesis
```

Directory structure after mounting:
```
~/downleaf/
  project-a/
    main.tex
    refs.bib
    figures/
      fig1.png
  project-b/
    ...
```

Press `Ctrl+C` to stop the server and unmount.

---

### `sync`

Push all unsynced local changes to Overleaf. Only used in `--zen` mode.

```
downleaf sync
```

This command sends a `SIGUSR1` signal to the running mount process to trigger a sync.

---

### `umount`

Unmount the filesystem.

```
downleaf umount [mountpoint]
```

Defaults to unmounting `~/downleaf`.

---

### `help`

Show help information. Does not require authentication.

```
downleaf help
```

---

## Environment Variables

Configured via a `.env` file or environment variables:

| Variable | Required | Description |
|----------|----------|-------------|
| `SITE` | Yes | Overleaf site URL, e.g. `https://www.overleaf.com/` |
| `COOKIES` | Yes | Session cookie copied from your browser |

Example `.env` file:
```
SITE=https://www.overleaf.com/
COOKIES=overleaf_session2=s%3A...
```
