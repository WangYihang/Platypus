package update

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/creativeprojects/go-selfupdate"

	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/utils/ui"
)

const Version = "1.5.1"

func ConfirmAndSelfUpdate() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log.Info("Detecting the latest version...")
	latest, found, err := selfupdate.DetectLatest(ctx, selfupdate.ParseSlug("WangYihang/Platypus"))
	if err != nil {
		log.Error("Error occurred while detecting version: %s", err)
		return
	}
	if !found || latest.LessOrEqual(Version) {
		log.Success("Current version is the latest")
		return
	}

	if !ui.PromptYesNo(fmt.Sprintf("Do you want to update to v%s?", latest.Version())) {
		return
	}

	exe, err := os.Executable()
	if err != nil {
		log.Error("Could not locate executable path")
		return
	}
	log.Info("Downloading from %s", latest.AssetURL)
	if err := selfupdate.UpdateTo(ctx, latest.AssetURL, latest.AssetName, exe); err != nil {
		log.Error("Error occurred while updating binary: %s", err)
		return
	}
	log.Success("Successfully updated to v%s", latest.Version())
}
