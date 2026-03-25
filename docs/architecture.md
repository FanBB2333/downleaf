# Downleaf 架构

## 整体结构

```
downleaf
├── cmd/downleaf/main.go        # CLI 入口，命令解析
├── internal/
│   ├── auth/auth.go            # Cookie 认证，CSRF token 提取
│   ├── api/
│   │   ├── client.go           # Overleaf REST API 客户端
│   │   └── socketio.go         # Socket.IO v0 客户端（xhr-polling）
│   ├── model/model.go          # 数据模型（Project, Folder, Doc, FileRef）
│   ├── cache/cache.go          # 线程安全的内存缓存，支持 TTL 和 dirty 标记
│   ├── fuse/fs.go              # FUSE 实现（保留，需要 macFUSE）
│   └── webdav/server.go        # WebDAV 实现（当前默认）
└── docs/                       # 文档
```

## 数据流

### 读取流程

```
用户读取文件 (cat/vim/VS Code)
  → WebDAV GET
    → OverleafFS.OpenFile()
      → 检查 cache（dirty 文件使用缓存）
      → Socket.IO joinDoc（.tex 文本文件）
        或 REST API DownloadFile（二进制文件）
      → 存入 cache
    → regularFile.Read()
```

### 写入流程（普通模式）

```
用户保存文件
  → WebDAV PUT
    → OverleafFS.OpenFile(O_WRONLY|O_CREATE|O_TRUNC)
    → regularFile.Write()  → 追加到内存 buffer
    → regularFile.Close()
      → Cache.SetDirty()
      → Client.UploadFile()  → Overleaf 远端更新
      → Cache.ClearDirty()
```

### 写入流程（Batch 模式）

```
用户保存文件
  → WebDAV PUT
    → regularFile.Write() → 追加到内存 buffer
    → regularFile.Close()
      → Cache.SetDirty()   → 仅标记，不上传
      → registerMeta()     → 记录文件元信息

用户执行 downleaf sync
  → SIGUSR1 → mount 进程
    → FlushAll()
      → 遍历所有 dirty keys
      → Client.UploadFile() 逐个上传
      → Cache.ClearDirty()
```

## 关键组件

### Socket.IO v0 客户端

Overleaf 使用 Socket.IO **v0**（不是 v2/v3），传输层为 xhr-polling。

- **握手**: `GET /socket.io/1/?t=...&projectId=...` → 获取 session ID
- **轮询**: `GET /socket.io/1/xhr-polling/{sid}?t=...`
- **发送**: `POST /socket.io/1/xhr-polling/{sid}?t=...`

消息格式: `type:id:endpoint:data`
- Type 1 = connect
- Type 2 = heartbeat
- Type 5 = event（joinProjectResponse, connectionRejected）
- Type 6 = ack（joinDoc 响应）

需要转发 GCLB cookie（Google Cloud Load Balancer 会话粘性）。

### WebDAV 文件系统

实现 `golang.org/x/net/webdav.FileSystem` 接口：

| 方法 | 映射到 Overleaf API |
|------|---------------------|
| `Stat` | 查询 cache 或 project tree |
| `OpenFile` (读) | Socket.IO joinDoc / REST DownloadFile |
| `OpenFile` (写) | 写入内存 buffer |
| `Close` (写后) | REST UploadFile |
| `Mkdir` | REST CreateFolder |
| `RemoveAll` | REST DeleteEntity |
| `Rename` | REST RenameEntity + MoveEntity |

### 路径映射

```
WebDAV 路径              →  Overleaf 实体
/                        →  项目列表
/ProjectName/            →  项目根目录（RootFolder）
/ProjectName/sub/        →  子文件夹
/ProjectName/main.tex    →  Doc（通过 Socket.IO 读取内容）
/ProjectName/fig.png     →  FileRef（通过 REST API 下载）
```

### 缓存策略

- TTL: 5 分钟（非 dirty 条目过期后重新拉取）
- Dirty 条目永不过期（直到 ClearDirty）
- 每次 Open 时重新从远端拉取最新版本（除非本地有未同步修改）
- 文件元信息（projectID, folderID, name）存储在 metaMap 中，供 FlushAll 使用

## Overleaf API 端点

| 端点 | 用途 |
|------|------|
| `GET /user/projects` | 项目列表 |
| `GET /project/{id}` | 项目详情页（解析 CSRF token） |
| `GET /project/{id}/entities` | 扁平文件树 |
| `GET /project/{pid}/file/{fid}` | 下载二进制文件 |
| `POST /project/{pid}/doc` | 创建文档 |
| `POST /project/{pid}/folder` | 创建文件夹 |
| `POST /project/{pid}/upload?folder_id={fid}` | 上传文件 |
| `DELETE /project/{pid}/{type}/{id}` | 删除实体 |
| `POST /project/{pid}/{type}/{id}/rename` | 重命名 |
| `POST /project/{pid}/{type}/{id}/move` | 移动 |
