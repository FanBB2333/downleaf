# Architecture

## Project Structure

```
downleaf
├── cmd/downleaf/main.go        # CLI entry point, command parsing
├── internal/
│   ├── auth/auth.go            # Cookie authentication, CSRF token extraction
│   ├── api/
│   │   ├── client.go           # Overleaf REST API client
│   │   └── socketio.go         # Socket.IO v0 client (xhr-polling)
│   ├── model/model.go          # Data models (Project, Folder, Doc, FileRef)
│   ├── cache/cache.go          # Thread-safe in-memory cache with TTL and dirty tracking
│   ├── fuse/fs.go              # FUSE implementation (retained, requires macFUSE)
│   └── webdav/server.go        # WebDAV implementation (current default)
└── docs/                       # Documentation
```

## Data Flow

### Read Path

```
User reads a file (cat/vim/VS Code)
  → WebDAV GET
    → OverleafFS.OpenFile()
      → Check cache (dirty files use cached version)
      → Socket.IO joinDoc (for .tex text files)
        or REST API DownloadFile (for binary files)
      → Store in cache
    → regularFile.Read()
```

### Write Path (Normal Mode)

```
User saves a file
  → WebDAV PUT
    → OverleafFS.OpenFile(O_WRONLY|O_CREATE|O_TRUNC)
    → regularFile.Write()  → append to in-memory buffer
    → regularFile.Close()
      → Cache.SetDirty()
      → Client.UploadFile()  → remote Overleaf update
      → Cache.ClearDirty()
```

### Write Path (Zen Mode)

```
User saves a file
  → WebDAV PUT
    → regularFile.Write() → append to in-memory buffer
    → regularFile.Close()
      → Cache.SetDirty()   → mark dirty only, no upload
      → registerMeta()     → record file metadata

User runs downleaf sync
  → SIGUSR1 → mount process
    → FlushAll()
      → Iterate all dirty keys
      → Client.UploadFile() for each
      → Cache.ClearDirty()
```

## Key Components

### Socket.IO v0 Client

Overleaf uses Socket.IO **v0** (not v2/v3), with xhr-polling as the transport layer.

- **Handshake**: `GET /socket.io/1/?t=...&projectId=...` → obtain session ID
- **Polling**: `GET /socket.io/1/xhr-polling/{sid}?t=...`
- **Sending**: `POST /socket.io/1/xhr-polling/{sid}?t=...`

Message format: `type:id:endpoint:data`
- Type 1 = connect
- Type 2 = heartbeat
- Type 5 = event (joinProjectResponse, connectionRejected)
- Type 6 = ack (joinDoc response)

GCLB cookies (Google Cloud Load Balancer session stickiness) must be forwarded.

### WebDAV Filesystem

Implements the `golang.org/x/net/webdav.FileSystem` interface:

| Method | Maps to Overleaf API |
|--------|---------------------|
| `Stat` | Query cache or project tree |
| `OpenFile` (read) | Socket.IO joinDoc / REST DownloadFile |
| `OpenFile` (write) | Write to in-memory buffer |
| `Close` (after write) | REST UploadFile |
| `Mkdir` | REST CreateFolder |
| `RemoveAll` | REST DeleteEntity |
| `Rename` | REST RenameEntity + MoveEntity |

### Path Mapping

```
WebDAV Path              →  Overleaf Entity
/                        →  Project list
/ProjectName/            →  Project root directory (RootFolder)
/ProjectName/sub/        →  Subfolder
/ProjectName/main.tex    →  Doc (content via Socket.IO)
/ProjectName/fig.png     →  FileRef (download via REST API)
```

### Caching Strategy

- TTL: 5 minutes (non-dirty entries are re-fetched after expiration)
- Dirty entries never expire (until ClearDirty is called)
- Each Open re-fetches the latest version from remote (unless local modifications are unsynced)
- File metadata (projectID, folderID, name) is stored in metaMap for use by FlushAll

## Overleaf API Endpoints

| Endpoint | Purpose |
|----------|---------|
| `GET /user/projects` | List projects |
| `GET /project/{id}` | Project detail page (parse CSRF token) |
| `GET /project/{id}/entities` | Flat file tree |
| `GET /project/{pid}/file/{fid}` | Download binary file |
| `POST /project/{pid}/doc` | Create document |
| `POST /project/{pid}/folder` | Create folder |
| `POST /project/{pid}/upload?folder_id={fid}` | Upload file |
| `DELETE /project/{pid}/{type}/{id}` | Delete entity |
| `POST /project/{pid}/{type}/{id}/rename` | Rename |
| `POST /project/{pid}/{type}/{id}/move` | Move |
