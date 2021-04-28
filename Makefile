mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
mkfile_dir := $(dir $(mkfile_path))

build: prepare
	echo "Building platypus"
	go build -o build/platypus platypus.go

release: prepare
	env GOOS=linux GOARCH=amd64 go build -o ./build/Platypus_linux_amd64 platypus.go
	env GOOS=darwin GOARCH=amd64 go build -o ./build/Platypus_darwin_amd64 platypus.go
	env GOOS=windows GOARCH=amd64 go build -o ./build/Platypus_windows_amd64.exe platypus.go

prepare: dependency
	bash -c "[[ -d termites ]] || mkdir termites"
	bash -c "[[ -d build ]] || mkdir build"
	echo "Building frontend"
	cd $(mkfile_dir)html/frontend && yarn install && yarn build
	echo "Building ttyd"
	cd $(mkfile_dir)html/ttyd && yarn install && yarn build
	echo "Building termite"
	env GOOS=linux GOARCH=amd64 go build -o $(mkfile_dir)termites/termite_linux_amd64 termite.go
	env GOOS=darwin GOARCH=amd64 go build -o $(mkfile_dir)termites/termite_darwin_amd64 termite.go
	echo "Collecting static files"
	cd $(mkfile_dir) && go-bindata -pkg resource -o ./lib/util/resource/resource.go ./termites/... ./lib/runtime/... ./html/ttyd/dist/... ./html/frontend/build/...

dependency:
	sudo apt-get install software-properties-common gpg
	sudo add-apt-repository ppa:longsleep/golang-backports
	sudo apt-get update
	sudo apt-key adv --keyserver keyserver.ubuntu.com --recv-keys 52B59B1571A79DBC054901C0F6BC817356A3D45E
	sudo apt-get install golang-go
	sudo apt install nodejs npm go-bindata upx
	sudo npm install -g yarn

clean:
	rm -rf build
	rm -rf termites
	rm -rf $(mkfile_dir)html/frontend/build
	rm -rf $(mkfile_dir)html/ttyd/build
