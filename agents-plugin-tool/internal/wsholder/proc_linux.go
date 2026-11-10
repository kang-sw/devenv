//go:build linux

package wsholder

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// userHZ is the clock tick /proc reports process times in; it is 100 on
// every Linux architecture Go supports.
const userHZ = 100

// processStartTicks returns pid's start on the boot clock, in clock ticks
// since boot (/proc/<pid>/stat). The value never changes while the process
// lives, so it identifies the process exactly whatever the wall clock does.
func processStartTicks(pid int) (uint64, error) {
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, err
	}
	// The command name, in parentheses, may itself hold spaces and
	// parentheses; the fields after its last ")" start at the state.
	text := string(raw)
	end := strings.LastIndexByte(text, ')')
	if end < 0 {
		return 0, errors.New("unreadable /proc stat line")
	}
	fields := strings.Fields(text[end+1:])
	const startField = 22 - 3 // starttime is field 22; fields[0] is field 3
	if len(fields) <= startField {
		return 0, errors.New("short /proc stat line")
	}
	ticks, err := strconv.ParseUint(fields[startField], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("process start ticks: %w", err)
	}
	return ticks, nil
}

// processStartTime returns the wall-clock time the OS started pid: its start
// ticks against the system's uptime now, both on the boot clock. The result
// moves with every later step of the wall clock, which is why a holder that
// recorded its start ticks is matched on those instead.
func processStartTime(pid int) (time.Time, error) {
	ticks, err := processStartTicks(pid)
	if err != nil {
		return time.Time{}, err
	}
	now := time.Now()
	up, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return time.Time{}, err
	}
	upFields := strings.Fields(string(up))
	if len(upFields) == 0 {
		return time.Time{}, errors.New("unreadable /proc/uptime")
	}
	uptime, err := strconv.ParseFloat(upFields[0], 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("system uptime: %w", err)
	}
	age := uptime - float64(ticks)/userHZ
	return now.Add(-time.Duration(age * float64(time.Second))), nil
}
