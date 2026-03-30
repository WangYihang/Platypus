all: release

build: build_platypus

install_dependency:
	sudo apt update
	# Nodejs
	node --version || (curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.1/install.sh | bash && bash -c "source ${HOME}/.nvm/nvm.sh && nvm install 22 && npm install -g yarn")
	# Golang
	axel --version || sudo apt install -y axel
	unar --version || sudo apt install -y unar
	git --version || sudo apt install -y git
	go version || (sudo axel https://go.dev/dl/go1.24.0.linux-amd64.tar.gz && sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.24.0.linux-amd64.tar.gz)
	go env -w GO111MODULE=on
	# upx
	sudo apt install -y upx

prepare:
	bash -c "[[ -d build ]] || mkdir build"

build_frontend: prepare
	echo "Building frontend"
	cd web/frontend && bash -c "source ${HOME}/.nvm/nvm.sh && yarn install && NODE_OPTIONS='--max-old-space-size=1024' yarn build"
	echo "Building ttyd"
	cd web/ttyd && bash -c "source ${HOME}/.nvm/nvm.sh && yarn install && NODE_OPTIONS='--max-old-space-size=1024' yarn build"

proto:
	protoc --go_out=pkg/proto/agent/v1 --go_opt=paths=source_relative proto/agent/v1/agent.proto

build_platypus: prepare
	echo "Building platypus-server"
	go build -ldflags="-s -w " -trimpath -o ./build/platypus-server ./cmd/platypus-server
	echo "Building platypus-admin"
	go build -ldflags="-s -w " -trimpath -o ./build/platypus-admin ./cmd/platypus-admin
	echo "Building platypus-agent"
	go build -ldflags="-s -w " -trimpath -o ./build/platypus-agent ./cmd/platypus-agent

release: install_dependency build_frontend
	# Linux
	env GOOS=linux GOARCH=amd64 go build -ldflags="-s -w " -trimpath -o ./build/platypus-server_linux_amd64 ./cmd/platypus-server
	env GOOS=linux GOARCH=arm64 go build -ldflags="-s -w " -trimpath -o ./build/platypus-server_linux_arm64 ./cmd/platypus-server
	# MacOS
	env GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w " -trimpath -o ./build/platypus-server_darwin_amd64 ./cmd/platypus-server
	env GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w " -trimpath -o ./build/platypus-server_darwin_arm64 ./cmd/platypus-server
	# Windows
	env GOOS=windows GOARCH=amd64 go build -ldflags="-s -w " -trimpath -o ./build/platypus-server_windows_amd64.exe ./cmd/platypus-server

clean:
	rm -rf build
	rm -rf web/frontend/build
	rm -rf web/ttyd/build
