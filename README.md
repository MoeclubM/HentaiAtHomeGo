# HentaiAtHomeGo

`HentaiAtHomeGo` 是 `Hentai@Home 1.6.4` 的 Go 实现，目标是与现有管理端和 CDN 工作流保持兼容。

- 本地只配置 `Client ID`、`Client Key` 和各类数据目录
- 端口、主机、带宽、连接数等运行参数由管理端远程下发
- `HentaiAtHome_1.6.4_src/` 仅保留为 Java 参考源码

## 快速开始

### Linux 一键安装

交互式安装：

```bash
curl -fsSL https://raw.githubusercontent.com/MoeclubM/HentaiAtHomeGo/main/scripts/bootstrap-install.sh | \
  bash -s -- --version=v0.0.5 --install-dir=/opt/hathgo --systemd --force
```

无人值守安装：

```bash
curl -fsSL https://raw.githubusercontent.com/MoeclubM/HentaiAtHomeGo/main/scripts/bootstrap-install.sh | \
  bash -s -- \
    --version=v0.0.5 \
    --install-dir=/opt/hathgo \
    --client-id=51839 \
    --client-key='你的20位ClientKey' \
    --systemd \
    --yes \
    --force
```

自定义数据目录：

```bash
curl -fsSL https://raw.githubusercontent.com/MoeclubM/HentaiAtHomeGo/main/scripts/bootstrap-install.sh | \
  bash -s -- \
    --version=v0.0.5 \
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

### 启动与升级

安装为 `systemd` 服务后会自动启动，也可以手动执行：

```bash
/opt/hathgo/run-hathgo.sh
```

升级到最新版：

```bash
curl -fsSL https://raw.githubusercontent.com/MoeclubM/HentaiAtHomeGo/main/scripts/bootstrap-install.sh | \
  bash -s -- --version=v0.0.5 --install-dir=/opt/hathgo --systemd --yes --force
```

### 常用命令

```bash
systemctl status hathgo --no-pager
journalctl -u hathgo -f
tail -f /opt/hathgo/log/log_out
tail -f /opt/hathgo/log/log_err
systemctl restart hathgo
/opt/hathgo/configure-hathgo.sh
```
