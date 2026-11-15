package wsagent

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsholder"
)

// Match Manager.Stop: an existing holder is unknown until its first scan.
// The general fake can stop an unscanned holder, which masks this crash window.
type archiveScanHolders struct{ *fakeHolders }

func (f archiveScanHolders) Stop(id string) error {
	f.mu.Lock()
	h := f.holders[id]
	unknown := h == nil || !h.known
	f.mu.Unlock()
	if unknown {
		return wsholder.ErrUnknownHolder
	}
	return f.fakeHolders.Stop(id)
}

func TestArchivedLeadRetainsHolderFenceThroughFirstScanStopFailure(t *testing.T) {
	h := newLeadHarness(t)
	h.opts.Holders = archiveScanHolders{h.holders}
	h.start()
	id := h.lead(BackendClaude, LeadOptions{}).ID
	h.send(id, "first")
	holder := h.holder(id)
	h.holders.out(holder, turnLines("sess-1", "u1", "done")...)
	h.idle(id)
	h.runner.Close()
	// Crash after the sidecar commit but before the holder stopped, or an
	// older runtime that ignores the retained sidecar and starts the process.
	if err := wsdashboard.WriteSessionHidden(h.dir, id, true); err != nil {
		t.Fatal(err)
	}
	h.holders.restart()
	h.holders.set(func() { h.holders.stopErr = errors.New("stop refused") })
	entered, release := h.gateScan()
	defer release()
	h.start()
	wait(t, "gated archive scan", entered)
	if got := h.record(id).HolderID; got != holder {
		t.Fatalf("archive forgot unscanned holder: got %q, want %q", got, holder)
	}
	if _, err := h.runner.SetHidden(id, true); !isConflict(err) {
		t.Fatalf("hide during scan = %v, want retryable conflict", err)
	}
	// An existing Stop control must not turn pre-scan ErrUnknownHolder into
	// evidence the restored process is gone either.
	if _, err := h.runner.Stop(id); err != nil {
		t.Fatal(err)
	}
	if got := h.record(id).HolderID; got != holder {
		t.Fatalf("pre-scan Stop forgot holder: %q", got)
	}
	release()
	eventually(t, "failed archive stop after scan", func() bool {
		return strings.Contains(h.state(id).Error, "stop refused") && h.holders.opCount("stop "+holder) > 0
	})
	h.quiesce(id)
	if got := h.record(id).HolderID; got != holder {
		t.Fatalf("stop failure lost holder %q: %q", holder, got)
	}
	if _, err := h.runner.SetHidden(id, false); err != nil {
		t.Fatal(err)
	}
	h.send(id, "must not duplicate")
	h.quiesce(id)
	if h.holders.opCount("launch") != 1 || h.record(id).HolderID != holder {
		t.Fatalf("unhide bypassed retained-holder fence: %v, %+v", h.holders.opList(), h.record(id))
	}
	// Once a real stop succeeds, the ordinary queued-input path may resume.
	h.holders.set(func() { h.holders.stopErr = nil })
	h.runner.InboxChanged(id)
	eventually(t, "resume after successful holder stop", func() bool { return h.holders.opCount("launch") == 2 })
	if h.holder(id) == holder {
		t.Fatal("resumed on the failed old holder")
	}
}

func TestArchivedLeadHideStopFailureRestoresVisibilityAndRetainsFence(t *testing.T) {
	h := newLeadHarness(t).start()
	id := h.lead(BackendClaude, LeadOptions{}).ID
	h.send(id, "first")
	holder := h.holder(id)
	h.holders.out(holder, turnLines("sess-1", "u1", "done")...)
	h.idle(id)
	h.holders.set(func() { h.holders.stopErr = errors.New("stop refused") })
	out := h.route(localBrowser, http.MethodPost, "/agents/"+id+"/hide", nil)
	if out.status != http.StatusInternalServerError || !strings.Contains(out.body, "not archived") {
		t.Fatalf("actual holder-stop failure = %d %s", out.status, out.body)
	}
	if hidden, err := wsdashboard.ReadSessionHidden(h.dir, id); err != nil || hidden {
		t.Fatalf("failed archive visibility = %v, %v", hidden, err)
	}
	if st := h.state(id); st.Hidden || st.Lifecycle != wsdashboard.SessionAsleep {
		t.Fatalf("failed archive state = %+v", st)
	}
	h.send(id, "must not duplicate")
	if h.holders.opCount("launch") != 1 || h.record(id).HolderID != holder {
		t.Fatalf("failed stop lost holder fence: %v, %+v", h.holders.opList(), h.record(id))
	}
}

func TestArchivedLeadPassiveInputAndMailStayQueued(t *testing.T) {
	for _, kind := range []string{"message", "mail"} {
		t.Run(kind, func(t *testing.T) {
			h := newLeadHarness(t).start()
			id := h.lead(BackendClaude, LeadOptions{}).ID
			key := h.record(id).WSKey
			if _, err := h.runner.SetHidden(id, true); err != nil {
				t.Fatal(err)
			}
			if kind == "message" {
				h.send(id, "passive message")
			} else {
				h.mail.deliver(key, "passive mail")
			}
			h.mailSettled(id)
			c := h.runner.conv(id)
			c.do(func() { c.inboxChanged(); c.mailChanged() })
			if st := h.state(id); !st.Hidden || st.Process {
				t.Fatalf("archived input woke session: %+v", st)
			}
			if ops := h.holders.opList(); len(ops) != 0 {
				t.Fatalf("archived launch: %v", ops)
			}
			if kind == "mail" && !slices.Equal(h.mail.waiting(key), []string{"passive mail"}) {
				t.Fatalf("mail was claimed while archived: %q", h.mail.waiting(key))
			}
			h.restart()
			h.mailSettled(id)
			if st := h.state(id); !st.Hidden || st.Process {
				t.Fatalf("restart lost archive gate: %+v", st)
			}
			if _, err := h.runner.SetHidden(id, false); err != nil {
				t.Fatal(err)
			}
			eventually(t, "queued input wakes after unhide", func() bool { return h.state(id).Process })
			got := h.userInputs(h.holder(id))
			if len(got) != 1 || !strings.Contains(got[0], "passive "+kind) {
				t.Fatalf("queued input = %q", got)
			}
			if len(h.mail.waiting(key)) != 0 {
				t.Fatal("unhidden mail remains queued")
			}
		})
	}
}

func TestArchivedLeadManualUnhideSleepsAndPageInputWakes(t *testing.T) {
	h := newLeadHarness(t).start()
	id := h.lead(BackendClaude, LeadOptions{}).ID
	if _, err := h.runner.SetHidden(id, true); err != nil {
		t.Fatal(err)
	}
	if _, err := h.runner.SetHidden(id, false); err != nil {
		t.Fatal(err)
	}
	h.mailSettled(id)
	h.runner.conv(id).do(func() {})
	if st := h.state(id); st.Hidden || st.Process {
		t.Fatalf("manual unhide launched an empty interaction: %+v", st)
	}
	h.send(id, "next message")
	eventually(t, "visible asleep message wake", func() bool { return h.state(id).Process })
	if _, err := h.runner.SetHidden(id, true); err != nil {
		t.Fatal(err)
	}
	if st := h.state(id); !st.Hidden || st.Process {
		t.Fatalf("hide did not stop: %+v", st)
	}
	h.send(id, "own page") // held first, as the dashboard composer writes it
	if err := h.runner.PageInput(id); err != nil {
		t.Fatal(err)
	}
	eventually(t, "own-page wake", func() bool { st := h.state(id); return !st.Hidden && st.Process })
	if got := h.userInputs(h.holder(id)); len(got) != 1 || !strings.Contains(got[0], "own page") {
		t.Fatalf("own-page turn = %q", got)
	}
}

func TestArchivedLeadRestoresOnReopenAndReadFailureBlocksWake(t *testing.T) {
	h := newLeadHarness(t).start()
	id := h.lead(BackendClaude, LeadOptions{}).ID
	if _, err := h.runner.SetHidden(id, true); err != nil {
		t.Fatal(err)
	}
	if _, err := h.runner.CloseConversation(id); err != nil {
		t.Fatal(err)
	}
	h.restart()
	if st, err := h.runner.Reopen(id); err != nil || !st.Hidden || st.Process {
		t.Fatalf("reopen = %+v, %v", st, err)
	}
	if _, err := h.runner.Restart(id); !isConflict(err) {
		t.Fatalf("restart bypassed archive gate: %v", err)
	}
	h.runner.Close()
	file := filepath.Join(wsdashboard.SessionDir(h.dir, id), wsdashboard.SessionVisibilityFile)
	if err := os.WriteFile(file, []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.start()
	h.send(id, "must wait")
	h.runner.conv(id).do(func() {})
	if st := h.state(id); !st.Hidden || st.Process || !strings.Contains(st.Error, "visibility") {
		t.Fatalf("bad sidecar failed open: %+v", st)
	}
}

func TestArchiveRoutesAreTrustedAndHostedOnly(t *testing.T) {
	h := newLeadHarness(t).start()
	id := h.lead(BackendClaude, LeadOptions{}).ID
	for _, action := range []string{"hide", "unhide"} {
		for _, who := range []viewer{lanBrowser, localClient} {
			r := h.route(who, http.MethodPost, "/agents/"+id+"/"+action, nil)
			if r.status != http.StatusNotFound && r.status != http.StatusForbidden {
				t.Fatalf("%+v %s = %d", who, action, r.status)
			}
		}
		r := h.route(localBrowser, http.MethodPost, "/agents/"+id+"/"+action, nil)
		if r.status != http.StatusOK {
			t.Fatalf("trusted %s = %d %s", action, r.status, r.body)
		}
	}
	external := h.terminalSession("externallead", LeadSession{Key: "external-key", Harness: BackendClaude, Session: "sess-1", Cwd: h.root}, nil)
	if _, err := h.runner.SetHidden(external.ReplyID, true); err == nil {
		t.Fatal("external row archived")
	}
	if got := h.row(external.ReplyID); got != external {
		t.Fatal("external ownership changed")
	}
}
