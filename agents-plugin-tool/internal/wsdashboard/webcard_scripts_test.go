package wsdashboard

import (
	"os/exec"
	"testing"
)

// TestWebCardScripts runs the node cases of the web card's injected script
// (webinject.js: protocol origins, selector, pick mode, console ring) and
// its wrapper relay (webframe.js) against a fake DOM.
func TestWebCardScripts(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; run testdata/webinject.test.cjs and testdata/webframe.test.cjs locally")
	}
	for _, file := range []string{"testdata/webinject.test.cjs", "testdata/webframe.test.cjs"} {
		out, err := exec.Command(node, file).CombinedOutput()
		if err != nil {
			t.Errorf("%s failed: %v\n%s", file, err, out)
		}
	}
}
