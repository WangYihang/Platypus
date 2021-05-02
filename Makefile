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
	# echo -e "Building \e[32mtermite_aix_ppc64\e[0m"
	# env GOOS=aix GOARCH=ppc64 go build -o $(mkfile_dir)termites/termite_aix_ppc64 termite.go
	# echo -e "Building \e[32mtermite_android_386\e[0m"
	# env GOOS=android GOARCH=386 go build -o $(mkfile_dir)termites/termite_android_386 termite.go
	# echo -e "Building \e[32mtermite_android_amd64\e[0m"
	# env GOOS=android GOARCH=amd64 go build -o $(mkfile_dir)termites/termite_android_amd64 termite.go
	# echo -e "Building \e[32mtermite_android_arm\e[0m"
	# env GOOS=android GOARCH=arm go build -o $(mkfile_dir)termites/termite_android_arm termite.go
	# echo -e "Building \e[32mtermite_android_arm64\e[0m"
	# env GOOS=android GOARCH=arm64 go build -o $(mkfile_dir)termites/termite_android_arm64 termite.go
	# echo -e "Building \e[32mtermite_darwin_amd64\e[0m"
	# env GOOS=darwin GOARCH=amd64 go build -o $(mkfile_dir)termites/termite_darwin_amd64 termite.go
	# echo -e "Building \e[32mtermite_darwin_arm64\e[0m"
	# env GOOS=darwin GOARCH=arm64 go build -o $(mkfile_dir)termites/termite_darwin_arm64 termite.go
	# echo -e "Building \e[32mtermite_dragonfly_amd64\e[0m"
	# env GOOS=dragonfly GOARCH=amd64 go build -o $(mkfile_dir)termites/termite_dragonfly_amd64 termite.go
	# echo -e "Building \e[32mtermite_freebsd_386\e[0m"
	# env GOOS=freebsd GOARCH=386 go build -o $(mkfile_dir)termites/termite_freebsd_386 termite.go
	# echo -e "Building \e[32mtermite_freebsd_amd64\e[0m"
	# env GOOS=freebsd GOARCH=amd64 go build -o $(mkfile_dir)termites/termite_freebsd_amd64 termite.go
	# echo -e "Building \e[32mtermite_freebsd_arm\e[0m"
	# env GOOS=freebsd GOARCH=arm go build -o $(mkfile_dir)termites/termite_freebsd_arm termite.go
	# echo -e "Building \e[32mtermite_illumos_amd64\e[0m"
	# env GOOS=illumos GOARCH=amd64 go build -o $(mkfile_dir)termites/termite_illumos_amd64 termite.go
	# echo -e "Building \e[32mtermite_ios_arm64\e[0m"
	# env GOOS=ios GOARCH=arm64 go build -o $(mkfile_dir)termites/termite_ios_arm64 termite.go
	# echo -e "Building \e[32mtermite_js_wasm\e[0m"
	# env GOOS=js GOARCH=wasm go build -o $(mkfile_dir)termites/termite_js_wasm termite.go
	# echo -e "Building \e[32mtermite_linux_386\e[0m"
	# env GOOS=linux GOARCH=386 go build -o $(mkfile_dir)termites/termite_linux_386 termite.go
	# echo -e "Building \e[32mtermite_linux_amd64\e[0m"
	env GOOS=linux GOARCH=amd64 go build -o $(mkfile_dir)termites/termite_linux_amd64 termite.go
	# echo -e "Building \e[32mtermite_linux_arm\e[0m"
	# env GOOS=linux GOARCH=arm go build -o $(mkfile_dir)termites/termite_linux_arm termite.go
	# echo -e "Building \e[32mtermite_linux_arm64\e[0m"
	# env GOOS=linux GOARCH=arm64 go build -o $(mkfile_dir)termites/termite_linux_arm64 termite.go
	# echo -e "Building \e[32mtermite_linux_ppc64\e[0m"
	# env GOOS=linux GOARCH=ppc64 go build -o $(mkfile_dir)termites/termite_linux_ppc64 termite.go
	# echo -e "Building \e[32mtermite_linux_ppc64le\e[0m"
	# env GOOS=linux GOARCH=ppc64le go build -o $(mkfile_dir)termites/termite_linux_ppc64le termite.go
	# echo -e "Building \e[32mtermite_linux_mips\e[0m"
	# env GOOS=linux GOARCH=mips go build -o $(mkfile_dir)termites/termite_linux_mips termite.go
	# echo -e "Building \e[32mtermite_linux_mipsle\e[0m"
	# env GOOS=linux GOARCH=mipsle go build -o $(mkfile_dir)termites/termite_linux_mipsle termite.go
	# echo -e "Building \e[32mtermite_linux_mips64\e[0m"
	# env GOOS=linux GOARCH=mips64 go build -o $(mkfile_dir)termites/termite_linux_mips64 termite.go
	# echo -e "Building \e[32mtermite_linux_mips64le\e[0m"
	# env GOOS=linux GOARCH=mips64le go build -o $(mkfile_dir)termites/termite_linux_mips64le termite.go
	# echo -e "Building \e[32mtermite_linux_riscv64\e[0m"
	# env GOOS=linux GOARCH=riscv64 go build -o $(mkfile_dir)termites/termite_linux_riscv64 termite.go
	# echo -e "Building \e[32mtermite_linux_s390x\e[0m"
	# env GOOS=linux GOARCH=s390x go build -o $(mkfile_dir)termites/termite_linux_s390x termite.go
	# echo -e "Building \e[32mtermite_netbsd_386\e[0m"
	# env GOOS=netbsd GOARCH=386 go build -o $(mkfile_dir)termites/termite_netbsd_386 termite.go
	# echo -e "Building \e[32mtermite_netbsd_amd64\e[0m"
	# env GOOS=netbsd GOARCH=amd64 go build -o $(mkfile_dir)termites/termite_netbsd_amd64 termite.go
	# echo -e "Building \e[32mtermite_netbsd_arm\e[0m"
	# env GOOS=netbsd GOARCH=arm go build -o $(mkfile_dir)termites/termite_netbsd_arm termite.go
	# echo -e "Building \e[32mtermite_openbsd_386\e[0m"
	# env GOOS=openbsd GOARCH=386 go build -o $(mkfile_dir)termites/termite_openbsd_386 termite.go
	# echo -e "Building \e[32mtermite_openbsd_amd64\e[0m"
	# env GOOS=openbsd GOARCH=amd64 go build -o $(mkfile_dir)termites/termite_openbsd_amd64 termite.go
	# echo -e "Building \e[32mtermite_openbsd_arm\e[0m"
	# env GOOS=openbsd GOARCH=arm go build -o $(mkfile_dir)termites/termite_openbsd_arm termite.go
	# echo -e "Building \e[32mtermite_openbsd_arm64\e[0m"
	# env GOOS=openbsd GOARCH=arm64 go build -o $(mkfile_dir)termites/termite_openbsd_arm64 termite.go
	# echo -e "Building \e[32mtermite_plan9_386\e[0m"
	# env GOOS=plan9 GOARCH=386 go build -o $(mkfile_dir)termites/termite_plan9_386 termite.go
	# echo -e "Building \e[32mtermite_plan9_amd64\e[0m"
	# env GOOS=plan9 GOARCH=amd64 go build -o $(mkfile_dir)termites/termite_plan9_amd64 termite.go
	# echo -e "Building \e[32mtermite_plan9_arm\e[0m"
	# env GOOS=plan9 GOARCH=arm go build -o $(mkfile_dir)termites/termite_plan9_arm termite.go
	# echo -e "Building \e[32mtermite_solaris_amd64\e[0m"
	# env GOOS=solaris GOARCH=amd64 go build -o $(mkfile_dir)termites/termite_solaris_amd64 termite.go
	# echo -e "Building \e[32mtermite_windows_386\e[0m"
	# env GOOS=windows GOARCH=386 go build -o $(mkfile_dir)termites/termite_windows_386 termite.go
	# echo -e "Building \e[32mtermite_windows_amd64\e[0m"
	# env GOOS=windows GOARCH=amd64 go build -o $(mkfile_dir)termites/termite_windows_amd64 termite.go
	echo "Collecting static files"
	cd $(mkfile_dir) && go-bindata -pkg resource -o ./lib/util/resource/resource.go ./termites/... ./lib/runtime/... ./html/ttyd/dist/... ./html/frontend/build/...

dependency:
	sudo apt-get update
	sudo apt-get install -y software-properties-common gnupg
	sudo add-apt-repository -y ppa:longsleep/golang-backports
	sudo apt-key adv --keyserver keyserver.ubuntu.com --recv-keys 52B59B1571A79DBC054901C0F6BC817356A3D45E
	sudo apt install -y golang-go nodejs npm go-bindata upx
	sudo npm install -g yarn

clean:
	rm -rf build
	rm -rf termites
	rm -rf $(mkfile_dir)html/frontend/build
	rm -rf $(mkfile_dir)html/ttyd/build
