//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package execjob

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func configureCommand(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

func cancelProcess(pid int) error {
	if pid <= 0 {
		return nil
	}
	processes, listErr := processTree(pid)
	groupIDs := map[int]bool{pid: true}
	processIDs := map[int]bool{pid: true}
	for _, p := range processes {
		processIDs[p.pid] = true
		if p.pgid > 0 {
			groupIDs[p.pgid] = true
		}
	}
	current := syscall.Getpgrp()
	var first error
	for pgid := range groupIDs {
		if pgid > 0 && pgid != current {
			if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil && err != syscall.ESRCH && first == nil {
				first = err
			}
		}
	}
	for p := range processIDs {
		if p > 0 && p != os.Getpid() {
			if err := syscall.Kill(p, syscall.SIGKILL); err != nil && err != syscall.ESRCH && first == nil {
				first = err
			}
		}
	}
	if first != nil {
		return first
	}
	return listErr
}

type procInfo struct{ pid, ppid, pgid int }

func processTree(root int) ([]procInfo, error) {
	out, err := exec.Command("ps", "-axo", "pid=,ppid=,pgid=").Output()
	if err != nil {
		return nil, err
	}
	children := map[int][]procInfo{}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		pid, e1 := strconv.Atoi(f[0])
		ppid, e2 := strconv.Atoi(f[1])
		pgid, e3 := strconv.Atoi(f[2])
		if e1 == nil && e2 == nil && e3 == nil {
			children[ppid] = append(children[ppid], procInfo{pid, ppid, pgid})
		}
	}
	var res []procInfo
	var walk func(int)
	walk = func(pp int) {
		for _, c := range children[pp] {
			res = append(res, c)
			walk(c.pid)
		}
	}
	walk(root)
	return res, nil
}
