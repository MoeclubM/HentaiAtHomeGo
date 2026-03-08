# HentaiAtHomeGo

`HentaiAtHomeGo` 是 `Hentai@Home 1.6.4` 的 Go 实现，当前目标是 **CDN 功能兼容**：

- `/h` 文件请求、鉴权、缓存命中与回源
- `/t` speed test
- `/servercmd` 管理端命令
- `threaded_proxy_test`
- RPC、证书、管理端交互

仓库中的 `HentaiAtHome_1.6.4_src/` 仅保留作 Java 参考源码。

## 配置原则

- **本地只配置**：`Client ID`、`Client Key`、文件路径
- **本地不配置**：端口、主机名、带宽、连接数等运行参数
- 其余运行参数统一遵循原版流程，由管理端远程下发

程序本身也已限制命令行参数：本地 CLI 仅接受目录参数，其他运行参数会被忽略。

## Linux：直接从 GitHub 一键安装

最新版本：

```bash
curl -fsSL https://raw.githubusercontent.com/MoeclubM/HentaiAtHomeGo/main/scripts/bootstrap-install.sh | \
  bash -s -- --install-dir=/opt/hathgo --systemd --force
```

指定版本：

```bash
curl -fsSL https://raw.githubusercontent.com/MoeclubM/HentaiAtHomeGo/main/scripts/bootstrap-install.sh | \
  bash -s -- --version=v0.0.1 --install-dir=/opt/hathgo --systemd --force
```

安装脚本会自动：

- 从 GitHub Releases 下载对应架构的预编译包
- 解压并调用 release 包内的 `install.sh`
- 安装客户端到目标目录
- 交互输入 `Client ID` / `Client Key`
- 生成 `run-hathgo.sh`
- 可选安装并启动 `systemd` 服务

## Linux：无人值守安装

```bash
HATH_CLIENT_ID=51839 \
HATH_CLIENT_KEY=your20charclientkey \
curl -fsSL https://raw.githubusercontent.com/MoeclubM/HentaiAtHomeGo/main/scripts/bootstrap-install.sh | \
  bash -s -- --install-dir=/opt/hathgo --systemd --yes --force
```

也可以直接安装某个 release 包中的 `install.sh`：

```bash
bash ./install.sh --install-dir=/opt/hathgo --systemd --force
```

## 本地可配路径

如果你不想使用默认目录，可以在安装时指定：

```bash
curl -fsSL https://raw.githubusercontent.com/MoeclubM/HentaiAtHomeGo/main/scripts/bootstrap-install.sh | \
  bash -s -- \
    --install-dir=/opt/hathgo \
    --data-dir=/srv/hathgo/data \
    --log-dir=/srv/hathgo/log \
    --cache-dir=/srv/hathgo/cache \
    --temp-dir=/srv/hathgo/tmp \
    --download-dir=/srv/hathgo/download \
    --systemd \
    --force
```

## 安装后目录

默认安装目录下会生成：

- `hathgo`：主程序
- `install.sh`：release 安装脚本副本
- `configure-hathgo.sh`：重新写入凭据 / 重装服务
- `run-hathgo.sh`：启动脚本
- `data/client_login`：客户端 ID / Key

以及你指定或默认的：

- `data/`
- `log/`
- `cache/`
- `tmp/`
- `download/`

## 重新配置

重新写入凭据或重装服务：

```bash
/opt/hathgo/configure-hathgo.sh
```

无人值守重写凭据：

```bash
HATH_CLIENT_ID=51839 \
HATH_CLIENT_KEY=your20charclientkey \
/opt/hathgo/configure-hathgo.sh --yes
```

## 手动启动

```bash
/opt/hathgo/run-hathgo.sh
```

`run-hathgo.sh` 只会传入本地目录参数，不会附加 `host/port` 等本地运行设置。

## systemd 管理

如果安装时使用了 `--systemd`，可用：

```bash
systemctl status hathgo
systemctl restart hathgo
systemctl stop hathgo
journalctl -u hathgo -f
```

如果你传了 `--service-name=NAME`，把上面的 `hathgo` 替换掉即可。

## Release 包内安装脚本

release 包内的 `install.sh` 是 **release-only** 的：

- 不会编译源码
- 只会安装包内预编译二进制
- 只负责 `ID/Key` 与目录配置

## 本地验证

```bash
go test ./...
go run cmd/verify/main.go
go run cmd/check/main.go
```

## Release 自动构建

仓库已包含 GitHub Actions workflow：`.github/workflows/release.yml`

- 推送 `v*` tag 时自动测试、构建并发布 Release
- 产物覆盖 Linux / Windows / macOS

示例：

```bash
git tag v0.0.1
git push origin v0.0.1
```

## 目录说明

- `cmd/client/`：客户端入口
- `cmd/verify/`：协议兼容验证工具
- `cmd/check/`：完整性检查工具
- `internal/server/`：HTTP / CDN 请求处理
- `internal/download/`：回源与代理下载
- `internal/network/`：RPC / 管理端交互
- `internal/cache/`：缓存管理
- `scripts/bootstrap-install.sh`：GitHub 一键安装脚本
- `scripts/install.sh`：release 包内安装脚本
- `HentaiAtHome_1.6.4_src/`：Java 参考源码

## 许可证

本项目使用 GPL v3，见 `LICENSE`。
