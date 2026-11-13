package wslive

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// TestCommandsEventReplacesTheList pins the commands event: the list is
// normalized (a leading slash dropped, an invalid or repeated name dropped,
// an unknown layer cleared), replaces the held one, reaches the snapshot,
// and a reset clears it so an adapter that does not re-send one offers no
// commands.
func TestCommandsEventReplacesTheList(t *testing.T) {
	st := NewState(ownerKey)
	res := mustApply(t, st, Event{Kind: KindCommands, Commands: []SlashCommand{
		{Name: "/compact", Description: "Clear history\nkeep a summary", Source: "builtin"},
		{Name: "ws:lead-run", Source: "plugin", Layer: LayerUser},
		{Name: "compact", Description: "a repeat"},
		{Name: "two words"},
		{Name: "skill:x", Source: "skill", Layer: "both"},
	}}, 1_000)
	if !res.Changed || len(res.Ring) != 0 {
		t.Fatalf("result = %+v, want a change and no ring entry", res)
	}
	want := []SlashCommand{
		{Name: "compact", Description: "Clear history keep a summary", Source: "builtin"},
		{Name: "ws:lead-run", Source: "plugin", Layer: LayerUser},
		{Name: "skill:x", Source: "skill"},
	}
	if got := st.Snapshot(2_000).Commands; !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %+v, want %+v", got, want)
	}
	raw, _ := json.Marshal(st.Snapshot(2_000))
	if !strings.Contains(string(raw), `"commands":[{"name":"compact"`) {
		t.Fatalf("snapshot JSON = %s", raw)
	}
	if res := mustApply(t, st, Event{Kind: KindCommands, Commands: want}, 3_000); res.Changed {
		t.Fatal("the same list again is a change")
	}
	mustApply(t, st, Event{Kind: KindReset}, 4_000)
	if got := st.Snapshot(5_000).Commands; got != nil {
		t.Fatalf("after reset: commands = %+v", got)
	}
	raw, _ = json.Marshal(st.Snapshot(5_000))
	if strings.Contains(string(raw), `"commands"`) {
		t.Fatalf("snapshot without commands carries the field: %s", raw)
	}
	if _, err := st.Apply(Event{Kind: KindCommands, AgentID: "a1"}, 6_000); err == nil {
		t.Fatal("a subagent's commands event applied")
	}
}

// TestCommandEventWritesACommandLine pins the command event: a command ring
// entry carrying the typed line and the output text, after the open tool
// gap, with the session key scrubbed; an empty output stays empty (the page
// reads that as a command that showed nothing as text), and a line that is
// not a typed command fails.
func TestCommandEventWritesACommandLine(t *testing.T) {
	st := NewState(ownerKey)
	res := mustApply(t, st, Event{Kind: KindCommand, Command: "/context", Text: "used 40% of " + ownerKey}, 1_000)
	if len(res.Ring) != 1 {
		t.Fatalf("ring = %+v, want one command line", res.Ring)
	}
	got := res.Ring[0]
	if got.Kind != RingCommand || got.Command != "/context" || strings.Contains(got.Text, ownerKey) || !strings.HasPrefix(got.Text, "used 40% of ") {
		t.Fatalf("command line = %+v", got)
	}
	res = mustApply(t, st, Event{Kind: KindCommand, Command: "/config"}, 2_000)
	if len(res.Ring) != 1 || res.Ring[0] != (RingEntry{Kind: RingCommand, Command: "/config"}) {
		t.Fatalf("no-output ring = %+v", res.Ring)
	}
	for _, bad := range []Event{{Kind: KindCommand}, {Kind: KindCommand, Command: "compact"}, {Kind: KindCommand, Command: "/"}, {Kind: KindCommand, Command: "/x", AgentID: "a1"}} {
		if _, err := st.Apply(bad, 3_000); err == nil {
			t.Errorf("Apply(%+v) applied", bad)
		}
	}
}

// TestOptionalKindsNameTheCommandKinds pins that the intake announces both
// command kinds, so an adapter sends them only to a ws-mcp that takes them.
func TestOptionalKindsNameTheCommandKinds(t *testing.T) {
	want := []string{KindStatus, KindCompact, KindCommands, KindCommand, KindWake}
	if !reflect.DeepEqual(OptionalKinds, want) {
		t.Fatalf("OptionalKinds = %v, want %v", OptionalKinds, want)
	}
}
