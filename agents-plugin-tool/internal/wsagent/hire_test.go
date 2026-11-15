package wsagent

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

func TestHiringCancellationOnlyRollsBackItsCorrelatedCreation(t *testing.T) {
	h := newLeadHarness(t).start()
	ownerSession := h.lead("pi", LeadOptions{})
	provisional, err := h.runner.CreateHireLead("unfinished", h.root, wsdashboard.HireLaunch{Harness: "pi"})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err = h.runner.CancelHireLead("unfinished"); err != nil {
			t.Fatal(err)
		}
	}
	if h.state(provisional).Status != "closed" {
		t.Fatal("provisional creation was not closed")
	}
	if h.state(ownerSession.ID).Status == "closed" {
		t.Fatal("rollback closed an unrelated owner session")
	}
	if err = h.runner.CancelHireLead("not-created"); err != nil {
		t.Fatal(err)
	}
}

func TestHiringCreationUsesOwnerLeadAndSurvivesRetry(t *testing.T) {
	h := newLeadHarness(t).start()
	requester := h.lead("pi", LeadOptions{})
	launch := wsdashboard.HireLaunch{Harness: "pi", Model: "owner-model", Effort: "high", Label: "hired peer"}
	var wg sync.WaitGroup
	ids := make(chan string, 8)
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := h.runner.CreateHireLead("proposal-one", h.root, launch)
			ids <- id
			errs <- err
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var id string
	for got := range ids {
		if id != "" && id != got {
			t.Fatalf("duplicate sessions: %s / %s", id, got)
		}
		id = got
	}
	rec, err := ReadRecord(h.dir, id)
	if err != nil {
		t.Fatal(err)
	}
	if !rec.Lead || rec.Parent != nil || rec.Root != h.root || rec.Model != "owner-model" || rec.Effort != "high" || rec.WSKey == "" {
		t.Fatalf("not owner-created lead authority: %+v", rec)
	}
	meta, err := wsdashboard.ReadHiringMetadata(h.dir, id)
	if err != nil {
		t.Fatal(err)
	}
	if !meta.Hired || meta.Enabled || meta.RequestID != "proposal-one" {
		t.Fatalf("marker = %+v", meta)
	}
	raw, err := os.ReadFile(filepath.Join(wsdashboard.SessionDir(h.dir, id), "agent.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "proposal-one") || strings.Contains(string(raw), "hired_by") {
		t.Fatalf("hire relation leaked into runner record: %s", raw)
	}
	// A legacy typed writer changes the runner record without losing metadata.
	if err = writeRecord(wsdashboard.SessionDir(h.dir, id), rec); err != nil {
		t.Fatal(err)
	}
	h.restart()
	again, err := h.runner.CreateHireLead("proposal-one", h.root, launch)
	if err != nil || again != id {
		t.Fatalf("restart retry: %s %v", again, err)
	}
	// Closing the requester cannot cascade into this peer: no ParentRef was
	// stored. The peer's own controls also remain owner-gated.
	if _, err = h.runner.CloseConversation(requester.ID); err != nil {
		t.Fatal(err)
	}
	if peer := h.state(id); peer.Status == "closed" {
		t.Fatal("requester close cascaded into hired peer")
	}
	out := h.route(localClient, "POST", "/agents/"+id+"/stop", map[string]any{})
	if out.status != 403 {
		t.Fatalf("requester stop without owner Origin: %+v", out)
	}
	out = h.route(localClient, "POST", "/agents/"+id+"/close", map[string]any{})
	if out.status != 403 {
		t.Fatalf("requester close without owner Origin: %+v", out)
	}
}
