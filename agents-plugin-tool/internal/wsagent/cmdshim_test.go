package wsagent

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// piShimScript is where the fixture shim's script resolves, relative to the
// shim's directory.
var piShimScript = filepath.Join("node_modules", "@earendil-works", "pi-coding-agent", "dist", "bundle", "cli.js")

// readPiShim reads the fixture: a verbatim npm-generated pi.cmd.
func readPiShim(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "npm-cmd-shim-pi.cmd"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "\r\n") {
		t.Fatal("the fixture lost its CRLF line endings")
	}
	return string(b)
}

// claudeShimExe is where the Claude fixture shim's exe resolves, relative
// to the shim's directory.
var claudeShimExe = filepath.Join("node_modules", "@anthropic-ai", "claude-code", "bin", "claude.exe")

// readClaudeShim reads the fixture: a verbatim npm-generated claude.cmd for
// the native-binary @anthropic-ai/claude-code package.
func readClaudeShim(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "npm-cmd-shim-claude.cmd"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "\r\n") {
		t.Fatal("the fixture lost its CRLF line endings")
	}
	return string(b)
}

// setLookPath replaces lookPath for the test.
func setLookPath(t *testing.T, f func(string) (string, error)) {
	t.Helper()
	old := lookPath
	lookPath = f
	t.Cleanup(func() { lookPath = old })
}

// setHostWindows replaces hostWindows for the test.
func setHostWindows(t *testing.T, on bool) {
	t.Helper()
	old := hostWindows
	hostWindows = on
	t.Cleanup(func() { hostWindows = old })
}

func TestCmdShimArgvUnwrapsTheNpmShim(t *testing.T) {
	setLookPath(t, func(name string) (string, error) {
		if name == "node" {
			return "/path/bin/node", nil
		}
		return "", exec.ErrNotFound
	})
	dir := t.TempDir()
	shim := filepath.Join(dir, "pi.cmd")
	crlf := readPiShim(t)
	lf := strings.ReplaceAll(crlf, "\r\n", "\n")
	for name, text := range map[string]string{"crlf": crlf, "lf": lf} {
		for _, beside := range []bool{true, false} {
			exists := func(path string) bool { return beside && path == filepath.Join(dir, "node.exe") }
			prog, args, err := cmdShimArgv(shim, text, exists)
			if err != nil {
				t.Fatalf("%s beside=%v: %v", name, beside, err)
			}
			want := "/path/bin/node"
			if beside {
				want = filepath.Join(dir, "node.exe")
			}
			if prog != want {
				t.Errorf("%s beside=%v: program %q, want %q", name, beside, prog, want)
			}
			if wantArgs := []string{filepath.Join(dir, piShimScript)}; !reflect.DeepEqual(args, wantArgs) {
				t.Errorf("%s beside=%v: args %q, want %q", name, beside, args, wantArgs)
			}
		}
	}
}

func TestCmdShimArgvRefusesWhatIsNotANodeShim(t *testing.T) {
	setLookPath(t, func(string) (string, error) { return "", exec.ErrNotFound })
	shim := filepath.Join(t.TempDir(), "tool.cmd")
	never := func(string) bool { return false }
	for name, text := range map[string]string{
		"plain batch":       "@echo off\r\ntool.exe %*\r\n",
		"other interpreter": strings.ReplaceAll(readPiShim(t), `SET "_prog=node"`, `SET "_prog=sh"`),
		"no launch line":    strings.ReplaceAll(readPiShim(t), "%*", ""),
	} {
		if _, _, err := cmdShimArgv(shim, text, func(string) bool { return true }); !errors.Is(err, errShimRefused) || !strings.Contains(err.Error(), shim) {
			t.Errorf("%s: want a refusal naming %s, got %v", name, shim, err)
		}
	}
	if _, _, err := cmdShimArgv(shim, readPiShim(t), never); err == nil || !strings.Contains(err.Error(), shim) {
		t.Errorf("a shim with no node beside it or on PATH: want a refusal naming %s, got %v", shim, err)
	}
}

func TestCmdShimArgvUnwrapsTheExeShim(t *testing.T) {
	setLookPath(t, func(string) (string, error) { return "", exec.ErrNotFound })
	dir := t.TempDir()
	shim := filepath.Join(dir, "claude.cmd")
	exe := filepath.Join(dir, claudeShimExe)
	crlf := readClaudeShim(t)
	for name, text := range map[string]string{
		"crlf":      crlf,
		"lf":        strings.ReplaceAll(crlf, "\r\n", "\n"),
		"upper ext": strings.ReplaceAll(crlf, "claude.exe", "claude.EXE"),
	} {
		want := exe
		if name == "upper ext" {
			want = filepath.Join(dir, strings.TrimSuffix(claudeShimExe, ".exe")+".EXE")
		}
		exists := func(path string) bool { return path == want }
		prog, args, err := cmdShimArgv(shim, text, exists)
		if err != nil || prog != want || args != nil {
			t.Errorf("%s: got %q %q %v, want %q and no leading args", name, prog, args, err, want)
		}
	}
}

// TestCmdShimArgvRefusesExeShimEdges: an exe shim is unwrapped only when its
// target exists inside the shim's directory and runs with exactly %*.
func TestCmdShimArgvRefusesExeShimEdges(t *testing.T) {
	setLookPath(t, func(string) (string, error) { return "", exec.ErrNotFound })
	shim := filepath.Join(t.TempDir(), "claude.cmd")
	always := func(string) bool { return true }
	launch := `"%dp0%\node_modules\@anthropic-ai\claude-code\bin\claude.exe"   %*`
	if !strings.Contains(readClaudeShim(t), launch) {
		t.Fatal("the fixture lost its launch line")
	}
	for name, line := range map[string]string{
		"parent segment": `"%dp0%\..\claude.exe" %*`,
		"volume":         `"%dp0%\C:\claude.exe" %*`,
		"rooted":         `"%dp0%\\server\claude.exe" %*`,
		"extra args":     `"%dp0%\claude.exe" --flag %*`,
		"not an exe":     `"%dp0%\claude.js" %*`,
		"no dp0":         `"C:\bin\claude.exe" %*`,
	} {
		text := strings.ReplaceAll(readClaudeShim(t), launch, line)
		if _, _, err := cmdShimArgv(shim, text, always); !errors.Is(err, errShimRefused) || !strings.Contains(err.Error(), shim) {
			t.Errorf("%s: want a refusal naming %s, got %v", name, shim, err)
		}
	}
	if _, _, err := cmdShimArgv(shim, readClaudeShim(t), func(string) bool { return false }); !errors.Is(err, errShimRefused) || !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("a missing exe target: want a refusal, got %v", err)
	}
}

func TestHarnessCommandOnWindowsUnwrapsAShim(t *testing.T) {
	setHostWindows(t, true)
	dir := t.TempDir()
	shim := filepath.Join(dir, "pi.cmd")
	if err := os.WriteFile(shim, []byte(readPiShim(t)), 0o644); err != nil {
		t.Fatal(err)
	}
	node := filepath.Join(dir, "node.exe")
	if err := os.WriteFile(node, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "claude.exe")
	setLookPath(t, func(name string) (string, error) {
		switch name {
		case "pi":
			return shim, nil
		case "claude":
			return exe, nil
		}
		return name, nil
	})
	script := filepath.Join(dir, piShimScript)
	for _, command := range []string{"", shim} {
		prog, pre, err := harnessCommand(command, "pi")
		if err != nil || prog != node || !reflect.DeepEqual(pre, []string{script}) {
			t.Errorf("command %q: got %q %q %v, want %q [%q]", command, prog, pre, err, node, script)
		}
	}
	if prog, pre, err := harnessCommand("", "claude"); err != nil || prog != exe || pre != nil {
		t.Errorf("an .exe passes through, got %q %q %v", prog, pre, err)
	}
	claudeShim := filepath.Join(dir, "claude.cmd")
	if err := os.WriteFile(claudeShim, []byte(readClaudeShim(t)), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, claudeShimExe)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	if prog, pre, err := harnessCommand(claudeShim, "claude"); err != nil || prog != target || pre != nil {
		t.Errorf("the npm claude.cmd unwraps to its exe, got %q %q %v, want %q", prog, pre, err, target)
	}
}

// TestHarnessCommandRefusalNamesARemedy: a refused shim's error names the
// shim and a remedy for the harness being started; only Claude's names
// claude.exe.
func TestHarnessCommandRefusalNamesARemedy(t *testing.T) {
	setHostWindows(t, true)
	setLookPath(t, func(name string) (string, error) { return name, nil })
	dir := t.TempDir()
	bad := filepath.Join(dir, "harness.cmd")
	if err := os.WriteFile(bad, []byte("@echo off\r\nharness.exe %*\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := harnessCommand(bad, "claude")
	if err == nil || !strings.Contains(err.Error(), bad) || !strings.Contains(err.Error(), "native claude.exe") || !strings.Contains(err.Error(), "ahead of "+dir+" on PATH") {
		t.Errorf("claude: want a refusal naming %s and the claude.exe remedy, got %v", bad, err)
	}
	_, _, err = harnessCommand(bad, "pi")
	if err == nil || !strings.Contains(err.Error(), bad) || !strings.Contains(err.Error(), "install pi's native build") || strings.Contains(err.Error(), "claude") {
		t.Errorf("pi: want a refusal naming %s with a neutral remedy, got %v", bad, err)
	}
}

// TestHarnessCommandOnWindowsShimEdges: the shim extension matches in any
// case, and a shim that cannot be read is refused with its path.
func TestHarnessCommandOnWindowsShimEdges(t *testing.T) {
	setHostWindows(t, true)
	dir := t.TempDir()
	node := filepath.Join(dir, "node.exe")
	if err := os.WriteFile(node, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	setLookPath(t, func(name string) (string, error) { return name, nil })
	script := filepath.Join(dir, piShimScript)
	for _, name := range []string{"PI.CMD", "pi.Bat"} {
		shim := filepath.Join(dir, name)
		if err := os.WriteFile(shim, []byte(readPiShim(t)), 0o644); err != nil {
			t.Fatal(err)
		}
		prog, pre, err := harnessCommand(shim, "pi")
		if err != nil || prog != node || !reflect.DeepEqual(pre, []string{script}) {
			t.Errorf("%s: got %q %q %v, want %q [%q]", name, prog, pre, err, node, script)
		}
	}
	missing := filepath.Join(dir, "gone.cmd")
	if _, _, err := harnessCommand(missing, "pi"); err == nil || !strings.Contains(err.Error(), "read "+missing) {
		t.Errorf("an unreadable shim is refused with its path, got %v", err)
	}
}

func TestHarnessCommandElsewhereIsUnchanged(t *testing.T) {
	setHostWindows(t, false)
	setLookPath(t, func(name string) (string, error) {
		if name == "pi" {
			return "/usr/local/bin/pi.cmd", nil
		}
		return "", exec.ErrNotFound
	})
	if prog, pre, err := harnessCommand("", "pi"); err != nil || prog != "/usr/local/bin/pi.cmd" || pre != nil {
		t.Errorf("a resolved command passes through, got %q %q %v", prog, pre, err)
	}
	if prog, pre, err := harnessCommand("/opt/pi.cmd", "pi"); err != nil || prog != "/opt/pi.cmd" || pre != nil {
		t.Errorf("a set command passes through unresolved, got %q %q %v", prog, pre, err)
	}
}

// TestHarnessStartsGoThroughTheShimUnwrap: every hosted start site refuses a
// .cmd that is not a cmd-shim before it launches anything.
func TestHarnessStartsGoThroughTheShimUnwrap(t *testing.T) {
	setHostWindows(t, true)
	setLookPath(t, func(name string) (string, error) { return name, nil })
	bad := filepath.Join(t.TempDir(), "harness.cmd")
	if err := os.WriteFile(bad, []byte("@echo off\r\nharness.exe %*\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	starts := map[string]func(StartSpec) (Process, error){
		"pi": func(s StartSpec) (Process, error) { return PiBackend{Command: bad}.Start(context.Background(), s) },
		"pi lead": func(s StartSpec) (Process, error) {
			s.Lead = &LeadSpec{}
			return PiBackend{Command: bad}.Start(context.Background(), s)
		},
		"claude": func(s StartSpec) (Process, error) { return ClaudeBackend{Command: bad}.Start(context.Background(), s) },
		"claude lead": func(s StartSpec) (Process, error) {
			s.Lead = &LeadSpec{}
			return ClaudeBackend{Command: bad}.Start(context.Background(), s)
		},
	}
	for name, start := range starts {
		spec := StartSpec{Root: t.TempDir(), Dir: t.TempDir()}
		if _, err := start(spec); err == nil || !strings.Contains(err.Error(), bad) {
			t.Errorf("%s: want a refusal naming %s, got %v", name, bad, err)
		}
	}
}
