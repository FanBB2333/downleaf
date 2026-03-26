# Troubleshooting

## Authentication Issues

### "SITE and COOKIES must be set"

Make sure a `.env` file exists in the project root and contains both `SITE` and `COOKIES` variables.

### "authentication failed"

The cookie has expired. Re-copy it from your browser:
1. Log in to Overleaf
2. DevTools → Application → Cookies
3. Copy the value of `overleaf_session2`
4. Update the `.env` file

> Cookies are typically valid for about 7 days.

### Self-hosted Overleaf Instance

Set `SITE` to the URL of your instance:
```
SITE=https://overleaf.example.com/
```

## Mount Issues

### mount_webdav Failed

If auto-mount fails, you can mount manually:

```bash
# macOS - via Finder
# Cmd+K → enter http://localhost:9090

# macOS - command line
mkdir -p ~/downleaf
mount_webdav http://localhost:9090 ~/downleaf

# Linux
sudo mount -t davfs http://localhost:9090 ~/downleaf
```

### Port Already in Use

```bash
# Use a different port
downleaf mount --port 8080
```

### umount: "Resource busy"

A process is still using the mount directory:

```bash
# Find the process
lsof +D ~/downleaf

# Force unmount (macOS)
diskutil unmount force ~/downleaf
```

## File Read/Write Issues

### File Content Is Empty or Size Shows 0

File sizes may show as 0 on the first directory listing (not yet cached). The correct size appears after opening the file. This is expected behavior.

### Changes Not Reflected on Overleaf

- **Normal mode**: Check the mount terminal logs for upload errors
- **Zen mode**: Run `downleaf sync` to push changes
- The CSRF token may have expired after long-running mounts — restart the mount to refresh it

### "no running mount found" (sync command)

The mount process is not running, or the PID file `/tmp/downleaf.pid` does not exist. Make sure the mount process is active.

## Network Issues

### Request Timeouts

Overleaf uses Google Cloud Load Balancer, which requires GCLB cookie session stickiness. Downleaf handles this automatically. If timeouts persist, the network may be unstable — restart the mount.

### Socket.IO Connection Failed

Socket.IO joinDoc automatically falls back to the REST API on timeout. If a file consistently fails to load, it may be locked by another user or there may be a permission issue.

## Performance

### Opening Files Is Slow

Each open fetches the latest version from Overleaf. For large projects, you can:
1. Use `--project` to mount only the project you need
2. Use the `download` command to work locally, then upload manually when done
