//go:build windows

package wsagent

import "testing"

func TestQuoteHookArgWindows(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", `""`},
		{"simple", "hello", `"hello"`},
		{"with spaces", `C:\Program Files\ws-mcp.exe`, `"C:\Program Files\ws-mcp.exe"`},
		{"embedded double quote", `say "hello"`, `"say \"hello\""`},
		{"path with spaces and quotes", `C:\My "Dir"\ws-mcp.exe`, `"C:\My \"Dir\"\ws-mcp.exe"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := quoteHookArg(tc.input)
			if got != tc.want {
				t.Errorf("quoteHookArg(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
