package mcp

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsconfigservice"
	"github.com/kang-sw/devenv/internal/wspackage"
)

func TestDashboardSettingsUICatalog(t *testing.T) {
	t.Setenv(wspackage.EnvDevCheckout, "")
	raw, err := os.ReadFile("../wsdashboard/assets/dashboard-settings-catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	var ui []struct {
		Key, Section, Control, Label, Description string
		Selectors                                 []string
	}
	if err := json.Unmarshal(raw, &ui); err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(ui))
	for _, row := range ui {
		keys = append(keys, row.Key)
		if row.Label == row.Description || !strings.HasPrefix(row.Label, "js.settings.key.") ||
			!strings.HasPrefix(row.Description, "js.settings.key.") {
			t.Errorf("%s: label and description require distinct settings translation IDs", row.Key)
		}
	}
	for _, name := range []string{"ws", "wsflow"} {
		t.Run(name, func(t *testing.T) {
			catalog, err := DashboardConfigCatalog(name)
			if err != nil {
				t.Fatal(err)
			}
			if err := catalog.Compare(keys); err != nil {
				t.Fatal(err)
			}
			for _, broken := range [][]string{keys[1:], append(slices.Clone(keys), keys[0]), append(slices.Clone(keys), "unknown.setting")} {
				if err := catalog.Compare(broken); err == nil {
					t.Error("accepted changed UI keys")
				}
			}

			projection := catalog.Entries(wsconfig.ScopeGlobal, wsconfigservice.Layers{})
			entries := make(map[string]wsconfigservice.Entry)
			for _, entry := range projection {
				entries[wsconfigservice.FamilyKey(entry.Key)] = entry
			}
			for _, row := range ui {
				entry, ok := entries[row.Key]
				if !ok {
					if !slices.Contains([]string{"agent.profile.*", "worktree.profile.*"}, row.Key) || row.Control != "profile" ||
						row.Section != "profiles" || len(row.Selectors) != 0 {
						t.Errorf("unexpected unprojected row: %+v", row)
					}
					continue
				}
				if len(entry.Selectors) != len(row.Selectors) {
					t.Errorf("%s: UI selectors %v, daemon selectors %v", row.Key, row.Selectors, entry.Selectors)
				}
				for _, selector := range row.Selectors {
					if _, ok := entry.Selectors[selector]; !ok {
						t.Errorf("%s: selector %s unavailable", row.Key, selector)
					}
				}
				switch entry.Type {
				case "boolean", "integer", "enum", "tier":
					want := map[string]string{"boolean": "toggle", "integer": "number", "enum": "choice", "tier": "tier"}[entry.Type]
					if row.Control != want {
						t.Errorf("%s: UI control %q for daemon type %s, want %s", row.Key, row.Control, entry.Type, want)
					}
				}
				if prefix, _, _ := strings.Cut(row.Key, "."); slices.Contains([]string{"claude", "agy", "dashboard", "pi"}, prefix) && row.Section != prefix {
					t.Errorf("%s: adapter section = %s", row.Key, row.Section)
				}
			}
		})
	}
}
