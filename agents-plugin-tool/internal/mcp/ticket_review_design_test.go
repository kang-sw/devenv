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
				"| small | gpt-5.6-luna | medium |", "| medium | gpt-5.6-terra | high |", "| large | gpt-5.6-sol | high |",
			}},
			{"claude", "the Explore agent", []string{
				"| small | haiku | |", "| medium | sonnet | |", "| large | opus | |",
			}},
			{"unknown-host", "an exploration agent", []string{
				"| small | gpt-5.6-luna | medium |", "| medium | gpt-5.6-terra | high |", "| large | gpt-5.6-sol | high |",
			}},
		} {
			for _, config := range []string{"default", "custom", "unset", "invalid"} {
				t.Run(product.namespace+"/"+tc.harness+"/"+config, func(t *testing.T) {
					t.Setenv("WS_MCP_NAMESPACE", product.namespace)
					t.Setenv("WS_MCP_NO_AGENT", map[bool]string{true: "1", false: "0"}[product.namespace == "wsflow"])
					opts := wsconfig.Options{CacheHome: t.TempDir()}
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
					body, _, err := renderPlaybookBody(newTestServerWithHarness(t, tc.harness), root, "ticket-reviewer-design", nil, opts, "", "", "", nil)
					if err != nil {
						t.Fatal(err)
					}
					text := strings.Join(strings.Fields(body), " ")
					for _, want := range append(rows,
						"Dispatch "+tc.explorer+" through the host-native spawn mechanism",
						"Use explorers when code evidence would materially inform that plan",
						"Choose the smallest useful set of bounded exploration questions and the appropriate configured tier for each",
						"Use host-native explorers for codebase discovery",
						"You may open exact artifacts cited by the ticket or an explorer to verify load-bearing claims",
						"Give each explorer the ticket path, a bounded question, relevant artifacts, and a read-only boundary",
						"Keep exploration within the ticket and cross-ticket boundaries above",
						"Require located file, test, or symbol citations, evidence gaps, follow-up needs, and omissions",
						"Verify load-bearing claims from the cited artifacts",
						"apply unless a confirmed ticket decision changes it",
						"an unexplained contract conflict as a missing decision",
						"`spawn_agent.model` and `spawn_agent.reasoning_effort`",
						"Use the selected model and any nonempty reasoning-effort binding explicitly",
						"Record unavailable or rejected bindings in `omitted:`",
						"omitted: <checks or evidence not obtained and why> | none",
					) {
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

func TestReferenceDiscoveryTicketQuerySchema(t *testing.T) {
	for _, product := range []struct{ pkg, namespace string }{
		{"agents-plugin", "ws"}, {"agents-plugin-wsflow", "wsflow"},
	} {
		t.Run(product.namespace, func(t *testing.T) {
			t.Setenv("WS_MCP_NAMESPACE", product.namespace)
			t.Setenv("WS_MCP_NO_AGENT", map[bool]string{true: "1", false: "0"}[product.namespace == "wsflow"])
			root := filepath.Join("..", "..", "..", product.pkg, "rsrc")
			body, _, err := renderPlaybookBody(&Server{}, root, "reference-discovery", nil, wsconfig.Options{CacheHome: t.TempDir()}, "", "", "", nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, status := range []string{"ready", "todo", "idea"} {
				want := product.namespace + `/tickets.query(statuses: ["` + status + `"])`
				if !strings.Contains(body, want) {
					t.Errorf("rendered reference discovery is missing %q", want)
				}
			}
			if strings.Contains(body, product.namespace+`/tickets.query(status:`) {
				t.Errorf("rendered reference discovery uses unsupported status argument:\n%s", body)
			}
		})
	}
}

func TestTicketFactPopulatorGroundingBoundary(t *testing.T) {
	for _, pkg := range []string{"agents-plugin", "agents-plugin-wsflow"} {
		root := filepath.Join("..", "..", "..", pkg, "rsrc")
		body, _, err := renderPlaybookBody(&Server{}, root, "ticket-fact-populator", nil, wsconfig.Options{CacheHome: t.TempDir()}, "", "", "", nil)
		if err != nil {
			t.Fatal(err)
		}
		text := strings.Join(strings.Fields(body), " ")
		for _, want := range []string{
			"You edit exactly one file: the ticket at the path you were given",
			"Verify each against matching tree artifacts, using scope-bounded search when necessary",
			"Replace a contradicted factual claim in place with the true fact and its evidence",
			"keep every existing `- Convention:` line that exactly matches a current row",
			"union of those retained valid lines and the path-matched lines",
			"Count every added, removed, or changed convention line as a correction",
			"Preserve every `### Result` section, `#### Edition` entry, decision, and phase goal",
			"Report a gap that requires a product, contract, or architecture choice as a decision gap",
			"Limit prose corrections to verifiable facts the ticket claims",
			"Report a claim you could not settle as unverified",
			"Edit only the ticket file. Do not commit",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("%s missing %q", pkg, want)
			}
		}
	}
}

func TestTicketFactPopulatorPreservesConstraintsFixture(t *testing.T) {
	for _, pkg := range []string{"agents-plugin", "agents-plugin-wsflow"} {
		t.Run(pkg, func(t *testing.T) {
			root := filepath.Join("..", "..", "..", pkg, "rsrc")
			body, _, err := renderPlaybookBody(&Server{}, root, "ticket-fact-populator", nil, wsconfig.Options{CacheHome: t.TempDir()}, "", "", "", nil)
			if err != nil {
				t.Fatal(err)
			}
			text := strings.Join(strings.Fields(body), " ")
			for _, want := range []string{
				"a ticket that names both a `project/rsrc/` path and a `project-tool/internal/mcp/` path",
				"keeps a valid existing MCP-manual convention line",
				"adds any missing matching rsrc convention lines",
				"reports `corrections: 1` or more when that union changes the section",
			} {
				if !strings.Contains(text, want) {
					t.Errorf("%s missing constraint-preservation fixture %q", pkg, want)
				}
			}
		})
	}
}

// The rendered prompt is the contract boundary for this model-executed check;
// these assertions do not claim to execute a model's contradiction judgment.
func TestTicketDesignReviewContradictionAnchors(t *testing.T) {
	for _, product := range []struct{ pkg, namespace string }{
		{"agents-plugin", "ws"},
		{"agents-plugin-wsflow", "wsflow"},
	} {
		t.Run(product.namespace, func(t *testing.T) {
			root := filepath.Join("..", "..", "..", product.pkg, "rsrc")
			t.Setenv("WS_MCP_NAMESPACE", product.namespace)
			if product.namespace == "wsflow" {
				t.Setenv("WS_MCP_NO_AGENT", "1")
			} else {
				t.Setenv("WS_MCP_NO_AGENT", "0")
			}
			body, _, err := renderPlaybookBody(&Server{}, root, "ticket-reviewer-design", nil, wsconfig.Options{}, "", "", "", nil)
			if err != nil {
				t.Fatal(err)
			}
			text := strings.Join(strings.Fields(body), " ")
			for _, want := range []string{
				product.namespace + `/tickets.query(statuses: ["ready"])`,
				"every returned ticket body except the ticket under review",
				"even when no `related:` edge names them",
				"ticket_stem: <parent>, include_done: true, include_dropped: true",
				"`related:` is not an independent contradiction anchor",
				"do not scan those directories or the whole ticket tree",
				"report the incomplete check",
			} {
				if !strings.Contains(text, want) {
					t.Errorf("rendered review is missing %q", want)
				}
			}
			checklist := strings.Split(strings.Split(body, "## Checklist")[1], "## Heuristics")[0]
			heuristics := strings.Split(strings.Split(body, "## Heuristics")[1], "## Output")[0]
			for name, section := range map[string]string{"checklist": checklist, "severity": heuristics} {
				if !strings.Contains(section, "`ready/`") || !strings.Contains(section, "`parent:`") || strings.Contains(section, "`related:`") {
					t.Errorf("%s must anchor on ready inventory and parent only: %s", name, section)
				}
			}
			if !strings.Contains(checklist, "**Duct-tape detection**") || !strings.Contains(checklist, "**Right-problem check**") {
				t.Error("single-ticket coherence checks were lost")
			}
		})
	}
}
