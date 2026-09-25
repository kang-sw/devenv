package wsconfig

import (
	"fmt"
	"os"
	"testing"
)

// TestMain points WS_CONFIG_HOME at an empty temp dir so tests that resolve
// through the global layer without an explicit Options.ConfigHome assert
// builtin defaults instead of the developer's ~/.ws/config.json. It is
// unconditional: an inherited WS_CONFIG_HOME is a real config just the same.
// Tests that exercise the env var itself override it with t.Setenv.
func TestMain(m *testing.M) {
	os.Exit(runTestMain(m))
}

func runTestMain(m *testing.M) int {
	configHome, err := os.MkdirTemp("", "ws-wsconfig-test-config-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: create temp config home: %v\n", err)
		return 1
	}
	defer os.RemoveAll(configHome)
	_ = os.Setenv(envConfigHome, configHome)
	return m.Run()
}
