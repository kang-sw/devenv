package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
)

// gitSearchFixture is a lead fixture whose ws-mcp allowlists git.search,
// with one repository (two commits: a.txt and sub/b.txt, then a third line
// in a.txt) under a root a plain session registration names.
type gitSearchFixture struct {
	*whiteboardFixture
	// root is the registered root, canonical; repo is the repository in it.
	root string
	repo string
}

func canonicalTestDir(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func newGitSearchFixture(t *testing.T) *gitSearchFixture {
	t.Helper()
	setTrackerGitEnv(t)
	// git.search drops every GIT_* variable, so its git reads HOME's
	// config; point HOME at an empty directory.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	f := newWhiteboardFixture(t)
	t.Setenv("WS_MCP_ALLOWED_TOOLS", gitSearchTool+",config.tune")

	root := canonicalTestDir(t, t.TempDir())
	repo := filepath.Join(root, "proj")
	mustWrite(t, repo, "a.txt", "hello\nworld\n")
	mustWrite(t, repo, "sub/b.txt", "foo bar\n")
	initGit(t, repo)
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-qm", "one")
	mustWrite(t, repo, "a.txt", "hello\nworld\nagain\n")
	runGit(t, repo, "commit", "-qam", "two")
	registerGitSearchSession(t, "plainsession", root, false, time.Now())
	return &gitSearchFixture{whiteboardFixture: f, root: root, repo: repo}
}

func registerGitSearchSession(t *testing.T, replyID, project string, agent bool, seen time.Time) {
	t.Helper()
	dir, err := wsdashboard.Dir("")
	if err != nil {
		t.Fatal(err)
	}
	stamp := seen.UTC().Format(time.RFC3339)
	if err := wsdashboard.WriteRegistration(dir, wsdashboard.Registration{
		ReplyID: replyID, Project: project, Agent: agent, RegisteredAt: stamp, HeartbeatAt: stamp,
	}); err != nil {
		t.Fatal(err)
	}
}

func (f *gitSearchFixture) search(t *testing.T, args map[string]any) (string, bool) {
	t.Helper()
	resp := f.call(t, gitSearchTool, args)
	return toolText(t, resp), isToolError(resp)
}

func (f *gitSearchFixture) mustSearch(t *testing.T, args map[string]any) string {
	t.Helper()
	text, failed := f.search(t, args)
	if failed {
		t.Fatalf("git.search(%v) failed: %s", args, text)
	}
	return text
}

func TestGitSearchListsRootsWithoutRepo(t *testing.T) {
	f := newGitSearchFixture(t)
	agentRoot := canonicalTestDir(t, t.TempDir())
	registerGitSearchSession(t, "agentconversation", agentRoot, true, time.Now())
	knobRoot := canonicalTestDir(t, t.TempDir())
	missing := filepath.Join(knobRoot, "missing")
	value := strings.Join([]string{knobRoot, "relative/dir", missing}, string(filepath.ListSeparator))
	if resp := f.call(t, "config.tune", map[string]any{"key": wsconfig.ItemAgentGitSearchRoots, "value": value}); isToolError(resp) {
		t.Fatalf("config.tune: %s", toolText(t, resp))
	}

	// A command without repo still only lists.
	text := f.mustSearch(t, map[string]any{"command": "log"})
	for _, want := range []string{
		f.root + " (session)",
		knobRoot + " (" + wsconfig.ItemAgentGitSearchRoots + ")",
		"relative/dir: not an absolute path",
		missing + ": does not exist or is not a directory",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("listing lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, agentRoot) {
		t.Errorf("an agent conversation's root must not be listed:\n%s", text)
	}
}

func TestGitSearchRefusesUnknownSession(t *testing.T) {
	f := newGitSearchFixture(t)
	text, failed := f.search(t, map[string]any{"session_key": "no-such-key", "repo": f.repo, "command": "log"})
	if !failed || !strings.Contains(text, "unknown_session") {
		t.Fatalf("an unknown key must be refused as unknown: %s", text)
	}
}

func TestGitSearchPathScope(t *testing.T) {
	f := newGitSearchFixture(t)
	outside := canonicalTestDir(t, t.TempDir())
	initGit(t, outside)

	text, failed := f.search(t, map[string]any{"repo": outside, "command": "ls-files"})
	if !failed || !strings.Contains(text, "outside every root") || !strings.Contains(text, f.root+" (session)") {
		t.Fatalf("a path outside every root must be refused with the roots listed: %s", text)
	}
	if text, failed := f.search(t, map[string]any{"repo": "proj", "command": "ls-files"}); !failed || !strings.Contains(text, "not an absolute path") {
		t.Fatalf("a relative repo must be refused: %s", text)
	}

	// At the root itself: discovery climbs no higher than the root.
	text, failed = f.search(t, map[string]any{"repo": f.root, "command": "ls-files"})
	if !failed || !strings.Contains(strings.ToLower(text), "not a git repository") {
		t.Fatalf("the root holds no repository, and discovery must stop there: %s", text)
	}
	if got := f.mustSearch(t, map[string]any{"repo": f.repo, "command": "ls-files"}); got != "a.txt\nsub/b.txt\n" {
		t.Fatalf("ls-files at the repository = %q", got)
	}
	if got := f.mustSearch(t, map[string]any{"repo": filepath.Join(f.repo, "sub"), "command": "ls-files"}); got != "b.txt\n" {
		t.Fatalf("ls-files in a subdirectory = %q", got)
	}

	// A symlink outside every root, pointing at a directory inside one, is
	// read as its target.
	in := filepath.Join(outside, "link-in")
	if err := os.Symlink(filepath.Join(f.repo, "sub"), in); err != nil {
		t.Fatal(err)
	}
	if got := f.mustSearch(t, map[string]any{"repo": in, "command": "ls-files"}); got != "b.txt\n" {
		t.Fatalf("ls-files through a symlink into a root = %q", got)
	}
	// A symlink inside a root pointing outside every root is refused.
	out := filepath.Join(f.repo, "link-out")
	if err := os.Symlink(outside, out); err != nil {
		t.Fatal(err)
	}
	if text, failed := f.search(t, map[string]any{"repo": out, "command": "ls-files"}); !failed || !strings.Contains(text, "outside every root") {
		t.Fatalf("a symlink out of every root must be refused: %s", text)
	}
}

func TestGitSearchRegistrationsAndKnobRoots(t *testing.T) {
	f := newGitSearchFixture(t)
	repoAt := func(name string) string {
		dir := filepath.Join(canonicalTestDir(t, t.TempDir()), name)
		mustWrite(t, dir, "x.txt", "x\n")
		initGit(t, dir)
		runGit(t, dir, "add", ".")
		runGit(t, dir, "commit", "-qm", "x")
		return dir
	}
	refused := func(repo string) bool {
		_, failed := f.search(t, map[string]any{"repo": repo, "command": "ls-files"})
		return failed
	}

	agent := repoAt("agent")
	registerGitSearchSession(t, "agentconversation", agent, true, time.Now())
	if !refused(agent) {
		t.Errorf("an agent conversation's root must not count")
	}
	stale := repoAt("stale")
	registerGitSearchSession(t, "stalesession", stale, false, time.Now().Add(-wsdashboard.RegistryRetention-time.Hour))
	if !refused(stale) {
		t.Errorf("a registration past the retention window must not count")
	}
	plain := repoAt("plain")
	registerGitSearchSession(t, "plainother", plain, false, time.Now())
	if refused(plain) {
		t.Errorf("a plain registration's root must count")
	}

	knob := repoAt("knob")
	if !refused(knob) {
		t.Fatalf("an unlisted directory must be refused")
	}
	if resp := f.call(t, "config.tune", map[string]any{"key": wsconfig.ItemAgentGitSearchRoots, "value": knob, "scope": "project"}); !isToolError(resp) {
		t.Fatalf("the knob is global-only: %s", toolText(t, resp))
	}
	if resp := f.call(t, "config.tune", map[string]any{"key": wsconfig.ItemAgentGitSearchRoots, "value": knob}); isToolError(resp) {
		t.Fatalf("config.tune: %s", toolText(t, resp))
	}
	if got := f.mustSearch(t, map[string]any{"repo": knob, "command": "ls-files"}); got != "x.txt\n" {
		t.Fatalf("ls-files under a knob root = %q", got)
	}
}

// TestGitSearchRootsKnobIsItsOwnKey pins that agent.git_search_roots
// resolves as its own lead-only scalar key while agent.profile.<name> keeps
// its structured family.
func TestGitSearchRootsKnobIsItsOwnKey(t *testing.T) {
	entry, ok := resolveConfigEntryForKey(wsconfig.ItemAgentGitSearchRoots)
	if !ok || entry.Key != wsconfig.ItemAgentGitSearchRoots || entry.Structured || !entry.RequiresLeadAuthority || !entry.GlobalOnly() {
		t.Fatalf("agent.git_search_roots entry = %+v, %v", entry, ok)
	}
	profile, ok := resolveConfigEntryForKey("agent.profile.demo")
	if !ok || profile.Key != "agent.profile.demo" || !profile.Structured {
		t.Fatalf("agent.profile.demo entry = %+v, %v", profile, ok)
	}
}

func TestGitSearchRefusals(t *testing.T) {
	f := newGitSearchFixture(t)
	written := filepath.Join(t.TempDir(), "written")
	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"grep -O", map[string]any{"command": "grep", "pattern": "x", "options": []any{"-O"}}, "-O runs a program"},
		{"grep -O attached", map[string]any{"command": "grep", "pattern": "x", "options": []any{"-Oless"}}, "-O runs a program"},
		{"grep open-files-in-pager", map[string]any{"command": "grep", "pattern": "x", "options": []any{"--open-files-in-pager=less"}}, "--open-files-in-pager runs a pager"},
		{"grep no-index", map[string]any{"command": "grep", "pattern": "x", "options": []any{"--no-index"}}, "--no-index searches outside"},
		{"grep -f", map[string]any{"command": "grep", "pattern": "x", "options": []any{"-f"}}, "-f reads patterns from a file"},
		{"grep -f attached", map[string]any{"command": "grep", "pattern": "x", "options": []any{"-f/etc/passwd"}}, "-f reads patterns from a file"},
		{"blame contents", map[string]any{"command": "blame", "paths": []any{"a.txt"}, "options": []any{"--contents=/etc/passwd"}}, "--contents reads a file"},
		{"blame -S", map[string]any{"command": "blame", "paths": []any{"a.txt"}, "options": []any{"-S"}}, "-S reads revisions from a file"},
		{"blame -S attached", map[string]any{"command": "blame", "paths": []any{"a.txt"}, "options": []any{"-S/etc/passwd"}}, "-S reads revisions from a file"},
		{"blame ignore-revs-file", map[string]any{"command": "blame", "paths": []any{"a.txt"}, "options": []any{"--ignore-revs-file=/etc/passwd"}}, "--ignore-revs-file reads a file"},
		{"log ext-diff", map[string]any{"command": "log", "options": []any{"--ext-diff"}}, "--ext-diff runs an external diff"},
		{"show textconv", map[string]any{"command": "show", "options": []any{"--textconv"}}, "--textconv runs textconv"},
		{"log output", map[string]any{"command": "log", "options": []any{"--output=" + written}}, "--output writes its output to a file"},
		{"show output", map[string]any{"command": "show", "options": []any{"--output=" + written}}, "--output writes"},
		{"grep output", map[string]any{"command": "grep", "pattern": "x", "options": []any{"--output=" + written}}, "--output writes"},
		{"blame output", map[string]any{"command": "blame", "paths": []any{"a.txt"}, "options": []any{"--output=" + written}}, "--output writes"},
		{"ls-files output", map[string]any{"command": "ls-files", "options": []any{"--output=" + written}}, "--output writes"},
		{"log format %G", map[string]any{"command": "log", "options": []any{"--format=%h %G?"}}, "%G signature placeholder"},
		{"log pretty %G", map[string]any{"command": "log", "options": []any{"--pretty=format:%GS"}}, "%G signature placeholder"},
		{"show format %G", map[string]any{"command": "show", "options": []any{"--format=tformat:%GK"}}, "%G signature placeholder"},
		{"log pretty alias", map[string]any{"command": "log", "options": []any{"--pretty=mine"}}, "names neither a built-in format"},
		{"grep -c config", map[string]any{"command": "grep", "pattern": "x", "options": []any{"-c", "core.pager=evil"}}, `"-c" is a git global option`},
		{"log -c attached", map[string]any{"command": "log", "options": []any{"-ccore.pager=evil"}}, "is a git global option"},
		{"grep -C dir", map[string]any{"command": "grep", "pattern": "x", "options": []any{"-C", "/tmp"}}, `"-C" is a git global option`},
		{"blame -C attached dir", map[string]any{"command": "blame", "paths": []any{"a.txt"}, "options": []any{"-C/tmp"}}, "is a git global option"},
		{"log git-dir", map[string]any{"command": "log", "options": []any{"--git-dir=/tmp/x"}}, "is a git global option"},
		{"log git-dir separate", map[string]any{"command": "log", "options": []any{"--git-dir", "/tmp/x"}}, "is a git global option"},
		{"show work-tree", map[string]any{"command": "show", "options": []any{"--work-tree=/tmp"}}, "is a git global option"},
		{"log exec-path", map[string]any{"command": "log", "options": []any{"--exec-path=/tmp"}}, "is a git global option"},
		{"ls-files namespace", map[string]any{"command": "ls-files", "options": []any{"--namespace=x"}}, "is a git global option"},
		{"revision with dash", map[string]any{"command": "log", "revisions": []any{"--output=" + written}}, "starts with -"},
		{"revision single dash", map[string]any{"command": "show", "revisions": []any{"-p"}}, "starts with -"},
		{"ls-files revision", map[string]any{"command": "ls-files", "revisions": []any{"HEAD"}}, "takes no revisions"},
		{"blame two revisions", map[string]any{"command": "blame", "revisions": []any{"HEAD", "HEAD~1"}, "paths": []any{"a.txt"}}, "at most one revision"},
		{"blame no path", map[string]any{"command": "blame"}, "exactly one path"},
		{"grep no pattern", map[string]any{"command": "grep"}, "grep needs a pattern"},
		{"pattern outside grep", map[string]any{"command": "log", "pattern": "x"}, "pattern is for grep only"},
		{"unknown command", map[string]any{"command": "push"}, "is not one of"},
		{"missing command", map[string]any{}, "command is required"},
		{"bare value", map[string]any{"command": "log", "options": []any{"5"}}, "is not a flag"},
		{"end of options", map[string]any{"command": "log", "options": []any{"--end-of-options"}}, "placed by the tool"},
		{"valued flag without value", map[string]any{"command": "log", "options": []any{"--max-count"}}, "write it as --max-count=<value>"},
		{"short flag without value", map[string]any{"command": "log", "options": []any{"-n"}}, "write it as -n<value>"},
		{"bad number", map[string]any{"command": "log", "options": []any{"-nfive"}}, "takes a whole number"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.args["repo"] = f.repo
			text, failed := f.search(t, tc.args)
			if !failed || !strings.Contains(text, tc.want) {
				t.Fatalf("want a refusal containing %q, got (error %v): %s", tc.want, failed, text)
			}
		})
	}
	if _, err := os.Stat(written); !os.IsNotExist(err) {
		t.Fatalf("a refused --output must write nothing: %v", err)
	}

	// An unlisted flag names the subcommand's accepted flags.
	text, failed := f.search(t, map[string]any{"repo": f.repo, "command": "blame", "paths": []any{"a.txt"}, "options": []any{"--incremental-nope"}})
	if !failed || !strings.Contains(text, "is not accepted by blame") || !strings.Contains(text, "blame accepts:") || !strings.Contains(text, "-L<value>") || !strings.Contains(text, "--porcelain") {
		t.Fatalf("an unlisted flag must list the accepted ones: %s", text)
	}
}

func TestGitSearchCommands(t *testing.T) {
	f := newGitSearchFixture(t)
	// An inherited GIT_* variable must not steer the tool's git.
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "nowhere"))

	if got := f.mustSearch(t, map[string]any{"repo": f.repo, "command": "log", "options": []any{"--format=%s"}}); got != "two\none\n" {
		t.Errorf("log = %q", got)
	}
	if got := f.mustSearch(t, map[string]any{"repo": f.repo, "command": "log", "options": []any{"--format=%s", "-n1"}, "paths": []any{"sub"}}); got != "one\n" {
		t.Errorf("log of a path = %q", got)
	}
	if got := f.mustSearch(t, map[string]any{"repo": f.repo, "command": "log", "options": []any{"--oneline", "-Sagain"}}); !strings.Contains(got, "two") || strings.Contains(got, "one") {
		t.Errorf("log pickaxe = %q", got)
	}
	if got := f.mustSearch(t, map[string]any{"repo": f.repo, "command": "show", "revisions": []any{"HEAD~1:a.txt"}}); got != "hello\nworld\n" {
		t.Errorf("show <rev>:<path> = %q", got)
	}
	if got := f.mustSearch(t, map[string]any{"repo": f.repo, "command": "show", "options": []any{"--stat", "--format=%s"}, "revisions": []any{"HEAD"}}); !strings.Contains(got, "two") || !strings.Contains(got, "a.txt | 1 +") {
		t.Errorf("show --stat = %q", got)
	}
	if got := f.mustSearch(t, map[string]any{"repo": f.repo, "command": "grep", "pattern": "hello", "options": []any{"-n"}}); got != "a.txt:1:hello\n" {
		t.Errorf("grep = %q", got)
	}
	if got := f.mustSearch(t, map[string]any{"repo": f.repo, "command": "grep", "pattern": "world", "revisions": []any{"HEAD~1"}, "paths": []any{"a.txt"}}); got != "HEAD~1:a.txt:world\n" {
		t.Errorf("grep in a revision = %q", got)
	}
	if got := f.mustSearch(t, map[string]any{"repo": f.repo, "command": "grep", "pattern": "nothing-matches-this"}); got != "no matches\n" {
		t.Errorf("grep without a match = %q", got)
	}
	if got := f.mustSearch(t, map[string]any{"repo": f.repo, "command": "blame", "options": []any{"-L3,3", "-s"}, "revisions": []any{"HEAD"}, "paths": []any{"a.txt"}}); !strings.HasSuffix(got, "3) again\n") {
		t.Errorf("blame at a revision = %q", got)
	}
	if got := f.mustSearch(t, map[string]any{"repo": f.repo, "command": "blame", "options": []any{"--porcelain"}, "paths": []any{"a.txt"}}); !strings.Contains(got, "\thello\n") || !strings.Contains(got, "summary two") {
		t.Errorf("blame --porcelain = %q", got)
	}
	if got := f.mustSearch(t, map[string]any{"repo": f.repo, "command": "ls-files", "paths": []any{"sub"}}); got != "sub/b.txt\n" {
		t.Errorf("ls-files = %q", got)
	}

	// A git failure answers git's stderr.
	text, failed := f.search(t, map[string]any{"repo": f.repo, "command": "show", "revisions": []any{"no-such-rev"}})
	if !failed || !strings.Contains(text, "no-such-rev") {
		t.Errorf("a bad revision must answer git's error: %s", text)
	}
}

func TestGitSearchCapsOutput(t *testing.T) {
	f := newGitSearchFixture(t)
	line := strings.Repeat("x", 99) + "\n"
	mustWrite(t, f.repo, "big.txt", strings.Repeat(line, 2000))
	runGit(t, f.repo, "add", "big.txt")
	runGit(t, f.repo, "commit", "-qm", "big")
	got := f.mustSearch(t, map[string]any{"repo": f.repo, "command": "show", "revisions": []any{"HEAD:big.txt"}})
	if !strings.Contains(got, "[output truncated at 64 KiB") {
		t.Fatalf("a large output must carry the truncation note: %d bytes", len(got))
	}
	if len(got) > gitSearchOutputCap+512 {
		t.Fatalf("output is %d bytes, over the cap", len(got))
	}
}

// TestGitSearchDashPaths pins the per-subcommand argument shape: a path
// starting with - stays a path for every subcommand, blame included with and
// without a revision, and ls-files with no "--" before its paths.
func TestGitSearchDashPaths(t *testing.T) {
	f := newGitSearchFixture(t)
	mustWrite(t, f.repo, "-dash.txt", "dash line\n")
	runGit(t, f.repo, "add", "--", "-dash.txt")
	runGit(t, f.repo, "commit", "-qm", "dash")
	paths := []any{"-dash.txt"}
	if got := f.mustSearch(t, map[string]any{"repo": f.repo, "command": "ls-files", "paths": paths}); got != "-dash.txt\n" {
		t.Errorf("ls-files = %q", got)
	}
	if got := f.mustSearch(t, map[string]any{"repo": f.repo, "command": "log", "options": []any{"--format=%s"}, "paths": paths}); got != "dash\n" {
		t.Errorf("log = %q", got)
	}
	if got := f.mustSearch(t, map[string]any{"repo": f.repo, "command": "grep", "pattern": "dash", "paths": paths}); got != "-dash.txt:dash line\n" {
		t.Errorf("grep = %q", got)
	}
	for _, revs := range [][]any{nil, {"HEAD"}} {
		args := map[string]any{"repo": f.repo, "command": "blame", "options": []any{"-s"}, "paths": paths}
		if revs != nil {
			args["revisions"] = revs
		}
		if got := f.mustSearch(t, args); !strings.HasSuffix(got, "1) dash line\n") {
			t.Errorf("blame (revisions %v) = %q", revs, got)
		}
	}
}
