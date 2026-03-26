# Getting Started

## Installation

### Prerequisites

- Go 1.21+
- An Overleaf account

### Build

```bash
git clone https://github.com/FanBB2333/downleaf.git
cd downleaf
go build -o downleaf ./cmd/downleaf
```

## Configuration

### Obtain Cookies

1. Log in to Overleaf in your browser
2. Open DevTools (F12) → Application → Cookies
3. Copy the full value of `overleaf_session2`

### Create a .env File

Create a `.env` file in the project root:

```
SITE=https://www.overleaf.com/
COOKIES=overleaf_session2=s%3ASv8eHMJoBr4...
```

> If you use a self-hosted Overleaf instance, set `SITE` to its URL.

### Verify Authentication

```bash
./downleaf ls
```

If the project list is printed, authentication is successful.

## Basic Usage

### Mount Projects Locally

```bash
./downleaf mount
```

This will:
1. Start a WebDAV server on `localhost:9090`
2. Mount all projects to `~/downleaf/`

### Open with an Editor

```bash
# VS Code
code ~/downleaf/test-thesis

# vim
vim ~/downleaf/test-thesis/main.tex

# Claude Code
cd ~/downleaf/test-thesis && claude
```

### Stop

Press `Ctrl+C`, or run in another terminal:

```bash
./downleaf umount
```

## Using Zen Mode

Ideal for tools like Claude Code that read and write files frequently. All changes are kept local and synced in one go when you're done.

```bash
# Terminal 1: mount in zen mode
./downleaf mount --zen

# Terminal 2: edit the project
cd ~/downleaf/test-thesis
claude  # or vim, code, etc.

# Terminal 3: sync when finished
./downleaf sync
```

## Mount a Single Project

To avoid seeing all projects, use interactive selection:

```bash
./downleaf mount -i
```

Or specify a project by name or ID:

```bash
./downleaf mount --project test-thesis
./downleaf mount --project 692fce31ee51890d4f6f14af
```

## Download a Project (Without Mounting)

You can download an entire project locally without mounting:

```bash
./downleaf download 692fce31ee51890d4f6f14af ~/projects
```
