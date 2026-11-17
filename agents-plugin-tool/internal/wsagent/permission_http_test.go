package wsagent

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPermissionHTTPSelection(t *testing.T) {
	h := permissionHarness(t).start()
	for _, v := range []viewer{lanBrowser, localClient} {
		before := permissionHTTPFiles(t, h.dir)
		res := h.route(v, http.MethodPost, "/agents/leads/permission", map[string]any{"root": h.root, "harness": BackendClaude, "permission_level": "full", "permission_full_ack": true})
		if (res.status != 404 && res.status != 403) || !reflect.DeepEqual(before, permissionHTTPFiles(t, h.dir)) || len(h.runner.List()) != 0 {
			t.Fatalf("create user gate: %d %s", res.status, res.body)
		}
	}
	for _, backend := range []string{BackendClaude, BackendPi} {
		body := map[string]any{"root": h.root, "harness": backend, "permission_level": "read-only"}
		res := h.route(localBrowser, http.MethodPost, "/agents/leads/permission", body)
		if res.status != 201 {
			t.Fatalf("create: %d %s", res.status, res.body)
		}
		st := decodeState(t, res)
		if st.PermissionLevel != PermissionReadOnly {
			t.Fatalf("state: %+v", st)
		}
		path := "/agents/" + st.ID + "/settings/permission"
		for _, v := range []viewer{lanBrowser, localClient} {
			r := h.route(v, http.MethodPost, path, map[string]any{"permission_level": "full", "permission_full_ack": true, "model": "opus"})
			if r.status != 404 && r.status != 403 {
				t.Fatalf("gate: %d %s", r.status, r.body)
			}
			if h.state(st.ID).PermissionLevel != PermissionReadOnly {
				t.Fatal("gate wrote selection")
			}
		}
		r := h.route(localBrowser, http.MethodPost, path, map[string]any{"permission_level": "full"})
		if r.status != 400 {
			t.Fatalf("missing ack: %d %s", r.status, r.body)
		}
		r = h.route(localBrowser, http.MethodPost, path, map[string]any{"permission_level": "full", "permission_full_ack": true, "model": "opus", "effort": "high"})
		if r.status != 200 {
			t.Fatalf("full: %d %s", r.status, r.body)
		}
		got := h.state(st.ID)
		if got.PermissionLevel != PermissionFull || got.Model != "opus" || got.Effort != "high" {
			t.Fatalf("settings: %+v", got)
		}
		if _, err := h.runner.SetPermissionLevel(st.ID, PermissionReadOnly); err != nil {
			t.Fatal(err)
		}
		if _, err := h.runner.SetPermissionLevel(st.ID, PermissionFull); err == nil {
			t.Fatal("agent raised")
		}
	}
}

func TestPermissionHTTPInvalidZeroWrites(t *testing.T) {
	h := permissionHarness(t).start()
	st := h.lead(BackendClaude, LeadOptions{})
	path := "/agents/" + st.ID + "/settings/permission"
	invalid := []map[string]any{
		{}, {"permission_level": nil}, {"permission_level": ""}, {"permission_level": "bogus"}, {"permission_level": 3},
		{"permission_level": "read-only", "permission_full_ack": nil}, {"permission_level": "read-only", "permission_full_ack": "yes"},
		{"permission_level": "read-only", "unknown": true}, {"permission_level": "read-only", "permission_mode": "plan"},
		{"permission_level": "read-only", "model": "bad model"}, {"permission_level": "read-only", "effort": "Bad"},
		{"permission_level": "read-only", "model": nil}, {"permission_full_ack": true}, {"permission_level": "full"},
	}
	for _, body := range invalid {
		before := permissionHTTPFiles(t, h.dir)
		for _, target := range []string{path, "/agents/leads/permission"} {
			b := map[string]any{"root": h.root, "harness": BackendClaude}
			if target == path {
				b = map[string]any{}
			}
			for k, v := range body {
				b[k] = v
			}
			res := h.route(localBrowser, http.MethodPost, target, b)
			if res.status != 400 {
				t.Fatalf("%s %v: %d %s", target, b, res.status, res.body)
			}
		}
		if !reflect.DeepEqual(before, permissionHTTPFiles(t, h.dir)) {
			t.Fatalf("invalid request wrote: %v", body)
		}
		if got := h.state(st.ID); got.Model != "" || got.Effort != "" {
			t.Fatalf("partial settings: %+v", got)
		}
	}
	for _, target := range []string{"/agents/leads", "/agents/" + st.ID + "/settings"} {
		for _, field := range []string{"permission_level", "permission_full_ack"} {
			before := permissionHTTPFiles(t, h.dir)
			res := h.route(localBrowser, http.MethodPost, target, map[string]any{"root": h.root, "harness": BackendClaude, field: nil, "model": "opus"})
			if res.status != 400 || !reflect.DeepEqual(before, permissionHTTPFiles(t, h.dir)) {
				t.Fatalf("legacy intent: %d %s", res.status, res.body)
			}
		}
	}
}

func permissionHTTPFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Lock sidecars are synchronization, not persisted request data.
		// Windows may deny even a read while another goroutine holds one.
		if d.IsDir() || strings.HasSuffix(d.Name(), ".lock") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err == nil {
			out[path] = string(b)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestPermissionHTTPFilesZeroWritesIgnoresOnlyLockSidecars(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "sessions", "test-session")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(dir, "registration.json")
	lock := data + ".lock"
	for path, contents := range map[string]string{data: "original", lock: "lock bookkeeping"} {
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	before := permissionHTTPFiles(t, root)
	if !reflect.DeepEqual(before, map[string]string{data: "original"}) {
		t.Fatalf("snapshot must contain data only: %v", before)
	}
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, permissionHTTPFiles(t, root)) {
		t.Fatal("lock removal changed the data snapshot")
	}
	if err := os.WriteFile(data, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(before, permissionHTTPFiles(t, root)) {
		t.Fatal("data write was hidden from the snapshot")
	}
}

func TestPermissionHTTPRegistryAbsentAndLegacy(t *testing.T) {
	h := newLeadHarness(t).start()
	before := permissionHTTPFiles(t, h.dir)
	res := h.route(localBrowser, http.MethodPost, "/agents/leads/permission", map[string]any{"root": h.root, "harness": BackendClaude, "permission_level": "read-only"})
	if res.status != 400 || len(h.runner.List()) != 0 || !reflect.DeepEqual(before, permissionHTTPFiles(t, h.dir)) {
		t.Fatalf("absent registry: %d %s", res.status, res.body)
	}
	res = h.route(localBrowser, http.MethodPost, "/agents/leads", map[string]any{"root": h.root, "harness": BackendClaude, "permission_mode": "plan"})
	if res.status != 201 {
		t.Fatalf("legacy create: %d %s", res.status, res.body)
	}
	st := decodeState(t, res)
	before = permissionHTTPFiles(t, h.dir)
	unsupported := h.route(localBrowser, http.MethodPost, "/agents/"+st.ID+"/settings/permission", map[string]any{"permission_level": "read-only", "model": "opus"})
	if unsupported.status != 400 || !reflect.DeepEqual(before, permissionHTTPFiles(t, h.dir)) {
		t.Fatalf("unsupported settings wrote: %d %s", unsupported.status, unsupported.body)
	}
	res = h.route(localBrowser, http.MethodPost, "/agents/"+st.ID+"/settings", map[string]any{"permission_mode": "bypassPermissions"})
	if res.status != 200 || h.state(st.ID).PermissionMode != "bypassPermissions" {
		t.Fatalf("legacy native: %d %s", res.status, res.body)
	}
	// Common-aware native legacy UI may raise; the public agent setter may not.
	p := permissionHarness(t).start()
	lead := p.lead(BackendClaude, LeadOptions{PermissionMode: "plan"})
	if _, err := p.runner.SetPermissionMode(lead.ID, "acceptEdits"); err == nil {
		t.Fatal("agent native raise accepted")
	}
	res = p.route(localBrowser, http.MethodPost, "/agents/"+lead.ID+"/settings", map[string]any{"permission_mode": "acceptEdits"})
	if res.status != 200 {
		t.Fatalf("trusted native raise: %d %s", res.status, res.body)
	}
}

func TestPermissionHTTPWorkerDeferred(t *testing.T) {
	h := permissionHarness(t)
	// The builtin worker has ws tools; satisfy its capability attachment check
	// with a command the fake holder never executes.
	h.opts.WSMCP = func(string) (MCPCommand, error) { return MCPCommand{Command: "/fake/ws-mcp"}, nil }
	h.start()
	st, err := h.runner.Create("pi-worker", h.root)
	if err != nil {
		t.Fatal(err)
	}
	before := permissionHTTPFiles(t, h.dir)
	path := "/agents/" + st.ID + "/settings/permission"
	for _, tc := range []struct {
		v      viewer
		status int
	}{{lanBrowser, 404}, {localClient, 403}, {localBrowser, 400}} {
		res := h.route(tc.v, http.MethodPost, path, map[string]any{"permission_level": "read-only", "model": "opus"})
		if res.status != tc.status {
			t.Fatalf("worker: %d %s", res.status, res.body)
		}
	}
	if !reflect.DeepEqual(before, permissionHTTPFiles(t, h.dir)) {
		t.Fatal("worker refusal wrote")
	}
	res := h.route(localBrowser, http.MethodGet, "/agents/"+st.ID, nil)
	for _, field := range []string{"permission_level", "permission_source", "permission_enforcement"} {
		if strings.Contains(res.body, `"`+field+`"`) {
			t.Fatalf("worker implies support: %s", res.body)
		}
	}
}

func TestPermissionSupport(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		h := newLeadHarness(t)
		if enabled {
			for name, reg := range h.opts.Backends {
				reg.PermissionLevels = LeadPermissionLevels()
				h.opts.Backends[name] = reg
			}
		}
		h.start()
		s := NewSupport(SupportOptions{Runner: h.runner, Dir: t.TempDir()})
		choices := s.Choices(context.Background())
		for _, entry := range choices.Harnesses {
			if enabled && entry.Lead {
				if !reflect.DeepEqual(entry.PermissionLevels, LeadPermissionLevels()) {
					t.Fatalf("support: %+v", entry)
				}
				entry.PermissionLevels[0] = PermissionFull
			} else if len(entry.PermissionLevels) != 0 {
				t.Fatalf("false support: %+v", entry)
			}
		}
		if enabled {
			for _, entry := range s.Choices(context.Background()).Harnesses {
				if entry.Lead && entry.PermissionLevels[0] != PermissionReadOnly {
					t.Fatal("choices array aliases registry")
				}
			}
		}
		raw, _ := json.Marshal(choices)
		if !enabled && strings.Contains(string(raw), "permission_levels") {
			t.Fatal("absent support serialized")
		}
	}
}

func TestPermissionPinnedRouter(t *testing.T) {
	h := permissionHarness(t).start()
	s := NewSupport(SupportOptions{Runner: h.runner, Dir: t.TempDir()})
	preflight, err := json.Marshal(s.Choices(context.Background()))
	if err != nil {
		t.Fatal(err)
	}
	// Exact fixture declarations always compile; compare with Git when available.
	source := `package permissionlegacy
import (
 "encoding/json"
 "errors"
 "io"
 "mime"
 "net/http"
 "net/http/httptest"
 "net/url"
 "os"
 "strings"
 "testing"
 current "github.com/kang-sw/devenv/internal/wsagent"
)
const RoutePrefix="/agents"
const leadsRoute="leads"
const tokenRoute="by-token"
const requestBodyLimit=256<<10
type State=current.State
type LeadOptions=current.LeadOptions
type requestError struct {msg string; status int}
func(e *requestError) Error()string{return e.msg}
func badRequest(f string,args ...any)error{return errors.New(f)}
type Runner struct {
 *current.Runner
 opts struct{Trusted func(*http.Request)(bool,bool)}
 creates,settings int
}
func(r *Runner) IsLead(string)bool{return false}
func(r *Runner) CreateLead(string,string,LeadOptions)(State,error){r.creates++;return State{},nil}
func(r *Runner) Create(string,string)(State,error){r.creates++;return State{},nil}
func(r *Runner) SetSettings(string,*string,*string)(State,error){r.settings++;return State{},nil}
func(r *Runner) serveToken(http.ResponseWriter,*http.Request,string){panic("unexpected token")}
`
	source += permissionLegacyDeclarations(t, "wsagent/http.go", "createRequest", "sendRequest", "settingsRequest", "leadRequest", "approvalRequest", "interruptAnswer", "serve", "leadControl", "leadMove", "writeAllowed", "decode", "writeAnswer")
	source += `func TestPinned(t *testing.T){
 r:=&Runner{}
 r.opts.Trusted=func(*http.Request)(bool,bool){return true,true}
 dir:=t.TempDir()
 var choices current.ChoicesAnswer
 if err:=json.Unmarshal([]byte(os.Getenv("PERMISSION_PREFLIGHT")),&choices);err!=nil {t.Fatal(err)}
 supported:=false
 for _,h:=range choices.Harnesses {if h.Harness=="claude" {for _,level:=range h.PermissionLevels {if level==current.PermissionFull {supported=true}}}}
 if !supported {t.Fatal("positive preflight prerequisite missing")}
 // A client just saw positive current-server preflight. Replacement is this old
 // dispatch; it sends once to the selection path, never to a legacy fallback.
 for _,tc:=range []struct{path string;status int}{
 {"/agents/leads/permission",405},{"/agents/id/settings/permission",404},
 {"/agents/workers",405},{"/agents/workers/permission",404},
 } {
  // Marshal paths rather than concatenating them: Windows temp directories
  // contain backslashes, which must be escaped in a valid JSON string.
  raw,err:=json.Marshal(map[string]any{"root":dir,"harness":"claude","permission_level":"full","permission_full_ack":true})
  if err!=nil {t.Fatal(err)}
  body:=string(raw)
  req:=httptest.NewRequest("POST","http://127.0.0.1:8123"+tc.path,strings.NewReader(body))
  req.Header.Set("Content-Type","application/json")
  req.Header.Set("Origin","http://127.0.0.1:8123")
  if !writeAllowed(req){t.Fatal("write prerequisite missing")}
  // Exercise the exact permissive decoder with a valid selection payload.
  var payload leadRequest
  check:=httptest.NewRequest("POST","http://127.0.0.1:8123",strings.NewReader(body))
  if !decode(httptest.NewRecorder(),check,&payload)||payload.Harness!="claude"||payload.Root!=dir {t.Fatal("valid decode prerequisite missing")}
  w:=httptest.NewRecorder();r.serve(w,req)
  if w.Code!=tc.status {t.Fatalf("%s: %d %s",tc.path,w.Code,w.Body.String())}
 }
 if r.creates!=0||r.settings!=0 {t.Fatalf("writes: creates=%d settings=%d",r.creates,r.settings)}
 entries,err:=os.ReadDir(dir);if err!=nil||len(entries)!=0 {t.Fatalf("files: %v %v",entries,err)}
}
`
	dir := permissionLegacyPackage(t, source)
	cmd := exec.Command("go", "test", "-mod=readonly", ".", "-v", "-count=1", "-run", "^TestPinned$")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local", "GOWORK=off", "PERMISSION_PREFLIGHT="+string(preflight))
	output, err := cmd.CombinedOutput()
	t.Logf("pinned router:\n%s", output)
	if err != nil {
		t.Fatalf("pinned router: %v", err)
	}
}
