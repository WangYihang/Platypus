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
	env GOOS=linux GOARCH=amd64 go build -o ./build/termite/termite_linux_amd64 cmd/termite/main.go

collect_resource: build_frontend build_termite
	echo "Collecting resource files"
	go-bindata -pkg resource -o ./lib/util/resource/resource.go ./build/termite/... ./lib/runtime/... ./web/ttyd/dist/... ./web/frontend/build/...

build_platypus: collect_resource
	echo "Building platypus"
	env go build -o ./build/platypus/platypus cmd/platypus/main.go
	find build -type f -executable | xargs upx

release: install_dependency_github_action collect_resource
	env GOOS=linux GOARCH=amd64 go build -o ./build/platypus/Platypus_linux_amd64 cmd/platypus/main.go
	env GOOS=darwin GOARCH=amd64 go build -o ./build/platypus/Platypus_darwin_amd64 cmd/platypus/main.go
	env GOOS=windows GOARCH=amd64 go build -o ./build/platypus/Platypus_windows_amd64.exe cmd/platypus/main.go
	find build -type f -executable | xargs upx

clean:
	rm -rf build
	rm -rf termites
	rm -rf web/frontend/build
	rm -rf web/ttyd/build
