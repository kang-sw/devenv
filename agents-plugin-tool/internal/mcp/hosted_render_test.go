package mcp

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wspackage"
)

// TestHostedPlaybookCommandRender: a hosted lead whose backend has
// PlaybookCommands (Claude, agy) reaches ws only as the daemon's own `ws`
// server and has no ws skills, so its manual and playbooks name `ws/`
// tools whatever package the daemon serves, a skill reference is the
// `/lead-*` dashboard command, delegation is agent.spawn's (no Claude
// native-spawn overlay, the Explore role carries no doubled tier phrase).
// The same lead without PlaybookCommands renders as before.
func TestHostedPlaybookCommandRender(t *testing.T) {
	setupWorkflowManualNoteEnv(t)
	t.Setenv(envNamespace, "wsflow")
	root := t.TempDir()
	initGit(t, root)
	key, err := NewAgentKeyStore().MintLead(root)
	if err != nil {
		t.Fatal(err)
	}
	foreign := regexp.MustCompile(`\bwsflow[/:]|\bws:lead-`)
	for _, harness := range []string{"claude", "agy"} {
		render := HostedRender{Harness: harness, PlaybookCommands: true}
		manual, err := RenderLeadManual(os.Environ(), root, render, key)
		if err != nil {
			t.Fatalf("%s manual: %v", harness, err)
		}
		if m := foreign.FindString(manual); m != "" {
			t.Fatalf("%s manual names %q:\n%s", harness, m, manual)
		}
		for _, want := range []string{"## Session Key\n" + key, "ws/agent.spawn", "a read-only `ws/agent.spawn` subagent at the tier the question needs"} {
			if !strings.Contains(manual, want) {
				t.Fatalf("%s manual lacks %q:\n%s", harness, want, manual)
			}
		}
		for _, absent := range []string{"Claude delegate spawn", "at tier small with read-only authority", "{{"} {
			if strings.Contains(manual, absent) {
				t.Fatalf("%s manual carries %q:\n%s", harness, absent, manual)
			}
		}

		body, err := RenderLeadPlaybook(os.Environ(), root, render, key, "lead-discuss")
		if err != nil {
			t.Fatalf("%s playbook: %v", harness, err)
		}
		if body == "" || body != strings.TrimSpace(body) || strings.Contains(body, "{{") {
			t.Fatalf("%s playbook is not a trimmed, rendered body:\n%s", harness, body)
		}
		if m := foreign.FindString(body); m != "" {
			t.Fatalf("%s playbook names %q:\n%s", harness, m, body)
		}
		if !strings.Contains(body, "ws/agent.spawn") {
			t.Fatalf("%s playbook does not delegate through agent.spawn:\n%s", harness, body)
		}
	}

	plain, err := RenderLeadManual(os.Environ(), root, HostedRender{Harness: "claude"}, key)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plain, "wsflow/") || !strings.Contains(plain, "Claude delegate spawn") {
		t.Fatalf("a lead without PlaybookCommands renders under the package namespace with its native overlay:\n%s", plain)
	}

	if _, err := RenderLeadPlaybook(os.Environ(), root, HostedRender{Harness: "claude", PlaybookCommands: true}, "never-minted-key", "lead-run"); err == nil || !strings.Contains(err.Error(), "has no record") {
		t.Fatalf("an unknown key must fail the playbook render: %v", err)
	}
}

// TestHostedPlaybookCommandReadMatchesTheConnection: the daemon's render of
// a playbook command is the playbook.read text of the lead's own hosted
// connection, so the turn carries what the session would have read.
func TestHostedPlaybookCommandReadMatchesTheConnection(t *testing.T) {
	setupWorkflowManualNoteEnv(t)
	regs := descriptorRegistrations()
	reg := regs[wsagent.BackendClaude]
	reg.PlaybookCommands = true
	regs[wsagent.BackendClaude] = reg
	s, f := hostedBackendConnection(t, wsagent.BackendClaude, regs)
	read, err := s.readPlaybook("lead-run", f.hosted, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(envHostedSession, "")
	rendered, err := RenderLeadPlaybook(os.Environ(), f.root, HostedRender{Harness: "claude", PlaybookCommands: true}, f.hosted, "lead-run")
	if err != nil {
		t.Fatal(err)
	}
	if rendered != strings.TrimSpace(read) {
		t.Fatalf("the daemon render differs from the connection's read:\n--- render\n%s\n--- read\n%s", rendered, read)
	}
}

// TestHostedPlaybookCommandsList: the dashboard list is the package's
// self-preloading lead skills, described, without the workflow manual's
// own skill (the prompt carries the manual) or the retired alias.
func TestHostedPlaybookCommandsList(t *testing.T) {
	checkout, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(wspackage.EnvDevCheckout, checkout)
	for _, ns := range []string{"ws", "wsflow"} {
		list, err := HostedPlaybookCommands([]string{envNamespace + "=" + ns})
		if err != nil {
			t.Fatalf("%s: %v", ns, err)
		}
		var names []string
		for _, cmd := range list {
			names = append(names, cmd.Name)
			if cmd.Description == "" {
				t.Errorf("%s: %s has no description", ns, cmd.Name)
			}
		}
		if !slices.Contains(names, "lead-run") || slices.Contains(names, workflowManualPlaybookName) || slices.Contains(names, "lead-proceed") || slices.Contains(names, "lead-revive") {
			t.Fatalf("%s commands = %q", ns, names)
		}
	}
}
