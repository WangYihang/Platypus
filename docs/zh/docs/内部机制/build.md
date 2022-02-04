# 编译

!!! Warning
    由于 Web 前端的编译依赖于某些 Linux 的特性，因此暂不支持在 Windows 平台对 Platypus 进行编译。

## 完整编译

准备一个纯净的 Ubuntu 20.04 环境，然后执行如下命令：

```bash
sudo apt update && \
sudo apt install -y curl make && \
git clone https://github.com/WangYihang/Platypus.git && \
cd Platypus && \
make install_dependency && \
make release
```

编译成功后，发行版将会位于 `./build` 文件夹中。

!!! Warning
    使用 [Makefile](https://github.com/WangYihang/Platypus/blob/master/Makefile) 安装依赖的时候会通过 `raw.githubusercontent.com` 下载 `nvm` 的安装文件，因此需要确保您可以正常访问 `raw.githubusercontent.com`。如果您不能正常访问该域名，则需要您根据 Makefile 中的依赖安装部分手动安装所需依赖。

## 单独编译

### 安装编译环境

!!! 编译环境依赖如下程序
    * golang >= 1.6
    * node >= 14
    * yarn
    * upx

```bash
make install_dependency
```

### 编译 Web 前端

```bash
make build_frontend
```

### 编译 Termite

```bash
make build_termite
```

为了保证 Platypus 只有单个文件，因此在编译 Platypus 时，会将所有 Termite 的二进制文件直接打包到 Platypus 的可执行文件中。

但为了避免打包后的 Platypus 过大，目前暂时只配置了编译 `linux_amd64` 平台的 Termite，如需其他平台，可以修改 Makefile 
中 `build_termite` 的部分，如下：

```
env GOOS=linux GOARCH=amd64 go build -o termites/termite_linux_amd64 termite.go
```

可以通过 `go tool dist list` 列出所有 Golang 支持的操作系统与平台组合。

!!! Warning
    由于 Termite 依赖于 Linux 的伪终端特性，因此暂时不支持编译能在 Windows 上运行的 Termite 客户端。

### 整合资源文件

本步骤会将之前编译好的 Web 前端文件、Termite 可执行文件等统一打包用以编译 Platypus。 

```bash
make collect_assets
```

### 编译发布版本

```bash
make release
```

为了避免 GitHub Actions 编译太多目标平台的发行版导致资源消耗严重，因此默认只配置了 3 个
常见的目标平台，分别是：

* windows/amd64
* linux/amd64
* darwin/amd64

如果需要添加其他平台，可以通过修改 Makefile 中的 `release` 部分来实现。

```
env GOOS=linux GOARCH=amd64 go build -o ./build/Platypus_linux_amd64 platypus.go
```