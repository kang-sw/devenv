package wsagent

import (
	"strings"
	"testing"
)

func TestWorkerBackendsDropInheritedHarnessSessionEnv(t *testing.T) {
	inherited := []string{
		"PATH=/usr/bin", "HOME=/home/user", "USER_SETTING=kept",
		"WS_MAILBOX=parent@machine", "WS_MAILBOX_AUTO=1", "WS_MCP_ROOT=/parent/root",
		"WS_RSRC=/parent/rsrc", "WS_RSRC_ROOT=/parent/rsrc-root", "WS_SKILLS_ROOT=/parent/skills",
		"WS_HOSTED_SESSION=1", "WS_HOSTED_SESSION_KEY=parent-key",
	}
	tests := []struct {
		name string
		env  []string
		want map[string]string
	}{
		{
			name: "claude",
			env:  claudeEnv(append(inherited, "CLAUDECODE=1", "CLAUDE_CONFIG_DIR=/user/claude")),
			want: map[string]string{"CLAUDE_CONFIG_DIR": "/user/claude"},
		},
		{
			name: "pi",
			env:  piWorkerEnv(append(inherited, "WS_PI_ROLE=parent")),
			want: map[string]string{"PI_SKIP_VERSION_CHECK": "1", "WS_PI_SPAWN_ROLE": "worker"},
		},
		{
			name: "codex",
			env: codexEnv(StartSpec{
				Env:        append(inherited, "CODEX_THREAD_ID=parent", "CODEX_HOME=/user/codex"),
				MCPSession: &MCPSession{Token: "child-token"},
			}),
			want: map[string]string{"CODEX_HOME": "/user/codex", EnvMCPSessionToken: "child-token"},
		},
		{
			name: "agy",
			env:  agyEnv(StartSpec{Env: append(inherited, "CLAUDE_SESSION=parent", "AGY_HOME=/user/agy")}),
			want: map[string]string{"AGY_HOME": "/user/agy"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := workerEnvMap(test.env)
			for _, name := range []string{"PATH", "HOME", "USER_SETTING"} {
				if got[name] == "" {
					t.Errorf("ordinary user environment %s was dropped: %v", name, test.env)
				}
			}
			for name := range got {
				for _, prefix := range []string{"WS_MAILBOX", "WS_MCP_", "WS_RSRC", "WS_SKILLS_ROOT", "WS_HOSTED_"} {
					if strings.HasPrefix(name, prefix) && test.want[name] == "" {
						t.Errorf("inherited harness-session environment %s survived: %v", name, test.env)
					}
				}
			}
			for name, want := range test.want {
				if got[name] != want {
					t.Errorf("explicit child environment %s = %q, want %q: %v", name, got[name], want, test.env)
				}
			}
		})
	}
}

func workerEnvMap(env []string) map[string]string {
	out := make(map[string]string, len(env))
	for _, kv := range env {
		name, value, _ := strings.Cut(kv, "=")
		out[name] = value
	}
	return out
}
