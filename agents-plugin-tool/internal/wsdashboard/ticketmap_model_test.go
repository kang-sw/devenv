package wsdashboard

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestTicketMapModel runs the ticket map's pure-logic node tests
// (testdata/ticketmap-*.test.cjs): prefs, the stored view, the derived
// relation map, search, history, the Esc layering, the preview renderer,
// and the handle geometry, each loaded from its asset through vm.
func TestTicketMapModel(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; run node --test testdata/ticketmap-*.test.cjs locally")
	}
	files, err := filepath.Glob("testdata/ticketmap-*.test.cjs")
	if err != nil || len(files) < 2 {
		t.Fatalf("ticket map node tests = %v, %v; want the state and hier suites", files, err)
	}
	out, err := exec.Command(node, append([]string{"--test"}, files...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("ticket map model tests failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "# fail 0") && !strings.Contains(string(out), "ℹ fail 0") {
		t.Fatalf("ticket map model tests reported failures:\n%s", out)
	}
}
