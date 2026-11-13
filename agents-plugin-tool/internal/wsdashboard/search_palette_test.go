package wsdashboard

import (
	"os/exec"
	"strings"
	"testing"
)

func TestSearchPaletteAssets(t *testing.T) {
	tracker, err := dashboardFS.ReadFile("assets/tracker.js")
	if err != nil {
		t.Fatal(err)
	}
	page, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	css, err := dashboardFS.ReadFile("assets/tracker.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"class: 'tk tk-palette'", "d.showModal()", "function qScore(", "function qBoard(",
		"function qShortcut(", "O.searchRows", "O.searchPick", "e.isComposing || e.keyCode === 229",
		"$('tkqpop').addEventListener('cancel'", "js.tk.qs.sessions", "js.tk.qs.tracker",
		"O.fileFind", "O.fileList", "excerpts: true",
	} {
		if !strings.Contains(string(tracker), want) {
			t.Errorf("tracker lacks palette hook %q", want)
		}
	}
	// The renderer stays host-agnostic: the file hooks come from the host.
	for _, page := range []string{"WsPreview", "/fs/"} {
		if strings.Contains(string(tracker), page) {
			t.Errorf("tracker reaches the page's %q itself", page)
		}
	}
	if strings.Contains(string(tracker), `class="tk-qpop"`) || strings.Contains(string(css), ".tk .tk-qpop") {
		t.Error("old quick-switcher popover remains")
	}
	for _, want := range []string{"searchRows: paletteRows", "searchPick: palettePick", "paletteFiles({", `"&excerpts=1"`, "parsed.content.querySelector", `p.open = true`, `target.scrollIntoView({ block: "center" })`} {
		if !strings.Contains(string(page), want) {
			t.Errorf("host lacks palette hook %q", want)
		}
	}
	for _, want := range []string{".tk.tk-palette::backdrop", "min(1240px, calc(100vw - 24px))", "[data-step=wide]", "prefers-reduced-motion: reduce", ".palette-flash { animation: none; }"} {
		if !strings.Contains(string(css), want) {
			t.Errorf("palette CSS lacks %q", want)
		}
	}
}

func TestSearchPaletteBehavior(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; run testdata/search-palette.test.cjs locally")
	}
	out, err := exec.Command(node, "testdata/search-palette.test.cjs").CombinedOutput()
	if err != nil {
		t.Fatalf("palette cases failed: %v\n%s", err, out)
	}
}
