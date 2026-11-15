package wsdashboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestBoardTopicUnitsBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE for Chromium acceptance")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	f := startPageFixture(t)
	first, next := topicUnitPage(t, 112, false), topicUnitPage(t, 112, true)
	set := func(page PageContent) {
		f.src.setPage("boardsession", page)
		f.d.index.Nudge("boardsession", f.project)
		f.d.index.Settle()
	}
	set(first)
	updates := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { set(next); w.WriteHeader(http.StatusNoContent) }))
	defer updates.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "testdata/board-units.browser.cjs", f.base, updates.URL)
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("Chromium board units: %v", err)
	}
}
