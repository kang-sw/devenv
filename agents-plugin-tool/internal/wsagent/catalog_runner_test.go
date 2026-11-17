package wsagent

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// newCatalogLeadHarness is a lead harness whose claude backend registers
// profile.
func newCatalogLeadHarness(t *testing.T, profile *HarnessProfile) *leadHarness {
	h := newLeadHarness(t)
	reg := h.opts.Backends[BackendClaude]
	reg.Catalog = profile
	h.opts.Backends[BackendClaude] = reg
	return h.start()
}

// catalogInitLine is a lead turn's init at version with the given command
// list (absent when nil).
func catalogInitLine(version string, commands []string) string {
	init := map[string]any{"type": "system", "subtype": "init", "session_id": "sess-1", "permissionMode": "auto",
		"model": "claude-opus-5-5", "tools": []string{"Bash"}, "claude_code_version": version}
	if commands != nil {
		init["slash_commands"] = commands
	}
	return jsonLine(init)
}

func catalogTurn(uuid, version string, commands []string) []string {
	return []string{stateLine(HarnessRunning), catalogInitLine(version, commands), replyLine(uuid, "ok"), resultLine(), stateLine(HarnessIdle)}
}

// TestLeadCapabilitiesFromItsInit: a hosted Claude lead shows the catalog
// profile its process's version resolves to, marked unverified above the
// guaranteed range; a dashboard /compact reaches the process while
// compaction is allowed; an init whose command list lacks compact demotes
// it and the offered list drops it; a daemon that rejoins the process
// rebuilds the capabilities from the held stream.
func TestLeadCapabilitiesFromItsInit(t *testing.T) {
	h := newCatalogLeadHarness(t, ClaudeCatalog())
	h.holders.setControl("initialize", "", `{"commands":[{"name":"compact","description":"Free up context"},{"name":"review"}]}`)
	id, holder := h.runLead("one")
	eventually(t, "the launch's command list", func() bool {
		return slices.Equal(commandNames(h.snapshot(id).Commands), []string{"compact", "review"})
	})
	if caps := h.state(id).Capabilities; caps != nil {
		t.Fatalf("no capabilities before the process reports its version: %+v", caps)
	}

	h.holders.out(holder, catalogTurn("u1", "2.1.399", []string{"compact", "review"})...)
	h.idle(id)
	caps := h.state(id).Capabilities
	if caps == nil || !caps.Unverified || caps.Version != "2.1.399" || caps.ProfileVersion != "2.1.326" || caps.Role != RoleLead || len(caps.Demoted) != 0 {
		t.Fatalf("an unverified version on the newest bound's grades: %+v", caps)
	}
	raw, _ := json.Marshal(h.state(id))
	if !strings.Contains(string(raw), `"capabilities":{`) || !strings.Contains(string(raw), `"unverified":true`) {
		t.Fatalf("the dashboard reads the warning from the state: %s", raw)
	}
	if got := commandNames(h.snapshot(id).Commands); !slices.Equal(got, []string{"compact", "review"}) {
		t.Fatalf("native compaction stays offered: %v", got)
	}
	h.sendCommand(id, "compact", "")
	eventually(t, "the compaction to reach the process", func() bool {
		inputs := h.userInputs(holder)
		return len(inputs) > 0 && inputs[len(inputs)-1] == "/compact"
	})
	h.holders.out(holder, catalogTurn("u2", "2.1.326", []string{"review"})...)
	h.idle(id)
	caps = h.state(id).Capabilities
	if caps == nil || caps.Unverified || !slices.Equal(caps.Demoted, []CapabilityItem{ItemCompaction}) || caps.Allows(ItemCompaction) {
		t.Fatalf("a list without compact demotes it: %+v", caps)
	}
	if got := commandNames(h.snapshot(id).Commands); !slices.Equal(got, []string{"review"}) {
		t.Fatalf("the offered list: %v", got)
	}

	h.sync(id, holder)
	h.restart()
	eventually(t, "the rejoin to rebuild the capabilities", func() bool {
		caps := h.state(id).Capabilities
		return caps != nil && caps.Version == "2.1.326" && slices.Equal(caps.Demoted, []CapabilityItem{ItemCompaction})
	})
	if rec := h.record(id); strings.Contains(string(mustJSON(t, rec)), "capabilities") {
		t.Fatal("capabilities are never persisted")
	}
}

// TestLeadOffersNoCommandOfAnUnsupportedItem: a command list the process
// gave before its version was known loses the commands of items the
// resolved profile grades unsupported.
func TestLeadOffersNoCommandOfAnUnsupportedItem(t *testing.T) {
	h := newCatalogLeadHarness(t, &HarnessProfile{Oldest: "2.1.322", Newest: "2.1.326", Entries: []CapabilityEntry{
		{Item: ItemCompaction, Grade: GradeUnsupported},
	}})
	h.holders.setControl("initialize", "", `{"commands":[{"name":"compact","description":"Free up context"},{"name":"review"}]}`)
	id, holder := h.runLead("one")
	eventually(t, "the launch's command list", func() bool {
		return slices.Equal(commandNames(h.snapshot(id).Commands), []string{"compact", "review"})
	})
	h.holders.out(holder, catalogTurn("u1", "2.1.326", nil)...)
	h.idle(id)
	if got := commandNames(h.snapshot(id).Commands); !slices.Equal(got, []string{"review"}) {
		t.Fatalf("compaction is unsupported: offered %v", got)
	}
	if caps := h.state(id).Capabilities; caps == nil || len(caps.Demoted) != 0 || caps.Allows(ItemCompaction) {
		t.Fatalf("graded unsupported, not demoted: %+v", caps)
	}
}

// TestProfilelessBackendStaysHandleOnly: a backend registered without a
// profile (Pi today) shows no capabilities, demotes nothing and hides no
// command, whatever version its process reports.
func TestProfilelessBackendStaysHandleOnly(t *testing.T) {
	h := newLeadHarness(t).start()
	if h.opts.Backends[BackendClaude].Catalog != nil || h.opts.Backends[BackendPi].Catalog != nil {
		t.Fatal("the harness registers no profile")
	}
	h.holders.setControl("initialize", "", `{"commands":[{"name":"compact","description":"Free up context"},{"name":"review"}]}`)
	id, holder := h.runLead("one")
	h.holders.out(holder, catalogTurn("u1", "9.9.9", nil)...)
	h.idle(id)
	if caps := h.state(id).Capabilities; caps != nil {
		t.Fatalf("no profile, no capabilities: %+v", caps)
	}
	if got := commandNames(h.snapshot(id).Commands); !slices.Equal(got, []string{"compact", "review"}) {
		t.Fatalf("no command hidden: %v", got)
	}
}
