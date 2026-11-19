package wsdashboard

import (
	"strings"
	"testing"
)

// TestRailHarnessIcon pins the shared harness badge: one .hb definition keyed
// by data-h, a spinner ring (.hbr) drawn over a working badge, approval
// wobbles and hops it (and wins over working), failure shows as a static
// ring, and reduced motion stops both animations.
func TestRailHarnessIcon(t *testing.T) {
	css := mustAsset(t, "dashboard.css")
	for _, want := range []string{
		`.hb[data-h="claude"]::before { content: "Cl"; }`,
		`.hb[data-h="pi"]::before { content: "Pi"; }`,
		`.hb[data-h="codex"]::before { content: "Co"; }`,
		`.hb[data-h="agy"]::before { content: "Ag"; }`,
		`.hb::before { content: "??"; }`,
		".hb.sm { width: 22px; height: 14px;",
		"@keyframes hbspin { to { --hba: 360deg; } }",
		".hb .hbr { position: absolute; inset: -2px;",
		"@keyframes rail-nudge {",
		".rail .hb.perm { outline: 2px solid var(--wb-perm-ink); animation: rail-nudge 1.5s steps(4) infinite; }",
		".rail .hb.err { outline: 2px solid var(--wb-fail); }",
		".hb .hbr, .rail .hb.perm { animation: none; }",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("dashboard.css lacks %q", want)
		}
	}
	if strings.Contains(css, ".rail .hi ") || strings.Contains(css, ".rail .hi.") || strings.Contains(css, "stroke-dashoffset") {
		t.Error("dashboard.css still carries a retired per-site harness icon rule")
	}
	tmpl := mustAsset(t, "dashboard.tmpl")
	if !strings.Contains(tmpl, `{{if .Approval}} perm{{else if .Working}}{{else if .Failure}} err{{end}}`) ||
		!strings.Contains(tmpl, `{{if and .Working (not .Approval)}}<span class="hbr"></span>{{end}}`) {
		t.Error("the rail row lacks the spinner ring element or the approval-over-working order")
	}
}

// TestHarnessBadgeFont pins the bundled badge font: the stylesheet declares
// it under a hashed name the page serves, and its OFL text ships beside it.
func TestHarnessBadgeFont(t *testing.T) {
	if len(harnessFontWOFF2) < 100 || string(harnessFontWOFF2[:4]) != "wOF2" {
		t.Fatalf("the embedded badge font is not a woff2 file (%d bytes)", len(harnessFontWOFF2))
	}
	if raw, err := harnessFontLicense.ReadFile("assets/harness-font/OFL.txt"); err != nil || !strings.Contains(string(raw), "SIL OPEN FONT LICENSE Version 1.1") {
		t.Errorf("the badge font's OFL text is missing: %v", err)
	}
	if css := mustAsset(t, "dashboard.css"); !strings.Contains(css, `"`+harnessFontFamily+`"`) {
		t.Errorf("dashboard.css does not name the badge font family %q", harnessFontFamily)
	}
	if !strings.Contains(harnessFontFace(), "/s/"+harnessFontName()) {
		t.Error("the badge font face does not point at the served name")
	}
}
