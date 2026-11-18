package wsdashboard

import (
	"os/exec"
	"testing"
)

// TestAgentsIslandBehavior runs the agents island's node cases: compact rows
// projected from the tree's hidden roster with the full panel's phase pill,
// Escape collapsing even a pinned island while an outside click respects the
// pin, and the drawer's approval signal leaving that roster out.
func TestAgentsIslandBehavior(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; run testdata/agents-island.test.cjs locally")
	}
	out, err := exec.Command(node, "testdata/agents-island.test.cjs").CombinedOutput()
	if err != nil {
		t.Fatalf("agents island cases failed: %v\n%s", err, out)
	}
}
