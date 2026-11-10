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

// processStartTime returns the wall-clock time the OS started pid: the
// process's start since boot (/proc/<pid>/stat) against the system's uptime
// now, both on the boot clock.
func processStartTime(pid int) (time.Time, error) {
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return time.Time{}, err
	}
	// The command name, in parentheses, may itself hold spaces and
	// parentheses; the fields after its last ")" start at the state.
	text := string(raw)
	end := strings.LastIndexByte(text, ')')
	if end < 0 {
		return time.Time{}, errors.New("unreadable /proc stat line")
	}
	fields := strings.Fields(text[end+1:])
	const startField = 22 - 3 // starttime is field 22; fields[0] is field 3
	if len(fields) <= startField {
		return time.Time{}, errors.New("short /proc stat line")
	}
	ticks, err := strconv.ParseUint(fields[startField], 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("process start ticks: %w", err)
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
