# 启动过程

当 `platypus-server` 运行后，会检测当前目录下是否存在 `config.yml`；若不存在会根据 `assets/config.example.yml` 生成一份，然后读取该配置启动。

启动时 Server 会按顺序：

1. 检查是否存在新版本并提示升级（若 `update: true`）。
2. 检测每个 Ingress 绑定地址的公网 IP（用于日志展示）。
3. 拉起配置中声明的每一个 TLS Ingress 端口、Distributor 端口、REST API。
4. 在日志中输出可供管理员复制到被管理主机执行的 `curl | sh` 安装命令。

在默认配置下 Server 会监听 3 个端口：

* `0.0.0.0:13337` — TLS Ingress，Agent 回连入口
* `0.0.0.0:13339` — Distributor，HTTP 下发 Agent 二进制
* `0.0.0.0:7331`  — REST API + WebSocket（Bearer token 认证）

!!! Warning
    REST API 通过 Bearer token 认证（token 在首次启动时随机生成，并在日志中打印）。如果需要暴露到公网，建议在 REST API 前面加一层 TLS 反向代理（nginx、Caddy），并使用强随机 secret。
