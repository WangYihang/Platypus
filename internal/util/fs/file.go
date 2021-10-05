package fs

import (
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/WangYihang/Platypus/internal/util/assets"
	assetfs "github.com/elazarl/go-bindata-assetfs"
)

func ListFiles(path string) []string {
	names := make([]string, 0)
	files, _ := ioutil.ReadDir(path)
	for _, f := range files {
		names = append(names, f.Name())
	}
	return names
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
		Asset:     assets.Asset,
		AssetDir:  assets.AssetDir,
		AssetInfo: assets.AssetInfo,
		Prefix:    root,
		Fallback:  "",
	}
	return &binaryFileSystem{
		fs,
	}
}

func AppendFile(path string, content []byte) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		panic(err)
	}

	defer f.Close()

	if _, err = f.Write(content); err != nil {
		panic(err)
	}
}
