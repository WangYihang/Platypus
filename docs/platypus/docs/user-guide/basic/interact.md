# 交互式 Shell

!!! Warning
    由于 Windows 并未提供类似 Linux 伪终端的概念，因此 Platypus 暂不支持在 Windows Agent 上获取交互式 Shell。

Platypus 通过 Agent 与 Server 之间的 TLS + Protobuf 通道转发 PTY 数据，您可以对任意已上线的 Agent 打开完全交互式 Shell（体验接近 SSH）。

## 通过 Admin CLI

已知目标 Agent 的哈希为 `134dd4cad7b110a021d46bd9dfe68d62`：

```
platypus-admin --server http://127.0.0.1:7331 --secret <S> exec 134dd4cad7b110a021d46bd9dfe68d62 -- /bin/bash
```

## 通过 Desktop / Web UI

打开 **Sessions** Tab，在目标行点击 **Open Terminal**。Terminal 面板是基于 xterm.js 的全功能终端，支持 `vim`、`htop`、`Ctrl+C/Z`、窗口大小自适应等。

关闭 Terminal Tab 会请求 Agent 终止该进程；您可以为同一 Agent 打开任意数量的 Terminal，彼此互不影响。
