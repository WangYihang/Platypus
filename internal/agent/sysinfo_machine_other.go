//go:build !darwin

package agent

// applyDarwinChassisFallback is a no-op on non-macOS platforms.
// The main sysinfo_machine.go guards its call with runtime.GOOS
// already; this stub just keeps the compile units happy.
func applyDarwinChassisFallback(_ *machineSnapshot) {}
