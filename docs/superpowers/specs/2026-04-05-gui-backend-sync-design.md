# GUI Backend Sync Design

**Date**: 2026-04-05
**Status**: Approved

## Goal

Sync CLI features (pluggable mount backends, zen mode cache optimization) into the GUI desktop app, and add backend selection UI in Settings.

## Go Backend Changes (`internal/gui/app.go`)

### Struct Changes

- Remove `ofs *dav.OverleafFS` from App struct
- Add `backendName string` (default `"webdav"`)
- Add `backend mount.Backend` (active backend instance, nil when not mounted)

### New Methods

- `GetBackend() string` — returns current backend name
- `SetBackend(name string) error` — validates name via `mount.Get()`, rejects if currently mounted
- `ListBackends() []BackendInfo` — returns all registered backends with availability flag

```go
type BackendInfo struct {
    Name      string `json:"name"`
    Available bool   `json:"available"`
}
```

Available backends: `webdav` (available), `fuse` (coming soon — registered but returns error on Start).

### Refactored Methods

**Mount(projectNames, mountpoint, zenMode)**:
1. `mount.Get(a.backendName)` to create backend
2. Build `mount.Config` with client, addr, mountpoint, filters, zenMode, ignore
3. Store backend in `a.backend`
4. Call `backend.Start(cfg)` in a goroutine (it blocks)
5. Zen mode cache optimization is handled inside the webdav backend (`Cache.ZenMode = cfg.ZenMode`)

**Unmount()**:
1. Call `a.backend.Stop()` (handles flush + disconnect + unmount)
2. Set `a.backend = nil`, `a.mounted = false`

**Shutdown()**:
1. If mounted, show dirty summary via `a.backend.DirtySummary()`
2. Call `a.backend.Stop()`

**Sync()**:
1. Call `a.backend.FlushAll()`

**GetMountStatus()**:
1. Add `Backend` field to MountStatus struct

### MountStatus Struct Change

```go
type MountStatus struct {
    Mounted    bool     `json:"mounted"`
    Mountpoint string   `json:"mountpoint"`
    Project    []string `json:"project"`
    ZenMode    bool     `json:"zenMode"`
    WebDAVAddr string   `json:"webdavAddr"`
    Backend    string   `json:"backend"`
}
```

## Frontend Changes

### use-store.ts

- Add state: `backend: string`, `backends: BackendInfo[]`
- On startup: fetch `GetBackend()` and `ListBackends()`
- Add `setBackend(name)` that calls Go `SetBackend(name)` and updates local state

### MainPage.tsx — Settings Dialog

Add a "Mount Backend" section below existing settings (theme, color scheme, font size):

- Label: "Mount Backend"
- Dropdown/select with all backends from `ListBackends()`
- Unavailable backends show "(coming soon)" suffix and are disabled
- Changing selection calls `setBackend(name)`
- Disabled while mounted (show tooltip: "Unmount first to change backend")

## Not Changed

- `Mount()` TypeScript call signature stays `Mount(projects, mountpoint, zenMode)`
- LoginPage unaffected
- Credential management unaffected
- Backend name is in-memory only (resets to "webdav" on app restart) — persistent config is a future enhancement
