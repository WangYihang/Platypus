# Platypus

[![GitHub stars](https://img.shields.io/github/stars/WangYihang/Platypus.svg)](https://github.com/WangYihang/Platypus/stargazers)
[![GitHub license](https://img.shields.io/github/license/WangYihang/Platypus.svg)](https://github.com/WangYihang/Platypus)

A modern multiple reverse shell sessions/clients manager via terminal written in go

#### Features

- [x] Multiple service listening port
- [x] Multiple client connections
- [x] RESTful API
- [x] Reverse shell as a service (Pop a reverse shell without remembering idle commands)
- [x] Full interactive shell
  - [x] Using vim gracefully in reverse shell
  - [x] Using CTRL+C and CTRL+Z in reverse shell

#### Usage
* [Attackers' guide](./USAGE.md)

#### Materials
##### Sides
* https://github.com/WangYihang/Presentations/blob/master/Platypus/Platypus.pdf

##### Videoes
[![](http://img.youtube.com/vi/Yfy6w8qXcQs/0.jpg)](http://www.youtube.com/watch?v=Yfy6w8qXcQs "Platypus")

##### Screenshots

![](https://upload-images.jianshu.io/upload_images/2355077-9ef699f1de815f9e.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/1240)
![](https://upload-images.jianshu.io/upload_images/2355077-bd729ecfe7d2dcc0.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/1240)

#### TODO
- [ ] [#19 Read command file when start up](https://github.com/WangYihang/Platypus/issues/19)
- [ ] [#12 Add capability of setting human-readable name of session](https://github.com/WangYihang/Platypus/issues/12)
- [ ] [#13 Add a display current prompt setting](https://github.com/WangYihang/Platypus/issues/13)
- [ ] [#10 Use database to record all events and interacting logs](https://github.com/WangYihang/Platypus/issues/10)
- [ ] [#11 Make STDOUT and STDERR distinguishable](https://github.com/WangYihang/Platypus/issues/11)
- [ ] [#6 Send one command to all clients at once(Meta Command)](https://github.com/WangYihang/Platypus/issues/6)
- [ ] [#15 Encryption support](https://github.com/WangYihang/Platypus/issues/15)
- [ ] More interfaces in RESTful API
- [ ] RESTful API should auth
- [ ] Use crontab
- [ ] Use HR package to detect the status of client (maybe `echo $random_string`)
- [ ] Provide full kernel API
- [ ] Upload file
- [ ] Download file
- [ ] List file
- [ ] Web UI
- [ ] Global Config (eg. BlockSameIP)
- [ ] User guide
- [ ] Benchmark
- [ ] Upgrade to Metepreter session
- [x] OS Detection (Linux|Windows) by [@EddieIvan01](https://github.com/EddieIvan01)
- [x] Upgrade common reverse shell session into full interactive session
- [x] Docker support (Added by [@yeya24](https://github.com/yeya24))


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


