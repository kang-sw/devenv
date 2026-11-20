package wsrsrc

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Exercise selection with a fake Go command so these guards never run suites.
func TestCITestShardSelection(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0755); err != nil {
		t.Fatal(err)
	}
	script, err := os.ReadFile(filepath.Join("..", "..", "scripts", "ci-test-shard.sh"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "scripts", "ci-test-shard.sh")
	if err := os.WriteFile(path, script, 0644); err != nil {
		t.Fatal(err)
	}
	fakeGo := `#!/usr/bin/env bash
set -eu
if [[ "$1" == list ]]; then
  if [[ "${FAIL_LIST:-}" == 1 ]]; then exit 42; fi
  printf '%s\n' example/cmd/ws-mcp example/internal/mcp example/internal/wsledger example/internal/wstrackerhost example/internal/wsledger/child
elif [[ "$2" == -list ]]; then
  printf '%s\n' TestD TestB TestA TestC ExampleZ FuzzX 'ok example/internal/mcp'
else
  printf '%s\n' "$@"
fi
`
	if err := os.WriteFile(filepath.Join(root, "go"), []byte(fakeGo), 0755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) []string {
		t.Helper()
		cmd := exec.Command(bash, append([]string{path}, args...)...)
		cmd.Env = append(os.Environ(), "PATH="+root+string(os.PathListSeparator)+os.Getenv("PATH"))
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return strings.Split(strings.TrimSpace(string(out)), "\n")
	}
	for _, tt := range []struct {
		mode string
		want []string
	}{
		{"all", []string{"example/cmd/ws-mcp", "example/internal/mcp", "example/internal/wsledger/child"}},
		{"rest", []string{"example/cmd/ws-mcp", "example/internal/wsledger/child"}},
		{"long", []string{"./internal/wsledger", "./internal/wstrackerhost"}},
	} {
		t.Run(tt.mode, func(t *testing.T) {
			want := append([]string{"test", "-count=1", "-timeout", "30m", "-list", "."}, tt.want...)
			if got := run(tt.mode, "-list", "."); !reflect.DeepEqual(got, want) {
				t.Fatalf("selection = %q, want %q", got, want)
			}
		})
	}
	// Sorted top-level tests partition without overlap, including examples/fuzz.
	for i, pattern := range []string{"^(ExampleZ|TestC)$", "^(FuzzX|TestD)$", "^(TestA)$", "^(TestB)$"} {
		index := string(rune('0' + i))
		want := []string{"test", "-count=1", "-timeout", "30m", "-run", pattern, "-list", ".", "./internal/mcp"}
		if got := run("mcp", index, "4", "-list", "."); !reflect.DeepEqual(got, want) {
			t.Fatalf("mcp %s = %q, want %q", index, got, want)
		}
	}
	t.Setenv("FAIL_LIST", "1")
	cmd := exec.Command(bash, path, "all", "-list", ".")
	cmd.Env = append(os.Environ(), "PATH="+root+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := cmd.Run(); err == nil {
		t.Fatal("failed package inventory must fail selection, not report a partial pass")
	}
}

func TestCIReleaseLongSkipGate(t *testing.T) {
	jq, err := exec.LookPath("jq")
	if err != nil {
		t.Skip("jq unavailable")
	}
	workflow, err := os.ReadFile(filepath.Join("..", "..", "..", ".github", "workflows", "ws-mcp-release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var predicate string
	for _, line := range strings.Split(string(workflow), "\n") {
		if strings.Contains(line, "ran=$(gh api") {
			_, tail, ok := strings.Cut(line, "--jq '")
			if ok {
				predicate, _, _ = strings.Cut(tail, "'")
			}
		}
	}
	if predicate == "" {
		t.Fatal("release job predicate missing")
	}
	type job struct {
		Name       string `json:"name"`
		Conclusion string `json:"conclusion"`
	}
	regular := job{"go test (windows-latest, rest)", "success"}
	for _, tt := range []struct {
		name string
		jobs []job
		want string
	}{
		{"empty", nil, "false"},
		{"legacy green", []job{regular}, "true"},
		{"default", []job{regular, {"go test (ubuntu-latest, long)", "skipped"}, {"go test (windows-latest, long)", "skipped"}}, "true"},
		{"long success", []job{regular, {"go test (windows-latest, long)", "success"}}, "true"},
		{"regular skipped", []job{{regular.Name, "skipped"}}, "false"},
		{"regular failure", []job{{regular.Name, "failure"}}, "false"},
		{"unknown skipped", []job{regular, {"go test (other, long)", "skipped"}}, "false"},
		{"long failure", []job{regular, {"go test (windows-latest, long)", "failure"}}, "false"},
		{"long cancelled", []job{regular, {"go test (ubuntu-latest, long)", "cancelled"}}, "false"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			jobs := tt.jobs
			if jobs == nil {
				jobs = []job{}
			}
			input, err := json.Marshal(map[string]any{"jobs": jobs})
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(jq, "-r", predicate)
			cmd.Stdin = strings.NewReader(string(input))
			out, err := cmd.CombinedOutput()
			if err != nil || strings.TrimSpace(string(out)) != tt.want {
				t.Fatalf("gate = %s (%v), want %s", out, err, tt.want)
			}
		})
	}
}

func TestCIReleasePublication(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable")
	}
	workflow, err := os.ReadFile(filepath.Join("..", "..", "..", ".github", "workflows", "ws-mcp-release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(workflow)
	_, publication, ok := strings.Cut(text, "      - name: Publish GitHub release assets\n")
	if !ok {
		t.Fatal("native publication step missing")
	}
	publication, _, ok = strings.Cut(publication, "\n  windows-smoke:")
	if !ok || !strings.Contains(publication, "if: startsWith(github.ref, 'refs/tags/') && !inputs.desktop_dry_run") {
		t.Fatal("native publication must remain gated on tags and dry-run veto")
	}
	_, body, ok := strings.Cut(publication, "        run: |\n")
	if !ok {
		t.Fatal("native publication shell missing")
	}
	var lines []string
	for _, line := range strings.Split(strings.TrimSuffix(body, "\n"), "\n") {
		if !strings.HasPrefix(line, "          ") {
			t.Fatalf("unexpected native publication indentation: %q", line)
		}
		lines = append(lines, strings.TrimPrefix(line, "          "))
	}
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "dist"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "dist", "native.bin"), []byte("fixture"), 0644); err != nil {
		t.Fatal(err)
	}
	gh := "#!/bin/sh\nif [ \"$2\" = view ]; then exit 1; fi\nprintf '<%s>\\n' \"$@\" >> \"$CALL_LOG\"\nprintf '<end>\\n' >> \"$CALL_LOG\"\n"
	if err := os.WriteFile(filepath.Join(root, "gh"), []byte(gh), 0755); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		tag    string
		canary bool
	}{
		{"v0.50.8", false},
		{"v0.50.9-canary.0", true},
		{"v12.3.4-canary.19", true},
		{"v0.50.9-canary.1-extra", false},
		{"v0.50.9-canary.x", false},
		{"v0.50.9-canary.1.2", false},
		{"v0.50.9-rc.1", false},
	} {
		t.Run(tt.tag, func(t *testing.T) {
			log := filepath.Join(t.TempDir(), "calls")
			cmd := exec.Command(bash, "-c", strings.Join(lines, "\n"))
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "PATH="+root+string(os.PathListSeparator)+os.Getenv("PATH"), "GITHUB_REF_NAME="+tt.tag, "CALL_LOG="+log)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("publication failed: %v\n%s", err, out)
			}
			calls, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			want := "<release>\n<create>\n<" + tt.tag + ">\n<--title>\n<" + tt.tag + ">\n<--notes>\n<Automated ws-mcp release assets for " + tt.tag + ".>\n"
			if tt.canary {
				want += "<--prerelease>\n<--latest=false>\n"
			}
			want += "<end>\n<release>\n<upload>\n<" + tt.tag + ">\n<dist/native.bin>\n<--clobber>\n<end>\n"
			if string(calls) != want {
				t.Fatalf("calls = %q, want %q", calls, want)
			}
		})
	}
	for _, required := range []string{
		"agents-plugin-tool/desktop-electron/dist/Werkschaft-${{ matrix.platform }}-${{ matrix.arch }}.zip",
		"agents-plugin-tool/desktop-electron/dist/Werkschaft-desktop-SHA256SUMS",
		"[['darwin', 'arm64'], ['win32', 'x64']]",
		"fs.writeFileSync(path.join(process.env.OUTPUT, SUMS), lines.join('\\n') + '\\n');",
		"gh release upload \"$GITHUB_REF_NAME\" \"$RUNNER_TEMP/desktop-release/\"* --repo \"$GITHUB_REPOSITORY\" --clobber",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("desktop release contract missing %q", required)
		}
	}
	_, desktop, ok := strings.Cut(text, "      - name: Publish desktop assets to the native assets' tag release\n")
	if !ok || !strings.Contains(desktop, "if: startsWith(github.ref, 'refs/tags/') && !inputs.desktop_dry_run") {
		t.Fatal("desktop upload must target the same tag release under the same dry-run veto")
	}
}
