# 故障排查

## 认证问题

### "SITE and COOKIES must be set"

确保项目根目录有 `.env` 文件，且包含 `SITE` 和 `COOKIES` 变量。

### "authentication failed"

Cookie 已过期。重新从浏览器复制：
1. 登录 Overleaf
2. 开发者工具 → Application → Cookies
3. 复制 `overleaf_session2` 的值
4. 更新 `.env` 文件

> Cookie 通常有效期约 7 天。

### 自部署 Overleaf 实例

将 `SITE` 设为你的实例地址：
```
SITE=https://overleaf.example.com/
```

## 挂载问题

### mount_webdav 失败

如果自动挂载失败，可以手动挂载：

```bash
# macOS - 通过 Finder
# Cmd+K → 输入 http://localhost:9090

# macOS - 命令行
mkdir -p ~/downleaf
mount_webdav http://localhost:9090 ~/downleaf

# Linux
sudo mount -t davfs http://localhost:9090 ~/downleaf
```

### 端口被占用

```bash
# 使用其他端口
downleaf mount --port 8080
```

### umount: "Resource busy"

有进程仍在使用挂载目录：

```bash
# 查看占用进程
lsof +D ~/downleaf

# 强制卸载（macOS）
diskutil unmount force ~/downleaf
```

## 文件读写问题

### 文件内容为空或大小显示为 0

首次列目录时文件大小可能显示为 0（未缓存），打开文件后会显示正确大小。这是正常行为。

### 写入后 Overleaf 端未更新

- **普通模式**: 检查 mount 终端的日志输出，看是否有 upload 错误
- **Zen 模式**: 需要执行 `downleaf sync` 才会推送
- 确认 CSRF token 未过期（长时间运行后可能失效，重启 mount 即可）

### "no running mount found" (sync 命令)

mount 进程未运行，或 PID 文件 `/tmp/downleaf.pid` 不存在。确保 mount 进程在运行。

## 网络问题

### 请求超时

Overleaf 使用 Google Cloud Load Balancer，需要保持 GCLB cookie 的会话粘性。downleaf 会自动处理这个问题。如果反复超时，可能是网络不稳定，重启 mount 即可。

### Socket.IO 连接失败

Socket.IO joinDoc 超时后会自动回退到 REST API 下载。如果某个文件始终无法读取，可能是该文件正在被其他用户编辑，或者存在权限问题。

## 性能

### 打开文件很慢

每次 open 都会从 Overleaf 拉取最新版本。对于大型项目，可以：
1. 使用 `--project` 只挂载需要的项目
2. 使用 `download` 命令先下载到本地，编辑完再手动上传
