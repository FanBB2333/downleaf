# Downleaf CLI Reference

## Synopsis

```
downleaf <command> [arguments] [flags]
```

Downleaf 通过 WebDAV 将 Overleaf 项目挂载为本地文件系统，使你可以用任意本地编辑器（VS Code、vim、Claude Code 等）直接编辑 Overleaf 项目。

---

## Commands

### `ls`

列出当前账户下的所有项目。

```
downleaf ls
```

输出示例：
```
52 projects:
  692fce31ee51890d4f6f14af  test-thesis
  67cd600ad849d892c3f220f1  Resume
  ...
```

---

### `tree`

显示指定项目的文件树结构（包含 entity ID）。

```
downleaf tree <project-id>
```

示例：
```
downleaf tree 692fce31ee51890d4f6f14af
```

输出：
```
Project: test-thesis
main.tex (doc: 692fce31ee51890d4f6f14b4)
```

---

### `cat`

打印文档内容。对 `.tex` 等文本文件使用 Socket.IO joinDoc 获取，对二进制文件自动回退到 REST API 下载。

```
downleaf cat <project-id> <doc-id>
```

示例：
```
downleaf cat 692fce31ee51890d4f6f14af 692fce31ee51890d4f6f14b4
```

---

### `download`

将整个项目下载到本地目录。

```
downleaf download <project-id> [dest-dir]
```

- `dest-dir` 默认为当前目录 `.`
- 会在目标目录下创建以项目名命名的子目录

示例：
```
downleaf download 692fce31ee51890d4f6f14af ~/projects
# 结果: ~/projects/test-thesis/main.tex
```

---

### `mount`

启动 WebDAV 服务器并挂载到本地目录。

```
downleaf mount [mountpoint] [flags]
```

**Flags:**

| Flag | 说明 |
|------|------|
| `--project <name\|id>` | 只挂载指定项目 |
| `--batch` | 批量模式：写入缓存到本地，不自动同步。使用 `downleaf sync` 手动推送 |
| `-i`, `--interactive` | 交互式选择要挂载的项目 |
| `--port <port>` | 指定 WebDAV 服务器端口（默认 9090） |

**默认值:**
- 挂载点: `~/downleaf`
- 端口: `9090`

示例：
```bash
# 挂载所有项目
downleaf mount

# 交互式选择项目 + 批量模式
downleaf mount -i --batch

# 指定挂载点和端口
downleaf mount /mnt/overleaf --port 8080

# 直接指定项目
downleaf mount --project test-thesis
```

挂载后的目录结构：
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

按 `Ctrl+C` 停止服务并卸载。

---

### `sync`

将本地所有未同步的修改推送到 Overleaf。仅在 `--batch` 模式下使用。

```
downleaf sync
```

该命令通过发送 `SIGUSR1` 信号通知正在运行的 mount 进程执行同步。

---

### `umount`

卸载已挂载的文件系统。

```
downleaf umount [mountpoint]
```

默认卸载 `~/downleaf`。

---

### `help`

显示帮助信息。无需认证。

```
downleaf help
```

---

## 环境变量

通过 `.env` 文件或环境变量配置：

| 变量 | 必填 | 说明 |
|------|------|------|
| `SITE` | 是 | Overleaf 站点 URL，如 `https://www.overleaf.com/` |
| `COOKIES` | 是 | 浏览器中复制的 session cookie |

`.env` 文件示例：
```
SITE=https://www.overleaf.com/
COOKIES=overleaf_session2=s%3A...
```
