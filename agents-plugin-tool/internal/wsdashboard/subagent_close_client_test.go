package wsdashboard

import (
	"os/exec"
	"testing"
)

func TestSubagentCloseClient(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	out, err := exec.Command(node, "--test", "testdata/subagent-close.test.cjs").CombinedOutput()
	if err != nil {
		t.Fatalf("subagent close client: %v\n%s", err, out)
	}
	t.Log(string(out))
}
