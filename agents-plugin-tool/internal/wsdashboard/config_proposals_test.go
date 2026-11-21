package wsdashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wsstate"
)

func TestSettingsProposalDeliversOnceToAnActiveLeadWithoutWritingRepo(t *testing.T) {
	backend := settingsTestBackend(t)
	f := startPageFixture(t, func(o *DaemonOptions) { o.Config = backend })
	target := settingsID("worktree", f.project)
	body := `{"target":"` + target + `","operations":[{"action":"set","key":"workflow.profile","selectors":{"harness":"codex"},"value":"orchestrate"},{"action":"reset","key":"workflow.profile","selectors":{"harness":"*"}}]}`
	own := "http://127.0.0.1:43210"
	try := func(body, origin string) *httptest.ResponseRecorder {
		return settingsRequest(t, f.d, http.MethodPost, "/api/config/proposal", body, origin)
	}
	for _, tc := range []struct {
		body, origin, code string
		status             int
	}{
		{body, "http://evil.example", "forbidden", 403},
		{`{"target":"` + target + `","operations":[]}`, own, "invalid_request", 400},
		{strings.Replace(body, target, f.project, 1), own, "target_not_found", 404},
		{strings.Replace(body, `"harness":"*"`, `"harness":"codex"`, 1), own, "invalid_request", 400},
		{strings.Replace(body, "workflow.profile", "agent.profile.test", 1), own, "invalid_request", 400},
	} {
		if got := try(tc.body, tc.origin); got.Code != tc.status || !strings.Contains(got.Body.String(), tc.code) {
			t.Errorf("rejected proposal = %d %s", got.Code, got.Body)
		}
	}
	if got := try(body, own); got.Code != 200 || !strings.Contains(got.Body.String(), `"delivered":true`) {
		t.Fatalf("proposal = %d %s", got.Code, got.Body)
	}
	store := (wsmailbox.Paths{State: wsstate.Options{CacheHome: filepath.Dir(f.dir)}}).Store()
	winner, err := store.ReplyQueue("boardsession")
	if err != nil || len(winner) != 1 {
		t.Fatalf("winner's mail = %v, %v", winner, err)
	}
	other, err := store.ReplyQueue("livesession")
	if err != nil || len(other) != 0 {
		t.Fatalf("other lead's mail = %v, %v", other, err)
	}
	path, _ := wsconfig.RepoPath(wsconfig.Options{RepoRoot: f.project})
	var message configProposalMessage
	if err := json.Unmarshal([]byte(winner[0].Content), &message); err != nil {
		t.Fatal(err)
	}
	if message.Target != target || message.Path != path || len(message.Operations) != 2 || message.Operations[1].Action != "reset" {
		t.Errorf("mail has wrong identity or patch: %+v", message)
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Errorf("proposal touched the repo config directory: %v", err)
	}
	// Keep the old index entry live while the registration changes under it.
	reg, err := ReadRegistration(f.dir, "boardsession")
	if err != nil {
		t.Fatal(err)
	}
	reg.EndedAt = stampFor(time.Now())
	if err := WriteRegistration(f.dir, reg); err != nil {
		t.Fatal(err)
	}
	if got := try(body, own); got.Code != 409 || !strings.Contains(got.Body.String(), "no_active_lead") {
		t.Errorf("stale lead proposal = %d %s", got.Code, got.Body)
	}
	if mail, err := store.ReplyQueue("livesession"); err != nil || len(mail) != 0 {
		t.Errorf("stale target sent to a substitute lead: %v, %v", mail, err)
	}
}
