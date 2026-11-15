package wsdashboard

import (
	"errors"
	"strings"
	"testing"
)

// dashboardTestAsset reads logical sources as the renderer does. Individual
// fragments are intentionally not valid standalone scripts.
func dashboardTestAsset(name string) ([]byte, error) {
	return readDashboardSource(embeddedDashboardAsset, strings.TrimPrefix(name, "assets/"))
}

func TestDashboardSourceAssembly(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files []string
	}{
		{"dashboard.js", []string{"dashboard.js", "dashboard-board.js", "dashboard-conversation.js", "dashboard-bundles.js", "dashboard-composer.js", "dashboard-actions.js", "dashboard-drawer.js", "dashboard-frames.js", "dashboard-git.js", "dashboard-tracker.js", "dashboard-selection.js", "dashboard-stream.js", "dashboard-rail.js", "dashboard-rail-popovers.js", "dashboard-leads.js", "dashboard-lifecycle.js"}},
		{"dashboard.css", []string{"dashboard.css", "dashboard-conversation.css", "dashboard-board.css", "dashboard-layout.css", "dashboard-drawer.css", "dashboard-menus.css", "dashboard-rail.css", "dashboard-shell.css"}},
		{"rail.js", []string{"rail.js"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			raw, err := readDashboardSource(func(name string) ([]byte, error) {
				calls = append(calls, name)
				return []byte("<" + name + ">"), nil
			}, tc.name)
			want := "<" + strings.Join(tc.files, "><") + ">"
			if err != nil || string(raw) != want || strings.Join(calls, ",") != strings.Join(tc.files, ",") {
				t.Fatalf("assembly = %q, %v, reads %v; want %q, reads %v", raw, err, calls, want, tc.files)
			}
			for _, missing := range tc.files {
				failure := errors.New("missing " + missing)
				_, err := readDashboardSource(func(name string) ([]byte, error) {
					if name == missing {
						return nil, failure
					}
					return []byte(name), nil
				}, tc.name)
				if !errors.Is(err, failure) {
					t.Errorf("missing %s: got %v", missing, err)
				}
			}
		})
	}
}
