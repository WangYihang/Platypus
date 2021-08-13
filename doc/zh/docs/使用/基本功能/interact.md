# 交互式 Shell

!!! Warning
    由于 Windows 并未提供类似 Linux 伪终端的概念，因此 Platypus 暂不支持获取 Windows 的交互式 Shell。

Platypus 提供 2 种方式实现交互式 Shell。

## 方案一：Termite（推荐）

!!! Tips
    Termite 客户端是 Platypus 特有的客户端，支持交互式 Shell、文件传输以及网络隧道等功能，您可以参考本文来了解关于如何获取 Termite Shell 以及 [Termite](./termite.md) 的更多信息。

与哈希为 `134dd4cad7b110a021d46bd9dfe68d62` 的 Termite 客户端交互。

```
» Jump 134dd4cad7b110a021d46bd9dfe68d62
» Interact
```

暂时终止与当前 Shell 交互。

```
exit
```

## 方案二：`PTY`（不推荐）

与哈希为 `134dd4cad7b110a021d46bd9dfe68d62` 的客户端交互。

```
» Jump 134dd4cad7b110a021d46bd9dfe68d62
» PTY
» Interact
```

!!! Tips
    本功能的实现逻辑参考自[本文](https://blog.ropnop.com/upgrading-simple-shells-to-fully-interactive-ttys/)，本质是在目标机器上执行 `pty.spawn("/bin/bash")`，然后通过将攻击者的终端设置为 `raw` 模式来实现交互式 Shell。


暂时终止与当前 Shell 交互。

!!! Tips
    由于 Platypus 需要提供**暂时终止与当前 Shell 进行交互**的功能，另外在裸 Shell 中我们很难去判断用户输入 `exit` 时，是否是对 `shell` 进行操作的（如：用户在 `vim` 中输入 `exit`），所以就不能通过用户是否输入 `exit` 来判断用户是否想要终止与当前 Shell 进行交互，因此 Platypus 定义了自己的退出命令 `platyquit`

```
platyquit
```

