//go:build !windows

package wsagent

// quoteHookArg quotes a single argument for embedding in a shell hook command
// string on POSIX platforms. It uses POSIX single-quote quoting so that any
// argument value (including backslashes, spaces, dollar signs, and glob chars)
// is passed literally to the shell.
func quoteHookArg(value string) string {
	return shellQuote(value)
}
