package wsdashboard

import (
	"os/exec"
	"strings"
	"testing"
)

// TestGalleryModel runs the paged docking gallery's pure model tests
// (assets/gallery.js): storage migration and reconcile, the split tree, the
// layout mode mapping, and the bounce-snap and hover timing, each scripted
// under node's test runner.
func TestGalleryModel(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; run node --test testdata/gallery-*.test.cjs locally")
	}
	out, err := exec.Command(node, "--test",
		"testdata/gallery-state.test.cjs",
		"testdata/gallery-tree.test.cjs",
		"testdata/gallery-wheel.test.cjs",
	).CombinedOutput()
	if err != nil {
		t.Fatalf("gallery model tests failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "# fail 0") && !strings.Contains(string(out), "ℹ fail 0") {
		t.Fatalf("gallery model tests reported failures:\n%s", out)
	}
}

// TestGalleryShipsWithCards pins that the card framework's script carries
// the gallery model ahead of cards.js, so window.WsGallery exists when
// WsCards mounts.
func TestGalleryShipsWithCards(t *testing.T) {
	a, err := loadDashboardAssets()
	if err != nil {
		t.Fatal(err)
	}
	js := string(a.data.CardsScript)
	g, c := strings.Index(js, "window.WsGallery = G"), strings.Index(js, "window.WsCards = {")
	if g < 0 || c < 0 || g > c {
		t.Fatalf("CardsScript must carry gallery.js before cards.js (gallery at %d, cards at %d)", g, c)
	}
}
