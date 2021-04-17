package fs

import (
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/WangYihang/Platypus/lib/util/resource"
	assetfs "github.com/elazarl/go-bindata-assetfs"
)

func ListFiles(path string) func(string) []string {
	return func(line string) []string {
		names := make([]string, 0)
		files, _ := ioutil.ReadDir(path)
		for _, f := range files {
			names = append(names, f.Name())
		}
		return names
	}
}

// fileExists checks if a file exists and is not a directory before we
// try using it to prevent further errors.
func FileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

type binaryFileSystem struct {
	fs http.FileSystem
}

func (b *binaryFileSystem) Open(name string) (http.File, error) {
	return b.fs.Open(name)
}

func (b *binaryFileSystem) Exists(prefix string, filepath string) bool {

	if p := strings.TrimPrefix(filepath, prefix); len(p) < len(filepath) {
		if _, err := b.fs.Open(p); err != nil {
			return false
		}
		return true
	}
	return false
}

func BinaryFileSystem(root string) *binaryFileSystem {
	fs := &assetfs.AssetFS{
		Asset:     resource.Asset,
		AssetDir:  resource.AssetDir,
		AssetInfo: resource.AssetInfo,
		Prefix:    root,
		Fallback:  "",
	}
	return &binaryFileSystem{
		fs,
	}
}
