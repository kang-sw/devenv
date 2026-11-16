package wsagent

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsrsrc"
)

// An unavailable legacy/corrupt Pi bundle keeps the existing Claude backend
// error boundary. Neither held dashboard text nor claimed mail is consumed.
func TestPiDaemonWorkflowBackendFailureRetainsInput(t *testing.T) {
	h := newLeadHarness(t).start()
	t.Setenv(wsrsrc.EnvRsrcRoot, filepath.Join(t.TempDir(), "missing", "rsrc"))
	id := h.lead(BackendClaude, LeadOptions{}).ID
	h.send(id, "held dashboard input")
	eventually(t, "the existing Claude plugin error", func() bool {
		st := h.state(id)
		return !st.Process && st.Held == 1 && strings.Contains(st.Error, "bundled plugin")
	})
	key := h.record(id).WSKey
	eventually(t, "the mail watch", func() bool { return h.mail.watched(key) })
	h.mail.deliver(key, "From fixture: retained mail")
	eventually(t, "claimed mail retained after start failure", func() bool {
		mail := h.record(id).Mail
		return len(mail) == 1 && mail[0] == "From fixture: retained mail"
	})
	h.quiesce(id)
	// Held includes dashboard inbox plus claimed mail (runner.go state projection).
	if st := h.state(id); st.Process || st.Held != 2 || !strings.Contains(st.Error, "bundled plugin") {
		t.Fatalf("existing failure must retain held input: %+v", st)
	}
	if h.holders.opCount("spawn") != 0 {
		t.Fatal("invalid Claude package must not spawn a harness")
	}
}

// Exercise the unchanged Go readers, not a second model of their path rules.
// Python only imports copied launcher APIs and assembles offline fixtures; it
// never executes a launcher entrypoint, ws binary, daemon, Node or Pi process.
// Actual SDK/class identity is covered separately by
// testdata/pi-native-payload-identity.test.mjs (actual CLI-adjacent SDK).
func TestPiDaemonWorkflowOldReaderSeam(t *testing.T) {
	python := lookPython(t)
	repo, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	const fixtureScript = `
import importlib.util, json, pathlib, sys
path = pathlib.Path(sys.argv[1]) / "agents-plugin/tests/test_daemon_workflow_bundle.py"
spec = importlib.util.spec_from_file_location("workflow_contract_fixture", path)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
print(json.dumps(module.reader_fixture(pathlib.Path(sys.argv[2]), sys.argv[3])))
`
	for _, lane := range []string{"plain", "native-ws", "native-wsflow", "legacy-ws", "legacy-wsflow"} {
		t.Run(lane, func(t *testing.T) {
			base, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(python, "-I", "-B", "-c", fixtureScript, repo, base, lane)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			raw, err := cmd.Output()
			if err != nil {
				t.Fatalf("Python package/start-env fixture: %v\n%s", err, stderr.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("package-only fixture must not fall back: %s", stderr.String())
			}
			var fixture struct {
				PiRoot   string            `json:"pi_root"`
				Identity string            `json:"identity"`
				StartEnv map[string]string `json:"start_env"`
				HelloEnv map[string]string `json:"hello_env"`
			}
			if err := json.Unmarshal(raw, &fixture); err != nil {
				t.Fatalf("fixture JSON: %v\n%s", err, raw)
			}
			if fixture.HelloEnv[wsrsrc.EnvRsrcRoot] != filepath.Join(fixture.PiRoot, "rsrc") {
				t.Fatalf("caller Pi resources were changed: %v", fixture.HelloEnv)
			}
			for key, value := range fixture.StartEnv {
				t.Setenv(key, value)
			}
			root, err := bundledPluginRoot()
			if err != nil {
				t.Fatal(err)
			}
			wantRoot := filepath.Dir(fixture.StartEnv[wsrsrc.EnvRsrcRoot])
			if root != wantRoot || filepath.Base(root) != fixture.Identity {
				t.Fatalf("canonical Claude package = %q; want %q (%s)", root, wantRoot, fixture.Identity)
			}
			if fileExists(filepath.Join(root, "native-pi")) {
				t.Fatal("archive recursively embeds native payload instead of using the old sibling seam")
			}
			product := "agents-plugin"
			if fixture.Identity == "wsflow" {
				product += "-wsflow"
			}
			for _, name := range []string{".claude-plugin/plugin.json", "config-manifest.json", "hooks/hooks.json"} {
				got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
				if err != nil {
					t.Fatal(err)
				}
				want, err := os.ReadFile(filepath.Join(repo, product, filepath.FromSlash(name)))
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, want) {
					t.Fatalf("wrong owning Claude assets/defaults: %s", name)
				}
			}
			entries, err := piBundledEntries()
			if err != nil || len(entries) != 1 {
				t.Fatalf("old Pi reader must return a real entry, not the MCP bridge: %v, %v", entries, err)
			}
			wantEntry := filepath.Join(filepath.Dir(root), "agents-plugin-pi", "index.mjs")
			if entries[0] != wantEntry {
				t.Fatalf("old sibling entry = %v; want %s", entries, wantEntry)
			}
			shim, err := os.ReadFile(entries[0])
			if err != nil {
				t.Fatal(err)
			}
			encodedURI := strings.TrimSuffix(strings.TrimPrefix(string(shim), "export { default } from "), ";\n")
			var uri string
			if err := json.Unmarshal([]byte(encodedURI), &uri); err != nil {
				t.Fatalf("not an ESM forwarding entry: %q: %v", shim, err)
			}
			// Python's as_uri handles drive letters/escaping; resolving it in
			// Python verifies the actual original target on either platform.
			original := filepath.Join(fixture.PiRoot, "dist", "index.js")
			uriCheck := exec.Command(python, "-I", "-B", "-c", `import pathlib,sys; assert pathlib.Path(sys.argv[1]).resolve().as_uri() == sys.argv[2]`, original, uri)
			if output, err := uriCheck.CombinedOutput(); err != nil {
				t.Fatalf("shim relocated the original module target: %v\n%s", err, output)
			}
			for _, entry := range []string{entries[0], original} {
				if missing := piMissingLoadDeps(entry); len(missing) != 0 {
					t.Fatalf("dependency ancestor lost for %s: %v", entry, missing)
				}
			}
			if rel, err := filepath.Rel(fixture.PiRoot, entries[0]); err != nil || !strings.HasPrefix(filepath.ToSlash(rel), ".runtime/workflow/") {
				t.Fatalf("forwarding sibling lost the original dependency ancestor: %s, %v", rel, err)
			}
			if fileExists(filepath.Join(base, "agents-plugin-pi", "package.json")) {
				t.Fatal("test accidentally supplied a checkout Pi sibling")
			}
		})
	}
}
