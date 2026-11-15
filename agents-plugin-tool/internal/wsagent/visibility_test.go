package wsagent

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

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
