package wsdashboard

import (
	"strings"
	"testing"
	"time"
)

func TestHostedExitLocalized(t *testing.T) {
	code := 0
	reg := Registration{Hosted: true, HostStopped: true, HostExitCode: &code}
	row := RailRow{ReplyID: "test", Exit: hostExit(reg)}
	region, err := renderRailRegion(RailView{Workspaces: []RailWorkspace{{Path: "/p", Sessions: []RailRow{row}}}}, "fr")
	if err != nil {
		t.Fatal(err)
	}
	html := string(region.HTML)
	for _, want := range []string{"Arrêtée", "code de sortie 0"} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q: %s", want, html)
		}
	}
}

func TestHostedExitRows(t *testing.T) {
	now := time.Now()
	zero, failure, signal := 0, 17, 15
	for _, tc := range []struct {
		name            string
		hosted, stopped bool
		code, signal    *int
		reason, stamp   string
		want, absent    []string
	}{
		{name: "zero", hosted: true, stopped: true, code: &zero, reason: "finished <ok>", stamp: "2026-01-02T03:04:05Z", want: []string{"Stopped", "exit code 0", "finished &lt;ok&gt;", "2026-01-02T03:04:05Z"}},
		{name: "failure", hosted: true, stopped: true, code: &failure, want: []string{"Stopped", "exit code 17"}},
		{name: "signal", hosted: true, stopped: true, signal: &signal, want: []string{"Stopped", "signal 15"}, absent: []string{"exit code"}},
		{name: "both", hosted: true, stopped: true, code: &zero, signal: &signal, want: []string{"exit code 0", "signal 15"}},
		{name: "zero signal", hosted: true, stopped: true, signal: &zero, want: []string{"signal 0"}},
		{name: "unknown", hosted: true, stopped: true, want: []string{"Stopped"}, absent: []string{"exit code", "signal"}},
		{name: "running", hosted: true, code: &failure, reason: "old reason", absent: []string{"host-exit", "old reason"}},
		{name: "terminal", stopped: true, code: &failure, reason: "old reason", absent: []string{"host-exit", "old reason"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := liveSession("test", "/project", now)
			s.Hosted, s.HostStopped, s.HostExitCode, s.HostExitSignal, s.HostExitReason, s.HostStoppedAt = tc.hosted, tc.stopped, tc.code, tc.signal, tc.reason, tc.stamp
			idx, err := renderIndexRegions(buildIndexView([]indexInput{{Session: s}}, now, "test"))
			if err != nil {
				t.Fatal(err)
			}
			row := railRow(railInput{Session: s}, true, now)
			rail, err := renderRailRegion(RailView{Workspaces: []RailWorkspace{{Path: "/project", Sessions: []RailRow{row}}}}, "en")
			if err != nil {
				t.Fatal(err)
			}
			for name, html := range map[string]string{"index": string(idx[regionSessions].HTML), "rail": string(rail.HTML)} {
				for _, want := range tc.want {
					if !strings.Contains(html, want) {
						t.Errorf("%s missing %q: %s", name, want, html)
					}
				}
				for _, absent := range tc.absent {
					if strings.Contains(html, absent) {
						t.Errorf("%s unexpectedly contains %q: %s", name, absent, html)
					}
				}
			}
		})
	}
}
