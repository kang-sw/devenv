//go:build linux

package wsholder

import (
	"os"
	"strconv"
	"strings"
)

// A zombie cannot answer requests or hold a child, even if kill(pid, 0)
// succeeds until its parent reaps it. The comm field may contain parentheses.
func processZombie(pid int) bool {
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	return zombieStat(string(raw))
}

func zombieStat(stat string) bool {
	end := strings.LastIndexByte(stat, ')')
	if end < 0 {
		return false
	}
	fields := strings.Fields(stat[end+1:])
	return len(fields) > 0 && fields[0] == "Z"
}
