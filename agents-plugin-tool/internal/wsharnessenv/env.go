// Package wsharnessenv removes the identity of the harness session that
// started the dashboard daemon from environments for newly launched sessions.
package wsharnessenv

import (
	"runtime"
	"strings"
)

// dropPrefixes name inherited ws launcher and session state. User state
// locations such as WS_CACHE_HOME and WS_CONFIG_HOME intentionally stay.
var dropPrefixes = []string{"WS_MAILBOX", "WS_MCP_", "WS_RSRC", "WS_SKILLS_ROOT", "WS_HOSTED_"}

// Drop returns env without inherited ws harness-session identity. Callers add
// the new child's explicit identity after filtering.
func Drop(env []string) []string {
	return DropForOS(runtime.GOOS, env)
}

// DropForOS is Drop for a known target OS. Windows environment names are
// case-insensitive, including mixed-case names inherited from a parent.
func DropForOS(goos string, env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		matchName := name
		if goos == "windows" {
			matchName = strings.ToUpper(name)
		}
		drop := false
		for _, prefix := range dropPrefixes {
			if strings.HasPrefix(matchName, prefix) {
				drop = true
				break
			}
		}
		if !drop {
			out = append(out, kv)
		}
	}
	return out
}
