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
	bash -c "which go || (curl -LO https://get.golang.org/$(uname)/go_installer && chmod +x go_installer && ./go_installer && rm go_installer)"
	sudo apt install nodejs npm go-bindata
	sudo npm install -g yarn

clean:
	rm -rf build
	rm -rf termites
	rm -rf $(mkfile_dir)html/frontend/build
	rm -rf $(mkfile_dir)html/ttyd/build
