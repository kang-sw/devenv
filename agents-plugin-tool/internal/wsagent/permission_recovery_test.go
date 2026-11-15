package wsagent

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

// Admission failures must still block new launches, but cannot orphan a lead
// already running under its holder when a replacement daemon rejoins it.
func TestPermissionAttachAdmissionAdvisories(t *testing.T) {
	for _, failure := range []string{"corrupt-selection", "root-key"} {
		t.Run(failure, func(t *testing.T) {
			h := permissionHarness(t)
			keys := &permissionRootKeys{fakeLeadKeys: h.keys}
			h.opts.LeadKeys = keys
			h.start()
			st := h.lead(BackendClaude, LeadOptions{PermissionLevel: PermissionFull, PermissionFullAck: true, permissionUser: true})
			h.send(st.ID, "original input")
			holder := h.holder(st.ID)
			h.holders.out(holder, initLine("advisory-rejoin", "bypassPermissions"), stateLine(HarnessRunning))
			h.sync(st.ID, holder)
			original := h.userInputs(holder)
			h.runner.Close()
			h.holders.restart()
			path := filepath.Join(wsdashboard.SessionDir(h.dir, st.ID), "permission.json")
			corrupt := []byte(`{"permission_level":"broken"}`)
			wantDiagnostic := "root identity unavailable"
			if failure == "corrupt-selection" {
				if err := os.WriteFile(path, corrupt, 0o600); err != nil {
					t.Fatal(err)
				}
				wantDiagnostic = "invalid permission_level"
			} else {
				keys.reject = true
			}
			h.start()
			h.sync(st.ID, holder)
			permissionRejoined(h, st.ID)
			if got := h.state(st.ID); !got.Process || got.Status != StatusRunning || got.ActiveMode != "bypassPermissions" {
				t.Fatalf("admission failure orphaned held process: %+v", got)
			}
			if !strings.Contains(h.log.String(), wantDiagnostic) {
				t.Fatalf("advisory missing: %s", h.log.String())
			}
			// No selection can be safely reconstructed from corrupt bytes. Rejoin
			// observes the existing mode, but must not deliver or repair a target.
			h.holders.out(holder, resultLine(), stateLine(HarnessIdle))
			h.idle(st.ID)
			if failure == "corrupt-selection" {
				if modes := permissionDeliveredModes(h, holder); len(modes) != 0 {
					t.Fatalf("corruption delivered a mode: %v", modes)
				}
				got := h.state(st.ID)
				if !strings.Contains(got.Error, wantDiagnostic) || got.PermissionLevel != "" || got.PermissionSource != "" {
					t.Fatalf("corruption silently fell back: %+v", got)
				}
				if _, err := h.runner.SetPermissionLevel(st.ID, PermissionReadOnly); err == nil {
					t.Fatal("corrupt selection was accepted for mutation")
				}
				raw, err := os.ReadFile(path)
				if err != nil || string(raw) != string(corrupt) {
					t.Fatalf("corrupt selection was repaired: %s (%v)", raw, err)
				}
			}
			if h.holders.opCount("launch") != 1 || h.holders.opCount("stop") != 0 || h.holder(st.ID) != holder || !slices.Equal(h.userInputs(holder), original) {
				t.Fatalf("rejoin duplicated process/input: %v; %q", h.holders.opList(), h.userInputs(holder))
			}
		})
	}
}

func TestPermissionStatePreservesExistingError(t *testing.T) {
	h := permissionHarness(t)
	r := NewRunner(h.opts)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "permission.json"), []byte(`{"permission_level":"broken"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, existing := range []string{"", "backend process exited unexpectedly"} {
		got := r.permissionState(State{Error: existing}, Record{Lead: true, Backend: BackendClaude}, dir)
		if !strings.Contains(got.Error, "invalid permission_level") || (existing != "" && !strings.Contains(got.Error, existing)) {
			t.Fatalf("lost diagnostic with existing %q: %+v", existing, got)
		}
	}
	worker := State{Error: "worker failure"}
	if got := r.permissionState(worker, Record{Backend: BackendClaude}, dir); got.Error != worker.Error {
		t.Fatalf("worker diagnostic changed: %+v", got)
	}
}
