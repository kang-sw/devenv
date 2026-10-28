package mcp

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Rendering delegate-implementer from a top-level lead reserves the
// delegation's write-once plan; these tests drive the real playbook.render
// entry against the shipped rsrc tree in both product modes.

func setupPlanRenderServer(t *testing.T, namespace string) (*Server, string, string) {
	t.Helper()
	useLeadProfile(t)
	t.Setenv(envNamespace, namespace)
	if namespace == "wsflow" {
		t.Setenv(envNoAgent, "1")
	}
	t.Setenv("WS_RSRC_ROOT", filepath.Join("..", "..", "..", "agents-plugin", "rsrc"))
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	root := initGitRepo(t)
	s := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, s, 1, root, nil))
	return s, root, key
}

func plansDir(root string) string {
	return filepath.Join(root, "ai-docs", ".plans")
}

func assertNoPlans(t *testing.T, root string) {
	t.Helper()
	if _, err := os.Stat(plansDir(root)); !os.IsNotExist(err) {
		entries, _ := filepath.Glob(filepath.Join(plansDir(root), "*", "*"))
		t.Fatalf("render must leave no plan under %s, found %v (stat err %v)", plansDir(root), entries, err)
	}
}

// renderLines returns the non-empty response lines of a successful render.
func renderLines(t *testing.T, resp string) []string {
	t.Helper()
	if toolIsError(t, resp) {
		t.Fatalf("playbook.render returned error: %s", resp)
	}
	return strings.Split(strings.TrimSpace(toolText(t, resp)), "\n")
}

func TestPlaybookRenderDelegateImplementerReservesPlanForTopLevelLead(t *testing.T) {
	for _, namespace := range []string{"ws", "wsflow"} {
		t.Run(namespace, func(t *testing.T) {
			s, root, key := setupPlanRenderServer(t, namespace)
			lines := renderLines(t, callToolOnce(t, s, 2, "playbook.render", map[string]any{
				"name":        "delegate-implementer",
				"session_key": key,
				"context":     map[string]any{"plan_stem": "fix-widget"},
			}))
			// The prompt path stays the first line (callers read it there);
			// the plan path is the last line.
			if len(lines) != 2 || !strings.HasSuffix(lines[0], "-delegate-implementer.md") {
				t.Fatalf("render output shape = %q", lines)
			}
			planPath, ok := strings.CutPrefix(lines[1], "plan-path: ")
			if !ok {
				t.Fatalf("last line is not a plan-path line: %q", lines[1])
			}
			wantDir := plansDir(canonicalRootForTest(t, root))
			if !regexp.MustCompile(`^\d{4}-\d{2}/\d{2}-\d{4}-fix-widget\.md$`).MatchString(strings.TrimPrefix(filepath.ToSlash(planPath), filepath.ToSlash(wantDir)+"/")) {
				t.Fatalf("plan path %q is not a dated plan under %s", planPath, wantDir)
			}
			data, err := os.ReadFile(planPath)
			if err != nil {
				t.Fatalf("plan file not reserved: %v", err)
			}
			if len(data) != 0 {
				t.Fatalf("reserved plan must be empty for the lead to write, got %q", data)
			}
			prompt, err := os.ReadFile(lines[0])
			if err != nil {
				t.Fatalf("read rendered prompt: %v", err)
			}
			if !strings.HasSuffix(string(prompt), "\n\nplan-path: "+planPath+"\n") {
				t.Fatalf("rendered floor must end with the plan-path line:\n%s", prompt)
			}
			if !strings.Contains(string(prompt), "## Plan") {
				t.Fatalf("rendered floor lost its plan section:\n%s", prompt)
			}
		})
	}
}

func TestPlaybookRenderDelegateImplementerPlanStemFallsBackToDelegate(t *testing.T) {
	for _, stem := range []any{nil, "   "} {
		s, _, key := setupPlanRenderServer(t, "ws")
		args := map[string]any{"name": "delegate-implementer", "session_key": key}
		if stem != nil {
			args["context"] = map[string]any{"plan_stem": stem}
		}
		lines := renderLines(t, callToolOnce(t, s, 2, "playbook.render", args))
		if len(lines) != 2 || !strings.HasSuffix(lines[1], "-delegate.md") {
			t.Fatalf("stem %v: render output = %q, want a plan named after the default stem", stem, lines)
		}
	}
}

func TestPlaybookRenderDelegateImplementerPlanFollowsRootOverride(t *testing.T) {
	s, root, key := setupPlanRenderServer(t, "ws")
	delegateRoot := initGitRepo(t)
	lines := renderLines(t, callToolOnce(t, s, 2, "playbook.render", map[string]any{
		"name":          "delegate-implementer",
		"session_key":   key,
		"root_override": delegateRoot,
		"context":       map[string]any{"plan_stem": "elsewhere"},
	}))
	if len(lines) != 2 || !strings.HasPrefix(lines[1], "plan-path: "+plansDir(canonicalRootForTest(t, delegateRoot))) {
		t.Fatalf("plan must be reserved under the override root, got %q", lines)
	}
	assertNoPlans(t, root)
}

func TestPlaybookRenderDelegateImplementerWorkerKeyGetsNoPlan(t *testing.T) {
	s, root, leadKey := setupPlanRenderServer(t, "ws")
	// A worker key is a lead-scope child of the lead (what a worker playbook
	// render mints); its leaves are governed by the ticket, not a plan.
	workerKey, err := s.sessions.mint(canonicalRootForTest(t, root), roleLead, leadKey)
	if err != nil {
		t.Fatalf("mint worker key: %v", err)
	}
	lines := renderLines(t, callToolOnce(t, s, 2, "playbook.render", map[string]any{
		"name":        "delegate-implementer",
		"session_key": workerKey,
		"context":     map[string]any{"plan_stem": "leaf"},
	}))
	if len(lines) != 1 || strings.Contains(lines[0], "plan-path:") {
		t.Fatalf("worker render must return only the prompt path, got %q", lines)
	}
	prompt, err := os.ReadFile(lines[0])
	if err != nil {
		t.Fatalf("read rendered prompt: %v", err)
	}
	if strings.Contains(string(prompt), "plan-path: ") {
		t.Fatalf("worker-rendered floor must carry no plan-path line:\n%s", prompt)
	}
	assertNoPlans(t, root)
}

func TestPlaybookRenderDelegateImplementerFailedRenderLeavesNoPlan(t *testing.T) {
	s, root, key := setupPlanRenderServer(t, "ws")
	resp := callToolOnce(t, s, 2, "playbook.render", map[string]any{
		"name":        "delegate-implementer",
		"session_key": key,
		"context":     map[string]any{"plan_stem": "doomed", "undeclared": "x"},
	})
	if !toolIsError(t, resp) {
		t.Fatalf("an undeclared context variable must fail the render, got %s", resp)
	}
	assertNoPlans(t, root)
}

func TestPlaybookRenderPlanStemIsNotConsumedForOtherPlaybooks(t *testing.T) {
	s, root, key := setupPlanRenderServer(t, "ws")
	resp := callToolOnce(t, s, 2, "playbook.render", map[string]any{
		"name":        "reviewer",
		"session_key": key,
		"context":     map[string]any{"plan_stem": "x"},
	})
	if !toolIsError(t, resp) || !strings.Contains(toolText(t, resp), "plan_stem") {
		t.Fatalf("plan_stem is an undeclared variable for every other playbook, got %s", resp)
	}
	assertNoPlans(t, root)
}
