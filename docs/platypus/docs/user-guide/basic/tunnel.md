# 隧道

Agent 支持 4 种隧道模式，分别是：

* Pull
* Push
* Dynamic
* Internet

基本的命令格式为：

```
platypus-admin --server ... --secret ... tunnel <mode> <src> <dst>
```

或在 Desktop / Web UI 的 **Tunnels** Tab 中使用图形界面配置。

## Pull

将被管理主机所在网络中的某个端口（`192.168.1.1:80`）**拉**到 Platypus Server 主机的某个端口（`127.0.0.1:8080`）。

本功能类似于 ssh **本地端口转发**，即：*Make Remote Resources Accessible on Your Local System*。

```
tunnel pull --src 127.0.0.1:8080 --dst 192.168.1.1:80
```

此时，访问 Server 主机的 `127.0.0.1:8080` 即相当于访问被管理主机网络中的 `192.168.1.1:80`。

## Push

与 Pull 相反，将 Server 网络中的某个端口**推**到被管理主机的某个端口。

本功能类似于 ssh **远程端口转发**，即：*Make Local Resources Accessible on a Remote System*。

```
tunnel push --src 192.168.1.254:1080 --dst 127.0.0.1:1090
```

此时，访问被管理主机的 `127.0.0.1:1090` 即相当于访问 Server 网络中的 `192.168.1.254:1080`。

## Dynamic

将被管理主机所在网络通过 socks5 协议转发到 Server 的某个端口。适用于需要通过 Agent 跳板机访问被管理网络中其他机器的场景。

本功能类似于 ssh **动态端口转发**，即：*Use Agent Host as a Proxy*。

!!! Hint
    该功能的本质是在被管理主机上开启 socks5 代理，然后通过 Pull 功能**拉**到 Server。

```
tunnel dynamic
```

创建成功后，命令行中会输出远程 socks5 端口号与本地 socks5 端口号。Server 上挂上本地端口号的代理即可直接访问被管理主机的网络。

## Internet

将 Server 所在网络通过 socks5 协议转发到 Agent 所在主机的某个端口。适用于被管理主机本身无法访问互联网、但又需要在其上拉取互联网资源的场景。

```
tunnel internet --src 127.0.0.1:1090 --dst 127.0.0.1:1080
```

!!! Hint
    该功能的本质是在 Server 上开启 socks5 代理（`127.0.0.1:1090`），然后通过 Push 功能**推**到被管理主机（`127.0.0.1:1080`）。

在被管理主机上使用 `proxychains` 挂上该代理（`socks5 127.0.0.1 1080`）即可访问 Server 所在网络（通常是互联网）。
