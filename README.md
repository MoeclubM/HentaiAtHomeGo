# HentaiAtHomeGo

`HentaiAtHomeGo` 是 `Hentai@Home 1.6.4` 的 Go 实现，目标是与现有管理端和 CDN 工作流保持兼容。

- 本地只配置：`Client ID`、`Client Key`、数据目录路径
- 运行参数如端口、主机、带宽、连接数等：由管理端远程下发
- 仓库中的 `HentaiAtHome_1.6.4_src/` 仅保留为 Java 参考源码

## 快速开始

### Linux 一键安装

交互式安装：

```bash
curl -fsSL https://raw.githubusercontent.com/MoeclubM/HentaiAtHomeGo/main/scripts/bootstrap-install.sh | \
  bash -s -- --version=v0.0.3 --install-dir=/opt/hathgo --systemd --force
```

无人值守安装：

```bash
curl -fsSL https://raw.githubusercontent.com/MoeclubM/HentaiAtHomeGo/main/scripts/bootstrap-install.sh | \
  bash -s -- \
    --version=v0.0.3 \
    --install-dir=/opt/hathgo \
    --client-id=51839 \
    --client-key='你的20位ClientKey' \
    --systemd \
    --yes \
    --force
```

如需自定义数据目录：

```bash
curl -fsSL https://raw.githubusercontent.com/MoeclubM/HentaiAtHomeGo/main/scripts/bootstrap-install.sh | \
  bash -s -- \
    --version=v0.0.3 \
    --install-dir=/opt/hathgo \
    --client-id=51839 \
    --client-key='你的20位ClientKey' \
    --data-dir=/srv/hathgo/data \
    --log-dir=/srv/hathgo/log \
    --cache-dir=/srv/hathgo/cache \
    --temp-dir=/srv/hathgo/tmp \
    --download-dir=/srv/hathgo/download \
    --systemd \
    --yes \
    --force
```

### 安装后启动

使用 `systemd` 安装时会自动启动，手动启动命令如下：

```bash
/opt/hathgo/run-hathgo.sh
```

### 基础维护命令

查看状态：

```bash
systemctl status hathgo --no-pager
```

实时日志：

```bash
journalctl -u hathgo -f
tail -f /opt/hathgo/log/log_out
tail -f /opt/hathgo/log/log_err
```

重启 / 停止 / 启动：

```bash
systemctl restart hathgo
systemctl stop hathgo
systemctl start hathgo
```

重写凭据或重装服务：

```bash
/opt/hathgo/configure-hathgo.sh
```

升级到新版：

```bash
curl -fsSL https://raw.githubusercontent.com/MoeclubM/HentaiAtHomeGo/main/scripts/bootstrap-install.sh | \
  bash -s -- --version=v0.0.3 --install-dir=/opt/hathgo --systemd --yes --force
```
