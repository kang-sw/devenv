//go:build darwin

package wsholder

import (
	"time"

	"golang.org/x/sys/unix"
)

// processStartTime returns the wall-clock time the OS started pid.
func processStartTime(pid int) (time.Time, error) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return time.Time{}, err
	}
	tv := kp.Proc.P_starttime
	return time.Unix(int64(tv.Sec), int64(tv.Usec)*1000), nil
}
