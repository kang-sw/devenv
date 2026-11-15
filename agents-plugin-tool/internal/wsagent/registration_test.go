package wsagent

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestPermissionModesLegacyFallback(t *testing.T) {
	registry := map[string]BackendRegistration{
		BackendClaude: {PermissionModes: []string{"review"}},
		BackendPi:     {PermissionModes: []string{"pi-native"}},
		"fixture":     {PermissionModes: []string{"run"}},
		"empty":       {PermissionModes: []string{}},
	}
	for _, tt := range []struct {
		name, backend string
		registries    []map[string]BackendRegistration
		want          []string
	}{
		{"standalone claude", BackendClaude, nil, LeadPermissionModes},
		{"standalone pi", BackendPi, nil, LeadPermissionModes},
		{"standalone extension", "fixture", nil, nil},
		{"registered claude", BackendClaude, []map[string]BackendRegistration{registry}, []string{"review"}},
		{"registered pi retains legacy modes", BackendPi, []map[string]BackendRegistration{registry}, LeadPermissionModes},
		{"registered extension", "fixture", []map[string]BackendRegistration{registry}, []string{"run"}},
		{"empty extension modes", "empty", []map[string]BackendRegistration{registry}, []string{}},
		{"absent extension", "absent", []map[string]BackendRegistration{registry}, nil},
		{"nil registry claude", BackendClaude, []map[string]BackendRegistration{nil}, nil},
		{"nil registry pi", BackendPi, []map[string]BackendRegistration{nil}, LeadPermissionModes},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := permissionModes(tt.backend, tt.registries); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("permissionModes(%q) = %v; want %v", tt.backend, got, tt.want)
			}
		})
	}
}

func TestRegisteredBackendAdmission(t *testing.T) {
	backends := map[string]BackendRegistration{"fixture": {Backend: &fakeBackend{}, LeadProfile: "fixture-lead", PermissionModes: []string{"review"}}}
	for _, raw := range []string{`{"backend":"fixture"}`, `{"backend":"fixture","lead":true,"permission_mode":"review"}`} {
		if _, err := ParseProfile(json.RawMessage(raw), backends); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ParseProfile(json.RawMessage(`{"backend":"unregistered"}`), backends); err == nil {
		t.Fatal("unregistered backend admitted")
	}
	if _, err := ParseProfile(json.RawMessage(`{"backend":"fixture","unknown":true}`), backends); err == nil {
		t.Fatal("unknown stored field admitted")
	}
	if got := LeadProfileFor("fixture", backends); got != "fixture-lead" {
		t.Fatalf("lead profile = %q", got)
	}
	schema := ProfileValueSchema(backends)["properties"].(map[string]any)["backend"].(map[string]any)["enum"]
	if !reflect.DeepEqual(schema, []string{"fixture"}) {
		t.Fatalf("backend enum = %v", schema)
	}
	modes := ProfileValueSchema(backends)["properties"].(map[string]any)["permission_mode"].(map[string]any)["enum"]
	if !reflect.DeepEqual(modes, []string{"review"}) {
		t.Fatalf("registered permission enum = %v", modes)
	}
	production := map[string]BackendRegistration{
		BackendClaude: {LeadProfile: LeadProfileClaude, PermissionModes: LeadPermissionModes},
		BackendPi:     {LeadProfile: LeadProfilePi},
	}
	if !reflect.DeepEqual(ProfileValueSchema(), ProfileValueSchema(production)) {
		t.Fatal("production profile schema changed from the builtin-only contract")
	}
}

func TestRegisteredLeadOverrideChecksResolvedBackend(t *testing.T) {
	for _, tt := range []struct {
		name, backend, leadProfile string
		want                       bool
	}{
		{"worker-only override", "fixture", "", false},
		{"lead-capable extension override", "fixture", "fixture-lead", true},
		{"builtin cross-backend override", BackendPi, LeadProfilePi, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := newLeadHarness(t)
			h.opts.Backends[tt.backend] = BackendRegistration{Backend: &fakeBackend{}, LeadProfile: tt.leadProfile}
			h.globalProfile(LeadProfileClaude, `{"backend":"`+tt.backend+`","lead":true}`)
			h.start()
			st, err := h.runner.CreateLead(BackendClaude, h.root, LeadOptions{})
			if tt.want {
				if err != nil || st.Backend != tt.backend || !st.Lead {
					t.Fatalf("eligible override = %+v, %v", st, err)
				}
				h.quiesce(st.ID)
				return
			}
			if !errors.Is(err, ErrInvalidProfile) || requestStatus(err) != 400 || !strings.Contains(err.Error(), `backend "fixture" cannot host a lead profile`) {
				t.Fatalf("worker-only override = %+v, %v", st, err)
			}
			h.keys.mu.Lock()
			minted := h.keys.n
			h.keys.mu.Unlock()
			if minted != 0 || len(h.runner.List()) != 0 || len(h.holders.opList()) != 0 {
				t.Fatalf("refused override minted %d keys or created a conversation/holder", minted)
			}
		})
	}
}

func TestRunnerRegistrationErrorsAtBoundary(t *testing.T) {
	h := newHarness(t, map[string]string{"invalid": `{"backend":"not-registered"}`, "unavailable": `{"backend":"@other"}`})
	launchCause := errors.New("fixture launch failure")
	backend := &fakeBackend{startErr: launchCause}
	r := NewRunner(Options{Backends: map[string]BackendRegistration{"fixture": {Backend: backend}}})
	c := &conv{r: r, rec: Record{Backend: "fixture", Profile: "fixture-profile"}}
	for _, tt := range []struct {
		name   string
		run    func() error
		kind   error
		status int
		text   string
	}{
		{"invalid profile", func() error { _, err := h.runner.Create("invalid", h.root); return err }, ErrInvalidProfile, 400, "backend must be"},
		{"unavailable backend", func() error { _, err := h.runner.Create("unavailable", h.root); return err }, ErrBackendUnavailable, 400, "is not available in this daemon"},
		{"launch failure", func() error { _, err := c.startBackend(StartSpec{}); return err }, ErrLaunchFailed, 500, launchCause.Error()},
		{"absent mode", func() error { return controlsFor(&fakeProc{}).setPermissionMode("review") }, ErrUnsupported, 500, ErrUnsupported.Error()},
		{"absent model form", func() error { return controlsFor(&piProcess{}).SetModel("ambiguous") }, ErrUnsupported, 500, ErrUnsupported.Error()},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			if !errors.Is(err, tt.kind) || !strings.Contains(err.Error(), tt.text) {
				t.Fatalf("error = %v, want %v / %q", err, tt.kind, tt.text)
			}
			if tt.kind == ErrLaunchFailed && !errors.Is(err, launchCause) {
				t.Fatal("launch lost underlying cause")
			}
			w := httptest.NewRecorder()
			writeAnswer(w, 200, nil, err)
			if w.Code != tt.status || w.Body.String() != err.Error()+"\n" {
				t.Fatalf("HTTP = %d %q", w.Code, w.Body.String())
			}
		})
	}
}

func TestRegistrationApprovalAbsenceNeverAnswers(t *testing.T) {
	for _, c := range []*conv{
		{proc: &fakeProc{}, approvals: []ApprovalRequest{{ID: "request-1", ToolUseID: "tool-1"}}},
		{proc: claudeProcess{lead: true}},
		{replaying: true, approvals: []ApprovalRequest{{ID: "request-1"}}},
	} {
		before := append([]ApprovalRequest(nil), c.approvals...)
		err := c.answerApproval("request-1", true, "")
		w := httptest.NewRecorder()
		writeAnswer(w, 200, nil, err)
		if w.Code != http.StatusConflict || !reflect.DeepEqual(before, c.approvals) {
			t.Fatalf("missing capability/request/replay answered a prompt: %d %v", w.Code, err)
		}
	}
}

func TestRunnerErrorKindsPreserveHTTP(t *testing.T) {
	for _, tt := range []struct {
		name   string
		err    error
		kind   error
		status int
		text   string
	}{
		{"unavailable", categorized(ErrBackendUnavailable, badRequest("unavailable")), ErrBackendUnavailable, 400, "unavailable"},
		{"profile", categorized(ErrInvalidProfile, badRequest("profile")), ErrInvalidProfile, 400, "profile"},
		{"launch", categorized(ErrLaunchFailed, errors.New("launch")), ErrLaunchFailed, 500, "launch"},
		{"unsupported", ErrUnsupported, ErrUnsupported, 500, "not supported in-stream"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if !errors.Is(tt.err, tt.kind) {
				t.Fatalf("kind = %v", tt.err)
			}
			w := httptest.NewRecorder()
			writeAnswer(w, http.StatusOK, nil, tt.err)
			if w.Code != tt.status || w.Body.String() != tt.text+"\n" {
				t.Fatalf("HTTP = %d %q", w.Code, w.Body.String())
			}
		})
	}
}
