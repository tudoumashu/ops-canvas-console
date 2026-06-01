# opsc 本地安装与自启动

本文档只覆盖 local workspace 使用路径：`opsc serve` 提供本机 loopback API，`opsc executor --watch` 作为单机单 workspace worker。local workspace 仍是 canonical source；VPS API 只作为 hybrid ecommerce 执行后端。

## 构建

```bash
git clone https://github.com/tudoumashu/ops-canvas-console.git
cd ops-canvas-console
mkdir -p ~/.local/bin
go build -o ~/.local/bin/opsc ./cmd/opsc
```

如果本机没有 Go，可用 Docker 构建：

```bash
mkdir -p dist
docker run --rm -v "$PWD":/src -w /src golang:1.25-alpine \
  /usr/local/go/bin/go build -o dist/opsc ./cmd/opsc
```

## 手动启动

本地日常使用 Web UI 时建议先用生产模式启动前端，避免 `next dev` 在页面切换时按需编译：

```bash
cd web
API_BASE_URL=http://127.0.0.1:8080 npm run local:prod
```

开发热更新仍可使用：

```bash
cd web
API_BASE_URL=http://127.0.0.1:8080 npm run local:dev
```

```bash
opsc workspace init --workspace ~/OpsCanvas
opsc serve --workspace ~/OpsCanvas --origin http://localhost:3000
```

另开终端启动 worker：

```bash
opsc executor --workspace ~/OpsCanvas --watch --poll-interval 5s
```

`opsc serve` 默认只监听 `127.0.0.1`。runtime metadata、`bearer.token`、一次性 `launch.secret`、session 与 executor runtime 文件都在 workspace 外的 XDG state 目录；默认 CLI/HTTP 输出不打印 token、launch secret 或 workspace 绝对路径。

## systemd --user

创建 `~/.config/systemd/user/opsc-serve.service`：

```ini
[Unit]
Description=Ops Canvas local workspace API

[Service]
Type=simple
Environment=OPSC_WORKSPACE=%h/OpsCanvas
ExecStart=%h/.local/bin/opsc serve --workspace %h/OpsCanvas --origin http://localhost:3000
Restart=on-failure
RestartSec=3

[Install]
WantedBy=default.target
```

创建 `~/.config/systemd/user/opsc-executor.service`：

```ini
[Unit]
Description=Ops Canvas local workspace executor
After=opsc-serve.service

[Service]
Type=simple
Environment=OPSC_WORKSPACE=%h/OpsCanvas
# 可选：local-first 电商模板未在导入 metadata 中写入素材库路径时，用该环境变量提供本机 anime_ip 素材库。
# Environment=OPSC_LOCAL_ECOMMERCE_MATERIAL_LIBRARY=<local_anime_ip_library>
# 如 hybrid backend credential 使用 env secretRef，可放到仅当前用户可读的 env file。
# EnvironmentFile=%h/.config/opsc/hybrid.env
ExecStart=%h/.local/bin/opsc executor --workspace %h/OpsCanvas --watch --poll-interval 5s
Restart=on-failure
RestartSec=3

[Install]
WantedBy=default.target
```

启用：

```bash
systemctl --user daemon-reload
systemctl --user enable --now opsc-serve.service opsc-executor.service
systemctl --user status opsc-serve.service opsc-executor.service
```

排查：

```bash
journalctl --user -u opsc-serve.service -f
journalctl --user -u opsc-executor.service -f
opsc workspace doctor --workspace ~/OpsCanvas --json
```

## macOS 最小路径

macOS 上建议先用两个长期 shell / tmux / launchd wrapper 分别运行：

```bash
opsc serve --workspace ~/OpsCanvas --origin http://localhost:3000
opsc executor --workspace ~/OpsCanvas --watch --poll-interval 5s
```

正式 launchd plist 尚未作为本仓库 contract 固化；如需长期使用，先沿用上述命令并把 credential 放在用户级环境或安全文件引用中，不写入浏览器或 workspace 普通 JSON。
