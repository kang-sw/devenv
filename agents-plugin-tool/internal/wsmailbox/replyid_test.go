package wsmailbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wskey"
)

func TestReplyIDDeterministicWithoutPersistence(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "uncreated")
	t.Setenv("WS_CACHE_HOME", cache)
	for key, want := range map[string]string{
		"amber-tide-fox":    "headlessroutineremodelerpassage",
		"other-session-key": "staunchmarriedungluetrapezoid",
	} {
		got := ReplyID(key)
		if got != want || got != ReplyID(key) || got != strings.ReplaceAll(wskey.DeriveFull(key, 4), "-", "") || !IsValidReplyID(got) {
			t.Fatalf("ReplyID(%q) = %q", key, got)
		}
	}
	if _, err := os.Stat(cache); !os.IsNotExist(err) {
		t.Fatalf("pure derivation touched cache: %v", err)
	}
}
