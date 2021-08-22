build: build_platypus

install_dependency:
	sudo apt-get update
	sudo apt-get install -y software-properties-common gnupg
	sudo add-apt-repository -y ppa:longsleep/golang-backports
	sudo apt-key adv --keyserver keyserver.ubuntu.com --recv-keys 52B59B1571A79DBC054901C0F6BC817356A3D45E
	# Nodejs
	curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.38.0/install.sh | sudo bash
	sudo bash -c "source ${HOME}/.nvm/nvm.sh && nvm install --lts && npm install -g yarn"
	# Golang
	sudo apt install -y golang-go
	sudo apt install -y go-bindata
	go env -w GO111MODULE=on
	go env -w GOPROXY=https://goproxy.cn,direct
	# upx
	sudo apt install -y upx

install_dependency_github_action:
	sudo apt install go-bindata

prepare: 
	bash -c "[[ -d termites ]] || mkdir termites"
	bash -c "[[ -d build ]] || mkdir build"

build_frontend: prepare
	echo "Building frontend"
	cd web/frontend && bash -c "source ${HOME}/.nvm/nvm.sh && yarn install && yarn build"
	echo "Building ttyd"
	cd web/ttyd && bash -c "source ${HOME}/.nvm/nvm.sh && yarn install && yarn build"

build_termite: prepare
	echo "Building termite"
	# echo -e "Building termite_linux_amd64"
	env GOOS=linux GOARCH=amd64 go build -ldflags="-s -w " -trimpath -o ./build/termite/termite_linux_amd64 cmd/termite/main.go

collect_assets: build_frontend build_termite
	echo "Collecting assets files"
	go-bindata -pkg assets -o ./internal/util/assets/assets.go ./assets/config.example.yml ./assets/template/rsh/... ./web/ttyd/dist/... ./web/frontend/build/... ./build/termite/...

dev:
	env GOOS=linux GOARCH=amd64 go build -ldflags="-s -w " -trimpath -o ./build/termite/termite_linux_amd64 cmd/termite/main.go
	go-bindata -pkg assets -o ./internal/util/assets/assets.go ./assets/config.example.yml ./assets/template/rsh/... ./web/ttyd/dist/... ./web/frontend/build/... ./build/termite/...
	env go build -ldflags="-s -w " -trimpath -o ./build/platypus/platypus cmd/platypus/main.go

build_platypus: collect_assets
	echo "Building platypus"
	env go build -ldflags="-s -w " -trimpath -o ./build/platypus/platypus cmd/platypus/main.go

release: install_dependency_github_action collect_assets
	env GOOS=linux GOARCH=amd64 go build -ldflags="-s -w " -trimpath -o ./build/platypus/Platypus_linux_amd64 cmd/platypus/main.go
	env GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w " -trimpath -o ./build/platypus/Platypus_darwin_amd64 cmd/platypus/main.go
	env GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w " -trimpath -o ./build/platypus/Platypus_darwin_arm64 cmd/platypus/main.go
	env GOOS=windows GOARCH=amd64 go build -ldflags="-s -w " -trimpath -o ./build/platypus/Platypus_windows_amd64.exe cmd/platypus/main.go
	find build -type f -executable | xargs upx --ultra-brute

clean:
	rm -rf build
	rm -rf internal/util/assets/assets.go
	rm -rf web/frontend/build
	rm -rf web/ttyd/build
