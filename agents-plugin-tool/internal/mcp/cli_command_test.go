package mcp

import "testing"

func TestMailboxCLICommandName(t *testing.T) {
	const fallback = "python3 <plugin>/bin/ws-mcp-launcher.py"
	for _, command := range []string{
		"ws-cli",
		"wsflow-cli",
		"python3 /plugin/bin/ws-mcp-launcher.py",
		"/opt/homebrew/opt/python@3.13/bin/python3 /plugin/bin/ws-mcp-launcher.py",
		"'/opt/Python @3.13/bin/python3' '/plugin cache/@release+build/bin/ws-mcp-launcher.py'",
		"python3 /plugin@release+build,1=2/bin/ws-mcp-launcher.py",
		"python3 '/plugin cache/日本語/bin/ws-mcp-launcher.py'",
		`'C:\Program Files\Python313\python.exe' 'C:\plugin cache\bin\ws-mcp-launcher.py'`,
	} {
		t.Run(command, func(t *testing.T) {
			if got := CLICommandNameFrom(func(string) string { return command }); got != command {
				t.Fatalf("CLICommandNameFrom(%q) = %q", command, got)
			}
		})
	}
	for _, command := range []string{
		"", "ws-mcp", "/runtime/ws-mcp", "python3 /runtime/ws-mcp",
		"ws-cli; echo injected", "wsflow-cli extra", "python3 /plugin/$(echo injected)/ws-mcp-launcher.py",
		"python3 /plugin/bin/ws-mcp-launcher.py; echo injected",
		"python3 '/plugin/bin/ws-mcp-launcher.py'; echo injected",
		"python3 '/plugin\n/bin/ws-mcp-launcher.py'",
	} {
		t.Run(command, func(t *testing.T) {
			if got := CLICommandNameFrom(func(string) string { return command }); got != fallback {
				t.Fatalf("CLICommandNameFrom(%q) = %q, want fallback", command, got)
			}
		})
	}
}
