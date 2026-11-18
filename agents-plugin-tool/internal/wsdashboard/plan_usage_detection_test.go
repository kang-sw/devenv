package wsdashboard

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kang-sw/devenv/internal/wsstate"
)

func TestPlanUsageDetectionRetriesReadFailureAcrossForcedRefresh(t *testing.T) {
	for _, tc := range []struct {
		name, path, raw, plan string
		codex, pi             bool
	}{
		{name: "Claude", path: ".claude/.credentials.json", raw: `{"claudeAiOauth":{"subscriptionType":"max"}}`, plan: "max"},
		{name: "Codex", path: ".codex/auth.json", raw: `{"auth_mode":"chatgpt","tokens":{}}`, codex: true},
		{name: "Pi", path: ".pi/agent/auth.json", raw: `{"openai-codex":{}}`, pi: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := usageTestStore(t)
			path := filepath.Join(s.home, filepath.FromSlash(tc.path))
			usageWrite(t, path, tc.raw)
			before, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			fail, reads := true, 0
			s.readFile = func(p string) ([]byte, error) {
				if p == path {
					reads++
					if fail {
						return nil, &os.PathError{Op: "read", Path: p, Err: os.ErrPermission}
					}
				}
				return os.ReadFile(p)
			}
			// No real credential values or upstream requests are needed: the
			// probe rejects these empty tokens after detection succeeds.
			s.writeFile = func(string, string, []byte) error { return nil }
			if plan, codex, pi := s.detect(); plan != "" || codex || pi {
				t.Fatal("failed read reported a login")
			}
			if reads != 1 {
				t.Fatalf("initial reads = %d, want 1", reads)
			}
			fail = false
			s.refresh(context.Background(), true)
			s.probes.Wait()
			if plan, codex, pi := s.detect(); plan != tc.plan || codex != tc.codex || pi != tc.pi {
				t.Fatalf("after recovery: plan=%q codex=%v pi=%v", plan, codex, pi)
			}
			if reads != 2 {
				t.Fatalf("reads after recovery = %d, want one failure and one successful refill", reads)
			}
			after, err := os.Stat(path)
			if err != nil || !wsstate.SameFileMetadata(before, after) {
				t.Fatalf("credential metadata changed: %v", err)
			}
		})
	}
}
