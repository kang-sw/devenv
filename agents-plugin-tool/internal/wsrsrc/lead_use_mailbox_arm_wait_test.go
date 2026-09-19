package wsrsrc

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestLeadUseMailboxArmTheWaitHarnessComposition guards the
// lead-use-mailbox "On: arm the wait" include split
// (260919-feat-mailbox-arm-wait-harness-include) against the two failure
// modes a byte-mirror/hash check cannot catch: the pi overlay silently
// stopping being selected (or leaking base content alongside it), and the
// host-neutral base losing the harness-specific nuance a non-pi render must
// still carry (Codex's registered-address-conditional wake, Claude's
// background-task-completion wake) or the {{.MailboxWaitCommand}}
// placeholder that render-time injection substitutes.
func TestLeadUseMailboxArmTheWaitHarnessComposition(t *testing.T) {
	root := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")

	const (
		headingMarker      = "## On: arm the wait"
		waitCommandVar     = "{{.MailboxWaitCommand}}"
		baseImperative     = "as a background process"
		codexNuance        = "Codex's `Stop`"
		claudeExample      = "Claude's own background-task notification"
		piOverlayMarker    = "already arms a background mailbox waiter"
		piNoLaunchSentence = "you never launch anything from"
	)

	cases := []struct {
		harness string
		isPi    bool
	}{
		{harness: ""},
		{harness: "claude"},
		{harness: "codex"},
		{harness: "pi", isPi: true},
	}

	for _, tc := range cases {
		t.Run("harness="+tc.harness, func(t *testing.T) {
			pb, err := Load(root, "lead-use-mailbox", tc.harness, nil)
			if err != nil {
				t.Fatalf("Load(harness=%q): %v", tc.harness, err)
			}
			body := pb.Body

			// The included "On: arm the wait" section is always appended at
			// the very end of the body (Load: fullBody = body + includeText),
			// so scope every content check to that section rather than the
			// whole document: unrelated skeleton text (for example the
			// Invariants "Wake" paragraph) legitimately reuses phrases like
			// "as a background process" and must not count as leakage.
			idx := strings.LastIndex(body, headingMarker)
			if idx < 0 {
				t.Fatalf("harness=%q: missing %q heading", tc.harness, headingMarker)
			}
			section := body[idx:]

			if tc.isPi {
				if !strings.Contains(section, piOverlayMarker) || !strings.Contains(section, piNoLaunchSentence) {
					t.Errorf("harness=%q: missing pi overlay text", tc.harness)
				}
				if strings.Contains(section, baseImperative) {
					t.Errorf("harness=%q: base include content leaked alongside the pi overlay", tc.harness)
				}
				if strings.Contains(section, codexNuance) || strings.Contains(section, claudeExample) {
					t.Errorf("harness=%q: base harness examples leaked alongside the pi overlay", tc.harness)
				}
				return
			}

			if !strings.Contains(section, waitCommandVar) {
				t.Errorf("harness=%q: missing %s placeholder for render-time substitution", tc.harness, waitCommandVar)
			}
			if !strings.Contains(section, codexNuance) {
				t.Errorf("harness=%q: missing Codex registered-address-conditional nuance", tc.harness)
			}
			if !strings.Contains(section, claudeExample) {
				t.Errorf("harness=%q: missing Claude background-task-completion example", tc.harness)
			}
			if strings.Contains(section, piOverlayMarker) {
				t.Errorf("harness=%q: unexpectedly picked up the pi overlay", tc.harness)
			}
		})
	}
}
