// Code generated for package resource by go-bindata DO NOT EDIT. (@generated)
// sources:
// lib/runtime/template/rsh/awk.tpl
// lib/runtime/template/rsh/bash.tpl
// lib/runtime/template/rsh/go.tpl
// lib/runtime/template/rsh/lua.tpl
// lib/runtime/template/rsh/nc.tpl
// lib/runtime/template/rsh/perl.tpl
// lib/runtime/template/rsh/php.tpl
// lib/runtime/template/rsh/python.tpl
// lib/runtime/template/rsh/ruby.tpl
package resource

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func bindataRead(data []byte, name string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("Read %q: %v", name, err)
	}

	var buf bytes.Buffer
	_, err = io.Copy(&buf, gz)
	clErr := gz.Close()

	if err != nil {
		return nil, fmt.Errorf("Read %q: %v", name, err)
	}
	if clErr != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

type asset struct {
	bytes []byte
	info  os.FileInfo
}

type bindataFileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
}

// Name return file name
func (fi bindataFileInfo) Name() string {
	return fi.name
}

// Size return file size
func (fi bindataFileInfo) Size() int64 {
	return fi.size
}

// Mode return file mode
func (fi bindataFileInfo) Mode() os.FileMode {
	return fi.mode
}

// Mode return file modify time
func (fi bindataFileInfo) ModTime() time.Time {
	return fi.modTime
}

// IsDir return file whether a directory
func (fi bindataFileInfo) IsDir() bool {
	return fi.mode&os.ModeDir != 0
}

// Sys return file is sys mode
func (fi bindataFileInfo) Sys() interface{} {
	return nil
}

var _libRuntimeTemplateRshAwkTpl = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x4c\x8d\xc1\x4a\xc4\x30\x18\x84\x5f\x65\x0c\x62\x93\x53\x8a\x78\x0b\xed\x41\x10\xf5\x62\x45\x7b\xff\x91\xf4\xaf\x0d\x1b\xd2\xb2\xc9\x6e\x17\xb2\x7d\xf7\xa5\xb4\x87\x3d\xcd\x61\xbe\xf9\xe6\x6f\x3e\xa0\x78\x7d\x7b\xff\xfc\x42\x8e\xa8\x20\xb4\x0b\x9c\x74\xb2\x93\x2e\x35\xd1\x47\xf3\xdb\x12\x69\xa2\xef\xe6\xa7\x25\x12\x06\xf3\xe0\x3c\xcb\x97\x67\x85\x8c\x6e\xcc\x98\x8e\x2e\xa4\x1e\x22\x0e\xec\x7d\x2d\x70\x7d\x42\x34\x88\x6b\xfe\x73\xf2\x2e\x30\xac\x81\xeb\xa5\x55\x79\x1b\x43\x4a\x7b\x57\x2b\xd4\x28\xd5\xe6\xc1\x63\xb9\x0b\xac\x1f\x23\x4b\xab\x0c\x16\x2c\xfb\xa9\xc5\x43\x05\xc1\x17\x97\x84\xda\x81\xb8\x02\x4b\x01\xdd\xf1\x59\x87\x93\xf7\xb7\x00\x00\x00\xff\xff\x70\x0b\x64\xcc\xd0\x00\x00\x00")

func libRuntimeTemplateRshAwkTplBytes() ([]byte, error) {
	return bindataRead(
		_libRuntimeTemplateRshAwkTpl,
		"lib/runtime/template/rsh/awk.tpl",
	)
}

func libRuntimeTemplateRshAwkTpl() (*asset, error) {
	bytes, err := libRuntimeTemplateRshAwkTplBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "lib/runtime/template/rsh/awk.tpl", size: 208, mode: os.FileMode(438), modTime: time.Unix(1611397827, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _libRuntimeTemplateRshBashTpl = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x4a\x4a\x2c\xce\x50\xd0\x4d\x56\x50\x87\x30\x32\x15\xec\xf4\x53\x52\xcb\xf4\x4b\x92\x0b\xf4\xe3\xe3\x3d\xfc\x83\x43\xe2\xe3\xf5\xe3\xe3\x03\xfc\x83\x42\xe2\xe3\x15\x0c\xec\xd4\x0c\xd5\x01\x01\x00\x00\xff\xff\x3b\xa0\xa2\xba\x32\x00\x00\x00")

func libRuntimeTemplateRshBashTplBytes() ([]byte, error) {
	return bindataRead(
		_libRuntimeTemplateRshBashTpl,
		"lib/runtime/template/rsh/bash.tpl",
	)
}

func libRuntimeTemplateRshBashTpl() (*asset, error) {
	bytes, err := libRuntimeTemplateRshBashTplBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "lib/runtime/template/rsh/bash.tpl", size: 50, mode: os.FileMode(438), modTime: time.Unix(1611395707, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _libRuntimeTemplateRshGoTpl = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x54\xcc\xcd\x4a\xc5\x30\x10\x05\xe0\x57\x09\xb3\xb8\x37\x85\x4b\xb2\x6f\xa9\x1b\x5d\xb8\xab\xb4\xdd\x0f\x71\x1a\xdb\xa0\x99\x84\x74\x02\x82\xf8\xee\xe2\x4f\x29\xee\xe6\x9b\x73\x38\x9e\xb6\xa4\xae\xd9\xd1\xab\x5b\xbd\x8a\x2e\x70\x17\x62\x4e\x45\x20\xed\xd6\xbf\x7b\x82\xc3\xec\x05\xba\x97\xca\xf4\xd3\xd2\xcd\x07\xdd\xb0\xed\xd9\x8b\x79\x08\xee\x4d\x83\x50\x86\x1b\x20\x3e\x0e\xd3\x8c\xd8\x22\x3e\x0d\xe3\x8c\x08\x4d\x47\x71\x69\xfb\xef\x2d\x73\x9f\x62\x74\xbc\x68\xb0\xcf\x81\xed\xbe\xfd\x86\x66\x92\x25\x70\x4f\xc7\x9d\xaa\x9c\xf0\xa5\xfc\x61\xac\xac\x9b\xcf\xab\xba\x53\x56\x62\xb6\x62\xd6\xa4\x2e\x17\xb5\x26\x55\x2a\xff\xff\x95\x78\xfa\x2b\x00\x00\xff\xff\xb9\x2e\x88\x0d\xe2\x00\x00\x00")

func libRuntimeTemplateRshGoTplBytes() ([]byte, error) {
	return bindataRead(
		_libRuntimeTemplateRshGoTpl,
		"lib/runtime/template/rsh/go.tpl",
	)
}

func libRuntimeTemplateRshGoTpl() (*asset, error) {
	bytes, err := libRuntimeTemplateRshGoTplBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "lib/runtime/template/rsh/go.tpl", size: 226, mode: os.FileMode(438), modTime: time.Unix(1611397767, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _libRuntimeTemplateRshLuaTpl = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x3c\xc9\xc1\x0a\xc2\x30\x0c\x80\xe1\x57\x09\x3b\x34\x2d\xb8\x0d\xec\xcd\xe8\xce\xde\x26\xea\x3d\x68\x08\x58\x94\x56\xd7\x14\x7c\x7c\x11\xc1\xdb\xcf\xff\x3d\xda\x05\x7a\x85\x6e\xd1\x57\x4b\x8b\x7a\xac\x45\xee\x6a\x18\xe8\x7f\x4a\xc5\x40\xb6\xfb\xc1\x60\xf2\xf4\x81\x6c\x23\x25\x67\x15\xf3\xc8\xbc\x9f\x4f\x67\x66\x5c\x21\xf3\x61\x3e\x7e\x33\x50\xa9\x83\xbe\x55\x9a\xa9\xc7\xf1\x9a\xf2\x58\x6f\xd0\x27\xd8\xba\x08\x93\x8b\xb0\x9e\x5c\xc4\x40\xdd\x27\x00\x00\xff\xff\xed\x9c\x93\x1a\x7f\x00\x00\x00")

func libRuntimeTemplateRshLuaTplBytes() ([]byte, error) {
	return bindataRead(
		_libRuntimeTemplateRshLuaTpl,
		"lib/runtime/template/rsh/lua.tpl",
	)
}

func libRuntimeTemplateRshLuaTpl() (*asset, error) {
	bytes, err := libRuntimeTemplateRshLuaTplBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "lib/runtime/template/rsh/lua.tpl", size: 127, mode: os.FileMode(438), modTime: time.Unix(1611397870, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _libRuntimeTemplateRshNcTpl = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\xca\x4b\x56\xd0\x4d\x56\xd0\x4f\xca\xcc\xd3\x4f\x4a\x2c\xce\x50\x88\x8f\xf7\xf0\x0f\x0e\x89\x8f\x57\x88\x8f\x0f\xf0\x0f\x0a\x89\x8f\x07\x04\x00\x00\xff\xff\x2d\xeb\xb9\xff\x21\x00\x00\x00")

func libRuntimeTemplateRshNcTplBytes() ([]byte, error) {
	return bindataRead(
		_libRuntimeTemplateRshNcTpl,
		"lib/runtime/template/rsh/nc.tpl",
	)
}

func libRuntimeTemplateRshNcTpl() (*asset, error) {
	bytes, err := libRuntimeTemplateRshNcTplBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "lib/runtime/template/rsh/nc.tpl", size: 33, mode: os.FileMode(438), modTime: time.Unix(1611397793, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _libRuntimeTemplateRshPerlTpl = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x5c\xcc\xd1\x4a\x03\x31\x10\x85\xe1\x57\x09\x43\xb0\x19\x48\xe9\x03\x84\x0a\xa2\x2b\x16\xb1\x29\x49\xbc\x1e\xb6\xe9\xa8\x41\x9d\x84\xdd\x08\x8a\xf8\xee\x52\x61\x6f\xbc\xfd\x38\xe7\x6f\x3c\xbd\xa9\x35\xab\xd5\xc7\xcc\x2a\xd6\xfc\xca\xdd\xe9\xb2\x05\xa2\x3b\x1f\x13\x11\x38\xdd\xb6\x44\x07\x1f\x12\x91\x9b\xff\x06\x26\xda\xc3\x2d\xed\xf6\x43\xb2\xd1\x5f\xdf\x53\x4c\x61\xb8\x7a\xb0\xcf\xdc\xdb\x54\x7b\x3d\x7e\xc9\xf8\xce\x06\x7a\x6e\x80\xe8\xca\x93\xc9\x55\x84\xf3\xf9\x77\x0e\x8c\xa7\xd3\x44\x45\x8c\x6e\xb6\x08\x77\x1a\x7b\x15\xa3\x0b\x22\xe2\x77\x6d\x2c\x26\xa6\x9b\xdd\xde\xc2\xe5\x45\x04\x74\x8b\xf8\xc7\xf4\x9f\x86\x10\x16\xe2\x4f\xce\x06\x36\xc7\x22\x9b\xf9\x45\xad\x0b\xa0\xfb\x71\xab\xdf\x00\x00\x00\xff\xff\xde\xba\xf1\xb3\xdd\x00\x00\x00")

func libRuntimeTemplateRshPerlTplBytes() ([]byte, error) {
	return bindataRead(
		_libRuntimeTemplateRshPerlTpl,
		"lib/runtime/template/rsh/perl.tpl",
	)
}

func libRuntimeTemplateRshPerlTpl() (*asset, error) {
	bytes, err := libRuntimeTemplateRshPerlTplBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "lib/runtime/template/rsh/perl.tpl", size: 221, mode: os.FileMode(438), modTime: time.Unix(1611397683, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _libRuntimeTemplateRshPhpTpl = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x2a\xc8\x28\x50\xd0\x2d\x52\x50\x57\x29\xce\x4f\xce\xb6\x4d\x03\x91\xf9\x05\xa9\x79\x1a\x4a\xf1\xf1\x1e\xfe\xc1\x21\xf1\xf1\x4a\x3a\xf1\xf1\x01\xfe\x41\x21\xf1\xf1\x9a\xd6\xc5\x19\xa9\x39\x39\xf1\xa9\x15\xa9\xc9\x1a\x4a\xfa\x49\x99\x79\xfa\xc5\x19\x0a\xba\x99\x0a\x36\x6a\xc6\x0a\x76\x6a\xc6\x0a\x46\x76\x6a\xc6\x4a\x9a\xd6\xea\x80\x00\x00\x00\xff\xff\x3d\xfd\xbe\xc3\x54\x00\x00\x00")

func libRuntimeTemplateRshPhpTplBytes() ([]byte, error) {
	return bindataRead(
		_libRuntimeTemplateRshPhpTpl,
		"lib/runtime/template/rsh/php.tpl",
	)
}

func libRuntimeTemplateRshPhpTpl() (*asset, error) {
	bytes, err := libRuntimeTemplateRshPhpTplBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "lib/runtime/template/rsh/php.tpl", size: 84, mode: os.FileMode(438), modTime: time.Unix(1611397718, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _libRuntimeTemplateRshPythonTpl = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x6c\x8e\x41\x8b\x83\x30\x14\x84\xff\x4a\xc8\xc5\x04\xb2\x71\xd7\x6b\xd8\x83\x2c\x2e\xbb\x94\xd6\xa2\xb9\x3f\x34\x4d\x51\xda\xe6\x05\x5f\xa4\xf8\xef\x4b\xd1\xa3\x97\x19\xbe\x8f\x39\x4c\x5c\xd2\x80\x81\x7d\x38\x96\x8d\x8f\x88\x53\x62\x84\xee\xe6\x93\xa2\xb9\x8f\x13\x3a\x4f\xa4\x90\x0c\x7d\xaf\x5a\xaf\x25\x36\x2a\x7f\xe1\xff\x54\x59\xb5\x61\x5b\xff\x1c\xa0\xb5\x4d\x55\x1e\xa5\x21\xed\x30\x04\xef\x92\x10\x1c\xe0\xaf\x6e\x2d\x00\x57\x00\xe7\xba\xb1\x00\x52\x1a\x24\x7d\x99\x63\x21\x48\x5f\xc7\xbb\x0f\x28\xa4\xfa\x94\x86\xed\xe8\xaf\xdd\x71\x21\xcd\x76\x39\xa6\xc5\xbc\x43\x53\xec\x9e\x41\xf0\xbc\x1f\x43\xde\x77\x34\x70\x99\xbd\x02\x00\x00\xff\xff\x8a\xc1\xbf\x5b\xe1\x00\x00\x00")

func libRuntimeTemplateRshPythonTplBytes() ([]byte, error) {
	return bindataRead(
		_libRuntimeTemplateRshPythonTpl,
		"lib/runtime/template/rsh/python.tpl",
	)
}

func libRuntimeTemplateRshPythonTpl() (*asset, error) {
	bytes, err := libRuntimeTemplateRshPythonTplBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "lib/runtime/template/rsh/python.tpl", size: 225, mode: os.FileMode(438), modTime: time.Unix(1611395611, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _libRuntimeTemplateRshRubyTpl = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x2a\x2a\x4d\xaa\x54\xd0\x2d\x2a\xce\x4f\xce\x4e\x2d\x51\xd0\x4d\x55\x4f\xb3\x0d\x71\x0e\x08\x06\x73\xf5\xf2\x0b\x52\xf3\x34\x94\xe2\xe3\x3d\xfc\x83\x43\xe2\xe3\x95\x74\xe2\xe3\x03\xfc\x83\x42\xe2\xe3\x35\xf5\x4a\xf2\xe3\x33\xad\x53\x2b\x52\x93\x15\x8a\x0b\x8a\x32\xf3\x4a\xd2\x34\x94\xf4\x93\x32\xf3\xf4\x8b\x33\x14\x74\x33\x15\x6c\xd4\x54\x53\x14\xec\x40\x84\x11\x88\x54\xd2\x49\x03\x41\x4d\x75\x40\x00\x00\x00\xff\xff\xe1\xb5\x6f\x8b\x6d\x00\x00\x00")

func libRuntimeTemplateRshRubyTplBytes() ([]byte, error) {
	return bindataRead(
		_libRuntimeTemplateRshRubyTpl,
		"lib/runtime/template/rsh/ruby.tpl",
	)
}

func libRuntimeTemplateRshRubyTpl() (*asset, error) {
	bytes, err := libRuntimeTemplateRshRubyTplBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "lib/runtime/template/rsh/ruby.tpl", size: 109, mode: os.FileMode(438), modTime: time.Unix(1611397743, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

// Asset loads and returns the asset for the given name.
// It returns an error if the asset could not be found or
// could not be loaded.
func Asset(name string) ([]byte, error) {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	if f, ok := _bindata[cannonicalName]; ok {
		a, err := f()
		if err != nil {
			return nil, fmt.Errorf("Asset %s can't read by error: %v", name, err)
		}
		return a.bytes, nil
	}
	return nil, fmt.Errorf("Asset %s not found", name)
}

// MustAsset is like Asset but panics when Asset would return an error.
// It simplifies safe initialization of global variables.
func MustAsset(name string) []byte {
	a, err := Asset(name)
	if err != nil {
		panic("asset: Asset(" + name + "): " + err.Error())
	}

	return a
}

// AssetInfo loads and returns the asset info for the given name.
// It returns an error if the asset could not be found or
// could not be loaded.
func AssetInfo(name string) (os.FileInfo, error) {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	if f, ok := _bindata[cannonicalName]; ok {
		a, err := f()
		if err != nil {
			return nil, fmt.Errorf("AssetInfo %s can't read by error: %v", name, err)
		}
		return a.info, nil
	}
	return nil, fmt.Errorf("AssetInfo %s not found", name)
}

// AssetNames returns the names of the assets.
func AssetNames() []string {
	names := make([]string, 0, len(_bindata))
	for name := range _bindata {
		names = append(names, name)
	}
	return names
}

// _bindata is a table, holding each asset generator, mapped to its name.
var _bindata = map[string]func() (*asset, error){
	"lib/runtime/template/rsh/awk.tpl":    libRuntimeTemplateRshAwkTpl,
	"lib/runtime/template/rsh/bash.tpl":   libRuntimeTemplateRshBashTpl,
	"lib/runtime/template/rsh/go.tpl":     libRuntimeTemplateRshGoTpl,
	"lib/runtime/template/rsh/lua.tpl":    libRuntimeTemplateRshLuaTpl,
	"lib/runtime/template/rsh/nc.tpl":     libRuntimeTemplateRshNcTpl,
	"lib/runtime/template/rsh/perl.tpl":   libRuntimeTemplateRshPerlTpl,
	"lib/runtime/template/rsh/php.tpl":    libRuntimeTemplateRshPhpTpl,
	"lib/runtime/template/rsh/python.tpl": libRuntimeTemplateRshPythonTpl,
	"lib/runtime/template/rsh/ruby.tpl":   libRuntimeTemplateRshRubyTpl,
}

// AssetDir returns the file names below a certain
// directory embedded in the file by go-bindata.
// For example if you run go-bindata on data/... and data contains the
// following hierarchy:
//     data/
//       foo.txt
//       img/
//         a.png
//         b.png
// then AssetDir("data") would return []string{"foo.txt", "img"}
// AssetDir("data/img") would return []string{"a.png", "b.png"}
// AssetDir("foo.txt") and AssetDir("notexist") would return an error
// AssetDir("") will return []string{"data"}.
func AssetDir(name string) ([]string, error) {
	node := _bintree
	if len(name) != 0 {
		cannonicalName := strings.Replace(name, "\\", "/", -1)
		pathList := strings.Split(cannonicalName, "/")
		for _, p := range pathList {
			node = node.Children[p]
			if node == nil {
				return nil, fmt.Errorf("Asset %s not found", name)
			}
		}
	}
	if node.Func != nil {
		return nil, fmt.Errorf("Asset %s not found", name)
	}
	rv := make([]string, 0, len(node.Children))
	for childName := range node.Children {
		rv = append(rv, childName)
	}
	return rv, nil
}

type bintree struct {
	Func     func() (*asset, error)
	Children map[string]*bintree
}

var _bintree = &bintree{nil, map[string]*bintree{
	"lib": &bintree{nil, map[string]*bintree{
		"runtime": &bintree{nil, map[string]*bintree{
			"template": &bintree{nil, map[string]*bintree{
				"rsh": &bintree{nil, map[string]*bintree{
					"awk.tpl":    &bintree{libRuntimeTemplateRshAwkTpl, map[string]*bintree{}},
					"bash.tpl":   &bintree{libRuntimeTemplateRshBashTpl, map[string]*bintree{}},
					"go.tpl":     &bintree{libRuntimeTemplateRshGoTpl, map[string]*bintree{}},
					"lua.tpl":    &bintree{libRuntimeTemplateRshLuaTpl, map[string]*bintree{}},
					"nc.tpl":     &bintree{libRuntimeTemplateRshNcTpl, map[string]*bintree{}},
					"perl.tpl":   &bintree{libRuntimeTemplateRshPerlTpl, map[string]*bintree{}},
					"php.tpl":    &bintree{libRuntimeTemplateRshPhpTpl, map[string]*bintree{}},
					"python.tpl": &bintree{libRuntimeTemplateRshPythonTpl, map[string]*bintree{}},
					"ruby.tpl":   &bintree{libRuntimeTemplateRshRubyTpl, map[string]*bintree{}},
				}},
			}},
		}},
	}},
}}

// RestoreAsset restores an asset under the given directory
func RestoreAsset(dir, name string) error {
	data, err := Asset(name)
	if err != nil {
		return err
	}
	info, err := AssetInfo(name)
	if err != nil {
		return err
	}
	err = os.MkdirAll(_filePath(dir, filepath.Dir(name)), os.FileMode(0755))
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(_filePath(dir, name), data, info.Mode())
	if err != nil {
		return err
	}
	err = os.Chtimes(_filePath(dir, name), info.ModTime(), info.ModTime())
	if err != nil {
		return err
	}
	return nil
}

// RestoreAssets restores an asset under the given directory recursively
func RestoreAssets(dir, name string) error {
	children, err := AssetDir(name)
	// File
	if err != nil {
		return RestoreAsset(dir, name)
	}
	// Dir
	for _, child := range children {
		err = RestoreAssets(dir, filepath.Join(name, child))
		if err != nil {
			return err
		}
	}
	return nil
}

func _filePath(dir, name string) string {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	return filepath.Join(append([]string{dir}, strings.Split(cannonicalName, "/")...)...)
}
