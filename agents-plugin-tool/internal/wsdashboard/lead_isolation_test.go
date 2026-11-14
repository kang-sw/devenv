package wsdashboard

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

func TestLeadPopoverLaunchBehavior(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; run testdata/lead-popover.test.cjs locally")
	}
	out, err := exec.Command(node, "testdata/lead-popover.test.cjs").CombinedOutput()
	if err != nil {
		t.Fatalf("lead popover cases failed: %v\n%s", err, out)
	}
}

// TestLeadPopoverIsolationWiring pins the plugin-isolation control to the
// lead spawn popover: its click handler is bound inside rpLeadMake (where
// the control and P exist), the neighbouring popover makers never name it,
// and the create request carries the pick.
func TestLeadPopoverIsolationWiring(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	js := strings.ReplaceAll(string(data), "\r\n", "\n")
	body := func(name string) string {
		t.Helper()
		m := regexp.MustCompile(`(?s)\n  function ` + name + `\(.*?\n  \}\n`).FindString(js)
		if m == "" {
			t.Fatalf("dashboard.js holds no %s", name)
		}
		return m
	}
	lead := body("rpLeadMake")
	for _, want := range []string{`iseg.addEventListener("click"`, `P.iso = b.getAttribute("data-iso")`, `isolation: P.iso`, `P.iso = "default"`} {
		if !strings.Contains(lead, want) {
			t.Errorf("rpLeadMake lacks %q", want)
		}
	}
	for _, name := range []string{"rpWtMake", "rpWsMake", "rpSessMake"} {
		if b := body(name); strings.Contains(b, "iseg") || strings.Contains(b, "data-iso") {
			t.Errorf("%s names the lead popover's isolation control", name)
		}
	}
}
