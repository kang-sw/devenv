package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

func TestTicketDesignReviewExplorationBindings(t *testing.T) {
	for _, product := range []struct{ pkg, namespace string }{
		{"agents-plugin", "ws"}, {"agents-plugin-wsflow", "wsflow"},
	} {
		for _, tc := range []struct {
			harness, explorer string
			rows              []string
		}{
			{"codex", "an explorer subagent", []string{
				"| small | gpt-6-luna | high |", "| medium | gpt-6-luna | xhigh |", "| large | gpt-6-sol | high |",
			}},
			{"claude", "the Explore agent", []string{
				"| small | haiku | high |", "| medium | sonnet | high |", "| large | opus | high |",
			}},
			{"unknown-host", "an exploration agent", []string{
				"| small | gpt-6-luna | high |", "| medium | gpt-6-luna | xhigh |", "| large | gpt-6-sol | high |",
			}},
		} {
			for _, config := range []string{"default", "custom", "unset", "invalid"} {
				t.Run(product.namespace+"/"+tc.harness+"/"+config, func(t *testing.T) {
					t.Setenv("WS_MCP_NAMESPACE", product.namespace)
					t.Setenv("WS_MCP_NO_AGENT", map[bool]string{true: "1", false: "0"}[product.namespace == "wsflow"])
					opts := wsconfig.Options{CacheHome: t.TempDir(), ConfigHome: t.TempDir()}
					rows := tc.rows
					if config == "custom" || config == "unset" {
						rows = nil
						for i, tier := range []string{"small", "medium", "large"} {
							effort := []string{"low", "medium", "xhigh"}[i]
							if config == "unset" {
								effort = ""
							}
							harness := tc.harness
							if harness == "unknown-host" {
								harness = ""
							}
							backend := "codex"
							if harness == "claude" {
								backend = "claude"
							}
							if _, err := wsconfig.SetAgentsTierForHarness(opts, tier, backend, "custom-"+tier, harness, effort); err != nil {
								t.Fatal(err)
							}
							rows = append(rows, "| "+tier+" | custom-"+tier+" | "+effort+" |")
						}
					}
					if config == "invalid" {
						if err := os.WriteFile(filepath.Join(opts.CacheHome, "config.json"), []byte("{invalid"), 0o644); err != nil {
							t.Fatal(err)
						}
						rows = []string{"| small | the small-tier model | |", "| medium | the medium-tier model | |", "| large | the large-tier model | |"}
					}
					root := filepath.Join("..", "..", "..", product.pkg, "rsrc")
					body, _, err := renderPlaybookBody(newTestServerWithHarness(t, tc.harness), root, "ticket-reviewer-design", nil, opts, "", "", "", nil, "")
					if err != nil {
						t.Fatal(err)
					}
					text := strings.Join(strings.Fields(body), " ")
					for _, want := range append(rows, tc.explorer) {
						if !strings.Contains(text, strings.Join(strings.Fields(want), " ")) {
							t.Errorf("missing %q", want)
						}
					}
					if strings.Contains(body, "{{") || strings.Contains(body, "<no value>") {
						t.Fatal("unresolved template binding in reviewer")
					}
				})
			}
		}
	}
}

func TestTicketReviewDocumentsLoad(t *testing.T) {
	for _, product := range []struct{ pkg, namespace string }{
		{"agents-plugin", "ws"}, {"agents-plugin-wsflow", "wsflow"},
	} {
		for _, name := range []string{"reference-discovery", "ticket-fact-populator"} {
			t.Run(product.namespace+"/"+name, func(t *testing.T) {
				t.Setenv("WS_MCP_NAMESPACE", product.namespace)
				t.Setenv("WS_MCP_NO_AGENT", map[bool]string{true: "1", false: "0"}[product.namespace == "wsflow"])
				root := filepath.Join("..", "..", "..", product.pkg, "rsrc")
				body, _, err := renderPlaybookBody(&Server{}, root, name, nil, wsconfig.Options{CacheHome: t.TempDir()}, "", "", "", nil, "")
				if err != nil {
					t.Fatal(err)
				}
				if strings.TrimSpace(body) == "" {
					t.Fatal("empty rendered document")
				}
				if !strings.Contains(body, product.namespace+"/") || strings.Contains(body, "{{") {
					t.Fatal("unresolved namespace binding")
				}
			})
		}
	}
}
