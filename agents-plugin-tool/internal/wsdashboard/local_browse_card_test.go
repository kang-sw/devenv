package wsdashboard

import (
	"os/exec"
	"strings"
	"testing"
)

func TestHoldersLocalBrowseAnchorCard(t *testing.T) {
	if !strings.Contains(mustAsset(t, "dashboard.js"), `fetch("/s/`+LocalBrowseAnchor+`/terminals"`) {
		t.Fatal("holders card does not read the stable anchor's unfiltered terminal list")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	out, err := exec.Command(node, "testdata/holders-anchor.test.cjs").CombinedOutput()
	if err != nil {
		t.Fatalf("holders anchor client: %v\n%s", err, out)
	}
	t.Log(string(out))
}
