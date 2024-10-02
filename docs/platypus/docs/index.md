# Overview

:::note
本文档对应 [Platypus v1.5.0](https://github.com/WangYihang/Platypus/releases/tag/v1.5.0)
:::

## What is Platypus?

Platypus 是一款支持**多会话**的**交互式**反向 Shell 管理器。

在实际的渗透测试中，为了解决 Netcat/Socat 等工具在文件传输、多会话管理方面的不足。该工具在多会话管理的基础上增加了在渗透测试中更加有用的功能（如：**交互式 Shell**、**文件操作**、**隧道**等），可以更方便灵活地对反向 Shell 会话进行管理。

## Platypus 解决的痛点有哪些？

作为一名渗透测试工程师：

* 您是否遇到过下图中的情况？辛辛苦苦拿到的 Shell 被一个不小心按下的 ++ctrl+c++ 杀掉。

![](./images/netcat.png)

* 您是否苦于 netcat 无法方便地在反向 Shell 中上传和下载文件？
* 您是否还在苦苦铭记那些冗长繁琐的反向 Shell 的命令？
* 您是否还在为您的每一个 Shell 开启一个新的 netcat 端口进行监听？

如果您曾经遇到并且苦于上述的情景，那么 [Platypus](https://github.com/WangYihang/Platypus) 将会是您的好伙伴！快来[上手](./getting-started.md)尝试一下吧！

## Platypus 未来计划是什么？

- [ ] 为 Termite 添加列目录功能
- [ ] 为 Termite 添加 Windows 支持
- [ ] RESTful API 添加认证功能
- [ ] 重新设计 Web UI
- [ ] 记录用户与 Shell 的交互，日后可以回放复盘（类似 [asciinema](https://asciinema.org/)）
- [ ] 添加主机发现等其他后渗透功能
- [ ] 多层 Termite 级联
- [ ] 集成提权功能
- [ ] 持久化
- [ ] 集成 rootkie
- [ ] 提供一键升级 Metepreter