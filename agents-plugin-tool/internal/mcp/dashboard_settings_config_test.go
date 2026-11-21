package mcp

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsconfigservice"
	"github.com/kang-sw/devenv/internal/wspackage"
)

func TestDashboardSettingsCatalog(t *testing.T) {
	t.Setenv(wspackage.EnvDevCheckout, "")
	var namespaces = map[string]int{"claude": 12, "agy": 2, "dashboard": 5, "pi": 15}
	for _, pkg := range []string{"ws", "wsflow"} {
		t.Run(pkg, func(t *testing.T) {
			catalog, err := DashboardConfigCatalog(pkg)
			if err != nil {
				t.Fatal(err)
			}
			actual := make(map[string]int)
			normalized := make(map[string]bool)
			for _, key := range catalog.Keys() {
				normalized[wsconfigservice.FamilyKey(key)] = true
				namespace, _, _ := strings.Cut(key, ".")
				if _, ok := namespaces[namespace]; ok {
					actual[namespace]++
				}
			}
			if !maps.Equal(actual, namespaces) {
				t.Fatalf("adapter namespaces = %v, want %v", actual, namespaces)
			}
			keys := slices.Sorted(maps.Keys(normalized))
			for _, family := range []string{"prompt.*", "agent.profile.*", "worktree.profile.*", "preference.*"} {
				if !slices.Contains(keys, family) {
					t.Errorf("missing family %q", family)
				}
			}
			if err := catalog.Compare(keys); err != nil {
				t.Fatal(err)
			}
			for _, broken := range [][]string{keys[1:], append(slices.Clone(keys), keys[0]), append(slices.Clone(keys), "unknown.setting")} {
				if err := catalog.Compare(broken); err == nil {
					t.Errorf("accepted mismatched UI catalog")
				}
			}
			entry := catalog.Entry("dashboard.pi_spawn_enabled", nil, wsconfig.ScopeGlobal, wsconfigservice.Layers{})
			if entry.Default != (pkg == "ws") {
				t.Errorf("dashboard.pi_spawn_enabled default for %s = %v", pkg, entry.Default)
			}
		})
	}
}
