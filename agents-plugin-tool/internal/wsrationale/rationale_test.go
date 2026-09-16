package wsrationale

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsgit"
)

// --- fixture helpers ---

func initRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "core.autocrlf", "false")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "Test User")
	return root
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// commitFixture writes files, stages everything, and commits with an explicit
// date and a verbatim message so the body is byte-exact. It returns the full
// commit hash.
func commitFixture(t *testing.T, root, date string, files map[string]string, message string) string {
	t.Helper()
	for rel, content := range files {
		writeFile(t, root, rel, content)
	}
	runGit(t, root, "add", "-A")
	cmd := exec.Command("git", "commit", "--cleanup=verbatim", "-m", message)
	cmd.Dir = root
	iso := date + "T12:00:00"
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+iso, "GIT_COMMITTER_DATE="+iso)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, string(out))
	}
	out, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").CombinedOutput()
	if err != nil {
		t.Fatalf("rev-parse failed: %v\n%s", err, string(out))
	}
	return strings.TrimSpace(string(out))
}

func query(t *testing.T, root string, opts Options) Result {
	t.Helper()
	res, err := Query(context.Background(), wsgit.ExecRunner{}, root, opts)
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	return res
}

// allRecords flattens every thread's records, deduped by pointer, so a
// duplicated cross-thread commit record is counted once.
func allRecordTexts(res Result) map[string]bool {
	out := map[string]bool{}
	for _, th := range res.Threads {
		for _, rec := range th.Records {
			out[rec.Text] = true
		}
	}
	return out
}

func threadByStem(res Result, stem string) (Thread, bool) {
	for _, th := range res.Threads {
		if th.Stem == stem {
			return th, true
		}
	}
	return Thread{}, false
}

func totalRecords(res Result) int {
	n := 0
	for _, th := range res.Threads {
		n += len(th.Records)
	}
	return n
}

// --- Test 1: bullet extraction ---

func TestBulletExtraction(t *testing.T) {
	root := initRepo(t)
	body := "subject line\n\n" +
		"Some prose that must not become a record.\n\n" +
		"## AI Context\n" +
		"- bullet one\n" +
		"  continued indented\n" +
		"- bullet two\n\n" +
		"## Ticket Updates\n" +
		"- 260101-feat-alpha: label\n" +
		"  > Forward: future finding\n"
	commitFixture(t, root, "2026-01-01", map[string]string{"a.txt": "x"}, body)

	res := query(t, root, Options{})
	if got := totalRecords(res); got != 3 {
		t.Fatalf("want 3 records, got %d: %+v", got, res.Threads)
	}
	texts := allRecordTexts(res)
	for _, want := range []string{
		"bullet one continued indented",
		"bullet two",
		"260101-feat-alpha: label > Forward: future finding",
	} {
		if !texts[want] {
			t.Fatalf("missing record %q; have %v", want, texts)
		}
	}
}

// --- Test 2: paragraph fallback ---

func TestParagraphFallback(t *testing.T) {
	root := initRepo(t)
	commitFixture(t, root, "2026-01-01", map[string]string{"a.txt": "x"}, "seed\n\n## AI Context\n- seed record\n")
	ticket := "---\ntitle: gamma\n---\n\n### Result (abc1234) - 2026-01-02\n\n" +
		"First paragraph line one.\nFirst paragraph line two.\n\n" +
		"Second paragraph.\n"
	writeFile(t, root, "ai-docs/tickets/.done/260102-feat-beta.md", ticket)

	res := query(t, root, Options{Kinds: []string{"ticket"}})
	th, ok := threadByStem(res, "260102-feat-beta")
	if !ok {
		t.Fatalf("no thread for 260102-feat-beta: %+v", res.Threads)
	}
	if len(th.Records) != 2 {
		t.Fatalf("want 2 paragraph records, got %d: %+v", len(th.Records), th.Records)
	}
	texts := allRecordTexts(res)
	if !texts["First paragraph line one. First paragraph line two."] || !texts["Second paragraph."] {
		t.Fatalf("paragraph texts wrong: %v", texts)
	}
}

// --- Test 3: stem linkage ---

func TestStemLinkage(t *testing.T) {
	root := initRepo(t)
	commitFixture(t, root, "2026-01-01", map[string]string{"a.txt": "1"},
		"multi\n\n## AI Context\n- did work spanning 260101-feat-alpha and 260102-feat-beta\n")
	commitFixture(t, root, "2026-01-02", map[string]string{"b.txt": "2"},
		"solo\n\n## AI Context\n- an unlinked decision\n")

	res := query(t, root, Options{})
	alpha, ok := threadByStem(res, "260101-feat-alpha")
	if !ok || len(alpha.Records) != 1 {
		t.Fatalf("alpha thread wrong: %+v", res.Threads)
	}
	beta, ok := threadByStem(res, "260102-feat-beta")
	if !ok || len(beta.Records) != 1 {
		t.Fatalf("beta thread wrong: %+v", res.Threads)
	}
	none, ok := threadByStem(res, "")
	if !ok || len(none.Records) != 1 {
		t.Fatalf("(no ticket) thread wrong: %+v", res.Threads)
	}
}

// --- Test 4: path filter ---

func TestPathFilter(t *testing.T) {
	root := initRepo(t)
	commitFixture(t, root, "2026-01-01",
		map[string]string{"internal/mcp/server.go": "package mcp\n"},
		"impl\n\n## AI Context\n- wired dispatch for 260103-feat-gamma\n")
	// A ticket whose body names no path; its only path link is the commit.
	writeFile(t, root, "ai-docs/tickets/ready/260103-feat-gamma.md",
		"---\ntitle: gamma\n---\n\n## Decisions\n\n- decided the shape\n")
	// A commit + ticket on an unrelated path, to prove the filter excludes.
	commitFixture(t, root, "2026-01-02",
		map[string]string{"docs/guide.md": "# guide\n"},
		"docs\n\n## AI Context\n- rewrote the guide for 260199-feat-other\n")
	writeFile(t, root, "ai-docs/tickets/ready/260199-feat-other.md",
		"---\ntitle: other\n---\n\n## Decisions\n\n- an off-path decision\n")

	res := query(t, root, Options{Paths: []string{"internal/mcp"}})
	texts := allRecordTexts(res)
	if !texts["wired dispatch for 260103-feat-gamma"] {
		t.Fatalf("commit record missing under path filter: %v", texts)
	}
	if !texts["decided the shape"] {
		t.Fatalf("ticket record not matched via thread commit path: %v", texts)
	}
	// The unrelated-path records must be excluded.
	if texts["rewrote the guide for 260199-feat-other"] {
		t.Fatalf("path filter wrongly kept an off-path commit record: %v", texts)
	}
	if texts["an off-path decision"] {
		t.Fatalf("path filter wrongly kept an off-path ticket record: %v", texts)
	}
}

// --- Test 5: exclude_stem ---

func TestExcludeStem(t *testing.T) {
	root := initRepo(t)
	commitFixture(t, root, "2026-01-01", map[string]string{"a.txt": "1"},
		"only\n\n## AI Context\n- change that names only 260104-feat-delta\n")
	commitFixture(t, root, "2026-01-02", map[string]string{"b.txt": "2"},
		"both\n\n## AI Context\n- change naming 260104-feat-delta and 260105-feat-epsilon\n")
	writeFile(t, root, "ai-docs/tickets/ready/260104-feat-delta.md",
		"---\ntitle: delta\n---\n\n## Decisions\n\n- delta's own decision\n")

	res := query(t, root, Options{ExcludeStem: "260104-feat-delta"})
	texts := allRecordTexts(res)
	if texts["delta's own decision"] {
		t.Fatalf("exclude_stem did not drop the ticket's own record: %v", texts)
	}
	if texts["change that names only 260104-feat-delta"] {
		t.Fatalf("exclude_stem did not drop the commit naming only it: %v", texts)
	}
	if !texts["change naming 260104-feat-delta and 260105-feat-epsilon"] {
		t.Fatalf("exclude_stem wrongly dropped a multi-stem commit: %v", texts)
	}
}

// --- Test 6: ranking ---

func TestRanking(t *testing.T) {
	root := initRepo(t)
	// Distinct stems put the two records in distinct threads, so relevance-driven
	// thread ordering is observable (intra-thread order is always date desc).
	commitFixture(t, root, "2026-01-01", map[string]string{"a.txt": "1"},
		"older\n\n## AI Context\n- a decision about the elephant migration path for 260201-feat-one\n")
	commitFixture(t, root, "2026-01-02", map[string]string{"b.txt": "2"},
		"newer\n\n## AI Context\n- an unrelated cleanup of whitespace for 260202-feat-two\n")

	// With query, the elephant thread outranks the one without the term.
	res := query(t, root, Options{Query: "elephant"})
	if len(res.Threads) == 0 || len(res.Threads[0].Records) == 0 {
		t.Fatalf("no records: %+v", res.Threads)
	}
	if got := res.Threads[0].Records[0].Text; !strings.Contains(got, "elephant") {
		t.Fatalf("top thread record should contain query term, got %q", got)
	}

	// Without query, newest first.
	res2 := query(t, root, Options{})
	if got := res2.Threads[0].Records[0].Text; !strings.Contains(got, "whitespace") {
		t.Fatalf("without query, newest record should be first, got %q", got)
	}
}

// --- Test 7: site mode ---

func TestSiteMode(t *testing.T) {
	root := initRepo(t)
	fn := func(ret int) string {
		return fmt.Sprintf("package sample\n\nfunc target() int {\n\treturn %d\n}\n", ret)
	}
	c1 := commitFixture(t, root, "2026-01-01",
		map[string]string{"sample.go": fn(1), "dup.go": "// foo one\n// foo two\n"},
		"create\n\n## AI Context\n- created target\n")
	commitFixture(t, root, "2026-01-02", map[string]string{"sample.go": fn(2)},
		"edit1\n\n## AI Context\n- bumped target to 2\n")
	c3 := commitFixture(t, root, "2026-01-03", map[string]string{"sample.go": fn(3)},
		"edit2\n\n## AI Context\n- bumped target to 3\n")

	res := query(t, root, Options{Site: "sample.go:/func target/,+3"})
	if res.Site == nil || res.Site.Changes != 3 {
		t.Fatalf("want 3 chain changes, got %+v", res.Site)
	}
	if res.Site.LastChanged == nil || res.Site.LastChanged.Hash != c3 {
		t.Fatalf("last_changed should be newest %s: %+v", c3, res.Site.LastChanged)
	}
	if res.Site.Introduced == nil || res.Site.Introduced.Hash != c1 {
		t.Fatalf("introduced should be oldest %s: %+v", c1, res.Site.Introduced)
	}
	if !containsOmit(res.Omitted, "renames not followed") {
		t.Fatalf("omitted should note renames not followed: %v", res.Omitted)
	}
	// Records newest first.
	none, ok := threadByStem(res, "")
	if !ok || len(none.Records) != 3 {
		t.Fatalf("chain should yield 3 records: %+v", res.Threads)
	}
	if none.Records[0].Date != "2026-01-03" {
		t.Fatalf("records not newest first: %+v", none.Records)
	}

	// A regex with two matches and no occurrence reports the K-locations line.
	res2 := query(t, root, Options{Site: "dup.go:/foo/,+1"})
	if !containsOmit(res2.Omitted, "site matched 2 locations; used occurrence 1") {
		t.Fatalf("expected K-locations omit note: %v", res2.Omitted)
	}
}

func containsOmit(omitted []string, want string) bool {
	for _, o := range omitted {
		if o == want {
			return true
		}
	}
	return false
}

// --- Test 8: pickaxe mode ---

func TestPickaxeMode(t *testing.T) {
	root := initRepo(t)
	c1 := commitFixture(t, root, "2026-01-01", map[string]string{"magic.go": "const K = \"MAGICTOKEN\"\n"},
		"introduce\n\n## AI Context\n- introduced the token\n")
	c2 := commitFixture(t, root, "2026-01-02", map[string]string{"magic.go": "const K = \"\"\n"},
		"remove\n\n## AI Context\n- removed the token\n")
	commitFixture(t, root, "2026-01-03", map[string]string{"other.go": "package other\n"},
		"unrelated\n\n## AI Context\n- something else entirely\n")

	res := query(t, root, Options{Pickaxe: "MAGICTOKEN"})
	hashes := map[string]bool{}
	for _, th := range res.Threads {
		for _, rec := range th.Records {
			hashes[rec.Hash] = true
		}
	}
	if !hashes[c1] || !hashes[c2] {
		t.Fatalf("pickaxe should return introducing and removing commits: %v", hashes)
	}
	if totalRecords(res) != 2 {
		t.Fatalf("pickaxe should return exactly the 2 touching commits, got %d: %+v", totalRecords(res), res.Threads)
	}
}

// --- Test 9: limit cap ---

func TestLimitCap(t *testing.T) {
	root := initRepo(t)
	body := func(prefix string, n int) string {
		var b strings.Builder
		b.WriteString(prefix + "\n\n## AI Context\n")
		for i := 0; i < n; i++ {
			fmt.Fprintf(&b, "- %s decision number %d\n", prefix, i)
		}
		return b.String()
	}
	commitFixture(t, root, "2026-01-01", map[string]string{"a.txt": "1"}, body("a", 40))
	commitFixture(t, root, "2026-01-02", map[string]string{"b.txt": "2"}, body("b", 40))
	commitFixture(t, root, "2026-01-03", map[string]string{"c.txt": "3"}, body("c", 40))

	res := query(t, root, Options{Limit: 500})
	if got := totalRecords(res); got != 100 {
		t.Fatalf("limit cap should return 100 records, got %d", got)
	}
	if !containsOmit(res.Omitted, "20 records past limit") {
		t.Fatalf("expected exact records-past-limit note, got %v", res.Omitted)
	}
	if res.ScannedCommits != 3 {
		t.Fatalf("scanned commits should be 3, got %d", res.ScannedCommits)
	}
}

// --- Test 10: errors ---

func TestErrors(t *testing.T) {
	root := initRepo(t)
	commitFixture(t, root, "2026-01-01", map[string]string{"a.txt": "1"}, "seed\n\n## AI Context\n- seed\n")

	cases := []struct {
		name string
		opts Options
	}{
		{"site with paths", Options{Site: "a.txt#L1-L2", Paths: []string{"a.txt"}}},
		{"malformed site", Options{Site: "no-delimiters-here"}},
		{"bad since", Options{Since: "not-a-date"}},
		{"invalid glob", Options{Paths: []string{"[invalid"}}},
	}
	for _, tc := range cases {
		if _, err := Query(context.Background(), wsgit.ExecRunner{}, root, tc.opts); err == nil {
			t.Fatalf("%s: expected error, got nil", tc.name)
		}
	}
}

// --- Test 11: output shape ---

func TestOutputShape(t *testing.T) {
	root := initRepo(t)
	commitFixture(t, root, "2026-01-01", map[string]string{"internal/mcp/x.go": "package mcp\n"},
		"seed\n\n## AI Context\n- a recorded decision for 260106-feat-zeta\n")

	// Non-empty case: compact text ends with an omitted line.
	res := query(t, root, Options{})
	assertEndsWithOmitted(t, FormatText(res))

	// Empty case: still ends with omitted line.
	empty := query(t, root, Options{Paths: []string{"nonexistent/path"}})
	if totalRecords(empty) != 0 {
		t.Fatalf("expected empty result, got %+v", empty.Threads)
	}
	assertEndsWithOmitted(t, FormatText(empty))

	// JSON shape matches the documented schema, top-level and nested.
	raw, err := json.Marshal(JSON(res))
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Threads []struct {
			Stem    *string `json:"stem"`
			Status  *string `json:"status"`
			Title   *string `json:"title"`
			Records []struct {
				Kind    string   `json:"kind"`
				Pointer string   `json:"pointer"`
				Date    string   `json:"date"`
				Text    string   `json:"text"`
				Paths   []string `json:"paths"`
				Stems   []string `json:"stems"`
				Score   float64  `json:"score"`
			} `json:"records"`
		} `json:"threads"`
		ScannedCommits int      `json:"scanned_commits"`
		Omitted        []string `json:"omitted"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json does not match documented shape: %v\n%s", err, raw)
	}
	// Verify a top-level "site" key exists (null is valid in non-site mode).
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	if _, ok := envelope["site"]; !ok {
		t.Fatalf("json missing site key: %s", raw)
	}
	if len(decoded.Threads) == 0 || len(decoded.Threads[0].Records) == 0 {
		t.Fatalf("expected at least one record to validate nested shape: %s", raw)
	}
	rec := decoded.Threads[0].Records[0]
	if rec.Kind == "" || rec.Pointer == "" || rec.Date == "" || rec.Text == "" {
		t.Fatalf("nested record missing required fields: %+v", rec)
	}
}

func assertEndsWithOmitted(t *testing.T, text string) {
	t.Helper()
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	last := lines[len(lines)-1]
	if !strings.HasPrefix(last, "omitted: scanned ") {
		t.Fatalf("output must end with omitted line, got last line %q\nfull:\n%s", last, text)
	}
}
