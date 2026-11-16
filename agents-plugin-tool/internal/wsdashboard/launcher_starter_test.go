package wsdashboard

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// TestLauncherStarterSingleFlight runs the process-boundary advisory-lock
// fixture on the remote Linux test host. The fixture mocks every ws starter;
// no ws executable is run, installed or pointed at shared daemon state.
func TestLauncherStarterSingleFlight(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process fixtures run only on the remote Linux host")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable")
	}
	script, err := filepath.Abs("../../../agents-plugin/tests/test_ws_mcp_launcher_start.py")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	cmd := exec.Command(python, script, "LauncherStarterProcessesTest", "-v")
	cmd.Env = append(os.Environ(), "WS_LAUNCHER_PROCESS_TESTS=1", "PYTHONDONTWRITEBYTECODE=1",
		"HOME="+root, "TMPDIR="+root, "WS_CACHE_HOME="+filepath.Join(root, "cache"),
		"WS_CONFIG_HOME="+filepath.Join(root, "config"), "CODEX_HOME="+filepath.Join(root, "codex"),
		"CLAUDE_CONFIG_DIR="+filepath.Join(root, "claude"))
	out, err := cmd.CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("launcher process fixture: %v", err)
	}
}

func TestStageExecutableFailedCopyKeepsForeignArtifacts(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	if err := os.WriteFile(source, []byte("binary"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(dir, "foreign.tmp")
	if err := os.WriteFile(foreign, []byte("owned elsewhere"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := copyExecutable(source, target); err == nil {
		t.Fatal("rename over directory should fail")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 3 {
		t.Fatalf("failed copy left its temp or removed foreign artifacts: %v, %v", entries, err)
	}
}
