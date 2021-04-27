package compiler

import (
	"os/exec"

	"github.com/WangYihang/Platypus/lib/util/log"
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
