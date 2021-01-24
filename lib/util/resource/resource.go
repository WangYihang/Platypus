// Code generated for package resource by go-bindata DO NOT EDIT. (@generated)
// sources:
// lib/runtime/config.example.yml
// lib/runtime/template/rsh/bash.tpl
// lib/runtime/template/rsh/go.tpl
// lib/runtime/template/rsh/lua.tpl
// lib/runtime/template/rsh/nc.tpl
// lib/runtime/template/rsh/perl.tpl
// lib/runtime/template/rsh/php.tpl
// lib/runtime/template/rsh/python.tpl
// lib/runtime/template/rsh/python2.tpl
// lib/runtime/template/rsh/python3.tpl
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

var _libRuntimeConfigExampleYml = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x2a\x4e\x2d\x2a\x4b\x2d\x2a\xb6\x52\xe0\x52\xd0\x55\xc8\xc8\x2f\x2e\xb1\x52\x50\x32\xd0\x03\x43\x25\x2e\x05\x05\x85\x82\xfc\xa2\x12\x2b\x05\x43\x63\x63\x73\xbc\x0a\xcc\x8d\x8d\x0d\x01\x01\x00\x00\xff\xff\x5a\x4d\x12\xa6\x4b\x00\x00\x00")

func libRuntimeConfigExampleYmlBytes() ([]byte, error) {
	return bindataRead(
		_libRuntimeConfigExampleYml,
		"lib/runtime/config.example.yml",
	)
}

func libRuntimeConfigExampleYml() (*asset, error) {
	bytes, err := libRuntimeConfigExampleYmlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "lib/runtime/config.example.yml", size: 75, mode: os.FileMode(436), modTime: time.Unix(1611493663, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _libRuntimeTemplateRshBashTpl = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\xd2\x2f\x2d\x2e\xd2\x4f\xca\xcc\xd3\xcf\xcb\xcf\x28\x2d\x50\x00\x33\x93\x12\x8b\x33\x14\x74\x93\x15\xd4\x91\x78\x99\x0a\x76\xfa\x29\xa9\x65\xfa\x25\xc9\x05\xfa\xf1\xf1\x1e\xfe\xc1\x21\xf1\xf1\xfa\xf1\xf1\x01\xfe\x41\x21\xf1\xf1\x0a\x06\x76\x6a\x86\xea\x0a\x6a\x80\x00\x00\x00\xff\xff\x4d\xee\xe4\xbf\x4d\x00\x00\x00")

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

	info := bindataFileInfo{name: "lib/runtime/template/rsh/bash.tpl", size: 77, mode: os.FileMode(436), modTime: time.Unix(1611493702, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _libRuntimeTemplateRshGoTpl = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x6c\x8e\xcd\x6a\xeb\x30\x10\x85\x5f\x65\x98\x85\x63\x43\xae\xb5\xb7\xf1\xdd\xb4\x8b\xee\x52\x92\x2c\x05\x83\x32\x56\x6d\xd1\x68\x24\xf4\x03\x2d\xa5\xef\x5e\x6a\xd2\x76\xd1\xee\xce\xf9\xbe\x61\x38\xaa\xe6\xa4\x2e\x4e\x94\x84\xb5\x46\xd8\xe2\xc5\xe4\x15\xfe\x31\xa0\xe5\x35\xc0\x2e\x1a\x7e\x36\x8b\x05\x6f\x9c\x8c\xce\xc7\x90\x8a\xc6\x90\x95\x7d\xb1\xac\xf1\x9b\x88\x2d\x1a\xc7\xa7\x2a\xbc\x5d\xb6\xdd\x1b\xef\x69\x98\xc4\x96\xfe\xde\x99\x6b\xab\xb1\x70\xd4\xb8\xd7\x48\xf4\x70\x38\x9d\x89\x06\xa2\xc7\xc3\xf1\x4c\xa4\xb1\x1b\xd9\xcf\xc3\xf4\xf9\xb2\xbf\x0b\xde\x1b\x99\x5b\x8d\xdb\x9a\xbc\xde\x74\x7f\x2a\xb3\x93\x89\xbf\x72\xa8\xe5\xa7\xd8\x94\x6e\xe5\x58\xa5\xed\xde\x77\xf0\x1f\x54\xf1\x51\xc5\xab\x29\xaf\xb1\xe6\x7e\x09\xd0\x34\xb0\x04\x48\x55\xfe\x54\xc9\xff\xc2\x08\xcd\x47\x00\x00\x00\xff\xff\x84\x02\x08\xea\x21\x01\x00\x00")

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

	info := bindataFileInfo{name: "lib/runtime/template/rsh/go.tpl", size: 289, mode: os.FileMode(436), modTime: time.Unix(1611495332, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _libRuntimeTemplateRshLuaTpl = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x4c\x8e\x31\x0b\xc2\x30\x10\x46\xff\xca\xd1\x21\x69\xc0\x36\x43\x37\xa3\xce\x6e\x15\x75\x14\x42\x7b\x1c\x34\x28\x49\x4d\x2e\xe0\xcf\x97\x36\x1d\xba\x3d\x1e\x8f\x8f\x4f\xe7\x14\xf5\xe8\xbc\xf6\x61\xca\x33\xac\x38\x0e\x69\x82\x06\x41\x7e\xf2\x00\x0d\x41\x15\xe9\x9b\x5d\xa4\x5a\xbe\xa4\x4c\x01\xdf\xc4\x0b\x29\xb3\xf7\x21\x15\xc7\xe7\x52\xb4\x8c\x73\xad\x0c\x1f\x31\x78\x4f\xc8\x6b\x64\xed\xb5\x7f\x3c\xad\x5d\xf8\x50\xc4\xad\xbf\x6f\x42\x99\x90\x5a\xfa\x11\x66\x2e\x93\xbb\x33\x0e\x4e\xa2\x83\x8b\xe8\x4a\x58\x49\x10\xff\x00\x00\x00\xff\xff\x6d\x29\x74\x9a\xba\x00\x00\x00")

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

	info := bindataFileInfo{name: "lib/runtime/template/rsh/lua.tpl", size: 186, mode: os.FileMode(436), modTime: time.Unix(1611505400, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _libRuntimeTemplateRshNcTpl = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\xd2\x2f\x2d\x2e\xd2\x4f\xca\xcc\xd3\xcf\xcb\xcf\x28\x2d\x50\x00\x33\x93\x12\x8b\x33\x14\x74\x93\x15\x94\x72\xb3\xd3\x32\xd3\xf2\x15\xf4\x4b\x72\x0b\xf4\xf5\x0a\x72\x12\x4b\x2a\x0b\x4a\x8b\xad\xf3\x92\x15\xe2\xe3\x3d\xfc\x83\x43\xe2\xe3\x15\xe2\xe3\x03\xfc\x83\x40\x0c\x03\x1b\x54\x55\x0a\x35\x48\x66\xd5\x28\x94\xa4\xa6\xa2\x19\xa3\xa4\xa0\x06\x08\x00\x00\xff\xff\xf9\x05\xd7\xf0\x7c\x00\x00\x00")

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

	info := bindataFileInfo{name: "lib/runtime/template/rsh/nc.tpl", size: 124, mode: os.FileMode(436), modTime: time.Unix(1611502634, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _libRuntimeTemplateRshPerlTpl = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x4c\x8f\xe1\x4a\xc3\x30\x14\x85\x5f\x25\x84\xb2\xe4\x42\x46\x1f\x20\x4c\x10\x9d\x38\xc4\x75\x2c\xf1\x9f\x70\x69\xb3\xab\x0b\xba\x9b\xd0\xa4\x3f\x86\xf8\xee\xd2\x41\xc1\x7f\x87\x8f\x73\x0e\x7c\xed\x54\xc6\x76\x88\xdc\x72\x3a\x4f\x59\xdc\xe2\xd0\x97\xb3\x58\x07\xa1\x32\x8d\xdf\x62\x4d\x42\xbd\x2b\x35\x15\x12\x2e\x85\x2f\xaa\xb6\x89\x1b\x89\xf8\xdc\x39\x8f\x28\x6d\x93\x37\x88\x87\xee\xe8\x11\x6d\xb9\x15\xb4\x33\x87\x27\xdc\xed\xb7\xde\xb8\xee\xe1\x05\x9d\x3f\x6e\xef\x5f\xcd\x27\xd5\x3c\xa6\x9a\x86\x2b\xf7\x17\xd2\xb2\x86\x2c\x01\x6c\xfc\xd0\x21\x31\x53\x98\x77\xf3\x41\x7f\x3a\x8d\x18\x59\x37\xd9\x44\xa6\x8a\x7d\x4d\xac\x9b\x08\x00\xf0\x93\x32\xb1\x76\xfe\x71\xb7\x37\xf2\x6e\xe5\x24\xd8\x85\x74\x6f\x7e\x41\xe5\x5a\x2a\x5d\xb4\xfc\x27\x13\x25\xd8\x5f\x3b\x7b\x28\xb1\xfa\x0b\x00\x00\xff\xff\x94\x97\xbe\x47\xf4\x00\x00\x00")

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

	info := bindataFileInfo{name: "lib/runtime/template/rsh/perl.tpl", size: 244, mode: os.FileMode(436), modTime: time.Unix(1611505142, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _libRuntimeTemplateRshPhpTpl = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\xd2\x2f\x2d\x2e\xd2\x4f\xca\xcc\xd3\xcf\xcb\xcf\x28\x2d\x50\x00\x33\x93\x12\x8b\x33\x14\x74\x93\x15\xd4\x0b\x32\x0a\x14\x74\x8b\x14\xd4\x63\xd4\xd5\x55\x8a\xf3\x93\xb3\x6d\xd3\x40\x64\x7e\x41\x6a\x9e\x86\x52\x7c\xbc\x87\x7f\x70\x48\x7c\xbc\x92\x4e\x7c\x7c\x80\x7f\x50\x48\x7c\xbc\xa6\x75\x71\x46\x6a\x4e\x4e\x7c\x6a\x45\x6a\xb2\x86\x12\x92\x49\x99\x0a\x36\x6a\xc6\x0a\x76\x6a\xc6\x4a\x9a\xd6\x20\xb3\xd4\x15\xd4\x00\x01\x00\x00\xff\xff\xb2\x19\x8f\xd6\x77\x00\x00\x00")

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

	info := bindataFileInfo{name: "lib/runtime/template/rsh/php.tpl", size: 119, mode: os.FileMode(436), modTime: time.Unix(1611500614, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _libRuntimeTemplateRshPythonTpl = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x6c\x8f\x41\x4b\xc4\x30\x10\x85\xff\x4a\xc8\xc1\x24\x10\x53\xf5\x1a\x3c\x14\xa9\x28\xa2\x95\x36\x47\x61\xb0\x31\xd2\xa2\xcd\x84\x4e\x72\xe8\xbf\x5f\xb6\x2d\x7b\xda\xd3\xbc\xf7\x31\x87\xf7\x55\x85\x96\x6a\x98\x62\x15\x71\x2c\x89\x6d\x71\xf8\xa6\x91\xdd\x7a\x26\xd2\x9a\x47\x8c\x5b\xfc\x12\x62\x9a\x13\x2e\x99\x11\xfa\xbf\x90\x35\x95\x21\x2d\xe8\x03\x91\x46\xb2\xf4\xb8\x63\xb3\x1f\x79\xb4\xfa\x19\x5e\x3f\x1a\xa7\x8f\xda\xb7\x4f\x6f\xd0\xbb\xae\xa9\xdf\x95\x25\xe3\x31\xc6\xe0\xb3\x94\x1c\xe0\xa5\xed\x1d\x00\xd7\x00\x9f\x6d\xe7\x00\x94\xb2\x48\xe6\xa7\xa4\x07\x49\xe6\x77\xfa\x0f\x11\xa5\xd2\x77\xca\xb2\x2b\xf8\x5e\xd9\x63\x1c\xd2\xf6\x40\x2b\xe5\x30\x4b\x7e\xd1\xe1\xea\x6c\x20\xd8\xcd\x29\x00\x00\xff\xff\x93\x93\x61\x5f\xf0\x00\x00\x00")

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

	info := bindataFileInfo{name: "lib/runtime/template/rsh/python.tpl", size: 240, mode: os.FileMode(436), modTime: time.Unix(1611500361, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _libRuntimeTemplateRshPython2Tpl = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x6c\x8f\x31\x4b\xc7\x30\x10\xc5\xbf\x4a\xc8\x60\x12\x88\xa9\x76\x0d\x0e\x45\x2a\x8a\x68\xa5\xcd\x28\x1c\x36\x46\x5a\xb4\xb9\xd0\x4b\x86\x7e\x7b\xb1\x2d\x4e\xff\xe9\xde\xfb\x71\xc3\xfb\x55\x85\xd6\x6a\x9c\x63\x15\x71\x2a\x89\xed\x71\xfc\xa0\x89\x5d\x7b\x26\xd2\x96\x27\x8c\xf5\x9e\xdf\x85\x98\x97\x84\x6b\x66\x84\xfe\x3b\x64\x4d\x65\x4c\x2b\xfa\x40\xa4\x91\x2c\xdd\x1d\xd8\x1c\x47\x9e\xad\x79\x80\xa7\xd7\xd6\xe9\xb3\x0e\xdd\xfd\x33\x0c\xae\x6f\x9b\x17\x65\xc9\x78\x8c\x31\xf8\x2c\x25\x07\x78\xec\x06\x07\xc0\x35\xc0\x5b\xd7\x3b\x00\xa5\x2c\x92\xf9\x2c\xa9\x96\x64\xbe\xe6\x9f\x10\x51\x2a\x7d\xa3\x2c\xbb\x80\x6f\x95\x3d\xc7\x21\xed\x0f\xb4\x51\x0e\x8b\xe4\xff\x3e\x5c\xfd\x19\x08\x76\xf5\x1b\x00\x00\xff\xff\xd0\x1d\xf4\x23\xf1\x00\x00\x00")

func libRuntimeTemplateRshPython2TplBytes() ([]byte, error) {
	return bindataRead(
		_libRuntimeTemplateRshPython2Tpl,
		"lib/runtime/template/rsh/python2.tpl",
	)
}

func libRuntimeTemplateRshPython2Tpl() (*asset, error) {
	bytes, err := libRuntimeTemplateRshPython2TplBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "lib/runtime/template/rsh/python2.tpl", size: 241, mode: os.FileMode(436), modTime: time.Unix(1611500371, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _libRuntimeTemplateRshPython3Tpl = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x6c\x8f\xcd\x4a\xc5\x30\x10\x85\x5f\x25\x64\x61\x12\x88\xa9\x3f\xcb\xe0\xa2\x48\x45\x11\xad\xb4\x59\x0a\x83\x8d\x91\x16\x6d\x26\x74\x92\x45\xdf\x5e\x6e\x5b\xee\xea\xae\xe6\x9c\x8f\x59\x9c\xaf\x2a\xb4\x54\xc3\x14\xab\x88\x63\x49\x6c\x8b\xc3\x17\x8d\xec\xda\x33\x91\xd6\x3c\x62\xbc\xdf\xf2\xa7\x10\xd3\x9c\x70\xc9\x8c\xd0\xff\x86\xac\xa9\x0c\x69\x41\x1f\x88\x34\x92\xa5\x87\x1d\x9b\xfd\xc8\xa3\xd5\x4f\xf0\xf2\xde\x38\x7d\xd4\xbe\x7d\x7c\x85\xde\x75\x4d\xfd\xa6\x2c\x19\x8f\x31\x06\x9f\xa5\xe4\x00\xcf\x6d\xef\x00\xb8\x06\xf8\x68\x3b\x07\xa0\x94\x45\x32\xdf\x25\xdd\x49\x32\x3f\xd3\x5f\x88\x28\x95\xbe\x51\x96\x5d\xc0\xb7\xca\x1e\xe3\x90\xb6\x07\x5a\x29\x87\x59\xf2\xb3\x0f\x57\x27\x03\xc1\xae\xfe\x03\x00\x00\xff\xff\x05\x94\x65\xf4\xf1\x00\x00\x00")

func libRuntimeTemplateRshPython3TplBytes() ([]byte, error) {
	return bindataRead(
		_libRuntimeTemplateRshPython3Tpl,
		"lib/runtime/template/rsh/python3.tpl",
	)
}

func libRuntimeTemplateRshPython3Tpl() (*asset, error) {
	bytes, err := libRuntimeTemplateRshPython3TplBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "lib/runtime/template/rsh/python3.tpl", size: 241, mode: os.FileMode(436), modTime: time.Unix(1611500377, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _libRuntimeTemplateRshRubyTpl = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\xd2\x2f\x2d\x2e\xd2\x4f\xca\xcc\xd3\xcf\xcb\xcf\x28\x2d\x50\x00\x33\x93\x12\x8b\x33\x14\x74\x93\x15\x94\x8a\x4a\x93\x2a\x15\x74\x8b\x8a\xf3\x93\xb3\x53\x4b\x14\x74\x53\x15\xd4\x53\x2b\x52\x93\x35\x62\x94\xe0\xca\x62\x94\x74\x62\x94\x74\x93\xc1\x14\x92\xde\x4c\x05\x3b\xfd\x94\xd4\x32\xfd\x92\xe4\x02\xfd\xf8\x78\x0f\xff\xe0\x90\xf8\x78\xfd\xf8\xf8\x00\xff\xa0\x90\xf8\x78\x05\x03\x3b\x35\xc3\x18\x25\x4d\x6b\x75\x25\x05\x35\x40\x00\x00\x00\xff\xff\xa0\x30\x32\x56\x80\x00\x00\x00")

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

	info := bindataFileInfo{name: "lib/runtime/template/rsh/ruby.tpl", size: 128, mode: os.FileMode(436), modTime: time.Unix(1611502052, 0)}
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
	"lib/runtime/config.example.yml":       libRuntimeConfigExampleYml,
	"lib/runtime/template/rsh/bash.tpl":    libRuntimeTemplateRshBashTpl,
	"lib/runtime/template/rsh/go.tpl":      libRuntimeTemplateRshGoTpl,
	"lib/runtime/template/rsh/lua.tpl":     libRuntimeTemplateRshLuaTpl,
	"lib/runtime/template/rsh/nc.tpl":      libRuntimeTemplateRshNcTpl,
	"lib/runtime/template/rsh/perl.tpl":    libRuntimeTemplateRshPerlTpl,
	"lib/runtime/template/rsh/php.tpl":     libRuntimeTemplateRshPhpTpl,
	"lib/runtime/template/rsh/python.tpl":  libRuntimeTemplateRshPythonTpl,
	"lib/runtime/template/rsh/python2.tpl": libRuntimeTemplateRshPython2Tpl,
	"lib/runtime/template/rsh/python3.tpl": libRuntimeTemplateRshPython3Tpl,
	"lib/runtime/template/rsh/ruby.tpl":    libRuntimeTemplateRshRubyTpl,
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
			"config.example.yml": &bintree{libRuntimeConfigExampleYml, map[string]*bintree{}},
			"template": &bintree{nil, map[string]*bintree{
				"rsh": &bintree{nil, map[string]*bintree{
					"bash.tpl":    &bintree{libRuntimeTemplateRshBashTpl, map[string]*bintree{}},
					"go.tpl":      &bintree{libRuntimeTemplateRshGoTpl, map[string]*bintree{}},
					"lua.tpl":     &bintree{libRuntimeTemplateRshLuaTpl, map[string]*bintree{}},
					"nc.tpl":      &bintree{libRuntimeTemplateRshNcTpl, map[string]*bintree{}},
					"perl.tpl":    &bintree{libRuntimeTemplateRshPerlTpl, map[string]*bintree{}},
					"php.tpl":     &bintree{libRuntimeTemplateRshPhpTpl, map[string]*bintree{}},
					"python.tpl":  &bintree{libRuntimeTemplateRshPythonTpl, map[string]*bintree{}},
					"python2.tpl": &bintree{libRuntimeTemplateRshPython2Tpl, map[string]*bintree{}},
					"python3.tpl": &bintree{libRuntimeTemplateRshPython3Tpl, map[string]*bintree{}},
					"ruby.tpl":    &bintree{libRuntimeTemplateRshRubyTpl, map[string]*bintree{}},
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
