//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package wsagent

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

type unixProcess struct {
	PID  int
	PPID int
	PGID int
}

func cancelAsyncProcessTree(pid int) error {
	if pid <= 0 {
		return nil
	}
	processes, listErr := unixProcessTree(pid)
	groupIDs := map[int]bool{pid: true}
	processIDs := map[int]bool{pid: true}
	for _, process := range processes {
		processIDs[process.PID] = true
		if process.PGID > 0 {
			groupIDs[process.PGID] = true
		}
	}

	currentGroup := syscall.Getpgrp()
	var firstErr error
	for pgid := range groupIDs {
		if pgid <= 0 || pgid == currentGroup {
			continue
		}
		if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil && !isNoSuchProcess(err) && firstErr == nil {
			firstErr = err
		}
	}
	for processID := range processIDs {
		if processID <= 0 || processID == os.Getpid() {
			continue
		}
		if err := syscall.Kill(processID, syscall.SIGKILL); err != nil && !isNoSuchProcess(err) && firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return firstErr
	}
	return listErr
}

func unixProcessTree(rootPID int) ([]unixProcess, error) {
	out, err := exec.Command("ps", "-axo", "pid=,ppid=,pgid=").Output()
	if err != nil {
		return nil, err
	}
	all := parseUnixProcessTable(string(out))
	children := make(map[int][]unixProcess)
	for _, process := range all {
		children[process.PPID] = append(children[process.PPID], process)
	}
	var result []unixProcess
	var walk func(int)
	walk = func(parent int) {
		for _, child := range children[parent] {
			result = append(result, child)
			walk(child.PID)
		}
	}
	walk(rootPID)
	return result, nil
}

func parseUnixProcessTable(raw string) []unixProcess {
	var processes []unixProcess
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, pidErr := strconv.Atoi(fields[0])
		ppid, ppidErr := strconv.Atoi(fields[1])
		pgid, pgidErr := strconv.Atoi(fields[2])
		if pidErr != nil || ppidErr != nil || pgidErr != nil {
			continue
		}
		processes = append(processes, unixProcess{PID: pid, PPID: ppid, PGID: pgid})
	}
	return processes
}

func isNoSuchProcess(err error) bool {
	return err == syscall.ESRCH
}
