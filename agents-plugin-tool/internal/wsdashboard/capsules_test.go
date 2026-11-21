package wsdashboard

import (
	"os/exec"
	"strings"
	"testing"
)

func TestInlineCapsulesBehavior(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; run testdata/capsules.test.cjs locally")
	}
	out, err := exec.Command(node, "testdata/capsules.test.cjs").CombinedOutput()
	if err != nil {
		t.Fatalf("inline capsule cases: %v\n%s", err, out)
	}
	t.Log(string(out))
}

func TestComposerInlineWireAndHelpers(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	out, err := exec.Command(node, "testdata/composer-inline.test.cjs").CombinedOutput()
	if err != nil {
		t.Fatalf("inline composer wire and helpers: %v\n%s", err, out)
	}
	t.Log(string(out))
}

func TestInlineCapsulesPageAndDraftOwners(t *testing.T) {
	script, err := dashboardTestAsset("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"window.WsCapsules.attach(ctext)", "bindCapsuleNotes();", "s.capsules = note.wsc.snapshot()",
		"capsules: ctext && ctext.wsc ? ctext.wsc.snapshot().map(shift) : []",
		"ctext.wsc.restore(window.WsCapsules.stripMarkers(d.text, d.capsules, lost).ranges)",
		"window.WsCapsules.attach(note).restore(s.capsules)", "a.isContentEditable",
		"after.parentNode.insertBefore(rows[h], after.nextSibling)",
	} {
		if !strings.Contains(string(script), want) {
			t.Errorf("missing capsule owner hook %q", want)
		}
	}
	page, err := dashboardFS.ReadFile("assets/dashboard.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	// Session, shell memo pad, and empty/index page all need the same module.
	if got := strings.Count(string(page), "<script>{{.Assets.MentionScript}}</script>"); got != 3 {
		t.Errorf("mention/capsule module has %d page mounts, want 3", got)
	}
	assets, err := loadDashboardAssets()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(assets.data.MentionScript), "window.WsCapsules =") {
		t.Error("embedded mention bundle omits the capsule module")
	}
}
