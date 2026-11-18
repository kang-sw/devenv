package mcp

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsrsrc"
)

const (
	profileProbeBase   = "PROFILE-PROBE-BASE"
	profileProbeOrch   = "PROFILE-PROBE-ORCHESTRATE"
	profileManualOrch  = "PROFILE-MANUAL-ORCHESTRATE"
	profileProbePbName = "profile-probe"
)

// newProfileFixture is a whiteboard fixture whose rsrc root is a copy of the
// shipped tree plus an orchestrate variant of the workflow manual and a
// profile-probe print playbook with a base and an orchestrate file.
func newProfileFixture(t *testing.T) *whiteboardFixture {
	t.Helper()
	f := newWhiteboardFixture(t)
	shipped := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	root := t.TempDir()
	err := filepath.WalkDir(shipped, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(shipped, path)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(root, rel), 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(root, rel), data, 0o644)
	})
	if err != nil {
		t.Fatalf("copy rsrc: %v", err)
	}
	manual, err := os.ReadFile(filepath.Join(root, "lead-workflow-manual", "lead-workflow-manual.md"))
	if err != nil {
		t.Fatalf("read manual: %v", err)
	}
	mustWrite(t, root, "lead-workflow-manual/lead-workflow-manual.orchestrate.md", string(manual)+"\n"+profileManualOrch+"\n")
	mustWrite(t, root, profileProbePbName+"/"+profileProbePbName+".md", "---\nkind: print\n---\n"+profileProbeBase+"\n")
	mustWrite(t, root, profileProbePbName+"/"+profileProbePbName+".orchestrate.md", "---\nkind: print\n---\n"+profileProbeOrch+"\n")
	m, err := wsrsrc.GenerateManifest(root)
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}
	if err := wsrsrc.WriteManifest(root, m); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}
	useResourceFixture(t, root)
	return f
}

func (f *whiteboardFixture) setProfile(t *testing.T, value string) {
	t.Helper()
	resp := f.call(t, "config.tune", map[string]any{"key": "workflow.profile", "value": value})
	if isToolError(resp) {
		t.Fatalf("config.tune workflow.profile=%s: %s", value, toolText(t, resp))
	}
}

func (f *whiteboardFixture) readProbe(t *testing.T, key string) string {
	t.Helper()
	args := map[string]any{"name": profileProbePbName}
	args["session_key"] = key
	resp := f.call(t, "playbook.read", args)
	if isToolError(resp) {
		t.Fatalf("playbook.read: %s", toolText(t, resp))
	}
	return toolText(t, resp)
}

func (f *whiteboardFixture) renderProbe(t *testing.T, key string) string {
	t.Helper()
	resp := f.call(t, "playbook.render", map[string]any{"name": profileProbePbName, "session_key": key})
	if isToolError(resp) {
		t.Fatalf("playbook.render: %s", toolText(t, resp))
	}
	path := strings.TrimSpace(strings.SplitN(toolText(t, resp), "\n", 2)[0])
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rendered %q: %v", path, err)
	}
	return string(data)
}

func wantProbe(t *testing.T, label, body, want string) {
	t.Helper()
	other := profileProbeOrch
	if want == profileProbeOrch {
		other = profileProbeBase
	}
	if !strings.Contains(body, want) || strings.Contains(body, other) {
		t.Fatalf("%s: want %s only, got %q", label, want, body)
	}
}

func TestPlaybookProfileDefaultDirectReadsBaseFiles(t *testing.T) {
	f := newProfileFixture(t)
	wantProbe(t, "read", f.readProbe(t, f.key), profileProbeBase)
	wantProbe(t, "render", f.renderProbe(t, f.key), profileProbeBase)
	if manual := toolText(t, f.call(t, "workflow_manual", map[string]any{})); strings.Contains(manual, profileManualOrch) {
		t.Fatalf("direct profile manual carries the orchestrate variant")
	}
}

func TestPlaybookProfileOrchestrateSelectsVariantForTopLevelLead(t *testing.T) {
	f := newProfileFixture(t)
	f.setProfile(t, "orchestrate")
	wantProbe(t, "read", f.readProbe(t, f.key), profileProbeOrch)
	wantProbe(t, "render", f.renderProbe(t, f.key), profileProbeOrch)
	if manual := toolText(t, f.call(t, "workflow_manual", map[string]any{})); !strings.Contains(manual, profileManualOrch) {
		t.Fatalf("orchestrate profile manual lacks the variant")
	}
	// FRESH with root mints a top-level lead and reads the global value
	// through the canonical-root resolver.
	fresh := toolText(t, f.call(t, "workflow_manual", map[string]any{"session_key": freshBootstrapKey, "root": f.root}))
	if !strings.Contains(fresh, profileManualOrch) {
		t.Fatalf("FRESH manual under orchestrate lacks the variant")
	}
}

func TestPlaybookProfileParentedAndKeylessLoadsReadBaseFiles(t *testing.T) {
	f := newProfileFixture(t)
	f.setProfile(t, "orchestrate")
	worker, err := f.s.sessions.mint(f.root, roleLead, f.key)
	if err != nil {
		t.Fatalf("mint worker: %v", err)
	}
	wantProbe(t, "parented read", f.readProbe(t, worker), profileProbeBase)
	wantProbe(t, "parented render", f.renderProbe(t, worker), profileProbeBase)
	if manual := toolText(t, f.call(t, "workflow_manual", map[string]any{"session_key": worker})); strings.Contains(manual, profileManualOrch) {
		t.Fatalf("a parented key's manual must be the base one")
	}
	wantProbe(t, "keyless read", f.readProbe(t, ""), profileProbeBase)
}
