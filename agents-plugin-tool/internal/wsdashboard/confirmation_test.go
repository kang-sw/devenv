package wsdashboard

import (
	"io/fs"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

func TestDashboardConfirmation(t *testing.T) {
	nativeConfirm := regexp.MustCompile(`\bconfirm\s*\(`)
	files, err := fs.Glob(dashboardFS, "assets/*.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if nativeConfirm.MatchString(mustAsset(t, strings.TrimPrefix(f, "assets/"))) {
			t.Errorf("%s still uses native confirmation", f)
		}
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; run node --test testdata/confirmation.test.cjs locally")
	}
	out, err := exec.Command(node, "--test", "testdata/confirmation.test.cjs").CombinedOutput()
	if err != nil {
		t.Fatalf("confirmation cases failed: %v\n%s", err, out)
	}
	t.Log(string(out))
}
