package wsagent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

// No runner loop or daemon is needed to inspect the admitted choices.
func registrationChoicesSupport(t *testing.T, backends map[string]BackendRegistration) *Support {
	t.Helper()
	dir, root := t.TempDir(), t.TempDir()
	r := NewRunner(Options{Dir: dir, Config: testConfig(t), Backends: backends})
	return NewSupport(SupportOptions{
		Runner: r, Dir: dir,
		StoreRoot: func(string) (string, error) { return root, nil },
		Now:       func() time.Time { return time.Unix(1700000000, 0) },
	})
}

func TestRegistrationChoicesMetadataAndJSON(t *testing.T) {
	stubLookPath(t)
	for _, piLead := range []bool{true, false} {
		name := "pi without lead"
		piProfile := ""
		if piLead {
			name, piProfile = "pi with lead", LeadProfilePi
		}
		t.Run(name, func(t *testing.T) {
			s := registrationChoicesSupport(t, map[string]BackendRegistration{
				BackendClaude: {Backend: &fakeBackend{}, LeadProfile: LeadProfileClaude, PermissionModes: LeadPermissionModes},
				BackendPi:     {Backend: &fakeBackend{}, LeadProfile: piProfile},
			})
			ans := s.Choices(context.Background())
			claude := harnessByName(t, ans, BackendClaude)
			pi := harnessByName(t, ans, BackendPi)
			if !claude.Lead || !reflect.DeepEqual(claude.PermissionModes, LeadPermissionModes) {
				t.Fatalf("claude metadata = %+v", claude)
			}
			if pi.Lead != piLead || pi.PermissionModes != nil {
				t.Fatalf("pi metadata = %+v, want lead=%v and nil permission modes", pi, piLead)
			}
			raw, err := json.Marshal(ans)
			if err != nil {
				t.Fatal(err)
			}
			var wire struct {
				Harnesses []map[string]json.RawMessage `json:"harnesses"`
			}
			if err := json.Unmarshal(raw, &wire); err != nil {
				t.Fatal(err)
			}
			if len(wire.Harnesses) != 2 {
				t.Fatalf("choices JSON = %s", raw)
			}
			if string(wire.Harnesses[0]["lead"]) != "true" {
				t.Fatalf("claude lead missing from %s", raw)
			}
			var modes []string
			if err := json.Unmarshal(wire.Harnesses[0]["permission_modes"], &modes); err != nil || !reflect.DeepEqual(modes, LeadPermissionModes) {
				t.Fatalf("permission_modes = %v, %v; JSON = %s", modes, err, raw)
			}
			if _, ok := wire.Harnesses[1]["permission_modes"]; ok {
				t.Fatalf("nil permission modes must be omitted: %s", raw)
			}
			if value, ok := wire.Harnesses[1]["lead"]; ok != piLead || (ok && string(value) != "true") {
				t.Fatalf("false lead must be omitted, true lead retained: %s", raw)
			}
		})
	}
}

func TestRegistrationChoicesAvailabilityUsesBackend(t *testing.T) {
	stubLookPath(t, "missing-fixture")
	listed := &fakeBackend{exe: "listed-fixture"}
	missing := &fakeBackend{exe: "missing-fixture"}
	// Expose only Backend's method set, not fakeBackend's ChoiceLister.
	bare := struct{ Backend }{Backend: &fakeBackend{exe: "missing-fixture"}}
	for _, tt := range []struct {
		name      string
		backend   Backend
		available bool
		listed    bool
		errorText string
	}{
		{"lister installed", listed, true, true, ""},
		{"lister missing", missing, false, false, "missing-fixture is not installed:"},
		{"no lister", bare, true, false, ""},
		{"nil backend", nil, false, false, "is not available in this daemon"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := registrationChoicesSupport(t, map[string]BackendRegistration{
				BackendClaude: {Backend: tt.backend, LeadProfile: LeadProfileClaude},
				"fixture":     {Backend: tt.backend, LeadProfile: "fixture-lead"},
			})
			ans := s.Choices(context.Background())
			for _, name := range []string{BackendClaude, "fixture"} {
				h := harnessByName(t, ans, name)
				if h.Available != tt.available || !h.Lead {
					t.Fatalf("%s availability/lead = %+v", name, h)
				}
				if (tt.errorText == "" && h.Error != "") || (tt.errorText != "" && !strings.Contains(h.Error, tt.errorText)) {
					t.Fatalf("%s error = %q, want %q", name, h.Error, tt.errorText)
				}
				wantModels, wantEfforts := []ModelChoice{}, []string{}
				if tt.listed {
					wantModels = []ModelChoice{{ID: "listed-fixture-model"}}
					wantEfforts = []string{"low", "high"}
				}
				if !reflect.DeepEqual(h.Models, wantModels) || !reflect.DeepEqual(h.Efforts, wantEfforts) {
					t.Fatalf("%s choices = %+v, want models %v efforts %v", name, h, wantModels, wantEfforts)
				}
			}
		})
	}
}

func TestRegistrationChoicesOrderingAndSupportDefaults(t *testing.T) {
	for _, tt := range []struct {
		name    string
		missing []string
		last    *Selection
		want    Selection
	}{
		{name: "claude first", want: Selection{Harness: BackendClaude}},
		{name: "pi fallback", missing: []string{"fake-claude"}, want: Selection{Harness: BackendPi}},
		{name: "lead only is not a fallback", missing: []string{"fake-claude", "fake-pi"}},
		{name: "last support selection", last: &Selection{Harness: BackendPi, Model: "chosen-model", Effort: "high"}, want: Selection{Harness: BackendPi, Model: "chosen-model", Effort: "high"}},
		{name: "unavailable last resets settings", missing: []string{"fake-pi"}, last: &Selection{Harness: BackendPi, Model: "old-model", Effort: "high"}, want: Selection{Harness: BackendClaude}},
		{name: "lead only last is ignored", last: &Selection{Harness: "alpha", Model: "old-model"}, want: Selection{Harness: BackendClaude}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stubLookPath(t, tt.missing...)
			s := registrationChoicesSupport(t, map[string]BackendRegistration{
				"zeta":        {Backend: &fakeBackend{}, LeadProfile: "zeta-lead"},
				BackendPi:     {Backend: &fakeBackend{exe: "fake-pi"}, LeadProfile: LeadProfilePi},
				"worker-only": {Backend: &fakeBackend{}},
				"alpha":       {Backend: &fakeBackend{}, LeadProfile: "alpha-lead"},
				BackendClaude: {Backend: &fakeBackend{exe: "fake-claude"}, LeadProfile: LeadProfileClaude},
			})
			s.state.Last = tt.last
			ans := s.Choices(context.Background())
			var names []string
			for _, h := range ans.Harnesses {
				names = append(names, h.Harness)
				if h.Harness == "alpha" || h.Harness == "zeta" {
					if h.Profile != "" || !h.Lead || !h.Available {
						t.Fatalf("lead-only choice = %+v", h)
					}
				}
			}
			if !reflect.DeepEqual(names, []string{BackendClaude, BackendPi, "alpha", "zeta"}) || ans.Last != tt.want {
				t.Fatalf("choices order %v, last %+v; want claude/pi/alpha/zeta, %+v", names, ans.Last, tt.want)
			}
			for _, name := range []string{"alpha", "zeta", "worker-only"} {
				_, err := s.Select("store", selectRequest{Harness: strp(name)})
				var re *requestError
				if !errors.As(err, &re) || re.status != http.StatusBadRequest || re.msg != "unknown support harness \""+name+"\"" {
					t.Fatalf("Select(%s) = %v, want unknown support harness (400)", name, err)
				}
			}
			if got := s.Choices(context.Background()).Last; got != tt.want {
				t.Fatalf("refused selection changed default to %+v", got)
			}
			if len(s.opts.Runner.List()) != 0 {
				t.Fatal("inspecting or refusing a choice created a conversation")
			}
		})
	}
}

func TestRegistrationChoicesBackendRegistrationsDefensiveCopies(t *testing.T) {
	backend := &fakeBackend{}
	r := NewRunner(Options{Backends: map[string]BackendRegistration{
		"fixture": {
			Backend: backend, LeadProfile: "fixture-lead",
			PermissionModes:  []string{"review", "run"},
			PermissionLevels: []PermissionLevel{PermissionReadOnly},
			DirectTools:      []string{"tickets.query"},
			Profiles: []BuiltinProfileRegistration{{Name: "fixture-worker", Definition: Profile{
				Backend: "fixture", Tools: []string{"read"}, WSTools: []string{"git.status"},
			}}},
		},
		"bare": {Backend: backend},
	}})
	got := r.BackendRegistrations()
	got["fixture"].PermissionModes[0] = "changed"
	got["fixture"].PermissionLevels[0] = PermissionFull
	got["fixture"].DirectTools[0] = "changed"
	got["fixture"].Profiles[0].Name = "changed"
	got["fixture"].Profiles[0].Definition.Tools[0] = "changed"
	got["fixture"].Profiles[0].Definition.WSTools[0] = "changed"
	got["fixture"] = BackendRegistration{LeadProfile: "changed-lead"}
	delete(got, "bare")
	got["injected"] = BackendRegistration{}
	fresh := r.BackendRegistrations()
	fixture := fresh["fixture"]
	if len(fresh) != 2 || fixture.Backend != backend || fixture.LeadProfile != "fixture-lead" ||
		!reflect.DeepEqual(fixture.PermissionModes, []string{"review", "run"}) ||
		!reflect.DeepEqual(fixture.PermissionLevels, []PermissionLevel{PermissionReadOnly}) ||
		!reflect.DeepEqual(fixture.DirectTools, []string{"tickets.query"}) ||
		len(fixture.Profiles) != 1 || fixture.Profiles[0].Name != "fixture-worker" ||
		!reflect.DeepEqual(fixture.Profiles[0].Definition.Tools, []string{"read"}) ||
		!reflect.DeepEqual(fixture.Profiles[0].Definition.WSTools, []string{"git.status"}) {
		t.Fatalf("caller mutations reached registry: %+v", fresh)
	}
	if reg, ok := fresh["bare"]; !ok || reg.Backend != backend || reg.PermissionModes != nil {
		t.Fatalf("bare registration = %+v, present=%v; want shared backend and nil modes", reg, ok)
	}
	fresh["fixture"].PermissionModes[1] = "changed-again"
	if modes := r.BackendRegistrations()["fixture"].PermissionModes; !reflect.DeepEqual(modes, []string{"review", "run"}) {
		t.Fatalf("a later returned slice aliases registry: %v", modes)
	}
}

func TestRegistrationChoicesCreateLeadAdmitsFakeProfile(t *testing.T) {
	for _, tt := range []struct {
		name  string
		modes []string
		mode  string
	}{
		{name: "without permission control"},
		{name: "registered permission control", modes: []string{"review", "run"}, mode: "review"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := newLeadHarness(t)
			backend := &fakeBackend{}
			h.opts.Backends = map[string]BackendRegistration{
				"fixture": {Backend: backend, LeadProfile: "fixture-lead", PermissionModes: tt.modes},
			}
			// The private global config admits the fake profile without adding
			// a builtin profile, stored field, or real harness implementation.
			h.globalProfile("fixture-lead", `{"backend":"fixture","lead":true,"model":"fixture-model"}`)
			h.start()
			if !h.runner.AdmitsLead("fixture") || h.runner.AdmitsLead("unregistered") {
				t.Fatal("lead admission did not follow the fake registry")
			}
			st, err := h.runner.CreateLead("fixture", h.root, LeadOptions{PermissionMode: tt.mode})
			if err != nil {
				t.Fatalf("CreateLead(fixture): %v", err)
			}
			if !st.Lead || st.Backend != "fixture" || st.Profile != "fixture-lead" || st.Root != h.root || st.Model != "fixture-model" || st.PermissionMode != tt.mode {
				t.Fatalf("fake lead = %+v", st)
			}
			h.quiesce(st.ID)
			rec := h.record(st.ID)
			if !rec.Lead || rec.Backend != "fixture" || rec.Profile != "fixture-lead" || rec.WSKey != "lead-key-1" || rec.PermissionMode != tt.mode {
				t.Fatalf("fake lead record = %+v", rec)
			}
			if backend.startCount() != 0 || len(h.holders.opList()) != 0 {
				t.Fatalf("admission launched a process: backend starts=%d holder ops=%v", backend.startCount(), h.holders.opList())
			}
		})
	}
}
