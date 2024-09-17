package version

import (
	"encoding/json"
	"os"
)

var (
	// Version is the version of the application
	Version = "0.0.1"
	// Commit is the git commit hash of the application
	Commit = "HEAD"
	// Date is the build date of the application
	Date = "1970-01-01T00:00:00Z"
)

// GetVersion returns the version information of the application
func GetVersion() (string, error) {
	info := map[string]string{
		"version": Version,
		"commit":  Commit,
		"date":    Date,
	}
	jsonData, err := json.Marshal(info)
	if err != nil {
		return "", err
	}
	return string(jsonData), nil
}

// PrintVersion prints the version information of the application
func PrintVersion() {
	versionString, _ := GetVersion()
	os.Stderr.WriteString(versionString)
	os.Exit(0)
}
