# Web 界面

## 创建新的服务端

点击页面顶部的 Create 按钮即可创建新的监听端口。

![](/images/webui/add.gif)

## 启动交互式 Shell

![](/images/webui/shell.gif)

## 快速分享 Shell

每一个上线的 Termite 客户端（假设其哈希为 `28257c3130906d896d72ee1c9eed7661`）都有一个唯一的 URL 来启动其 Shell，您可以将该 URL 发送给您的队友，他打开之后即可获取对应 Termite 客户端的 Shell，并且每个 Shell 互不影响。

```
http://127.0.0.1:7331/shell/?28257c3130906d896d72ee1c9eed7661
```