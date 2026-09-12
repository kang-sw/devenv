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
						"Decide whether exploration is useful, how many explorers to dispatch",
						"There are no required triggers, tier thresholds, or fan-out counts",
						"Do not directly search or navigate the codebase",
						"You may open exact artifacts cited by the ticket or returned by an explorer",
						"following uncited references is discovery and belongs to an explorer",
						"read-only boundary: no file writes, commits, or mutation tools",
						"the cross-ticket read boundary above",
						"exact file, test, or symbol citations with locations",
						"a subagent summary is never sole authority",
						"do not override a confirmed ticket decision intentionally changing it",
						"an unexplained contract conflict as a missing decision",
						"`spawn_agent.model` and `spawn_agent.reasoning_effort`",
						"An unset effort means omit that field",
						"unavailable, unsupported, or rejected, report the gap",
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
			"Within the ticket's scope, search for the actual terminology, paths, symbols, current logic, and tests",
			"Replace an unambiguous contradicted factual claim",
			"or rewrite a phase goal to match the current implementation",
			"An ambiguous terminology or behavior mapping is unverified or a decision gap, not a correction",
			"do not survey for strategy, reuse, or a plan",
			"Edit only the ticket file. Do not commit",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("%s missing %q", pkg, want)
			}
		}
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
				"Do not scan `todo/`, `idea/`, or the whole ticket tree",
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
