build: platypus

dependency:
	sudo apt-get update
	sudo apt-get install -y software-properties-common gnupg
	sudo add-apt-repository -y ppa:longsleep/golang-backports
	sudo apt-key adv --keyserver keyserver.ubuntu.com --recv-keys 52B59B1571A79DBC054901C0F6BC817356A3D45E
	# Nodejs
	curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.38.0/install.sh | bash
	bash -c "source ${HOME}/.nvm/nvm.sh && nvm install --lts && npm install -g yarn"
	# Golang
	sudo apt install -y golang-go
	go env -w GO111MODULE=on
	go env -w GOPROXY=https://goproxy.cn,direct
	# upx
	sudo apt install -y upx

go-bindata:
	go install github.com/go-bindata/go-bindata/...@latest

mkdir: 
	bash -c "[[ -d build ]] || mkdir build"

frontend: mkdir
	echo "Building frontend"
	cd web/frontend && bash -c "source ${HOME}/.nvm/nvm.sh && yarn install && yarn build"
	echo "Building ttyd"
	cd web/ttyd && bash -c "source ${HOME}/.nvm/nvm.sh && yarn install && yarn build"

termite: mkdir
	echo "Building termite"
	# echo -e "Building termite_linux_amd64"
	env GOOS=linux GOARCH=amd64 go build -ldflags="-s -w " -trimpath -o ./build/termite/termite_linux_amd64 cmd/termite/main.go

assets: go-bindata frontend termite
	echo "Collecting assets files"
	${HOME}/go/bin/go-bindata -pkg assets -o ./internal/util/assets/assets.go ./assets/config.example.yml ./assets/template/rsh/... ./web/ttyd/dist/... ./web/frontend/build/... ./build/termite/...

dev: go-bindata
	env GOOS=linux GOARCH=amd64 go build -ldflags="-s -w " -trimpath -o ./build/termite/termite_linux_amd64 cmd/termite/main.go
	${HOME}/go/bin/go-bindata -pkg assets -o ./internal/util/assets/assets.go ./assets/config.example.yml ./assets/template/rsh/... ./web/ttyd/dist/... ./web/frontend/build/... ./build/termite/...
	env GOOS=linux GOARCH=amd64 go build -ldflags="-s -w " -trimpath -o ./build/platypus/platypus cmd/platypus/main.go

release: assets
	env GOOS=linux GOARCH=amd64 go build -ldflags="-s -w " -trimpath -o ./build/platypus/Platypus_linux_amd64 cmd/platypus/main.go
	env GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w " -trimpath -o ./build/platypus/Platypus_darwin_amd64 cmd/platypus/main.go
	env GOOS=windows GOARCH=amd64 go build -ldflags="-s -w " -trimpath -o ./build/platypus/Platypus_windows_amd64.exe cmd/platypus/main.go
	find build -type f -executable | xargs upx

clean:
	rm -rf *.yml
	rm -rf build
	rm -rf compile
	rm -rf internal/util/assets/assets.go
	rm -rf web/frontend/build
	rm -rf web/ttyd/build
