# 文件操作

当跳转到某一个 Shell 之后，上传或下载文件。

!!! Hints
    目前 Platypus 只支持在 Cli 模式下进行文件上传下载操作

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