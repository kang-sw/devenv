//go:build !windows

package wsagent

import "testing"

func TestQuoteHookArgUnix(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", "''"},
		{"simple", "hello", "'hello'"},
		{"with spaces", "/path/to/my exe", "'/path/to/my exe'"},
		{"with single quote", "it's", "'it'\"'\"'s'"},
		{"with dollar sign", "$HOME", "'$HOME'"},
		{"backslash", `C:\Users\foo`, `'C:\Users\foo'`},
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
