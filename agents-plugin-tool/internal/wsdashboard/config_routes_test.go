package wsdashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsconfigservice"
)

func settingsTestBackend(t *testing.T) *ConfigBackend {
	t.Helper()
	catalog, err := wsconfigservice.NewCatalog([]wsconfigservice.Rule{
		{Key: "workflow.profile", Default: "direct", Harnesses: []string{"codex", "*"}},
		{Key: "global.test", Default: "a", GlobalOnly: true},
		{Key: "agents.tier", Harnesses: []string{"codex", "default"}},
		{Key: "agent.profile.*", Structured: true, ValidateName: func(string) error { return nil },
			NormalizeObject: func(any, wsconfig.Scope) (json.RawMessage, error) { return nil, wsconfigservice.ErrInvalid }},
		{Key: "preference.*"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &ConfigBackend{Catalog: catalog, Options: wsconfig.Options{
		CacheHome: filepath.Join(t.TempDir(), "cache"), ConfigHome: filepath.Join(t.TempDir(), "global"),
	}, OrchestrateWarning: func(string) string { return "Harness has not been verified." }}
}

func settingsRequest(t *testing.T, d *Daemon, method, path, body, origin string) *httptest.ResponseRecorder {
	t.Helper()
	r := authRequest(method, path, body)
	r.AddCookie(authCookie(t, d))
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	return authServe(d, r)
}

func TestSettingsRoutesAuthorizationAndWrites(t *testing.T) {
	backend := settingsTestBackend(t)
	d := authFixture(t)
	d.config = backend
	d.lan = true
	token, err := d.lanToken()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/settings", "/api/config/snapshot"} {
		r := authRequest(http.MethodGet, path+"?token="+url.QueryEscape(token), "")
		r.Host, r.RemoteAddr = "192.0.2.1:43210", "192.0.2.2:12345"
		if got := authServe(d, r); got.Code != 403 {
			t.Errorf("LAN %s = %d %s", path, got.Code, got.Body)
		}
		if got := settingsRequest(t, d, http.MethodGet, path, "", ""); got.Code != 200 {
			t.Errorf("local %s = %d %s", path, got.Code, got.Body)
		}
	}
	for _, dir := range []string{backend.Options.CacheHome, backend.Options.ConfigHome} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("read touched settings directory %s: %v", dir, err)
		}
	}
	body := `{"scope":"global","key":"workflow.profile","selectors":{"harness":"codex"},"value":"orchestrate"}`
	for _, origin := range []string{"", "http://evil.example"} {
		if got := settingsRequest(t, d, http.MethodPost, "/api/config/set", body, origin); got.Code != 403 {
			t.Errorf("origin %q = %d %s", origin, got.Code, got.Body)
		}
	}
	own := "http://127.0.0.1:43210"
	if got := settingsRequest(t, d, http.MethodPost, "/api/config/set", body, own); got.Code != 200 || !strings.Contains(got.Body.String(), "unverified_harness") {
		t.Fatalf("set = %d %s", got.Code, got.Body)
	}
	for _, tc := range []struct {
		path, body, code string
		status           int
	}{
		{"/api/config/set", `{"scope":"missing","key":"global.test","value":"x"}`, "invalid_request", 400},
		{"/api/config/set", `{"scope":"global","key":"global.test","value":"x","selectors":{"tier":"small"}}`, "invalid_request", 400},
		{"/api/config/set", `{"scope":"global","key":"global.test","value":false}`, "invalid_request", 400},
		{"/api/config/reset", `{"scope":"global","key":"global.test","value":null}`, "invalid_request", 400},
		{"/api/config/set", `{"scope":"builtin","key":"global.test","value":"x"}`, "scope_unsupported", 409},
		{"/api/config/reset", `{"scope":"session","target":"expired","key":"global.test"}`, "target_not_found", 404},
	} {
		if got := settingsRequest(t, d, http.MethodPost, tc.path, tc.body, own); got.Code != tc.status || !strings.Contains(got.Body.String(), tc.code) {
			t.Errorf("%s %s = %d %s", tc.path, tc.body, got.Code, got.Body)
		}
	}
}

func TestSettingsSnapshotContextAndRedaction(t *testing.T) {
	backend := settingsTestBackend(t)
	backend.Session = func(reg Registration) (map[string]string, wsconfig.SessionWriter, string, bool) {
		return map[string]string{"workflow.profile.all": "session-" + reg.ReplyID}, nil, "never-return-this-session-key", true
	}
	fixture := startPageFixture(t, func(o *DaemonOptions) { o.Config = backend })
	d := fixture.d
	repoOpts := backend.Options
	repoOpts.RepoRoot = fixture.project
	repo, _ := wsconfig.RepoPath(repoOpts)
	if err := os.MkdirAll(filepath.Dir(repo), 0o700); err != nil {
		t.Fatal(err)
	}
	raw := `{"overrides":{"workflow.profile.codex":"repo-specific","workflow.profile.all":"repo-all"},"structured_overrides":{"agent.profile.test":{"backend":"codex","nested":{"Api_Key":{"values":["secret-nested"]}},"mcp_servers":{"run":{"type":"stdio","command":"/private/bin/node --secret","args":["--api-key=secret-123"],"url":"https://user:secret@example.com/mcp?token=abc","env":{"OTHER":"secret-env"},"headers":{"Authorization":"secret-header"},"extra":{"unknown":["secret-other"]}}}}}}`
	if err := os.WriteFile(repo, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	global, err := wsconfig.GlobalPath(backend.Options)
	if err != nil {
		t.Fatal(err)
	}
	resolver := wsconfig.NewResolver(backend.Options, nil, nil, nil)
	if err := resolver.Set("workflow.profile.all", "global-all", wsconfig.SetOptions{ExplicitScope: wsconfig.ScopeGlobal}); err != nil {
		t.Fatal(err)
	}
	worktree, workspace := settingsID("worktree", fixture.project), settingsID("workspace", fixture.project)
	for _, tc := range []struct {
		query, value string
		source       wsconfig.Scope
		conditional  bool
		path         string
	}{
		{"", "global-all", wsconfig.ScopeGlobal, true, global},
		{"?scope=repo&target=" + worktree + "&worktree=" + worktree, "repo-specific", wsconfig.ScopeRepo, true, repo},
		{"?scope=builtin&worktree=" + worktree, "repo-specific", wsconfig.ScopeRepo, true, ""},
		{"?scope=builtin&worktree=" + worktree + "&session=livesession", "repo-specific", wsconfig.ScopeRepo, false, ""},
		{"?scope=session&target=boardsession&worktree=" + worktree + "&session=boardsession", "repo-specific", wsconfig.ScopeRepo, false, ""},
		{"?scope=project&target=" + workspace, "global-all", wsconfig.ScopeGlobal, true, mustProjectConfigPath(t, backend.Options)},
	} {
		result := settingsRequest(t, d, http.MethodGet, "/api/config/snapshot"+tc.query, "", "")
		if result.Code != 200 {
			t.Fatalf("%s = %d %s", tc.query, result.Code, result.Body)
		}
		for _, secret := range []string{"secret-nested", "secret-123", "example.com", "secret-env", "secret-header", "secret-other", "/private/bin", "never-return-this-session-key"} {
			if strings.Contains(result.Body.String(), secret) {
				t.Fatalf("%s exposed %q", tc.query, secret)
			}
		}
		var view configSnapshot
		if err := json.Unmarshal(result.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
		if (view.Path == nil && tc.path != "") || (view.Path != nil && *view.Path != tc.path) {
			t.Errorf("%s returned wrong scope path: %v", tc.query, view.Path)
		}
		for _, e := range view.Entries {
			if e.Key == "workflow.profile" && e.Selectors["harness"] == "codex" && (e.Value != tc.value || e.SourceScope != tc.source || e.Conditional != tc.conditional) {
				t.Errorf("%s %#v", tc.query, e)
			}
			for _, layer := range e.Layers {
				if !layer.Evaluated && (layer.Present != nil || layer.Value != nil) {
					t.Errorf("unevaluated layer has a value: %#v", layer)
				}
			}
		}
		if strings.Contains(tc.query, "worktree=") && (!strings.Contains(result.Body.String(), `"command":"node"`) || !strings.Contains(result.Body.String(), `"args":["[redacted]"]`) || !strings.Contains(result.Body.String(), `"type":"stdio"`)) {
			t.Errorf("missing profile structure in %s", tc.query)
		}
	}
	for _, query := range []string{"?scope=builtin&session=livesession", "?scope=builtin&worktree=stale", "?scope=session&target=boardsession&worktree=" + worktree + "&session=livesession"} {
		result := settingsRequest(t, d, http.MethodGet, "/api/config/snapshot"+query, "", "")
		if result.Code != 400 || !strings.Contains(result.Body.String(), "invalid_context") {
			t.Errorf("%s = %d %s", query, result.Code, result.Body)
		}
	}
	if got := settingsRequest(t, d, http.MethodPost, "/api/config/set", `{"scope":"global","key":"agent.profile.test","value":{"mcp_servers":{"args":["--api-key=secret-123"]}}}`, "http://127.0.0.1:43210"); got.Code != 400 || strings.Contains(got.Body.String(), "secret-123") {
		t.Errorf("invalid profile = %d %s", got.Code, got.Body)
	}
}

func mustProjectConfigPath(t *testing.T, opts wsconfig.Options) string {
	t.Helper()
	path, err := wsconfig.Path(opts)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSettingsTierResetTargetsOnlyTheSelectedHarnessAndTier(t *testing.T) {
	d := authFixture(t)
	d.config = settingsTestBackend(t)
	own := "http://127.0.0.1:43210"
	for _, leaf := range []struct{ tier, harness, model string }{
		{"small", "codex", "target"}, {"small", "default", "same-tier"}, {"medium", "codex", "same-harness"},
	} {
		body, err := json.Marshal(map[string]any{"scope": "global", "key": "agents.tier",
			"selectors": map[string]string{"tier": leaf.tier, "harness": leaf.harness},
			"value":     map[string]string{"backend": "codex", "model": leaf.model},
		})
		if err != nil {
			t.Fatal(err)
		}
		if got := settingsRequest(t, d, http.MethodPost, "/api/config/set", string(body), own); got.Code != 200 {
			t.Fatalf("set tier = %d %s", got.Code, got.Body)
		}
	}
	body := `{"scope":"global","key":"agents.tier","selectors":{"tier":"small","harness":"codex"}}`
	if got := settingsRequest(t, d, http.MethodPost, "/api/config/reset", body, own); got.Code != 200 {
		t.Fatalf("reset tier = %d %s", got.Code, got.Body)
	}
	files, err := wsconfig.FileLayers(d.config.Options)
	if err != nil {
		t.Fatal(err)
	}
	tiers := files[wsconfig.ScopeGlobal].Agents.ModelAliases
	if _, found := tiers["small"]["codex"]; found || tiers["small"]["default"].Model != "same-tier" || tiers["medium"]["codex"].Model != "same-harness" {
		t.Errorf("reset targeted the wrong leaf: %+v", tiers)
	}
}
