package mcp

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

type directNudgeHost struct {
	fakeLiveHost
	dir   string
	calls []wsdashboard.NudgeBody
}

func (h *directNudgeHost) NotifySessionChange(dir string, body wsdashboard.NudgeBody) (int, bool) {
	if dir != h.dir {
		return 0, false
	}
	h.calls = append(h.calls, body)
	return 4321, true
}

func TestDashboardDirectTransportAndFallback(t *testing.T) {
	for _, mode := range []string{"direct", "foreign", "unsupported"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			s := &Server{}
			s.dashboard.dir = func() (string, error) { return dir, nil }
			body := wsdashboard.NudgeBody{ReplyID: "abc", Project: "project"}
			s.dashboard.sessions = map[string]*dashboardSession{"key": {replyID: body.ReplyID, project: body.Project, hosted: true}}
			var fake *fakeDaemon
			if mode != "direct" {
				fake = startFakeDaemon(t, dir, http.StatusNoContent, false)
			}
			if mode != "unsupported" {
				hostDir := dir
				if mode == "foreign" {
					hostDir = t.TempDir()
				}
				s.live.host = &directNudgeHost{dir: hostDir}
			} else {
				s.live.host = &fakeLiveHost{}
			}
			queue := make(chan wsdashboard.NudgeBody, 3)
			queue <- body
			queue <- body
			queue <- body
			close(queue)
			s.runDashboardNudges(queue)
			if mode == "direct" {
				h := s.live.host.(*directNudgeHost)
				if len(h.calls) != 1 || s.dashboard.sessions["key"].servedPort != 4321 {
					t.Fatalf("calls=%v session=%+v", h.calls, s.dashboard.sessions["key"])
				}
				if _, err := os.Stat(filepath.Join(dir, "endpoint.json")); !os.IsNotExist(err) {
					t.Fatalf("endpoint exists or stat failed: %v", err)
				}
			} else if got := fake.received(); len(got) != 1 {
				t.Fatalf("fallback calls=%v", got)
			}
		})
	}
}
