package wsagent

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

func TestRegisteredPermissionModeCreateLeadAdmission(t *testing.T) {
	for _, tt := range []struct {
		name  string
		modes []string
		mode  string
	}{
		{name: "unlisted plan", modes: []string{"review", "run"}, mode: "plan"},
		{name: "nil modes with nonempty request", mode: "review"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := newLeadHarness(t)
			backend := &fakeBackend{}
			h.opts.Backends = map[string]BackendRegistration{
				"fixture": {Backend: backend, LeadProfile: "fixture-lead", PermissionModes: tt.modes},
			}
			h.globalProfile("fixture-lead", `{"backend":"fixture","lead":true}`)
			h.start()

			st, err := h.runner.CreateLead("fixture", h.root, LeadOptions{PermissionMode: tt.mode})
			if !errors.Is(err, ErrInvalidProfile) || requestStatus(err) != http.StatusBadRequest || !strings.Contains(err.Error(), `permission_mode "`+tt.mode+`" must be one of`) {
				t.Fatalf("CreateLead(%q) = %+v, %v; want invalid permission mode (400)", tt.mode, st, err)
			}
			h.keys.mu.Lock()
			minted := h.keys.n
			h.keys.mu.Unlock()
			if st.ID != "" || minted != 0 || len(h.runner.List()) != 0 {
				t.Fatalf("refused creation returned ID %q, minted %d keys, or created a conversation", st.ID, minted)
			}
			if backend.startCount() != 0 || len(h.holders.opList()) != 0 {
				t.Fatalf("refused creation launched a process: backend starts=%d holder ops=%v", backend.startCount(), h.holders.opList())
			}
		})
	}
}

func TestRegisteredPermissionModeSetAdmission(t *testing.T) {
	for _, tt := range []struct {
		name, backend, profile  string
		modes                   []string
		initial, mode, wantMode string
		wantErr                 error
		wantText                string
	}{
		{
			name: "listed review", backend: "fixture", profile: "fixture-lead",
			modes: []string{"review", "run"}, initial: "run", mode: "review", wantMode: "review",
		},
		{
			name: "unlisted plan", backend: "fixture", profile: "fixture-lead",
			modes: []string{"review", "run"}, initial: "run", mode: "plan", wantMode: "run",
			wantErr: ErrInvalidProfile, wantText: `permission_mode "plan" must be one of review, run`,
		},
		{
			name: "pi unsupported", backend: BackendPi, profile: LeadProfilePi,
			mode: "plan", wantErr: ErrUnsupported, wantText: "the permission mode applies to a supported lead session only",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := newLeadHarness(t)
			backend := &fakeBackend{}
			h.opts.Backends = map[string]BackendRegistration{
				tt.backend: {Backend: backend, LeadProfile: tt.profile, PermissionModes: tt.modes},
			}
			h.globalProfile(tt.profile, `{"backend":"`+tt.backend+`","lead":true}`)
			h.start()
			st := h.lead(tt.backend, LeadOptions{PermissionMode: tt.initial})
			if !st.Lead || st.Backend != tt.backend || st.Profile != tt.profile || st.PermissionMode != tt.initial {
				t.Fatalf("initial lead = %+v", st)
			}

			changed, err := h.runner.SetPermissionMode(st.ID, tt.mode)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("SetPermissionMode(%q) = %+v, %v; want %v", tt.mode, changed, err, tt.wantErr)
			}
			if tt.wantErr != nil {
				if requestStatus(err) != http.StatusBadRequest || !strings.Contains(err.Error(), tt.wantText) {
					t.Fatalf("SetPermissionMode(%q) = %v; want %q (400)", tt.mode, err, tt.wantText)
				}
			} else if changed.ID != st.ID || changed.PermissionMode != tt.wantMode {
				t.Fatalf("changed lead = %+v; want ID %q and mode %q", changed, st.ID, tt.wantMode)
			}

			h.quiesce(st.ID)
			if got := h.state(st.ID); got.PermissionMode != tt.wantMode || got.Process {
				t.Fatalf("lead after setting %q = %+v; want mode %q and no process", tt.mode, got, tt.wantMode)
			}
			rec, err := readRecord(wsdashboard.SessionDir(h.dir, st.ID))
			if err != nil {
				t.Fatal(err)
			}
			if rec.PermissionMode != tt.wantMode {
				t.Fatalf("recorded mode = %q; want %q", rec.PermissionMode, tt.wantMode)
			}
			if backend.startCount() != 0 || len(h.holders.opList()) != 0 {
				t.Fatalf("setting mode launched a process: backend starts=%d holder ops=%v", backend.startCount(), h.holders.opList())
			}
		})
	}
}
