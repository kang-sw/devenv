package wsdashboard

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"
)

type closeCaptureAgents struct{}

func (*closeCaptureAgents) Start(func(string, string)) {}
func (*closeCaptureAgents) InboxChanged(string)        {}
func (*closeCaptureAgents) Close()                     {}
func (*closeCaptureAgents) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
}
func (*closeCaptureAgents) SessionMeta(id string) (AgentSessionMeta, bool) {
	return AgentSessionMeta{IsChild: id == "boardsession", Active: true}, id == "boardsession"
}

func TestSubagentChildCloseBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE for browser acceptance")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	f := startPageFixture(t, func(o *DaemonOptions) { o.Agents = &closeCaptureAgents{} })
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "testdata/subagent-child-close.browser.cjs")
	cmd.Env = append(os.Environ(), "CHILD_BASE="+f.base)
	out, err := cmd.CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("child close browser: %v", err)
	}
}
