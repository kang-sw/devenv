//go:build windows

package wsstate

import (
	"os"
	"strings"
	"testing"
)

func TestProcessParentOfThisProcess(t *testing.T) {
	ppid, image, ok := ProcessParent(os.Getpid())
	if !ok {
		t.Fatal("ProcessParent(self) not found")
	}
	if ppid != os.Getppid() {
		t.Errorf("parent = %d, want %d", ppid, os.Getppid())
	}
	if !strings.HasSuffix(strings.ToLower(image), ".exe") {
		t.Errorf("image = %q, want an .exe name", image)
	}
}

func TestProcessParentUnknownPID(t *testing.T) {
	if _, _, ok := ProcessParent(0x7FFFFFF0); ok {
		t.Error("ProcessParent(unused pid) ok = true")
	}
	if _, _, ok := ProcessParent(0); ok {
		t.Error("ProcessParent(0) ok = true")
	}
}
