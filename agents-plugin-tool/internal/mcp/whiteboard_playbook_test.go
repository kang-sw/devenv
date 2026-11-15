package mcp

import (
	"strings"
	"testing"
)

func TestWhiteboardPlaybookStreamGuidanceProductModes(t *testing.T) {
	for _, namespace := range []string{"ws", "wsflow"} {
		t.Run(namespace, func(t *testing.T) {
			t.Setenv(envNamespace, namespace)
			if namespace == "wsflow" {
				t.Setenv(envNoAgent, "1")
			} else {
				t.Setenv(envNoAgent, "")
			}
			f := newWhiteboardFixture(t)
			body := toolText(t, f.call(t, "playbook.read", map[string]any{"name": "whiteboard"}))
			if strings.TrimSpace(body) == "" {
				t.Fatal("whiteboard playbook is empty")
			}
			for _, want := range []string{namespace + "/whiteboard.query", namespace + ":lead-ticket"} {
				if !strings.Contains(body, want) {
					t.Fatalf("%s render missing namespace token %q", namespace, want)
				}
			}
			for _, unresolved := range []string{"{{.McpNamespace}}", "{{.SkillNamespace}}"} {
				if strings.Contains(body, unresolved) {
					t.Fatalf("unresolved namespace token %q", unresolved)
				}
			}
			if namespace == "wsflow" && (strings.Contains(body, "`ws/") || strings.Contains(body, "`ws:")) {
				t.Fatal("wsflow guidance leaked the ws namespace")
			}
		})
	}
}
