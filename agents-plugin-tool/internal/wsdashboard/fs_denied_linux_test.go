package wsdashboard

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFSLocalDeniedLinuxProcEnvironments(t *testing.T) {
	o := startOutsideFixture(t)
	for _, path := range []string{"/proc/self/environ", "/proc/thread-self/environ", "/proc/123/environ", "/proc/123/task/456/environ"} {
		if !alwaysDenied(path, homeDirs()) {
			t.Errorf("process environment not denied: %s", path)
		}
		assertSensitiveReadsRefused(t, o, path)
	}
	for _, path := range []string{"/proc/self/status", "/tmp/proc/self/environ", "/proc/self/environ.txt"} {
		if deniedProcEnviron(path) {
			t.Errorf("ordinary path denied as process environment: %s", path)
		}
	}
	alias := filepath.Join(o.away, "environment.png")
	if err := os.Symlink("/proc/self/environ", alias); err != nil {
		t.Fatal(err)
	}
	assertSensitiveReadsRefused(t, o, alias)
}
