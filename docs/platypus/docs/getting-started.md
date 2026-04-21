# Getting Started

!!! Tips
    您可以在[**这里**](https://github.com/WangYihang/Platypus/releases)下载最新 Release，或参考[**这里**](/内部机制/build/)从源码编译 Platypus。

## 启动 Server

!!! Hint
    下文假设 Platypus Server 部署在具有公网 IP 的主机上，IP 为 `1.3.3.7`。

### Linux

```
./platypus-server
```

### Windows

```
.\platypus-server.exe
```

启动时 Server 会完成初始化、绑定配置文件中声明的每一个 Ingress 端口（默认 `13337`）与 Distributor 端口（默认 `13339`）、并拉起 REST API（默认 `127.0.0.1:7331`）。

!!! Tips
    如果您希望了解 Server 的具体启动流程，可以参考[本文](/内部机制/startup/)。

## 在管理的主机上安装 Agent

Platypus 采用 Agent-pull 模式：**由被管理主机主动连接 Server**。Server 的 Distributor 会把 connect-back 地址原地写入预编译好的 Agent 二进制中，只需维护一份二进制即可覆盖任意 Ingress。

在目标主机上执行：

```bash
curl -fsSL http://1.3.3.7:13339/agent/1.3.3.7:13337 -o /usr/local/bin/platypus-agent \
  && chmod +x /usr/local/bin/platypus-agent \
  && /usr/local/bin/platypus-agent
```

更完整的安装方式（含 systemd 单元）见 [Agent 文档](/使用/基本功能/agent/)。

Agent 启动后会立即回连 Server 完成 TLS 握手，成功后即可在 Server 端看到新上线的 Session。

## 与 Server 交互

Platypus 提供 3 种与 Server 交互的方式：

* [Admin CLI](/使用/交互方式/cli/)
* [Desktop / Web UI](/使用/交互方式/web/)
* [Python SDK](/使用/交互方式/sdk/)

### Admin CLI 常用命令

```bash
# 获取 Session 列表
platypus-admin --server http://127.0.0.1:7331 --secret <S> sessions

# 在某个 Session 上执行单条命令
platypus-admin --server http://127.0.0.1:7331 --secret <S> exec <session-hash> -- uname -a

# 查看 / 管理 Listener
platypus-admin --server http://127.0.0.1:7331 --secret <S> list

# 建立网络隧道
platypus-admin --server http://127.0.0.1:7331 --secret <S> tunnel ...
```

Session 的哈希由配置文件中的 `hashFormat` 决定；详见[哈希计算](/内部机制/hashing/)。

### 交互式 Shell、文件管理、隧道

上述高频功能在 Desktop / Web UI 中更为直观：

* **Sessions** Tab：查看所有在线 Agent。
* **Terminal** Tab：对指定 Agent 打开完全交互式 Shell（类 SSH 体验），详见 [Interact 文档](/使用/基本功能/interact/)。
* **Files** Tab：对远端文件进行读写、上传、下载。
* **Tunnels** Tab：建立 pull / push / dynamic SOCKS5 / internet 隧道，详见 [Tunnel 文档](/使用/基本功能/tunnel/)。
