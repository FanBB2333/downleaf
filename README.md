# Downleaf

将 Overleaf 项目挂载为本地目录，用你喜欢的编辑器直接编辑 LaTeX 项目。

Downleaf 在本地启动一个 WebDAV 服务器，将 Overleaf 上的项目文件映射为标准文件系统。无需安装内核扩展，macOS / Linux / Windows 均可使用。

## 快速开始

### 1. 构建

```bash
git clone https://github.com/FanBB2333/downleaf.git
cd downleaf
go build -o downleaf ./cmd/downleaf
```

### 2. 配置

从浏览器中复制 Overleaf 的 session cookie，创建 `.env` 文件：

```
SITE=https://www.overleaf.com/
COOKIES=overleaf_session2=s%3Axxxxxxxxxx...
```

> Cookie 获取方式：登录 Overleaf → F12 开发者工具 → Application → Cookies → 复制 `overleaf_session2` 的值

### 3. 挂载

```bash
./downleaf mount
```

所有项目会出现在 `~/downleaf/` 下：

```
~/downleaf/
├── My-Paper/
│   ├── main.tex
│   ├── refs.bib
│   └── figures/
│       └── fig1.png
├── Another-Project/
│   └── ...
```

然后就可以用任意编辑器打开：

```bash
code ~/downleaf/My-Paper          # VS Code
vim ~/downleaf/My-Paper/main.tex  # Vim
cd ~/downleaf/My-Paper && claude  # Claude Code
```

保存文件后修改会自动同步到 Overleaf。按 `Ctrl+C` 停止。

## 示例：用 Claude Code 编辑论文

```bash
# 终端 1：以 batch 模式挂载（编辑期间不同步，避免频繁上传）
./downleaf mount -i --batch

# 交互式选择项目：
#   Select a project to mount (52 projects):
#     0) [all projects]
#     1) My-Paper
#     2) Another-Project
#   Enter number (0 for all): 1
#   Selected: My-Paper (692fce31ee51890d4f6f14af)

# 终端 2：用 Claude Code 编辑
cd ~/downleaf/My-Paper
claude

# 终端 3：编辑完成后，一次性推送所有修改到 Overleaf
./downleaf sync
```

## 命令一览

| 命令 | 说明 |
|------|------|
| `downleaf ls` | 列出所有项目 |
| `downleaf mount` | 挂载项目到本地（默认 `~/downleaf`） |
| `downleaf mount -i --batch` | 交互选择项目，batch 模式 |
| `downleaf sync` | 推送 batch 模式下的本地修改 |
| `downleaf download <id>` | 下载项目到本地目录 |
| `downleaf umount` | 卸载 |
| `downleaf help` | 查看帮助 |

mount 支持的选项：`--project <name|id>`, `--batch`, `-i`, `--port <port>`

## 文档

- [CLI 命令参考](docs/cli-reference.md)
- [快速开始指南](docs/getting-started.md)
- [系统架构](docs/architecture.md)
- [故障排查](docs/troubleshooting.md)

## 工作原理

1. 通过 Overleaf 的 Cookie 认证获取访问权限
2. 使用 Socket.IO v0 协议获取项目文件树和文档内容
3. 在本地启动 WebDAV 服务器，将 Overleaf 的文件结构映射为标准文件系统
4. 通过 `mount_webdav`（macOS）或 `davfs`（Linux）挂载到本地目录
5. 文件修改通过 Overleaf REST API 回写

## License

MIT
