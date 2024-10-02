# 计算节点哈希

为了保证 Platypus 同时只维护一个来自相同主机的 Shell，需要对目标主机的唯一性进行判定，
Platypus 使用收集到的目标主机的信息对其进行哈希操作，以此来保证唯一性。

哈希所使用的参数可见配置文件：

```yaml
# Using TimeStamp allows us to track all connections from the same IP / Username / OS and MAC.
hashFormat: "%i %u %m %o %t"
```

其中 `%?` 的含义如下：

* `%i` 上线机器的 IP
* `%u` 上线机器的用户名
* `%m` 上线机器的网络接口
* `%o` 上线机器的操作系统
* `%t` 上线的时间戳

默认情况下，Platypus 将会按照配置文件中的哈希模式 `"%i %u %m %o %t"` 对上线的客户端进行哈希。
按照上述的哈希模式已经基本可以保证同一个 IP、用户上线的连接将只会保留一个，因此您不需要对其进行修改。