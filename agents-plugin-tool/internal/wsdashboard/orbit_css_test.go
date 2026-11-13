package wsdashboard

import (
	"strings"
	"testing"
)

// TestRailHarnessIcon pins the rail's harness icon: the icon itself stays
// still while working and a masked orbit line travels around its border,
// approval wobbles and hops it (and wins over working), failure shows as a
// static ring, and reduced motion stops both animations.
func TestRailHarnessIcon(t *testing.T) {
	css := mustAsset(t, "dashboard.css")
	for _, want := range []string{
		"@keyframes rail-orbit { to { transform: rotate(360deg); } }",
		".rail .hi .rrun { position: absolute;",
		"animation: rail-orbit 1.3s linear infinite; }",
		"@keyframes rail-nudge {",
		".rail .hi.perm { box-shadow: 0 0 0 2px var(--wb-perm-ink); animation: rail-nudge 1.5s ease-in-out infinite; }",
		".rail .hi.claude { background: #D97757; }",
		".rail .hi.codex { background: #10A37F; }",
		".rail .hi.err { box-shadow: 0 0 0 2px var(--wb-fail); }",
		".rail .hi .rrun, .rail .hi.perm { animation: none; }",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("dashboard.css lacks %q", want)
		}
	}
	if strings.Contains(css, ".rail .hi.run {") {
		t.Error("the running icon itself must not animate; the orbit line does")
	}
	if strings.Contains(css, "stroke-dashoffset") {
		t.Error("dashboard.css still carries the retired dash-offset ring")
	}
	tmpl := mustAsset(t, "dashboard.tmpl")
	if !strings.Contains(tmpl, `{{if .Approval}} perm{{else if .Working}} run`) ||
		!strings.Contains(tmpl, `{{if and .Working (not .Approval)}}<span class="orb"><span class="rrun"></span></span>{{end}}`) {
		t.Error("the rail row lacks the orbit line element or the approval-over-working order")
	}
}
