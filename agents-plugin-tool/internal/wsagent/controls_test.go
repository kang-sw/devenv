package wsagent

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsinbox"
)

// Concrete method sets are broader than some launched processes' controls.
// Keep the matrix tied to instances, not a union of backend capabilities.
func TestProcessControlsMatchTheLaunchedProcess(t *testing.T) {
	for _, tt := range []struct {
		name string
		proc Process
		want []string
	}{
		{"pi worker", &piProcess{}, []string{"steer", "reports", "commands", "turns", "recovery"}},
		{"pi lead", &piProcess{lead: true}, []string{"steer", "reports", "commands", "turns", "recovery"}},
		{"claude worker", claudeProcess{}, []string{"steer"}},
		{"claude lead", claudeProcess{lead: true}, []string{"approval", "mode", "mcp", "steer"}},
		{"minimal process", &fakeProc{}, nil},
		{"optional fake", fakeSteerProc{&fakeProc{}}, []string{"steer", "reports"}},
		{"no process", nil, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := controlsFor(tt.proc)
			var got []string
			for _, control := range []struct {
				name string
				on   bool
			}{
				{"approval", c.approval != nil}, {"mode", c.mode != nil}, {"mcp", c.mcp != nil},
				{"steer", c.steer != nil}, {"reports", c.reports != nil}, {"commands", c.commands != nil},
				{"turns", c.turns != nil}, {"recovery", c.recovery != nil},
			} {
				if control.on {
					got = append(got, control.name)
				}
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("controls = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBackendSelectionKeepsNamesAndUnavailableErrors(t *testing.T) {
	pi, claude := &fakeBackend{}, &fakeBackend{}
	r := NewRunner(Options{Backends: map[string]Backend{BackendPi: pi, BackendClaude: claude}})
	for _, tt := range []struct {
		name string
		want Backend
	}{{BackendPi, pi}, {BackendClaude, claude}} {
		got, err := r.backend("profile", tt.name)
		if err != nil || got != tt.want {
			t.Fatalf("backend(%q) = %v, %v", tt.name, got, err)
		}
	}
	for _, name := range []string{"", "unknown"} {
		if _, err := r.backend("profile", name); err == nil || !strings.Contains(err.Error(), "is not available in this daemon") {
			t.Fatalf("unavailable backend %q = %v", name, err)
		}
	}
	delete(r.opts.Backends, BackendPi)
	c := &conv{r: r, rec: Record{Profile: "profile", Backend: BackendPi}}
	if _, err := c.startBackend(StartSpec{}); err == nil || err.Error() != `agent profile "profile": backend "pi" is not available in this daemon` {
		t.Fatalf("unavailable launch = %v", err)
	}
}

func TestProcessControlsKeepCommandAndModeFallbacks(t *testing.T) {
	p := &fakeProc{sent: make(chan string, 1)}
	c := controlsFor(p)
	if err := c.runCommand(wsinbox.Command{Name: "custom", Args: "argument"}); err != nil {
		t.Fatal(err)
	}
	if got := <-p.sent; got != "/custom argument" {
		t.Fatalf("command fallback = %q", got)
	}
	if err := c.setPermissionMode("plan"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("missing permission mode control = %v", err)
	}
	// The real adapters still decide support from the control arguments.
	pi := controlsFor(&piProcess{})
	if err := pi.SetModel("fuzzy-model"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Pi fuzzy model = %v", err)
	}
	if err := pi.SetEffort(""); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Pi default effort = %v", err)
	}
	claude := controlsFor(claudeProcess{lead: true})
	if err := claude.setPermissionMode(""); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Claude empty mode = %v", err)
	}
}
