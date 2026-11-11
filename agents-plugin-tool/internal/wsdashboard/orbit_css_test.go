package wsdashboard

import (
	"strings"
	"testing"
)

// TestWorkingRingRunsOnTheCompositor pins the rail's working ring: its arc
// is a conic wedge turned by a transform behind a band mask, not a stroke
// dash offset (which restyled and repainted the page on every frame while
// any session worked), and reduced motion stops it and lights the whole
// band.
func TestWorkingRingRunsOnTheCompositor(t *testing.T) {
	css := mustAsset(t, "dashboard.css")
	for _, want := range []string{
		"@keyframes rail-orbit { to { transform: rotate(360deg); } }",
		"background: conic-gradient(var(--wb-primary) 0 24%, transparent 0); animation: rail-orbit 1.3s linear infinite; }",
		"mask: linear-gradient(#000 0 0) content-box exclude, linear-gradient(#000 0 0); }",
		".rail .hb .rrun { animation: none; background: var(--wb-primary); }",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("dashboard.css lacks %q", want)
		}
	}
	if strings.Contains(css, "stroke-dashoffset") {
		t.Error("dashboard.css still animates a stroke dash offset")
	}
	tmpl := mustAsset(t, "dashboard.tmpl")
	if !strings.Contains(tmpl, `<svg class="ring" aria-hidden="true"><rect class="rtrk"/></svg><span class="orb" aria-hidden="true"><i class="rrun"></i></span>`) {
		t.Error("the working row's host badge lacks the ring track and its orbit")
	}
}
