package wsdashboard

import (
	"os/exec"
	"testing"
)

func TestShellPostLayout(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; run testdata/shell-post-layout.test.cjs locally")
	}
	out, err := exec.Command(node, "testdata/shell-post-layout.test.cjs").CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("shell post layout: %v", err)
	}
}
