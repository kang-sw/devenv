package wsagent

import (
	"os/exec"
	"testing"
	"time"
)

type nopMapper struct{}

func (nopMapper) mapLine([]byte, lineReplies) []Event { return nil }

// TestExitedChildIsNotHeldByAGrandchildsStderr: a process the child left
// running with the child's stderr does not keep the exited child's events
// open past the wait delay.
func TestExitedChildIsNotHeldByAGrandchildsStderr(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 6 >/dev/null </dev/null & exit 0")
	p, err := startStreamProcess(cmd, nopMapper{}, claudeProtocol{})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.After(streamWaitDelay + 3*time.Second)
	for {
		select {
		case ev, ok := <-p.Events():
			if !ok {
				return
			}
			if ev.Fatal != nil {
				t.Fatalf("a clean exit with stderr held is no failure: %v", ev.Fatal)
			}
		case <-deadline:
			t.Fatal("the events stayed open while a grandchild held stderr")
		}
	}
}
