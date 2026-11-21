package mcp

import (
	"maps"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsconfigservice"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsmailbox"
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

func TestDashboardPromptSeedsFollowPlaybookMarkers(t *testing.T) {
	source := fstest.MapFS{"one.md": &fstest.MapFile{Data: []byte(`<!-- ws:override:Outer desc="outer" -->
opening
<!-- ws:override:Inner -->
inner contents
<!-- ws:/override:Inner -->
closing
<!-- ws:/override:Outer -->
<!-- ws:override:Empty -->
<!-- ws:/override:Empty -->`)}}
	points, seeds, err := scanOverridePointsAndSeeds(source)
	if err != nil || len(points) != 3 || seeds["Outer"] != "opening\ninner contents\nclosing" || seeds["Inner"] != "inner contents" {
		t.Fatalf("seed discovery = %#v, %#v, %v", points, seeds, err)
	}
	if empty, ok := seeds["Empty"]; !ok || empty != "" {
		t.Fatalf("empty point = %q, %t", empty, ok)
	}
}

func TestDashboardSettingsKeepsParentSessionResolutionSeparateFromItsOwnOverride(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", t.TempDir())
	src := NewDashboardSessionSource()
	root := t.TempDir()
	parent, err := src.store.mint(root, roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	child, err := src.store.mint(root, roleDelegate, parent)
	if err != nil {
		t.Fatal(err)
	}
	key := wsconfig.ItemWorkflowLang
	if err := src.store.setOverride(parent, key, "parent"); err != nil {
		t.Fatal(err)
	}
	reg := wsdashboard.Registration{Project: root, ReplyID: wsmailbox.ReplyID(child)}
	catalog, err := wsconfigservice.NewCatalog([]wsconfigservice.Rule{{Key: key}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []struct {
		value string
		local bool
	}{
		{"parent", false},
		{"child", true},
		{"parent", false},
	} {
		values, writer, resolved, ok := src.settingsSession(reg)
		if !ok || resolved != child {
			t.Fatal("child session is unavailable")
		}
		entry := catalog.Entry(key, nil, wsconfig.ScopeSession, wsconfigservice.Layers{Session: values, SessionAvailable: true})
		session := entry.Layers[0]
		if entry.Value != want.value || entry.SourceInherited == want.local || session.Inherited == want.local || (session.Selected != nil) != want.local {
			t.Errorf("wrong attribution for %q: %#v", want.value, entry)
		}
		if want.local {
			err = writer.DeleteOverride(resolved, key)
		} else {
			err = writer.SetOverride(resolved, key, "child")
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}
