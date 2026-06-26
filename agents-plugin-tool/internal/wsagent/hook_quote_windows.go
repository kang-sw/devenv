//go:build windows

package wsagent

import "strings"

// quoteHookArg quotes a single argument for embedding in a hook command string
// on Windows. It wraps the value in double quotes and escapes embedded
// double-quote characters as `\"` (backslash-double-quote), which is the
// ws-mcp / MSVCRT argv convention used when the hook string is parsed by the
// Go runtime's argv splitting. The exact shell cmd.exe uses to invoke the hook
// is confirmed empirically in Phase C; current call sites pass inputs that
// cannot contain `"` (Windows paths and sanitized agent names), so the
// double-quote wrapping is sufficient for them.
func quoteHookArg(value string) string {
	if value == "" {
		return `""`
	}
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}
