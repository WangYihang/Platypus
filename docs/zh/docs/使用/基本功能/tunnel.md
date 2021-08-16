# 隧道

Termite 提供 4 种隧道模式，分别是：

* Pull
* Push
* Dynamic
* Internet

基本的命令格式为：

```
» Tunnel Create [MODE] [SRC IP] [SRC PORT] [DST IP] [DST PORT]
```

## Pull

顾名思义，`Pull` 即将目标主机网络中的某个端口（`192.168.1.1:80`）**拉**到 Platypus 主机的某个端口（`127.0.0.1:8080`）。

本功能类似于 ssh **本地端口转发**，即：*Make Remote Resources Accessible on Your Local System*。

```
» Tunnel Create Pull 192.168.1.1 80 127.0.0.1 8080
```

此时，访问 Platypus 主机的 `127.0.0.1:8080` 即相当于访问目标主机网络中的 `192.168.1.1:80`。

## Push

与 `Pull` 功能的类似，将 Platypus 网络中的某个端口**推**到目标主机的某个端口。


本功能类似于 ssh **远程端口转发**，即：*Make Local Resources Accessible on a Remote System*。

```
» Tunnel Create Push 192.168.1.254 1080 127.0.0.1 1090
```

此时，访问目标主机的 `127.0.0.1:1090` 即相当于访问 Platypus 网络中的 `192.168.1.254:1080`。

## Dynamic

将目标主机网络通过 socks5 协议转发到 Platypus 主机上的某个端口。主要应用在内网渗透环节需要通过跳板机攻击内网中其他机器的场景。

本功能类似于 ssh **动态端口转发**，即：*Use Your Termite Client as a Proxy*。

!!! Hint
    该功能的本质是在目标主机上开启 socks5 代理，然后将其通过 `Pull` 功能**拉**到本地。

```
» Tunnel Create Dynamic x.x.x.x xxxxx x.x.x.x xxxxx
```

!!! Tips
    上述命令中的 x.x.x.x 和 xxxxx 可以随便填，Platypus 并未使用这两个位置的参数。
    上面这个奇怪的规定只是因为 Platypus 暂时只是简单解析了这 4 种模式的参数，后续会修正该问题。

当 socks5 代理创建成功后，在命令行中会输出远程 socks5 端口号与本地 socks5 端口号。
此时，Platypus 主机只需要挂上本地端口号的代理即可直接访问目标主机的网络进行后续的内网渗透。

## Internet

将 Platypus 主机所在网络通过 socks5 协议转发到目标服务器的某个端口。
主要应用在内网渗透环节目标机器无法访问互联网但是又需要在其上下载某些互联网上的资料的时候。

```
» Tunnel Create Internet 127.0.0.1 1090 127.0.0.1 1080
```

!!! Hint
    该功能的本质是在 Platypus 上开启 socks5 代理（`127.0.0.1:1090`），然后将其通过 `Push` 功能**推**到目标机器（`127.0.0.1:1080`）。

此时，只需要在目标主机上使用 `proxychains` 挂上代理（`socks5 127.0.0.1 1080`）即可访问互联网（即：Platypus 所在网络）。