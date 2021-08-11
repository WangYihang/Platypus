# 介绍

!!! Platypus
    一款支持多会话的交互式反向 Shell 管理器。

    在实际的渗透测试中，为了解决 Netcat/Socat 等工具在文件传输、多会话管理方面的不足。
    Platypus 在多会话管理的基础上增加了在渗透测试中更加有用的功能（如：交互式 Shell、文件操作、隧道等），
    可以更方便灵活地对反向 Shell 会话进行管理。

作为一个渗透测试工程师：

* 您是否遇到过下图中的情况？辛辛苦苦拿到的 Shell 被一个不小心按下的 ++ctrl+c++ 杀掉。

<figure>
  <img src="./images/netcat.png" width="300" />
</figure>

* 您是否苦于 netcat 无法方便地在反向 Shell 中上传和下载文件？
* 您是否还在苦苦铭记那些冗长繁琐的反向 Shell 的命令？
* 您是否还在为您的每一个 Shell 开启一个新的 netcat 端口进行监听？

如果您曾经遇到并且苦于上述的情景，那么 Platypus 将会是您的好伙伴！快来[上手](./start.md)尝试一下吧！
