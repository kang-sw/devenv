package wsrsrc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Namespace substitution exercises hash-verified plugin-root loading.
func TestWhiteboardSelectionGuidance(t *testing.T) {
	for _, pkg := range []string{"agents-plugin", "agents-plugin-wsflow", "agents-plugin-pi"} {
		t.Run(pkg, func(t *testing.T) {
			root, err := filepath.Abs(filepath.Join("..", "..", "..", pkg, "rsrc"))
			if err != nil {
				t.Fatal(err)
			}
			source, err := os.ReadFile(filepath.Join(root, "whiteboard", "whiteboard.md"))
			if err != nil {
				t.Fatal(err)
			}
			assertWhiteboardSelectionLoads(t, string(source))
			for _, namespace := range []string{"ws", "wsflow"} {
				t.Run(namespace, func(t *testing.T) {
					t.Setenv(EnvRsrcRoot, root)
					resolved, err := ResolveRoot()
					if err != nil || resolved != root {
						t.Fatalf("ResolveRoot() = %q, %v; want %q", resolved, err, root)
					}
					harness := ""
					if pkg == "agents-plugin-pi" {
						harness = "pi"
					}
					pb, err := Load(resolved, "whiteboard", harness, nil)
					if err != nil {
						t.Fatal(err)
					}
					declared := append(append([]string{}, pb.Meta.Variables...), ImplicitVariableNames...)
					body, err := substituteVars(pb.Body, declared, map[string]string{
						"McpNamespace": namespace, "SkillNamespace": namespace,
					})
					if err != nil {
						t.Fatal(err)
					}
					assertWhiteboardSelectionLoads(t, body)
					for _, want := range []string{namespace + "/whiteboard.update", namespace + ":lead-ticket", namespace} {
						if !strings.Contains(body, want) {
							t.Errorf("render missing %q", want)
						}
					}
					if strings.Contains(body, "{{.") || strings.Contains(body, "<!-- ws:") {
						t.Error("render leaked template tokens or product-mode markers")
					}
				})
			}
		})
	}
}

func assertWhiteboardSelectionLoads(t *testing.T, body string) {
	t.Helper()
	if strings.TrimSpace(body) == "" {
		t.Fatal("empty whiteboard guidance")
	}
	for _, leak := range []string{"agents-plugin/", "agents-plugin-tool/", "agents-plugin-wsflow/", "ai-docs/manuals/", "261114-", "ParseSelections", "SelectionBlock"} {
		if strings.Contains(body, leak) {
			t.Errorf("downstream guidance leaked %q", leak)
		}
	}
}
