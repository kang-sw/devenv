package wsagent

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// standinOutEnv names the directory a stand-in harness writes its records
// to. Neither harness environment filter drops it, so it reaches the child.
const standinOutEnv = "WSAGENT_STANDIN_OUT"

// TestMain turns this test binary into a stand-in harness when standinOutEnv
// is set: it records its arguments (NUL-separated, so an empty value
// survives), working directory, and environment, marks itself ready, and
// reads its input to EOF. A shell script would do the same on Unix only;
// Windows cannot execute one as a harness command.
func TestMain(m *testing.M) {
	if out := os.Getenv(standinOutEnv); out != "" {
		write := func(name, body string) {
			if err := os.WriteFile(filepath.Join(out, name), []byte(body), 0o644); err != nil {
				os.Exit(2)
			}
		}
		write("args", strings.Join(os.Args[1:], "\x00"))
		cwd, _ := os.Getwd()
		write("cwd", cwd)
		write("env", strings.Join(os.Environ(), "\n"))
		write("ready", "")
		_, _ = io.Copy(io.Discard, os.Stdin)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// standin is the stand-in harness command and the directory it records to;
// the caller adds the returned environment entry to the spec's Env.
func standin(t *testing.T) (command, out, env string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	out = t.TempDir()
	return exe, out, standinOutEnv + "=" + out
}

// sameDir reports whether two spellings name one directory (a Windows temp
// path can come back in its 8.3 short form, a macOS one through /private).
func sameDir(a, b string) bool {
	ea, errA := filepath.EvalSymlinks(a)
	eb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ea == eb
}

// isPrivate reports whether info's file is readable by its owner only.
// Windows files carry no POSIX permission bits (os.Chmod there only sets the
// read-only attribute), so any file passes there.
func isPrivate(info os.FileInfo) bool {
	return runtime.GOOS == "windows" || info.Mode().Perm() == 0o600
}

// lookPython is a python3 that runs, or skips the test. The Windows Store
// alias python3.exe is on PATH yet only prints an install hint and exits.
func lookPython(t *testing.T) string {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err == nil {
		err = exec.Command(python, "-c", "pass").Run()
	}
	if err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	return python
}
