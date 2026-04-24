//go:build darwin

package agent

import "golang.org/x/sys/unix"

// applyDarwinChassisFallback reads `hw.model` via sysctl and hands
// the result to applyAppleModelHeuristic. Split into a darwin-only
// file so the x/sys/unix import doesn't break Windows compilation.
func applyDarwinChassisFallback(s *machineSnapshot) {
	model, err := unix.Sysctl("hw.model")
	if err != nil {
		return
	}
	applyAppleModelHeuristic(s, model)
}
