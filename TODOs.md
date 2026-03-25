# Downleaf - 将 Overleaf 项目挂载为本地文件系统

## 项目概述

通过 FUSE (Filesystem in Userspace) 将 Overleaf 项目映射为本地磁盘目录，使得任意 CLI 工具（Claude Code、vim、emacs 等）都能像操作本地文件一样读写 Overleaf 项目。

技术栈：Go + macFUSE (macOS) / libfuse (Linux)

---

## Phase 0: 项目基础设施

- [ ] 初始化 Go module (`go mod init github.com/FanBB2333/downleaf`)
- [ ] 搭建项目目录结构：
  ```
  cmd/downleaf/        # CLI 入口
  internal/api/        # Overleaf REST API 客户端
  internal/auth/       # 认证 & Cookie 管理
  internal/fuse/       # FUSE 文件系统实现
  internal/model/      # 数据模型 (Project, File, Folder)
  internal/cache/      # 本地文件缓存层
  ```
- [ ] 引入 FUSE 库 `bazil.org/fuse`（基于接口的 API 设计，实现 `fs.Node` / `fs.Handle` 等接口即可）
- [ ] 确认 macFUSE 已安装（macOS 上 FUSE 的内核扩展依赖）

## Phase 1: Overleaf 认证

- [ ] 实现 Cookie 登录流程：
  - [ ] `GET /login` 获取 CSRF token（从 HTML meta 标签解析）和初始 session cookie
  - [ ] `POST /login` 使用邮箱/密码 + CSRF token 登录，获取认证后的 session cookie
- [ ] 实现直接 Cookie 注入模式（用户从浏览器复制 cookie 直接使用）
- [ ] Cookie 持久化存储（存到 `~/.config/downleaf/cookies.json`）
- [ ] 登录状态校验：`GET /user/projects` 返回 200 表示有效
- [ ] Session 过期检测与自动重新登录

## Phase 2: Overleaf API 客户端

- [ ] 基础 HTTP 客户端封装（自动附加 Cookie / CSRF Token / 公共 Header）
- [ ] 项目相关 API：
  - [ ] `GET /user/projects` — 获取项目列表
  - [ ] `GET /project/{id}` — 获取项目详情（解析 HTML 中的 meta 数据，提取 project JSON）
  - [ ] `GET /project/{id}/entities` — 获取项目文件树
- [ ] 文件操作 API：
  - [ ] `GET /project/{pid}/file/{fid}` — 下载文件（二进制文件/图片）
  - [ ] `POST /project/{pid}/doc` — 创建 .tex 文档
  - [ ] `POST /project/{pid}/upload?folder_id={fid}` — 上传文件（multipart/form-data）
  - [ ] `POST /project/{pid}/folder` — 创建文件夹
  - [ ] `DELETE /project/{pid}/{type}/{id}` — 删除文件/文件夹
  - [ ] `POST /project/{pid}/{type}/{id}/rename` — 重命名
  - [ ] `POST /project/{pid}/{type}/{id}/move` — 移动文件/文件夹
- [ ] 文档内容 API（通过 Socket.IO joinDoc 获取 .tex 文件内容）：
  - [ ] Socket.IO 连接建立（带 Cookie 认证）
  - [ ] `joinProject` 事件 — 加入项目获取完整文件树
  - [ ] `joinDoc` / `leaveDoc` — 获取 .tex 文件具体内容
  - [ ] `applyOtUpdate` — 推送本地编辑（OT 操作）

## Phase 3: 数据模型 & 本地缓存

- [ ] 定义核心数据结构：
  ```go
  type Project struct { ID, Name, RootDocID string; ... }
  type Folder  struct { ID, Name string; Docs []Doc; FileRefs []FileRef; Folders []Folder }
  type Doc     struct { ID, Name string; Version int }
  type FileRef struct { ID, Name string; Created time.Time }
  ```
- [ ] 本地缓存层设计：
  - [ ] 文件内容缓存（避免每次 read 都走网络）
  - [ ] 文件树缓存（带 TTL，定时刷新）
  - [ ] 脏标记追踪（哪些文件有未同步的本地修改）

## Phase 4: FUSE 文件系统实现

- [ ] 挂载点管理：
  - [ ] 顶层目录 = 用户所有项目（每个项目一个文件夹）
  - [ ] 项目内目录 = Overleaf 文件树结构
  - [ ] 指定挂载点路径（如 `~/overleaf/`）
- [ ] 实现 FUSE 只读操作（先做读后做写）：
  - [ ] `Readdir` — 列出项目列表 / 项目内文件
  - [ ] `Lookup` — 按名称查找文件/目录
  - [ ] `Getattr` — 返回文件元信息（大小、修改时间、权限）
  - [ ] `Open` / `Read` — 读取文件内容（先查缓存，miss 则从 API 拉取）
- [ ] 实现 FUSE 写操作：
  - [ ] `Write` — 写文件内容到缓存，标记 dirty
  - [ ] `Create` — 创建新文件（调用 upload/doc API）
  - [ ] `Mkdir` — 创建文件夹
  - [ ] `Unlink` / `Rmdir` — 删除文件/文件夹
  - [ ] `Rename` — 重命名 / 移动
  - [ ] `Flush` / `Release` — 文件关闭时将脏数据同步到 Overleaf
- [ ] 特殊文件处理：
  - [ ] `.tex` 文件通过 Socket.IO joinDoc 读取（获取最新 OT 版本）
  - [ ] 二进制文件（图片、PDF）通过 REST API 下载
  - [ ] 忽略 `.DS_Store`、`._*` 等 macOS 系统文件的创建请求

## Phase 5: 同步策略

- [ ] Write-back 策略：本地写入先存缓存，在 `Flush`/`Release` 时推送到 Overleaf
- [ ] 冲突检测：
  - [ ] 基于文件版本号检测远端是否有并发修改
  - [ ] 冲突时保留双方版本（创建 `.conflict` 文件）
- [ ] 远端变更感知（可选，增强体验）：
  - [ ] 通过 Socket.IO 监听 `reciveNewDoc`、`reciveNewFile`、`removeEntity`、`reciveEntityRename` 等事件
  - [ ] 收到远端变更后自动刷新缓存 & 使 FUSE inode 失效
- [ ] 优雅断线处理：网络断开时仍可读取缓存，恢复后批量同步

## Phase 6: CLI 交互

- [ ] `downleaf login` — 交互式登录（邮箱/密码 或 粘贴 cookie）
  - [ ] 支持自定义 Overleaf 服务器地址（self-hosted 实例）
- [ ] `downleaf mount [mountpoint]` — 挂载所有项目到指定目录
- [ ] `downleaf mount --project <name|id> [mountpoint]` — 只挂载单个项目
- [ ] `downleaf umount` — 卸载
- [ ] `downleaf ls` — 列出可用项目（无需挂载）
- [ ] `downleaf status` — 显示挂载状态、连接状态、缓存统计
- [ ] 前台/后台运行模式（`--foreground` 打印日志，默认 daemon 模式）

## Phase 7: 可靠性 & 体验优化

- [ ] 日志系统（结构化日志，支持 `--verbose` / `--debug`）
- [ ] 预取策略：挂载时预加载项目文件树，首次打开项目时预取常用文件
- [ ] inode 缓存优化：合理设置 entry/attr 缓存 TTL，减少 FUSE 内核回调
- [ ] 大文件处理：超过阈值的文件按需分块读取，不全量缓存
- [ ] 信号处理：`SIGINT`/`SIGTERM` 时优雅卸载，flush 所有脏数据
- [ ] 配置文件支持（`~/.config/downleaf/config.toml`）

## Phase 8: 测试 & 文档

- [ ] 单元测试：API 客户端、缓存层、数据模型
- [ ] 集成测试：对真实 Overleaf 实例（或 mock server）执行完整读写流程
- [ ] 手动验收场景：
  - [ ] `cd ~/overleaf/my-paper && claude` 启动 Claude Code 编辑 LaTeX 项目
  - [ ] `vim ~/overleaf/my-paper/main.tex` 直接编辑，保存后 Overleaf 实时更新
  - [ ] `cp figure.png ~/overleaf/my-paper/figures/` 上传图片
- [ ] README 编写：安装指南、使用说明、常见问题
