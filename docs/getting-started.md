# Downleaf 快速开始

## 安装

### 前置条件

- Go 1.21+
- Overleaf 账户

### 构建

```bash
git clone https://github.com/FanBB2333/downleaf.git
cd downleaf
go build -o downleaf ./cmd/downleaf
```

## 配置

### 获取 Cookies

1. 在浏览器中登录 Overleaf
2. 打开开发者工具（F12）→ Application → Cookies
3. 复制 `overleaf_session2` 的完整值

### 创建 .env 文件

在项目根目录创建 `.env` 文件：

```
SITE=https://www.overleaf.com/
COOKIES=overleaf_session2=s%3ASv8eHMJoBr4...
```

> 如果使用自部署的 Overleaf 实例，将 `SITE` 改为对应地址。

### 验证认证

```bash
./downleaf ls
```

如果输出项目列表，说明认证成功。

## 基本使用

### 挂载项目到本地

```bash
./downleaf mount
```

这会：
1. 在 `localhost:9090` 启动 WebDAV 服务器
2. 将所有项目挂载到 `~/downleaf/`

### 用编辑器打开

```bash
# VS Code
code ~/downleaf/test-thesis

# vim
vim ~/downleaf/test-thesis/main.tex

# Claude Code
cd ~/downleaf/test-thesis && claude
```

### 停止

按 `Ctrl+C`，或在另一个终端执行：

```bash
./downleaf umount
```

## 使用 Batch 模式

适合 Claude Code 等会频繁读写文件的场景。所有修改暂存本地，编辑完成后一次性同步。

```bash
# 终端 1：以 batch 模式挂载
./downleaf mount --batch

# 终端 2：编辑项目
cd ~/downleaf/test-thesis
claude  # 或 vim, code 等

# 终端 3：编辑完成后同步
./downleaf sync
```

## 挂载单个项目

如果不想看到所有项目，可以使用交互式选择：

```bash
./downleaf mount -i
```

或直接指定项目名/ID：

```bash
./downleaf mount --project test-thesis
./downleaf mount --project 692fce31ee51890d4f6f14af
```

## 下载项目（不挂载）

不需要挂载也能下载整个项目到本地：

```bash
./downleaf download 692fce31ee51890d4f6f14af ~/projects
```
