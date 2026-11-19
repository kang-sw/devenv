package wswhiteboard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// srcBoard is a project root holding a mockups folder, with an Env that
// reads panel sources from it.
type srcBoard struct {
	root string
	env  Env
}

func newSrcBoard(t *testing.T) srcBoard {
	t.Helper()
	root := t.TempDir()
	env := testEnv()
	env.SourceRoot = root
	return srcBoard{root: root, env: env}
}

func (b srcBoard) write(t *testing.T, rel, content string) string {
	t.Helper()
	path := filepath.Join(b.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func srcPanelXML(attrs string) string {
	return fmt.Sprintf(`<wb:topic name="t"><wb:panel recap="Mockup" %s/></wb:topic>`, attrs)
}

// refuse applies commands that must be refused whole and returns the problems.
func (b srcBoard) refuse(t *testing.T, st *State, commands string) []string {
	t.Helper()
	out, problems := Apply(st, commands, b.env)
	if len(problems) == 0 || out.Accepted != 0 {
		t.Fatalf("Apply(%s) must refuse: accepted %d, problems %q", commands, out.Accepted, problems)
	}
	if len(st.Topics) != 0 || st.PanelCounter != 0 {
		t.Fatalf("a refused src must store nothing: %+v", st)
	}
	return problems
}

func TestPanelSrcStoresFileContentNotPath(t *testing.T) {
	b := newSrcBoard(t)
	const html = "<style>.bar{color:red}</style>\n<div class=\"bar\">a &lt; b 가</div>\n"
	b.write(t, "ai-docs/assets/mockups/261119/topic/m.html", html)
	st := &State{}
	if _, problems := Apply(st, srcPanelXML(`name="mock" src="ai-docs/assets/mockups/261119/topic/m.html"`), b.env); len(problems) > 0 {
		t.Fatalf("refused: %q", problems)
	}
	es := entries(t, st, "t")
	if es[0].Kind != EntryPanel || es[0].Text != html || es[0].PanelName != "mock" || es[0].Recap != "Mockup" {
		t.Fatalf("panel = %+v, want the file content as payload", es[0])
	}
	if strings.Contains(es[0].Text, "m.html") {
		t.Fatalf("the stored panel holds content, not the path: %q", es[0].Text)
	}
	// An edit needs a new post, which versions the named panel like any re-post.
	b.write(t, "ai-docs/assets/mockups/261119/topic/m.html", "<p>v2</p>")
	if _, problems := Apply(st, srcPanelXML(`name="mock" src="ai-docs/assets/mockups/261119/topic/m.html"`), b.env); len(problems) > 0 {
		t.Fatalf("re-post refused: %q", problems)
	}
	var panels []Entry
	for _, e := range entries(t, st, "t") {
		if e.Kind == EntryPanel {
			panels = append(panels, e)
		}
	}
	if len(panels) != 2 || panels[0].Text != html || panels[1].Text != "<p>v2</p>" || panels[0].PanelID != panels[1].PanelID {
		t.Fatalf("re-post must append a second version of the same panel: %+v", panels)
	}
}

func TestPanelSrcPathForms(t *testing.T) {
	b := newSrcBoard(t)
	abs := b.write(t, "mock/a.md", "**md**")
	b.write(t, "mock/b.SVG", "<svg></svg>")
	b.write(t, "mock/c.htm", "<b>c</b>")
	for _, tc := range []struct{ name, src, want string }{
		{"relative", "mock/a.md", "**md**"},
		{"dot segments inside root", "mock/../mock/a.md", "**md**"},
		{"absolute inside root", abs, "**md**"},
		{"extension case", "mock/b.SVG", "<svg></svg>"},
		{"htm", "mock/c.htm", "<b>c</b>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ReadPanelSource(b.root, tc.src)
			if err != nil || got != tc.want {
				t.Fatalf("ReadPanelSource(%q) = %q, %v; want %q", tc.src, got, err, tc.want)
			}
		})
	}
	b.write(t, "mock/bom.md", "\ufeffhello")
	if got, err := ReadPanelSource(b.root, "mock/bom.md"); err != nil || got != "hello" {
		t.Fatalf("a UTF-8 BOM is dropped: %q, %v", got, err)
	}
}

func TestPanelSrcRefusals(t *testing.T) {
	b := newSrcBoard(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.html"), []byte("<p>secret</p>"), 0o644); err != nil {
		t.Fatal(err)
	}
	b.write(t, "mock/ok.html", "<p>ok</p>")
	b.write(t, "mock/notes.txt", "text")
	b.write(t, "mock/page.html", "<!DOCTYPE html>\n<html><body>x</body></html>")
	b.write(t, "mock/page2.html", "  <html>\n<body>x</body></html>")
	b.write(t, "mock/binary.html", string([]byte{0xff, 0xfe, 'x'}))
	b.write(t, "mock/big.html", strings.Repeat("a", MaxPanelBytes+1))
	if err := os.Mkdir(filepath.Join(b.root, "dir.html"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, attrs, problem string }{
		{"dotdot escape", `src="../outside.html"`, "outside the session root"},
		{"deep dotdot escape", `src="mock/../../outside.html"`, "outside the session root"},
		{"absolute outside", fmt.Sprintf(`src=%q`, filepath.Join(outside, "secret.html")), "outside the session root"},
		{"root itself", `src="."`, "outside the session root"},
		{"missing", `src="mock/none.html"`, "no such file"},
		{"extension", `src="mock/notes.txt"`, "only .html, .htm, .svg and .md"},
		{"no extension", `src="mock/ok"`, "only .html, .htm, .svg and .md"},
		{"directory", `src="dir.html"`, "not a regular file"},
		{"over cap", `src="mock/big.html"`, "exceeds 2 MiB"},
		{"not UTF-8", `src="mock/binary.html"`, "valid UTF-8"},
		{"full document doctype", `src="mock/page.html"`, "full HTML document"},
		{"full document html tag", `src="mock/page2.html"`, "full HTML document"},
		{"empty", `src=""`, "path is empty"},
		{"blank", `src="  "`, "path is empty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := &State{}
			problems := b.refuse(t, st, srcPanelXML(tc.attrs))
			wantProblems(t, problems, tc.problem)
			if tc.problem == "outside the session root" {
				wantProblems(t, problems, b.root)
			}
			if !hasProblem(problems, "line") || !hasProblem(problems, "src") {
				t.Fatalf("the refusal must carry the command position and name the src attribute: %q", problems)
			}
		})
	}
	t.Run("no project root", func(t *testing.T) {
		env := testEnv()
		st := &State{}
		_, problems := Apply(st, srcPanelXML(`src="mock/ok.html"`), env)
		wantProblems(t, problems, "no project root")
	})
	t.Run("one source per post", func(t *testing.T) {
		st := &State{}
		inline := `<wb:topic name="t"><wb:panel recap="x" src="mock/ok.html">inline</wb:panel></wb:topic>`
		_, problems := Apply(st, inline, b.env)
		wantProblems(t, problems, "two sources")
		if len(st.Topics) != 0 {
			t.Fatalf("a refused post must store nothing: %+v", st)
		}
	})
	t.Run("src with change bundles", func(t *testing.T) {
		st := &State{}
		mustApply(t, st, panelCommandXML("t", `name="work" recap="Original"`, "abc"))
		before := mustMarshal(t, st)
		xml := `<wb:topic name="t"><wb:panel name="work" src="mock/ok.html">` + panelChangeXML("abc", "x") + `</wb:panel></wb:topic>`
		_, problems := Apply(st, xml, b.env)
		wantProblems(t, problems, "wb:change")
		if string(mustMarshal(t, st)) != string(before) {
			t.Fatal("a refused post must leave the state untouched")
		}
	})
	t.Run("batch keeps other commands", func(t *testing.T) {
		st := &State{}
		xml := `<wb:topic name="t"><wb:panel recap="Bad" src="mock/none.html"/><wb:log>kept</wb:log></wb:topic>`
		out, problems := Apply(st, xml, b.env)
		wantProblems(t, problems, "no such file")
		if out.Accepted != 1 || !strings.Contains(out.Failed, "mock/none.html") {
			t.Fatalf("only the src command fails: accepted %d, failed %q", out.Accepted, out.Failed)
		}
	})
}

func TestPanelSrcLinksRefused(t *testing.T) {
	b := newSrcBoard(t)
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.md")
	if err := os.WriteFile(secret, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	b.write(t, "real/m.md", "inside")
	b.write(t, "real/x.txt", "x")
	link := func(target, name string) {
		t.Helper()
		if err := os.Symlink(target, filepath.Join(b.root, name)); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
	}
	link(secret, "link.md")                              // file link out of the root
	link(outside, "linkdir")                             // folder link out of the root
	link(filepath.Join(b.root, "real", "m.md"), "in.md") // file link that stays inside
	link(filepath.Join(b.root, "real"), "indir")         // folder link that stays inside
	link(filepath.Join(b.root, "real", "x.txt"), "x.md") // link to a disallowed type
	link("real", "rel")                                  // relative folder link
	for _, src := range []string{"link.md", "linkdir/secret.md", "in.md", "indir/m.md", "x.md", "rel/m.md", "real/../linkdir/secret.md"} {
		_, err := ReadPanelSource(b.root, src)
		if err == nil || !strings.Contains(err.Error(), "symlink or junction") || !strings.Contains(err.Error(), b.root) {
			t.Fatalf("ReadPanelSource(%q) = %v, want a refusal naming the link and the session root", src, err)
		}
	}
	if got, err := ReadPanelSource(b.root, "real/m.md"); err != nil || got != "inside" {
		t.Fatalf("a plain file next to the links still reads: %q, %v", got, err)
	}
	// A file where a folder is expected.
	if _, err := ReadPanelSource(b.root, "real/m.md/deeper.md"); err == nil || !strings.Contains(err.Error(), "not a folder") {
		t.Fatalf("a path through a file is refused: %v", err)
	}
}

func TestPanelSrcCapBoundary(t *testing.T) {
	b := newSrcBoard(t)
	b.write(t, "at.md", strings.Repeat("a", MaxPanelBytes))
	b.write(t, "over.md", strings.Repeat("a", MaxPanelBytes+1))
	if got, err := ReadPanelSource(b.root, "at.md"); err != nil || len(got) != MaxPanelBytes {
		t.Fatalf("a file at the cap is read: %d, %v", len(got), err)
	}
	if _, err := ReadPanelSource(b.root, "over.md"); err == nil || !strings.Contains(err.Error(), "2 MiB") {
		t.Fatalf("a file over the cap is refused: %v", err)
	}
}

func TestPanelSrcIsOptionalAndHiddenFromNoOneElse(t *testing.T) {
	// Without src the command is unchanged, and an unknown attribute is still refused.
	st := &State{}
	mustApply(t, st, panelXML("t", "Inline", "body"))
	wantProblems(t, refused(t, &State{}, `<wb:topic name="t"><wb:panel recap="x" path="a.md">b</wb:panel></wb:topic>`), "unknown attribute path", "src")
}
