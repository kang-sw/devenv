package wsdashboard

import (
	"strings"
	"testing"
)

// TestRailHarnessIcon pins the rail's harness icon: the working state turns
// the icon itself by a transform (no ring or orbit element), approval and
// failure show as a static ring, and reduced motion stops the turn.
func TestRailHarnessIcon(t *testing.T) {
	css := mustAsset(t, "dashboard.css")
	for _, want := range []string{
		"@keyframes rail-orbit { to { transform: rotate(360deg); } }",
		".rail .hi.run { animation: rail-orbit 1.3s linear infinite; }",
		".rail .hi.claude { background: #D97757; }",
		".rail .hi.codex { background: #10A37F; }",
		".rail .hi.perm { box-shadow: 0 0 0 2px var(--wb-perm-ink); }",
		".rail .hi.err { box-shadow: 0 0 0 2px var(--wb-fail); }",
		".rail .hi.run { animation: none; }",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("dashboard.css lacks %q", want)
		}
	}
	if strings.Contains(css, "stroke-dashoffset") || strings.Contains(css, ".rrun") {
		t.Error("dashboard.css still carries the retired working ring")
	}
	tmpl := mustAsset(t, "dashboard.tmpl")
	if strings.Contains(tmpl, `class="rrun"`) || strings.Contains(tmpl, `class="orb"`) {
		t.Error("the rail row still draws the retired working ring")
	}
}
