# Overview

## What is Platypus?

Platypus 是一个 **Linux 主机管理 Hub**。您在需要被管理的每一台主机上安装 Platypus Agent，Agent 通过 TLS + Protobuf 主动连接到 Platypus Server，Server 作为统一的控制平面，让管理员可以对所有在线主机进行：

* 完全交互式 Shell（体验接近 SSH）
* 文件读写、上传、下载（分块传输）
* 4 种网络隧道（pull / push / dynamic SOCKS5 / internet）

## 为什么使用 Platypus？

* **单一控制面**：通过一个 Server 统一管理数十、数百台分布在不同网络的主机，无需逐台 SSH。
* **Agent-pull 模式**：由被管理主机主动连接 Server，对处于 NAT / 防火墙后的主机天然友好。
* **加密传输**：Agent ↔ Server 之间的所有管理流量经 TLS + Protobuf 封装。
* **多客户端接入**：Desktop / Web UI / CLI / Python SDK 可同时连接同一 Server，互不影响。

快来[上手](./getting-started.md)尝试一下吧！

## 路线图

- [ ] Agent 端目录列表
- [ ] Windows Agent 支持
- [ ] REST API 加入更细粒度的 RBAC
- [ ] Session 交互录像与回放（类似 [asciinema](https://asciinema.org/)）
- [ ] Agent 级联（多级跳板）
- [ ] 配置下发与 ad-hoc 任务编排
