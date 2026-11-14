package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsgit"
)

// writeLines writes n lines tagged with the file name, so rename detection
// never pairs two fixture files with identical content.
func writeLines(t *testing.T, root, name string, n int) {
	t.Helper()
	writeTaggedLines(t, root, name, name, n)
}

func writeTaggedLines(t *testing.T, root, name, tag string, n int) {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(strings.Repeat(tag+" line\n", n)), 0o644); err != nil {
		t.Fatal(err)
	}
}

// sizeFixture commits base (path -> lines) and an optional .gitattributes on
// develop, then commits branch (path -> lines; -1 deletes) on
// impl/develop/size-unit. It returns the repo root and the branch name.
func sizeFixture(t *testing.T, attributes string, base, branch map[string]int) (string, string) {
	t.Helper()
	root := initGitRepo(t)
	runGit(t, root, "switch", "-C", "develop")
	writeLines(t, root, "keep.txt", 1)
	for name, n := range base {
		writeLines(t, root, name, n)
	}
	if attributes != "" {
		if err := os.WriteFile(filepath.Join(root, ".gitattributes"), []byte(attributes), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "base")
	unit := "impl/develop/size-unit"
	runGit(t, root, "switch", "-c", unit)
	for name, n := range branch {
		if n < 0 {
			runGit(t, root, "rm", "-q", "--", name)
			continue
		}
		writeLines(t, root, name, n)
	}
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-m", "unit change")
	return root, unit
}

func sizeDiagnostics(r implMergeResult) map[string]implMergeDiagnostic {
	out := map[string]implMergeDiagnostic{}
	for _, d := range r.Diagnostics {
		if strings.HasPrefix(d.Code, "size_") {
			// Key by the file the reason names (the second word of every text
			// is not stable, so match the known fixture names instead).
			for _, f := range []string{"cross.go", "above.go", "big.go", "notes.txt", "off.go", "limited.go", "limited_big.go", "limited_small.go", "gen.go", "vendor/lib.go", "added.go", "moved.go", "dir name/odd[1].go"} {
				if strings.Contains(d.Reason, " "+f+" ") {
					if _, dup := out[f]; dup {
						panic("two size diagnostics for " + f)
					}
					out[f] = d
				}
			}
		}
	}
	return out
}

func mergeWithGuard(t *testing.T, root, branch string, guard mergeSizeGuard) implMergeResult {
	t.Helper()
	r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "", mergeMessage(), implMergeAcknowledgement{}, nil, guard)
	if err != nil || r.Status != "merged" {
		t.Fatalf("result=%+v err=%v", r, err)
	}
	return r
}

func TestMergeSizeWarningRules(t *testing.T) {
	attributes := strings.Join([]string{
		"off.go -ws-size-guard",
		"limited.go ws-size-guard=30",
		"limited_big.go ws-size-guard=30",
		"limited_small.go ws-size-guard=30",
		"gen.go linguist-generated",
		"vendor/** linguist-vendored",
	}, "\n") + "\n"
	base := map[string]int{
		"cross.go": 5, "above.go": 12, "big.go": 25, "notes.txt": 5, "off.go": 5,
		"limited.go": 25, "limited_big.go": 31, "limited_small.go": 5, "gen.go": 5, "vendor/lib.go": 5,
		"old.go": 15,
	}
	branch := map[string]int{
		"cross.go": 15, "above.go": 15, "big.go": 26, "notes.txt": 15, "off.go": 50,
		"limited.go": 35, "limited_big.go": 40, "limited_small.go": 28, "gen.go": 50, "vendor/lib.go": 50,
		"added.go": 15,
	}
	root, unit := sizeFixture(t, attributes, base, branch)
	// A rename keeps its merge-base size: 15 -> 16 lines is already above N.
	runGit(t, root, "mv", "old.go", "moved.go")
	writeTaggedLines(t, root, "moved.go", "old.go", 16)
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-m", "rename")

	r := mergeWithGuard(t, root, unit, newMergeSizeGuard(10, 20, "go"))
	got := sizeDiagnostics(r)
	want := map[string]string{
		"cross.go":   "size_soft_limit", // 5 -> 15 crosses N=10
		"big.go":     "size_hard_limit", // edited at 26 >= M=20
		"limited.go": "size_soft_limit", // 25 -> 35 crosses its own limit 30
		"added.go":   "size_soft_limit", // a new file counts as 0 at the merge base
	}
	for file, code := range want {
		d, ok := got[file]
		if !ok || d.Code != code || d.Classification != "advisory" {
			t.Fatalf("%s: want %s advisory, got %+v (all %+v)", file, code, d, r.Diagnostics)
		}
		if !strings.Contains(d.Resolution, "Do not split this file on your own") || !strings.Contains(d.Resolution, "Ask the user whether refactoring this project is permitted and whether to do it now") {
			t.Fatalf("%s: resolution lacks the ask-the-user instruction: %q", file, d.Resolution)
		}
		if !strings.Contains(d.Resolution, "/"+file+" ws-size-guard=100") || !strings.Contains(d.Resolution, "/"+file+" -ws-size-guard") {
			t.Fatalf("%s: resolution lacks the .gitattributes lines: %q", file, d.Resolution)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("unexpected size diagnostics: %+v", got)
	}
	if d := got["cross.go"]; d.Reason != "This change pushed cross.go past 10 lines (now 15); splitting it is recommended." {
		t.Fatalf("soft text = %q", d.Reason)
	}
	if d := got["big.go"]; d.Reason != "Edited big.go at 26 lines (>= 20); splitting it is recommended." {
		t.Fatalf("hard text = %q", d.Reason)
	}
	if !strings.Contains(got["limited.go"].Reason, "past 30 lines, its ws-size-guard limit (now 35)") {
		t.Fatalf("limit text = %q", got["limited.go"].Reason)
	}
	// The diagnostics reach the text rendering, and the merge still landed.
	if text := r.text(); !strings.Contains(text, "diagnostic: size_hard_limit [advisory]") || !r.BranchDeleted {
		t.Fatalf("text=%s deleted=%v", text, r.BranchDeleted)
	}
}

func TestMergeSizeWarningZeroDisables(t *testing.T) {
	base := map[string]int{"cross.go": 5, "big.go": 25}
	branch := map[string]int{"cross.go": 15, "big.go": 26}
	for _, tc := range []struct {
		name  string
		guard mergeSizeGuard
		want  []string
	}{
		{"soft-off", newMergeSizeGuard(0, 20, "go"), []string{"big.go"}},
		{"hard-off", newMergeSizeGuard(10, 0, "go"), []string{"cross.go"}},
		{"both-off", newMergeSizeGuard(0, 0, "go"), nil},
		{"no-extensions", newMergeSizeGuard(10, 20, ""), nil},
		{"dotted-upper-extension", newMergeSizeGuard(10, 20, ".GO"), []string{"cross.go", "big.go"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, unit := sizeFixture(t, "", base, branch)
			got := sizeDiagnostics(mergeWithGuard(t, root, unit, tc.guard))
			if len(got) != len(tc.want) {
				t.Fatalf("got %+v, want %v", got, tc.want)
			}
			for _, f := range tc.want {
				if _, ok := got[f]; !ok {
					t.Fatalf("missing %s in %+v", f, got)
				}
			}
		})
	}
}

func TestMergeSizeWarningScope(t *testing.T) {
	t.Run("generic-promotion-silent", func(t *testing.T) {
		root, unit := sizeFixture(t, "", map[string]int{"cross.go": 5}, map[string]int{"cross.go": 15})
		runGit(t, root, "branch", "-m", unit, "feature/size")
		r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, "feature/size", "develop", mergeMessage(), implMergeAcknowledgement{}, nil, newMergeSizeGuard(10, 20, "go"))
		if err != nil || r.Status != "merged" || len(r.Diagnostics) != 0 {
			t.Fatalf("result=%+v err=%v", r, err)
		}
	})
	t.Run("goal-source-warns", func(t *testing.T) {
		root, unit := sizeFixture(t, "", map[string]int{"cross.go": 5}, map[string]int{"cross.go": 15})
		runGit(t, root, "branch", "-m", unit, "goal/develop/size")
		r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, "goal/develop/size", "develop", mergeMessage(), implMergeAcknowledgement{}, nil, newMergeSizeGuard(10, 20, "go"))
		if err != nil || r.Status != "merged" {
			t.Fatalf("result=%+v err=%v", r, err)
		}
		requireMergeDiagnostic(t, r, "size_soft_limit", "advisory")
	})
	t.Run("conflict-silent", func(t *testing.T) {
		root, unit := sizeFixture(t, "", map[string]int{"cross.go": 5}, map[string]int{"cross.go": 15})
		runGit(t, root, "switch", "develop")
		if err := os.WriteFile(filepath.Join(root, "cross.go"), []byte(strings.Repeat("target\n", 14)), 0o644); err != nil {
			t.Fatal(err)
		}
		runGit(t, root, "commit", "-am", "target change")
		r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, unit, "", mergeMessage(), implMergeAcknowledgement{}, nil, newMergeSizeGuard(10, 20, "go"))
		if err != nil || r.Status != "conflict" || len(r.Diagnostics) != 0 {
			t.Fatalf("result=%+v err=%v", r, err)
		}
	})
	t.Run("quoted-pattern", func(t *testing.T) {
		root, unit := sizeFixture(t, "", nil, map[string]int{"dir name/odd[1].go": 15})
		got := sizeDiagnostics(mergeWithGuard(t, root, unit, newMergeSizeGuard(10, 20, "go")))
		d, ok := got["dir name/odd[1].go"]
		if !ok || !strings.Contains(d.Resolution, `"/dir name/odd\\[1].go" ws-size-guard=100`) {
			t.Fatalf("got %+v", got)
		}
	})
}

func TestMergeSizeWarningEdgeCases(t *testing.T) {
	t.Run("pure-rename-silent", func(t *testing.T) {
		root, unit := sizeFixture(t, "", map[string]int{"big.go": 25}, map[string]int{"other.txt": 1})
		runGit(t, root, "mv", "big.go", "moved.go")
		runGit(t, root, "commit", "-m", "move only")
		if got := sizeDiagnostics(mergeWithGuard(t, root, unit, newMergeSizeGuard(10, 20, "go"))); len(got) != 0 {
			t.Fatalf("a move with no content change warned: %+v", got)
		}
	})
	t.Run("boundaries-and-attribute-values", func(t *testing.T) {
		attributes := "above.go ws-size-guard\ngen.go ws-size-guard=abc\n"
		root, unit := sizeFixture(t, attributes,
			map[string]int{"cross.go": 5, "big.go": 5, "above.go": 5, "gen.go": 5},
			map[string]int{"cross.go": 10, "big.go": 20, "above.go": 11, "gen.go": 50})
		got := sizeDiagnostics(mergeWithGuard(t, root, unit, newMergeSizeGuard(10, 20, "go")))
		// cross.go at exactly N stays silent; big.go at exactly M is hard; a
		// bare ws-size-guard applies the configured limits; a malformed
		// ws-size-guard value skips the path.
		if len(got) != 2 || got["big.go"].Code != "size_hard_limit" || got["above.go"].Code != "size_soft_limit" {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("check-failure-keeps-merged", func(t *testing.T) {
		root, unit := sizeFixture(t, "", map[string]int{"cross.go": 5}, map[string]int{"cross.go": 15})
		runner := mergeInterceptRunner{intercept: func(_ context.Context, _ string, args []string) ([]byte, error, bool) {
			if len(args) > 0 && args[0] == "--literal-pathspecs" {
				return nil, errors.New("injected grep failure"), true
			}
			return nil, nil, false
		}}
		r, err := mergeImplBranch(context.Background(), root, runner, unit, "", mergeMessage(), implMergeAcknowledgement{}, nil, newMergeSizeGuard(10, 20, "go"))
		if err != nil || r.Status != "merged" || !r.BranchDeleted {
			t.Fatalf("result=%+v err=%v", r, err)
		}
		d := requireMergeDiagnostic(t, r, "size_check_failed", "advisory")
		if !strings.Contains(d.Reason, "injected grep failure") || len(r.Diagnostics) != 1 {
			t.Fatalf("diagnostics = %+v", r.Diagnostics)
		}
	})
}

func TestGitattributesPattern(t *testing.T) {
	for in, want := range map[string]string{
		"main.go":       "/main.go",
		"a/b.go":        "/a/b.go",
		"#x.go":         "/#x.go",
		"a*b?.go":       `/a\*b\?.go`,
		"dir name/x.go": `"/dir name/x.go"`,
		`q"uote\[1].go`: `"/q\"uote\\\\\\[1].go"`,
		"tab\tname.go":  `"/tab\tname.go"`,
	} {
		if got := gitattributesPattern(in); got != want {
			t.Errorf("gitattributesPattern(%q) = %s, want %s", in, got, want)
		}
	}
	// The rendered pattern must match exactly its own path through git.
	root := initGitRepo(t)
	for _, name := range []string{"dir name/odd[1].go", "a*b.go", "plain.go"} {
		// check-attr reads only the path, so the file is optional: Windows
		// cannot create a name with '*', and its check still runs there.
		if runtime.GOOS != "windows" || !strings.ContainsAny(name, `*?`) {
			writeLines(t, root, name, 1)
		}
		if err := os.WriteFile(filepath.Join(root, ".gitattributes"), []byte(gitattributesPattern(name)+" -ws-size-guard\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		out := string(runGitOutput(t, root, "check-attr", "ws-size-guard", "--", name))
		if !strings.HasSuffix(strings.TrimSpace(out), ": unset") {
			t.Fatalf("pattern for %q does not match it: %s", name, out)
		}
	}
	if got := mergeSizeMargin(1050); got != 1300 {
		t.Fatalf("margin(1050) = %d", got)
	}
}

func TestGitMergeSizeConfigKeys(t *testing.T) {
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	root, unit := sizeFixture(t, "", map[string]int{"cross.go": 5, "notes.md": 5}, map[string]int{"cross.go": 15, "notes.md": 15})
	root = canonicalRootForTest(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	s := NewServer(root, "test")
	key, err := s.sessions.mint(root, roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	catalog := parseTuningCatalogResponse(t, keyedCall(t, s, 1, key, "config.list", map[string]any{"format": "json"}))
	for id, def := range map[string]string{
		wsconfig.ItemGitMergeSizeSoftLines:  "1000",
		wsconfig.ItemGitMergeSizeHardLines:  "2000",
		wsconfig.ItemGitMergeSizeExtensions: defaultMergeSizeExtensions,
	} {
		knob := requireTuningKnob(t, catalog, id)
		if !strings.Contains(mustMarshalJSON(t, knob.Current), `"value":"`+def+`"`) {
			t.Fatalf("%s current = %s, want default %q", id, mustMarshalJSON(t, knob.Current), def)
		}
	}
	for i, bad := range []string{"-1", "abc"} {
		if resp := keyedCall(t, s, 10+i, key, "config.tune", map[string]any{"key": wsconfig.ItemGitMergeSizeSoftLines, "value": bad}); !toolIsError(t, resp) {
			t.Fatalf("config.tune accepted soft lines %q: %s", bad, resp)
		}
	}
	for i, kv := range [][2]string{{wsconfig.ItemGitMergeSizeSoftLines, "10"}, {wsconfig.ItemGitMergeSizeHardLines, "0"}, {wsconfig.ItemGitMergeSizeExtensions, "md"}} {
		if resp := keyedCall(t, s, 20+i, key, "config.tune", map[string]any{"key": kv[0], "value": kv[1]}); toolIsError(t, resp) {
			t.Fatalf("config.tune %s=%s: %s", kv[0], kv[1], resp)
		}
	}
	// The dispatch reads the tuned keys: only notes.md (the tuned extension)
	// warns, at the tuned soft limit.
	resp := keyedCall(t, s, 30, key, "git.merge", map[string]any{"branch": unit, "title": "merge(test): size", "ai_context": []string{"Test tuned size keys."}, "format": "json"})
	var r implMergeResult
	if err := json.Unmarshal([]byte(toolText(t, resp)), &r); err != nil {
		t.Fatalf("%v: %s", err, resp)
	}
	d := requireMergeDiagnostic(t, r, "size_soft_limit", "advisory")
	if !strings.Contains(d.Reason, "notes.md past 10 lines") || len(r.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %+v", r.Diagnostics)
	}
	// A manifest may not claim the git. namespace.
	for _, reserved := range reservedConfigPrefixes {
		if reserved == "git." {
			return
		}
	}
	t.Fatal("git. is not a reserved config prefix")
}
