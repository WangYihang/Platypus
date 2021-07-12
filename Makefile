build: build_platypus

install_dependency:
	sudo apt-get update
	sudo apt-get install -y software-properties-common gnupg
	sudo add-apt-repository -y ppa:longsleep/golang-backports
	sudo apt-key adv --keyserver keyserver.ubuntu.com --recv-keys 52B59B1571A79DBC054901C0F6BC817356A3D45E
	# Nodejs
	curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.38.0/install.sh | sudo bash
	sudo nvm install --lts
	sudo npm install -g yarn
	# Golang
	sudo apt install -y golang-go
	sudo apt install -y go-bindata
	go env -w GO111MODULE=on
	go env -w GOPROXY=https://goproxy.cn,direct
	# upx
	sudo apt install -y upx

prepare: 
	bash -c "[[ -d termites ]] || mkdir termites"
	bash -c "[[ -d build ]] || mkdir build"

build_frontend: prepare
	echo "Building frontend"
	cd html/frontend && yarn install && yarn build
	echo "Building ttyd"
	cd html/ttyd && yarn install && yarn build

build_termite: prepare
	echo "Building termite"
	# echo -e "Building \e[32mtermite_linux_amd64\e[0m"
	env GOOS=linux GOARCH=amd64 go build -o termites/termite_linux_amd64 termite.go

collect_resource: build_frontend build_termite
	echo "Collecting resource files"
	go-bindata -pkg resource -o ./lib/util/resource/resource.go ./termites/... ./lib/runtime/... ./html/ttyd/dist/... ./html/frontend/build/...

build_platypus: collect_resource
	echo "Building platypus"
	env go build -o build/platypus platypus.go
	find build -type f -executable | xargs upx

release: collect_resource
	env GOOS=linux GOARCH=amd64 go build -o ./build/Platypus_linux_amd64 platypus.go
	env GOOS=darwin GOARCH=amd64 go build -o ./build/Platypus_darwin_amd64 platypus.go
	env GOOS=windows GOARCH=amd64 go build -o ./build/Platypus_windows_amd64.exe platypus.go
	find build -type f -executable | xargs upx

clean:
	rm -rf build
	rm -rf termites
	rm -rf html/frontend/build
	rm -rf html/ttyd/build
