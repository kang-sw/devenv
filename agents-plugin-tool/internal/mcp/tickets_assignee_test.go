package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeAssigneeTicket writes a ready-status ticket with an optional assignee
// list, without committing (discovery + point-resolve read the filesystem).
func writeAssigneeTicket(t *testing.T, root, stem string, assignee ...string) {
	t.Helper()
	fm := "---\ntitle: " + stem + "\n"
	if len(assignee) > 0 {
		fm += "assignee:\n"
		for _, a := range assignee {
			fm += "  - " + a + "\n"
		}
	}
	fm += "---\n# " + stem + "\n"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "ready", stem+".md"), fm)
}

// assigneeAwareRoot sets up a session-bound server with the ticket-assignee-aware
// flag optionally on in the committed repo scope and git user.email =
// test@example.com (from initGit). Returns the server and the minted session key.
// callToolWithKey returns the tool's already-extracted text (or JSON string).
func assigneeAwareRoot(t *testing.T, root string, flagOn bool) (*Server, string) {
	t.Helper()
	useLeadProfile(t)
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	if flagOn {
		mustWrite(t, root, ".ws-workflow/config.json",
			`{"schema_version":1,"overrides":{"ticket-assignee-aware":"on"}}`+"\n")
	}
	s := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, s, 900100, root, nil))
	return s, key
}

func TestTicketsQueryAssigneeWarningToken(t *testing.T) {
	root := t.TempDir()
	writeAssigneeTicket(t, root, "260101-feat-other", "bob@example.com")
	s, key := assigneeAwareRoot(t, root, true)

	// Point-resolve of an others'-assigned ticket carries the warning token.
	text := callToolWithKey(t, s, 1, key, "tickets.query", map[string]any{"ticket_stem": "260101-feat-other"})
	if !strings.Contains(text, "# NOT ASSIGNED TO YOU") {
		t.Fatalf("point-resolve missing warning token: %s", text)
	}
	if !strings.Contains(text, "assignee: bob@example.com") {
		t.Fatalf("point-resolve missing assignee line: %s", text)
	}

	// JSON projection carries the computed gate.
	respJSON := callToolWithKey(t, s, 2, key, "tickets.query", map[string]any{"ticket_stem": "260101-feat-other", "format": "json"})
	var gate struct {
		Assignee     []string `json:"assignee"`
		AssigneeGate *struct {
			AssignedToCurrent bool   `json:"assigned_to_current"`
			Warning           string `json:"warning"`
		} `json:"assignee_gate"`
	}
	if err := json.Unmarshal([]byte(respJSON), &gate); err != nil {
		t.Fatalf("json parse: %v\n%s", err, respJSON)
	}
	if gate.AssigneeGate == nil || gate.AssigneeGate.AssignedToCurrent || gate.AssigneeGate.Warning != "NOT ASSIGNED TO YOU" {
		t.Fatalf("json gate mismatch: %+v", gate.AssigneeGate)
	}
}

func TestTicketsQueryAssigneeSelfAndAnyNoWarning(t *testing.T) {
	root := t.TempDir()
	writeAssigneeTicket(t, root, "260101-feat-mine", "test@example.com")
	writeAssigneeTicket(t, root, "260102-feat-anyone")
	s, key := assigneeAwareRoot(t, root, true)

	self := callToolWithKey(t, s, 1, key, "tickets.query", map[string]any{"ticket_stem": "260101-feat-mine"})
	if strings.Contains(self, "NOT ASSIGNED TO YOU") {
		t.Fatalf("self-assigned ticket must not warn: %s", self)
	}
	if !strings.Contains(self, "assignee: test@example.com") {
		t.Fatalf("self-assigned ownership line expected: %s", self)
	}

	anyTicket := callToolWithKey(t, s, 2, key, "tickets.query", map[string]any{"ticket_stem": "260102-feat-anyone"})
	if strings.Contains(anyTicket, "assignee:") {
		t.Fatalf("assign-any ticket must print no assignee line: %s", anyTicket)
	}
}

func TestTicketsQueryAssigneeInertWhenFlagOff(t *testing.T) {
	root := t.TempDir()
	writeAssigneeTicket(t, root, "260101-feat-other", "bob@example.com")
	s, key := assigneeAwareRoot(t, root, false)

	text := callToolWithKey(t, s, 1, key, "tickets.query", map[string]any{"ticket_stem": "260101-feat-other"})
	if strings.Contains(text, "NOT ASSIGNED TO YOU") || strings.Contains(text, "assignee:") {
		t.Fatalf("feature must be inert with flag off: %s", text)
	}
	respJSON := callToolWithKey(t, s, 2, key, "tickets.query", map[string]any{"ticket_stem": "260101-feat-other", "format": "json"})
	if strings.Contains(respJSON, "assignee_gate") {
		t.Fatalf("assignee_gate must be absent with flag off: %s", respJSON)
	}
}

func TestTicketsQueryAssignedToMeOmitsOthers(t *testing.T) {
	root := t.TempDir()
	writeAssigneeTicket(t, root, "260101-feat-mine", "test@example.com")
	writeAssigneeTicket(t, root, "260102-feat-other", "bob@example.com")
	writeAssigneeTicket(t, root, "260103-feat-anyone")
	s, key := assigneeAwareRoot(t, root, true)

	filtered := callToolWithKey(t, s, 1, key, "tickets.query", map[string]any{"statuses": []any{"ready"}, "assigned_to_me": true})
	if strings.Contains(filtered, "260102-feat-other") {
		t.Fatalf("assigned_to_me must omit others'-assigned ticket: %s", filtered)
	}
	if !strings.Contains(filtered, "260101-feat-mine") || !strings.Contains(filtered, "260103-feat-anyone") {
		t.Fatalf("assigned_to_me dropped a self/assign-any ticket: %s", filtered)
	}

	// Without the filter the others'-assigned ticket stays visible (gated).
	all := callToolWithKey(t, s, 2, key, "tickets.query", map[string]any{"statuses": []any{"ready"}})
	if !strings.Contains(all, "260102-feat-other") {
		t.Fatalf("plain query must keep others'-assigned ticket visible: %s", all)
	}
}

func TestCreateEmptyStampsAssignee(t *testing.T) {
	readCreated := func(t *testing.T, root, resp string) string {
		t.Helper()
		var path string
		for _, tok := range strings.Fields(resp) {
			if strings.HasPrefix(tok, "ai-docs/tickets/") {
				path = tok
			}
		}
		if path == "" {
			t.Fatalf("no created path in response: %s", resp)
		}
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatalf("read created ticket: %v", err)
		}
		return string(raw)
	}

	t.Run("default true stamps self", func(t *testing.T) {
		root := t.TempDir()
		s, key := assigneeAwareRoot(t, root, true)
		resp := callToolWithKey(t, s, 1, key, "tickets.create_empty", map[string]any{"stem": "feat-foo", "initial_state": "idea"})
		body := readCreated(t, root, resp)
		if !strings.Contains(body, "assignee:\n  - test@example.com\n") {
			t.Fatalf("default set_assignee must stamp current git email: %q", body)
		}
	})

	t.Run("false leaves unassigned", func(t *testing.T) {
		root := t.TempDir()
		s, key := assigneeAwareRoot(t, root, true)
		resp := callToolWithKey(t, s, 1, key, "tickets.create_empty", map[string]any{"stem": "feat-foo", "initial_state": "idea", "set_assignee": false})
		body := readCreated(t, root, resp)
		if strings.Contains(body, "assignee:") {
			t.Fatalf("set_assignee=false must leave ticket unassigned: %q", body)
		}
	})

	t.Run("explicit emails", func(t *testing.T) {
		root := t.TempDir()
		s, key := assigneeAwareRoot(t, root, true)
		resp := callToolWithKey(t, s, 1, key, "tickets.create_empty", map[string]any{"stem": "feat-foo", "initial_state": "idea", "set_assignee": []any{"x@example.com", "y@example.com"}})
		body := readCreated(t, root, resp)
		if !strings.Contains(body, "assignee:\n  - x@example.com\n  - y@example.com\n") {
			t.Fatalf("explicit set_assignee not stamped: %q", body)
		}
	})

	t.Run("inert when flag off", func(t *testing.T) {
		root := t.TempDir()
		s, key := assigneeAwareRoot(t, root, false)
		resp := callToolWithKey(t, s, 1, key, "tickets.create_empty", map[string]any{"stem": "feat-foo", "initial_state": "idea"})
		body := readCreated(t, root, resp)
		if strings.Contains(body, "assignee:") {
			t.Fatalf("create must not stamp assignee with flag off: %q", body)
		}
	})
}
