// +build linux

package manager

import (
	"syscall"

	"github.com/Sirupsen/logrus"
)

func init() {
	// Shutdown when parent dies
	if _, _, err := syscall.RawSyscall(syscall.SYS_PRCTL, syscall.PR_SET_PDEATHSIG, uintptr(syscall.SIGTERM), 0); err != 0 {
		logrus.Fatal("Failed to set parent death sinal, err")
	}
}
