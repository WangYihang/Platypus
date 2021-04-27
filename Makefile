mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
mkfile_dir := $(dir $(mkfile_path))

build:
	bash -c "[[ -d termites ]] || mkdir termites"
	bash -c "[[ -d build ]] || mkdir build"
	echo "Building frontend"
	cd $(mkfile_dir)html/frontend && yarn install && yarn build
	echo "Building ttyd"
	cd $(mkfile_dir)html/ttyd && yarn install && yarn build
	echo "Building termite"
	env GOOS=linux GOARCH=amd64 go build -o termites/termite_linux_amd64 termite.go
	env GOOS=darwin GOARCH=amd64 go build -o termites/termite_darwin_amd64 termite.go
	echo "Collecting static files"
	go-bindata -pkg resource -o $(mkfile_dir)lib/util/resource/resource.go $(mkfile_dir)termites/... $(mkfile_dir)lib/runtime/... $(mkfile_dir)html/ttyd/dist/... $(mkfile_dir)html/frontend/build/...
	echo "Building platypus"
	go build -o build/platypus platypus.go

dependency:
	bash -c "which go || (curl -LO https://get.golang.org/$(uname)/go_installer && chmod +x go_installer && ./go_installer && rm go_installer)"
	sudo apt install nodejs npm go-bindata
	sudo npm install -g yarn

clean:
	rm -r build
	rm -r termite
	rm -rf $(mkfile_dir)html/frontend/build
	rm -rf $(mkfile_dir)html/ttyd/build
	rm -rf config.yml