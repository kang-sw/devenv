//go:build windows

package wsagent

import "strings"

// quoteHookArg quotes a single argument for embedding in a hook command string
// on Windows. It uses cmd.exe-correct double-quote wrapping: the value is
// wrapped in double quotes and any embedded double-quote characters are escaped
// as `\"` (backslash-double-quote), which is the Windows command-line
// convention. On Windows, POSIX single-quote quoting is meaningless to cmd.exe.
func quoteHookArg(value string) string {
	if value == "" {
		return `""`
	}
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}
