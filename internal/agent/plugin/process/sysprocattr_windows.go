//go:build windows

package process

import "syscall"

// sysProcAttr is a stub on Windows; we don't have process groups
// here. CreateNewProcessGroup achieves a similar isolation but is
// not needed for the (rare) Windows agent build path; revisit when
// process plugins land on Windows.
func sysProcAttr() *syscall.SysProcAttr {
	return nil
}
