# HentaiAtHomeGo

Go 实现的 `Hentai@Home` 客户端。当前目标是 **CDN 功能兼容**，重点对齐：

- `/h` 文件请求、鉴权、缓存命中与 miss 后回源
- `/t` speed test
- `/servercmd` 管理端命令
- `threaded_proxy_test`
- RPC、证书、管理端交互

当前仓库已经具备：

- 本地构建与运行能力
- 协议兼容回归测试
- tag 触发的 GitHub Release 自动构建
- Windows / Linux / macOS 安装入口

## 当前状态

- 目标：与现有 Java 管理端、CDN 工作流保持兼容
- 非目标：逐行复刻 Java 内部实现、日志格式、异常控制流
- 运行方式：优先作为命令行客户端运行

如果你关注的是“能否替换旧节点并继续参与现有 CDN 网络”，建议同时查看：

- `COMPATIBILITY_REPORT.md`
- `COMPATIBILITY_FIXES.md`

## 环境要求

### 运行预编译 Release

- 无需安装 Go
- 支持 Windows / Linux / macOS

### 从源码构建

- Go `1.21+`

### 运行测试

- 普通 Go 测试：只需要 Go
- Java oracle 兼容测试：需要本地可用的 `java` 与 `javac`

## 快速开始

### 方式一：直接构建

Linux / macOS:

```bash
go build -trimpath -o dist/hathgo ./cmd/client
```

Windows:

```powershell
go build -trimpath -o dist\hathgo.exe .\cmd\client
```

### 方式二：使用安装脚本

安装脚本支持两种模式：

- **源码模式**：在仓库内运行，脚本会调用 `go build`
- **Release 包模式**：在解压后的发布包内运行，脚本会直接安装包内二进制

Linux / macOS：

```bash
bash ./scripts/install.sh
```

或在解压后的 release 包目录中：

```bash
bash ./install.sh --install-dir=/opt/hathgo --force
```

Windows PowerShell：

```powershell
.\scripts\install.ps1
```

或在解压后的 release 包目录中：

```powershell
.\install.ps1 -InstallDir D:\HathGo -Force
```

安装脚本会：

- 构建或复制客户端二进制
- 创建运行目录
- 生成带固定目录参数的启动包装脚本

## 默认目录结构

程序默认会创建并使用以下目录：

- `data/`
- `log/`
- `cache/`
- `tmp/`
- `download/`

相关入口：`cmd/client/main.go`、`internal/config/settings.go`

## 首次启动

客户端首次启动时会：

1. 初始化目录
2. 启动日志系统
3. 读取 `data/client_login`
4. 如凭据无效，则提示输入 `Client ID` 与 `Client Key`
5. 连接管理端拉取运行配置
6. 初始化缓存并启动监听端口

相关代码：`cmd/client/main.go`

## 常用启动参数

程序使用 `--key=value` 风格参数；内部会自动把 `-` 归一化为 `_`。

示例：

```bash
./hathgo \
  --data-dir=./data \
  --log-dir=./log \
  --cache-dir=./cache \
  --temp-dir=./tmp \
  --download-dir=./download
```

### 参数表

- `--host`：上报给管理端的主机名或地址
- `--port`：监听端口
- `--data-dir`：数据目录
- `--log-dir`：日志目录
- `--cache-dir`：缓存目录
- `--temp-dir`：临时目录
- `--download-dir`：下载目录
- `--throttle-bytes`：带宽限制
- `--verify-cache`：启动时校验缓存
- `--rescan-cache`：启动时重扫缓存
- `--use-less-memory`：降低内存占用
- `--disable-bwm`：关闭带宽管理
- `--disable-download-bwm`：关闭下载带宽管理
- `--disable-file-verification`：关闭文件完整性校验
- `--disable-ip-origin-check`：关闭来源 IP 校验
- `--disable-flood-control`：关闭 flood control
- `--skip-free-space-check`：跳过磁盘空间检查
- `--image-proxy-type=http|socks`：图片代理类型
- `--image-proxy-host`：图片代理主机
- `--image-proxy-port`：图片代理端口
- `--flush-logs`：启用日志即时刷盘

参数解析入口：`internal/config/settings.go`

## 安装脚本参数

### `scripts/install.sh`

- `--install-dir=PATH`：安装目录
- `--binary-name=NAME`：安装后二进制名称
- `--force`：覆盖已有安装

默认安装到：`$HOME/.local/share/hathgo`

### `scripts/install.ps1`

- `-InstallDir`：安装目录
- `-BinaryName`：安装后二进制名称
- `-Force`：覆盖已有安装

默认安装到：`$env:LOCALAPPDATA\HentaiAtHomeGo`

## Release 自动构建

仓库已包含 GitHub Actions workflow：`.github/workflows/release.yml`

- `push` tag `v*`：执行测试、交叉构建、打包并发布 GitHub Release
- `workflow_dispatch`：用于手动演练测试和构建；默认不发布正式 Release
- 产物包含：Windows / Linux / macOS 压缩包与对应 `.sha256`

### 发布方式

```bash
git tag v0.1.0
git push origin v0.1.0
```

推送后 workflow 会自动创建对应 GitHub Release 并上传构建产物。

## 本地验证

```bash
go test ./...
go run cmd/verify/main.go
go run cmd/check/main.go
```

## 目录说明

- `cmd/client/`：客户端入口
- `cmd/verify/`：协议兼容验证工具
- `cmd/check/`：完整性检查工具
- `internal/server/`：HTTP / CDN 请求处理
- `internal/download/`：回源与代理下载
- `internal/network/`：RPC / 管理端交互
- `internal/cache/`：缓存管理
- `testdata/java_oracle/`：Java 行为 oracle 测试辅助

## 许可证

本项目使用 GPL v3，见 `LICENSE`。
