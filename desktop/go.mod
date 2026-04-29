module github.com/WangYihang/Platypus/desktop

go 1.25.0

// toolchain pin matches the root module so the desktop build picks
// up patch-level stdlib fixes. govulncheck flagged 16 stdlib vulns
// when this submodule was building against the bare 1.25.0 stdlib
// (crypto/x509, crypto/tls, net/url, net/http, encoding/asn1,
// encoding/pem, os — all fixed by 1.25.9). Bump in lockstep with
// root go.mod when raising the floor; the regression test in
// cmd/platypus-server/dependencies_test.go pins the floor.
toolchain go1.25.9

require (
	github.com/coder/websocket v1.8.14
	github.com/google/uuid v1.6.0
	github.com/wailsapp/wails/v2 v2.12.0
	github.com/zalando/go-keyring v0.2.8
)

require (
	git.sr.ht/~jackmordaunt/go-toast/v2 v2.0.3 // indirect
	github.com/bep/debounce v1.2.1 // indirect
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/jchv/go-winloader v0.0.0-20250406163304-c1995be93bd1 // indirect
	github.com/labstack/echo/v4 v4.15.1 // indirect
	github.com/labstack/gommon v0.5.0 // indirect
	github.com/leaanthony/go-ansi-parser v1.6.1 // indirect
	github.com/leaanthony/gosod v1.0.4 // indirect
	github.com/leaanthony/slicer v1.6.0 // indirect
	github.com/leaanthony/u v1.1.1 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.22 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/samber/lo v1.53.0 // indirect
	github.com/tkrajina/go-reflector v0.5.8 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasttemplate v1.2.2 // indirect
	github.com/wailsapp/go-webview2 v1.0.23 // indirect
	github.com/wailsapp/mimetype v1.4.1 // indirect
	golang.org/x/crypto v0.50.0 // indirect
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/text v0.36.0 // indirect
)

// replace github.com/wailsapp/wails/v2 v2.12.0 => /home/ubuntu/go/pkg/mod
