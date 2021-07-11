package update

import (
	"fmt"
	"os"

	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/WangYihang/Platypus/lib/util/ui"
	"github.com/blang/semver"
	"github.com/rhysd/go-github-selfupdate/selfupdate"
)

const Version = "1.4.3"

func ConfirmAndSelfUpdate() {
	log.Info("Detecting the latest version...")
	latest, found, err := selfupdate.DetectLatest("WangYihang/Platypus")
	if err != nil {
		log.Error("Error occurred while detecting version: %s", err)
		return
	}

	v := semver.MustParse(Version)
	if !found || latest.Version.LTE(v) {
		log.Success("Current version is the latest")
		return
	}

	if !ui.PromptYesNo(fmt.Sprintf("Do you want to update to v%s?", latest.Version)) {
		return
	}

	exe, err := os.Executable()
	if err != nil {
		log.Error("Could not locate executable path")
		return
	}
	log.Info("Downloading from %s", latest.AssetURL)
	if err := selfupdate.UpdateTo(latest.AssetURL, exe); err != nil {
		log.Error("Error occurred while updating binary: %s", err)
		return
	}
	log.Success("Successfully updated to v%s", latest.Version)
}
