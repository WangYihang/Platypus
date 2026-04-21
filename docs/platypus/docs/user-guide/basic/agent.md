# Agent

## Agent 是什么？

Platypus Agent 是部署在被管理主机上的常驻进程。启动后它会主动连接到 Platypus Server，由 Server 统一调度后续的管理操作。相比直连 SSH，通过 Agent 管理主机可以获得：

* 完全交互式 Shell（体验接近 SSH），支持同时启动多个独立 Shell 互不影响。
* 4 种不同的[网络隧道](./tunnel.md)（pull / push / dynamic SOCKS5 / internet）。
* 稳定可靠的文件读写、上传、下载（分块传输）。
* TLS + Protobuf 的加密控制通道，一次 TLS 握手后全程不再发送明文。

## 安装 Agent

Platypus Server 启动时会在 Distributor 端口（默认 `13339`）提供一个 HTTP 下载入口。Distributor 会把 connect-back 地址**原地写入**预编译好的 Agent 二进制中，从而只需要维护一份二进制即可覆盖任意 Ingress 地址。

在**您希望管理的主机**上执行：

```bash
curl -fsSL http://<server>:13339/agent/<server>:13337 -o /usr/local/bin/platypus-agent \
  && chmod +x /usr/local/bin/platypus-agent \
  && /usr/local/bin/platypus-agent
```

!!! Hint
    如果您希望减小 Agent 二进制体积，可以在 Server 端安装 `upx`（Ubuntu: `sudo apt install upx`），Platypus 会自动在下发前压缩。

## 作为 systemd 服务运行

生产环境中推荐将 Agent 注册为 systemd 单元，确保开机自启与异常崩溃后的自动重连：

```ini
# /etc/systemd/system/platypus-agent.service
[Unit]
Description=Platypus management agent
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/local/bin/platypus-agent
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

启用：

```bash
sudo systemctl enable --now platypus-agent
sudo systemctl status platypus-agent
```

## 在 Server 端管理已连接的 Agent

在 Admin CLI 或 Desktop/Web UI 中，已连接的 Agent 会以 Session 的形式出现。常用操作：

* **查看 Session 列表**：Desktop 的 Sessions Tab / `platypus-admin sessions`
* **打开交互式 Shell**：Desktop 的 Terminal / `platypus-admin exec <hash> -- /bin/bash`
* **读写文件**：Desktop 的 Files Tab
* **建立网络隧道**：Desktop 的 Tunnels Tab / `platypus-admin tunnel`

所有操作都经由 Server 的 REST / WebSocket API，Server 是唯一可直接与 Agent 通信的对端。
