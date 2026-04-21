package compiler

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/utils/str"
)

// connectBackPlaceholder is the 255-byte slot baked into the prebuilt agent
// binary. Rewriting it in-place lets a single compiled binary address any
// connect-back target without recompiling.
const connectBackPlaceholder = "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx.xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx.xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx.xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx:xxxxx"

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

// BuildTermiteFromPrebuildAssets takes a prebuilt agent binary and patches
// the connect-back-address placeholder in place. The name is kept for the
// embed-time contract — the file on disk under build/termite/ is still the
// canonical source for the asset.
func BuildTermiteFromPrebuildAssets(targetFilename string, targetAddress string) error {
	assetFilepath := "build/termite/termite_linux_amd64"
	content, err := os.ReadFile(assetFilepath)
	if err != nil {
		log.Error("Failed to read asset file: %s", assetFilepath)
		return err
	}

	replacement := make([]byte, len(connectBackPlaceholder))
	for i := 0; i < len(connectBackPlaceholder); i++ {
		replacement[i] = 0x20
	}
	copy(replacement, targetAddress)

	log.Success("Replacing placeholder with: `%s`", strings.TrimSpace(string(replacement)))
	content = bytes.Replace(content, []byte(connectBackPlaceholder), replacement, 1)

	if err := os.WriteFile(targetFilename, content, 0755); err != nil {
		log.Error("Failed to write file: %s", targetFilename)
		return err
	}
	return nil
}

func GenerateDirFilename() (string, string, error) {
	dir, err := os.MkdirTemp("", str.RandomString(0x08))
	if err != nil {
		return "", "", err
	}
	var filename string
	if runtime.GOOS == "windows" {
		filename = fmt.Sprintf("%d-%s-agent.exe", time.Now().UnixNano(), str.RandomString(0x10))
	} else {
		filename = fmt.Sprintf("%d-%s-agent", time.Now().UnixNano(), str.RandomString(0x10))
	}
	return dir, filepath.Join(dir, filename), nil
}
