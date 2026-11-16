package wsdashboard

import (
	"os/exec"
	"strings"
	"testing"
)

// The node-vm fixture instruments the real autosize setup and input listener.
// It forbids visible-field geometry reads, checks the CSS-sized paths never
// schedule measurements, and exercises the older-textarea frame fallback.
func TestComposerAutosizeClient(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; run testdata/composer-autosize.test.cjs locally")
	}
	out, err := exec.Command(node, "testdata/composer-autosize.test.cjs").CombinedOutput()
	if err != nil {
		t.Fatalf("composer autosize cases: %v\n%s", err, out)
	}
	t.Log(string(out))
}

// Placeholder swaps, font loading, and width changes still resize the legacy
// textarea. CSS-sized editors follow those changes without script measurement.
func TestComposerRegrowsOnPlaceholderAndWidth(t *testing.T) {
	data, err := dashboardTestAsset("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	for _, want := range []string{
		"t.placeholder = HINT[inputMode]; growCtext();",
		`wsT("js.cmd.args", cmdSel.name) : ctextHint);` + "\n    grow();",
		"if (w === ctextW) return;",
		"doc.fonts.ready.then(growCtext)",
		"if (ctext && !ctext.wsc && !ctextFieldSizing)",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("dashboard.js lacks %q", want)
		}
	}
	css, err := dashboardTestAsset("assets/dashboard.css")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(css), ".chl.chm {") {
		t.Error("dashboard.css lacks the .chl.chm measuring rule")
	}
}
