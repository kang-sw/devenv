package wsagent

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// Launch-time plugin planning must not gate attachment to a healthy process
// the previous daemon already launched, or the next turn could retire it.
func TestClaudeLeadRejoinIgnoresChangedPluginLaunchInputs(t *testing.T) {
	for _, changed := range []string{"malformed settings", "missing bundle"} {
		t.Run(changed, func(t *testing.T) {
			bundle := leadBundledFixture(t, "wsflow")
			h := newLeadHarness(t).start()
			id, holder := h.runLead("one")
			h.holders.out(holder, turnLines("sess-1", "u1", "first")...)
			h.idle(id)
			h.sync(id, holder)

			switch changed {
			case "malformed settings":
				writeLeadPluginFixture(t, filepath.Join(h.root, ".claude", "settings.local.json"), `{`)
			case "missing bundle":
				if err := os.Remove(filepath.Join(bundle, ".claude-plugin", "plugin.json")); err != nil {
					t.Fatal(err)
				}
			}
			h.restart()
			eventually(t, "the healthy Claude process to reattach", func() bool {
				st := h.state(id)
				return st.Process && st.Status == StatusIdle && st.Error == ""
			})
			h.send(id, "two")
			eventually(t, "the next turn to reach the same process", func() bool {
				return slices.Equal(h.userInputs(holder), []string{"one", "two"})
			})
			h.holders.out(holder, turnLines("sess-1", "u2", "second")...)
			h.idle(id)
			h.sync(id, holder)
			if h.record(id).HolderID != holder || h.holders.opCount("launch") != 1 || h.holders.opCount("spawn") != 1 || h.holders.opCount("stop") != 0 {
				t.Fatalf("recovery must keep the healthy holder without respawning: %v", h.holders.opList())
			}
			if st := h.state(id); !st.Process || st.Error != "" {
				t.Fatalf("the reattached Claude process must remain healthy: %+v", st)
			}
		})
	}
}
