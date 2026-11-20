package wsagent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

// Admission failures must still block new launches, but cannot orphan a lead
// already running under its holder when a replacement daemon rejoins it.
func TestPermissionAttachAdmissionAdvisories(t *testing.T) {
	for _, failure := range []string{"none", "corrupt-selection", "root-key", "both"} {
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
			corruptSelection := failure == "corrupt-selection" || failure == "both"
			rootFailure := failure == "root-key" || failure == "both"
			if corruptSelection {
				if err := os.WriteFile(path, corrupt, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			keys.reject = rootFailure
			h.start()
			h.sync(st.ID, holder)
			permissionRejoined(h, st.ID)
			if got := h.state(st.ID); !got.Process || got.Status != StatusRunning || got.ActiveMode != "bypassPermissions" {
				t.Fatalf("admission failure orphaned held process: %+v", got)
			}
			got := h.state(st.ID)
			for diagnostic, want := range map[string]bool{
				"invalid permission_level":                                    corruptSelection,
				"rejoin ws session key (advisory): root identity unavailable": rootFailure,
			} {
				if strings.Contains(got.Error, diagnostic) != want {
					t.Fatalf("rejoined state diagnostic %q present = %t, want %t: %+v", diagnostic, strings.Contains(got.Error, diagnostic), want, got)
				}
				if want && !strings.Contains(h.log.String(), diagnostic) {
					t.Fatalf("advisory %q missing: %s", diagnostic, h.log.String())
				}
			}
			if failure == "none" && got.Error != "" {
				t.Fatalf("successful rejoin has an error: %+v", got)
			}
			if failure == "both" {
				wantError := "earlier backend diagnostic; " + got.Error
				c := h.runner.conv(st.ID)
				c.do(func() {
					c.setStatus(func() { c.lastErr = "earlier backend diagnostic" })
					c.reconcileLeadPermission()
				})
				if got := h.state(st.ID); got.Error != wantError {
					t.Fatalf("reconciliation lost the earlier error: %+v", got)
				}
			}
			// No selection can be safely reconstructed from corrupt bytes. Rejoin
			// observes the existing mode, but must not deliver or repair a target.
			h.holders.out(holder, resultLine(), stateLine(HarnessIdle))
			h.idle(st.ID)
			if corruptSelection {
				if modes := permissionDeliveredModes(h, holder); len(modes) != 0 {
					t.Fatalf("corruption delivered a mode: %v", modes)
				}
				got := h.state(st.ID)
				if !strings.Contains(got.Error, "invalid permission_level") || got.PermissionLevel != "" || got.PermissionSource != "" {
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

// advisoryLeadKeys changes the current failure without racing State readers.
type advisoryLeadKeys struct {
	*fakeLeadKeys
	mu     sync.Mutex
	err    error
	checks int
}

func (k *advisoryLeadKeys) EnsureLead(root, key string) error {
	k.mu.Lock()
	k.checks++
	err := k.err
	k.mu.Unlock()
	if err != nil {
		return err
	}
	return k.fakeLeadKeys.EnsureLead(root, key)
}

func (k *advisoryLeadKeys) checkCount() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.checks
}

func (k *advisoryLeadKeys) fail(err error) {
	k.mu.Lock()
	k.err = err
	k.mu.Unlock()
}

func rejoinWithKeyAdvisory(t *testing.T, existing string) (*leadHarness, *advisoryLeadKeys, string, string) {
	t.Helper()
	h := permissionHarness(t)
	keys := &advisoryLeadKeys{fakeLeadKeys: h.keys}
	h.opts.LeadKeys = keys
	h.start()
	st := h.lead(BackendClaude, LeadOptions{})
	h.send(st.ID, "original input")
	holder := h.holder(st.ID)
	h.holders.out(holder, initLine("advisory-rejoin", "auto"), stateLine(HarnessRunning))
	h.sync(st.ID, holder)
	h.runner.Close()
	h.holders.restart()
	if existing != "" {
		rec := h.record(st.ID)
		rec.LastTurn = &PendingReport{Result: TurnResult{Status: TurnFailed, Error: existing}}
		if err := writeRecord(wsdashboard.SessionDir(h.dir, st.ID), rec); err != nil {
			t.Fatal(err)
		}
	}
	keys.fail(errors.New("root identity unavailable"))
	h.start()
	h.sync(st.ID, holder)
	permissionRejoined(h, st.ID)
	if got := h.state(st.ID); !got.Process || got.Status != StatusRunning || !strings.Contains(got.Error, "rejoin ws session key (advisory): root identity unavailable") {
		t.Fatalf("missing running rejoin/advisory: %+v", got)
	}
	return h, keys, st.ID, holder
}

func TestPermissionRejoinAdvisorySurvivesTurnStartsAndClears(t *testing.T) {
	for _, start := range []string{"send", "unsolicited"} {
		t.Run(start, func(t *testing.T) {
			h, keys, id, holder := rejoinWithKeyAdvisory(t, "")
			want := h.state(id).Error
			h.holders.out(holder, resultLine(), stateLine(HarnessIdle))
			h.idle(id)
			if start == "send" {
				h.send(id, "next turn")
			} else {
				h.holders.out(holder, stateLine(HarnessRunning))
				h.sync(id, holder)
			}
			if got := h.state(id); got.Status != StatusRunning || got.Error != want {
				t.Fatalf("turn start cleared a current advisory: %+v; want %q", got, want)
			}
			keys.fail(nil)
			if got := h.state(id); got.Error != "" || !got.Process || got.Status != StatusRunning {
				t.Fatalf("successful EnsureLead did not clear just the advisory: %+v", got)
			}
			checks := keys.checkCount()
			for range 10 {
				h.state(id)
				h.runner.List()
			}
			if got := keys.checkCount(); got != checks {
				t.Fatalf("healthy State touched the key store: %d checks, want %d", got, checks)
			}
		})
	}
}

func TestPermissionRejoinAdvisoryDoesNotMaskProcessExit(t *testing.T) {
	h, _, id, holder := rejoinWithKeyAdvisory(t, "")
	// A clean output close has no backend Fatal event of its own.
	h.holders.exit(holder, 0)
	eventually(t, "the process exit", func() bool { return !h.state(id).Process })
	h.quiesce(id)
	c := h.runner.conv(id)
	c.mu.Lock()
	settled := c.settled
	c.mu.Unlock()
	if settled.Status != TurnFailed || settled.Error != "the backend process exited" {
		t.Fatalf("advisory masked process-exit settlement: %+v", settled)
	}
	if got := h.state(id); !strings.HasPrefix(got.Error, "the backend process exited; rejoin ws session key (advisory):") {
		t.Fatalf("State lost terminal or current advisory: %+v", got)
	}
}

func TestPermissionRejoinCurrentDiagnosticsStayBounded(t *testing.T) {
	for _, cause := range []string{"key", "permission"} {
		t.Run(cause, func(t *testing.T) {
			h, keys, id, _ := rejoinWithKeyAdvisory(t, "")
			c := h.runner.conv(id)
			path := filepath.Join(c.dir, "permission.json")
			for i := range 20 {
				message := "root identity unavailable"
				permissionError := ""
				if cause == "key" {
					message = fmt.Sprintf("root failure %d", i)
					keys.fail(errors.New(message))
				} else {
					// Each invalid JSON token produces a distinct parser error.
					if err := os.WriteFile(path, []byte{byte('A' + i)}, 0o600); err != nil {
						t.Fatal(err)
					}
					c.do(func() { c.reconcileLeadPermission() })
					permissionError = "; " + h.runner.permissionState(State{}, c.record(), c.dir).Error
				}
				want := "rejoin ws session key (advisory): " + message + permissionError
				if got := h.state(id).Error; got != want {
					t.Fatalf("diagnostic %d retained stale errors: %q; want %q", i, got, want)
				}
			}
			if got := c.lastError(); got != "" {
				t.Fatalf("advisory contaminated terminal state: %q", got)
			}
			if cause == "permission" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			keys.fail(nil)
			if got := h.state(id); got.Error != "" {
				t.Fatalf("recovered diagnostics persisted: %+v", got)
			}
		})
	}
}

func TestPermissionRejoinConcurrentStateRefresh(t *testing.T) {
	h, keys, id, _ := rejoinWithKeyAdvisory(t, "")
	var readers sync.WaitGroup
	for range 3 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for range 20 {
				if _, err := h.runner.Get(id); err != nil {
					t.Error(err)
				}
				h.runner.List()
			}
		}()
	}
	for i := range 20 {
		keys.fail(fmt.Errorf("root failure %d", i))
	}
	readers.Wait()
	keys.fail(nil)
	if got := h.state(id).Error; got != "" {
		t.Fatalf("concurrent projection retained the advisory: %q", got)
	}
}

func TestPermissionDiagnosticsDoNotDropDistinctSubstrings(t *testing.T) {
	if got := appendDiagnostic("longer diagnostic", "diagnostic"); got != "longer diagnostic; diagnostic" {
		t.Fatalf("distinct substring diagnostic dropped: %q", got)
	}
}

func TestPermissionAttachPreservesTerminalError(t *testing.T) {
	const terminal = "earlier backend diagnostic"
	h, keys, id, _ := rejoinWithKeyAdvisory(t, terminal)
	c := h.runner.conv(id)
	if got := c.lastError(); got != terminal {
		t.Fatalf("attach changed existing terminal error: %q; want %q", got, terminal)
	}
	want := terminal + "; rejoin ws session key (advisory): root identity unavailable"
	if got := h.state(id).Error; got != want {
		t.Fatalf("attach lost a diagnostic: %q; want %q", got, want)
	}
	keys.fail(nil)
	if got := h.state(id).Error; got != terminal {
		t.Fatalf("successful EnsureLead cleared terminal error: %q", got)
	}
}

func TestPermissionStatePreservesExistingError(t *testing.T) {
	h := permissionHarness(t)
	r := NewRunner(h.opts)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "permission.json"), []byte(`{"permission_level":"broken"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	rec := Record{Lead: true, Backend: BackendClaude}
	diagnostic := r.permissionState(State{}, rec, dir).Error
	for _, existing := range []string{"", "backend process exited unexpectedly", diagnostic, "rejoin ws session key (advisory): unavailable; " + diagnostic} {
		got := r.permissionState(State{Error: existing}, rec, dir)
		if !strings.Contains(got.Error, "invalid permission_level") || (existing != "" && !strings.Contains(got.Error, existing)) {
			t.Fatalf("lost diagnostic with existing %q: %+v", existing, got)
		}
		if strings.Contains(existing, diagnostic) && got.Error != existing {
			t.Fatalf("duplicate diagnostic: %q", got.Error)
		}
	}
	worker := State{Error: "worker failure"}
	if got := r.permissionState(worker, Record{Backend: BackendClaude}, dir); !strings.Contains(got.Error, worker.Error) || !strings.Contains(got.Error, "invalid permission_level") {
		t.Fatalf("worker diagnostic lost admission evidence: %+v", got)
	}
}
