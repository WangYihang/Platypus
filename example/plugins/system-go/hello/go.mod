module github.com/WangYihang/Platypus/example/plugins/system-go/hello

go 1.24

require (
	github.com/WangYihang/Platypus/sdk/go/platypus-plugin v0.0.0
	github.com/extism/go-pdk v1.1.3
)

replace github.com/WangYihang/Platypus/sdk/go/platypus-plugin => ../../../../sdk/go/platypus-plugin
