package wsrsrc

import (
	"path/filepath"
	"strings"
	"testing"
)

// Exercise harness composition and the render-time wait-command variable.
func TestLeadUseMailboxArmTheWaitHarnessComposition(t *testing.T) {
	root := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	for _, harness := range []string{"", "claude", "codex", "pi"} {
		t.Run("harness="+harness, func(t *testing.T) {
			pb, err := Load(root, "lead-use-mailbox", harness, "", nil)
			if err != nil {
				t.Fatalf("Load(harness=%q): %v", harness, err)
			}
			if strings.TrimSpace(pb.Body) == "" {
				t.Fatal("empty mailbox playbook")
			}
			if got, want := strings.Contains(pb.Body, "{{.MailboxWaitCommand}}"), harness != "pi"; got != want {
				t.Errorf("wait-command placeholder present = %v, want %v", got, want)
			}
		})
	}
}
