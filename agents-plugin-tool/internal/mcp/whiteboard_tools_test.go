package mcp

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// whiteboardFixture is one lead session in a fresh git repository, with the
// shipped rsrc tree so refusals and manual renders read the real playbook.
type whiteboardFixture struct {
	s    *Server
	key  string
	root string
	id   int
}

func newWhiteboardFixture(t *testing.T) *whiteboardFixture {
	t.Helper()
	useLeadProfile(t)
	t.Setenv("WS_RSRC_ROOT", filepath.Join("..", "..", "..", "agents-plugin", "rsrc"))
	t.Setenv("WS_SKILLS_ROOT", filepath.Join(t.TempDir(), "missing-skills"))
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	s := NewServer(root, "test")
	// The harness the knob's bucket defaults to; tests switch it to model
	// another harness's server.
	s.sessionHarness = "claude"
	key, loginRoot := parseLoginResponse(t, callLogin(t, s, 910000, root, nil))
	return &whiteboardFixture{s: s, key: key, root: loginRoot, id: 1}
}

func (f *whiteboardFixture) call(t *testing.T, name string, args map[string]any) string {
	t.Helper()
	if _, ok := args["session_key"]; !ok {
		args["session_key"] = f.key
	}
	f.id++
	return callToolOnce(t, f.s, f.id, name, args)
}

func (f *whiteboardFixture) update(t *testing.T, payload string) string {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(payload), &v); err != nil {
		t.Fatalf("bad payload %s: %v", payload, err)
	}
	resp := f.call(t, "whiteboard.update", map[string]any{"payload": v})
	if isToolError(resp) {
		t.Fatalf("whiteboard.update(%s) failed: %s", payload, toolText(t, resp))
	}
	return toolText(t, resp)
}

func (f *whiteboardFixture) dir() string {
	return filepath.Join(f.root, "ai-docs", "whiteboards")
}

func (f *whiteboardFixture) replyID() string {
	return wsmailbox.ReplyID(f.key)
}

func (f *whiteboardFixture) stamp(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.dir(), f.replyID()+".stamp.js"))
	if err != nil {
		t.Fatalf("read stamp: %v", err)
	}
	return string(data)
}

func (f *whiteboardFixture) embeds(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	var names []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), f.replyID()+"-") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names
}

func isToolError(resp string) bool {
	return strings.Contains(resp, `"isError":true`) || strings.Contains(resp, `"error":{`)
}

func resultLine(t *testing.T, text, prefix string) string {
	t.Helper()
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix)
		}
	}
	t.Fatalf("result has no %q line:\n%s", prefix, text)
	return ""
}

func excludeEntries(t *testing.T, root string) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".git", "info", "exclude"))
	if err != nil {
		return 0
	}
	return strings.Count(string(data), "/ai-docs/whiteboards/\n")
}

func TestWhiteboardToolsAdvertiseMinimalSchema(t *testing.T) {
	useLeadProfile(t)
	s := NewServer(t.TempDir(), "test")
	var resp struct {
		Result struct {
			Tools []struct {
				Name        string         `json:"name"`
				Description string         `json:"description"`
				InputSchema map[string]any `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(callToolsList(t, s)), &resp); err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, tool := range resp.Result.Tools {
		if tool.Name != "whiteboard.update" && tool.Name != "whiteboard.query" {
			continue
		}
		found++
		props, _ := tool.InputSchema["properties"].(map[string]any)
		var names []string
		for name := range props {
			names = append(names, name)
		}
		sort.Strings(names)
		if strings.Join(names, ",") != "payload,session_key" {
			t.Fatalf("%s advertises %v, want only session_key and payload", tool.Name, names)
		}
		if payload, _ := props["payload"].(map[string]any); payload["type"] != "object" || payload["properties"] != nil {
			t.Fatalf("%s payload must be an opaque object: %v", tool.Name, payload)
		}
		if !strings.Contains(tool.Description, "Lead-only") || !strings.Contains(tool.Description, "`whiteboard` playbook") {
			t.Fatalf("%s description must be lead-only and point at the whiteboard playbook: %q", tool.Name, tool.Description)
		}
	}
	if found != 2 {
		t.Fatalf("tools/list advertised %d whiteboard tools, want 2", found)
	}
}

// A description naming a sibling tool must name the runtime namespace, so
// wsflow's tools/list does not point at ws tools it does not serve.
func TestToolDescriptionsNameRuntimeNamespace(t *testing.T) {
	useLeadProfile(t)
	t.Setenv(envNamespace, "wsflow")
	s := NewServer(t.TempDir(), "test")
	var resp struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(callToolsList(t, s)), &resp); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"whiteboard.update":  "wsflow/playbook.read",
		"whiteboard.query":   "wsflow/playbook.read",
		"tickets.sage_stamp": "wsflow/git.commit",
	}
	for _, tool := range resp.Result.Tools {
		ref, ok := want[tool.Name]
		if !ok {
			continue
		}
		delete(want, tool.Name)
		if !strings.Contains(tool.Description, ref) || strings.Contains(tool.Description, "(ws/") || strings.Contains(tool.Description, " ws/") {
			t.Fatalf("%s description must name %q, not ws/: %q", tool.Name, ref, tool.Description)
		}
	}
	if len(want) != 0 {
		t.Fatalf("tools/list did not advertise %v", want)
	}
}

func TestWhiteboardUpdateRendersPageEmbedAndStampInOrder(t *testing.T) {
	f := newWhiteboardFixture(t)
	if _, err := os.Stat(f.dir()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("no whiteboard folder may exist before the first render: %v", err)
	}
	if excludeEntries(t, f.root) != 0 {
		t.Fatalf("no exclude entry may exist before the first render")
	}
	// Todo writes before the first render write nothing.
	f.call(t, "todo.add", map[string]any{"key": "a", "title": "first"})
	if _, err := os.Stat(f.dir()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a todo write before the first render must not create the folder")
	}

	var order []string
	orig := whiteboardRename
	whiteboardRename = func(from, to string) error {
		order = append(order, filepath.Base(to))
		return orig(from, to)
	}
	defer func() { whiteboardRename = orig }()

	text := f.update(t, `{"add":[
		{"class":"policy","topic":"layout","title":"Two columns?","options":["yes","no"],"recommendation":"a: reads faster"},
		{"class":"technical","topic":"layout","title":"Sticky sidebar","rationale":"keeps todos visible"}]}`)
	if got := resultLine(t, text, "ids: "); got != "(01), (02)" {
		t.Fatalf("ids line = %q", got)
	}
	full := resultLine(t, text, "full: ")
	embed := resultLine(t, text, "embed: ")
	rid := f.replyID()
	if full != "ai-docs/whiteboards/"+rid+".html" || !strings.HasPrefix(embed, "ai-docs/whiteboards/"+rid+"-") {
		t.Fatalf("paths full=%q embed=%q", full, embed)
	}
	if len(order) != 3 || order[0] != filepath.Base(embed) || order[1] != rid+".html" || order[2] != rid+".stamp.js" {
		t.Fatalf("write order = %v, want embed, page, then stamp", order)
	}
	page, err := os.ReadFile(filepath.Join(f.root, full))
	if err != nil || !strings.HasPrefix(string(page), "<!DOCTYPE html>") || !strings.Contains(string(page), "Two columns?") {
		t.Fatalf("full page missing or wrong (%v):\n%.300s", err, page)
	}
	if !strings.Contains(string(page), `data-stamp="`+rid+`.stamp.js"`) {
		t.Fatalf("full page must poll its stamp")
	}
	if got := f.stamp(t); got != "wbStamp(\"1\");\n" {
		t.Fatalf("stamp = %q", got)
	}
	if n := excludeEntries(t, f.root); n != 1 {
		t.Fatalf("exclude entries after first render = %d, want 1", n)
	}
	leftovers, _ := filepath.Glob(filepath.Join(f.dir(), ".*.tmp"))
	if len(leftovers) != 0 {
		t.Fatalf("temp files left behind: %v", leftovers)
	}

	// A second session in the same repository does not add the entry again.
	key2, err := f.s.sessions.mint(f.root, roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	if resp := f.call(t, "whiteboard.update", map[string]any{"session_key": key2, "payload": map[string]any{}}); isToolError(resp) {
		t.Fatalf("second session update failed: %s", resp)
	}
	os.RemoveAll(f.dir())
	f.update(t, `{}`)
	if n := excludeEntries(t, f.root); n != 1 {
		t.Fatalf("exclude entries after re-creating the folder = %d, want 1", n)
	}
}

func TestWhiteboardEmbedRetentionAndDistinctNames(t *testing.T) {
	f := newWhiteboardFixture(t)
	first := resultLine(t, f.update(t, `{}`), "embed: ")
	second := resultLine(t, f.update(t, `{}`), "embed: ")
	if first == second {
		t.Fatalf("two identical renders must produce distinct embed names: %s", first)
	}
	for i := 0; i < 4; i++ {
		f.update(t, `{}`)
	}
	names := f.embeds(t, f.dir())
	if len(names) != 5 {
		t.Fatalf("embeds kept = %d (%v), want 5", len(names), names)
	}
	// Age is the render sequence, not mtime: make the oldest-by-sequence
	// embed the newest on disk, and it is still the one evicted.
	rec, _ := f.s.sessions.readState(f.key)
	oldest := rec.Whiteboard.Embeds[0].Name
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(filepath.Join(f.dir(), oldest), future, future); err != nil {
		t.Fatal(err)
	}
	newestBefore := rec.Whiteboard.Embeds[1].Name
	past := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(filepath.Join(f.dir(), newestBefore), past, past); err != nil {
		t.Fatal(err)
	}
	f.update(t, `{}`)
	if _, err := os.Stat(filepath.Join(f.dir(), oldest)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the lowest-sequence embed %s must be evicted despite its fresh mtime", oldest)
	}
	if _, err := os.Stat(filepath.Join(f.dir(), newestBefore)); err != nil {
		t.Fatalf("embed %s must survive despite its old mtime: %v", newestBefore, err)
	}
	if got := len(f.embeds(t, f.dir())); got != 5 {
		t.Fatalf("embeds kept after eviction = %d, want 5", got)
	}
}

func TestWhiteboardEmbedDir(t *testing.T) {
	f := newWhiteboardFixture(t)
	host := t.TempDir()
	payload := func(dir string) string {
		raw, _ := json.Marshal(map[string]any{"embed_dir": dir})
		return string(raw)
	}
	for _, bad := range []string{"relative/dir", filepath.Join(host, "missing")} {
		raw, _ := json.Marshal(map[string]any{"embed_dir": bad})
		var v any
		json.Unmarshal(raw, &v)
		if resp := f.call(t, "whiteboard.update", map[string]any{"payload": v}); !isToolError(resp) {
			t.Fatalf("embed_dir %q must be refused: %s", bad, resp)
		}
	}
	embed := resultLine(t, f.update(t, payload(host)), "embed: ")
	if strings.Contains(embed, "/") || !strings.HasPrefix(embed, f.replyID()+"-") {
		t.Fatalf("embed with embed_dir must be the bare file name: %q", embed)
	}
	if _, err := os.Stat(filepath.Join(host, embed)); err != nil {
		t.Fatalf("the setting call's own embed must land in embed_dir: %v", err)
	}
	if names := f.embeds(t, f.dir()); len(names) != 0 {
		t.Fatalf("no embed may land in the repository folder while embed_dir is set: %v", names)
	}
	// Remembered for later updates, and never evicted there.
	for i := 0; i < 6; i++ {
		later := resultLine(t, f.update(t, `{}`), "embed: ")
		if strings.Contains(later, "/") {
			t.Fatalf("later embed must still go to embed_dir: %q", later)
		}
	}
	if got := len(f.embeds(t, host)); got != 7 {
		t.Fatalf("embeds in embed_dir = %d, want all 7 kept", got)
	}
	// Repository GC never touches the host folder.
	orphan := "zzzorphan-0123456789.html"
	mustWrite(t, host, orphan, "x")
	f.s.maybeCollectWhiteboards(f.root)
	if _, err := os.Stat(filepath.Join(host, orphan)); err != nil {
		t.Fatalf("GC must not touch embed_dir: %v", err)
	}
	// A vanished folder refuses the update, naming it.
	gone := t.TempDir()
	f.update(t, payload(gone))
	os.RemoveAll(gone)
	resp := f.call(t, "whiteboard.update", map[string]any{"payload": map[string]any{}})
	if !isToolError(resp) || !strings.Contains(toolText(t, resp), gone) {
		t.Fatalf("update with a vanished embed_dir must be refused naming it: %s", resp)
	}
	// "" clears it, and embeds return to the repository folder.
	embed = resultLine(t, f.update(t, `{"embed_dir":""}`), "embed: ")
	if !strings.HasPrefix(embed, "ai-docs/whiteboards/") {
		t.Fatalf("cleared embed_dir must return embeds to the repository: %q", embed)
	}
}

func TestWhiteboardRenameRetriesSharingViolation(t *testing.T) {
	for _, target := range []string{"page", "stamp"} {
		t.Run(target, func(t *testing.T) {
			f := newWhiteboardFixture(t)
			suffix := map[string]string{"page": f.replyID() + ".html", "stamp": f.replyID() + ".stamp.js"}[target]
			sharing := errors.New("simulated sharing violation")
			origRename, origRetryable, origBackoff := whiteboardRename, whiteboardRenameRetryable, whiteboardRenameBackoff
			defer func() {
				whiteboardRename, whiteboardRenameRetryable, whiteboardRenameBackoff = origRename, origRetryable, origBackoff
			}()
			whiteboardRenameRetryable = func(err error) bool { return errors.Is(err, sharing) }
			whiteboardRenameBackoff = time.Millisecond
			failures := 2
			whiteboardRename = func(from, to string) error {
				if filepath.Base(to) == suffix && failures > 0 {
					failures--
					return sharing
				}
				return origRename(from, to)
			}
			f.update(t, `{"add":[{"class":"policy","topic":"t","title":"x"}]}`)
			if failures != 0 || f.stamp(t) != "wbStamp(\"1\");\n" {
				t.Fatalf("the %s rename must succeed after retries; failures left %d", target, failures)
			}
			for i := 0; i < 4; i++ {
				f.update(t, `{}`)
			}
			// A violation that outlasts the retries fails the render. The batch
			// is not persisted, and the embed files still match the record: the
			// failed render's own embed is gone and no recorded embed was evicted.
			whiteboardRename = func(from, to string) error {
				if filepath.Base(to) == suffix {
					return sharing
				}
				return origRename(from, to)
			}
			resp := f.call(t, "whiteboard.update", map[string]any{"payload": map[string]any{"add": []any{map[string]any{"class": "policy", "topic": "t", "title": "y"}}}})
			if !isToolError(resp) || !strings.Contains(toolText(t, resp), "write "+map[string]string{"page": "full page", "stamp": "stamp"}[target]) {
				t.Fatalf("persistent sharing violation must fail the render: %s", resp)
			}
			rec, _ := f.s.sessions.readState(f.key)
			if len(rec.Whiteboard.Items) != 1 {
				t.Fatalf("a failed render must not persist its batch: %d items", len(rec.Whiteboard.Items))
			}
			var recorded []string
			for _, e := range rec.Whiteboard.Embeds {
				recorded = append(recorded, e.Name)
			}
			sort.Strings(recorded)
			if got := f.embeds(t, f.dir()); strings.Join(got, ",") != strings.Join(recorded, ",") || len(got) != 5 {
				t.Fatalf("embeds on disk %v must match the record %v after a failed render", got, recorded)
			}
		})
	}
}

func TestWhiteboardExcludeFailureLeavesNoFolder(t *testing.T) {
	f := newWhiteboardFixture(t)
	exclude := filepath.Join(f.root, ".git", "info", "exclude")
	os.Remove(exclude)
	if err := os.MkdirAll(exclude, 0o755); err != nil {
		t.Fatal(err)
	}
	if resp := f.call(t, "whiteboard.update", map[string]any{"payload": map[string]any{}}); !isToolError(resp) {
		t.Fatalf("an unwritable exclude file must fail the first render: %s", resp)
	}
	if _, err := os.Stat(f.dir()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a failed exclude registration must leave no folder, so the next render retries it")
	}
	os.Remove(exclude)
	f.update(t, `{}`)
	if n := excludeEntries(t, f.root); n != 1 {
		t.Fatalf("the retried render must register the exclude: %d entries", n)
	}
}

func TestWhiteboardRefusalsCarryProblemsAndSchema(t *testing.T) {
	f := newWhiteboardFixture(t)
	resp := f.call(t, "whiteboard.update", map[string]any{"payload": `{"add":[]}`})
	text := toolText(t, resp)
	if !isToolError(resp) || !strings.Contains(text, "payload: must be an object; got string") {
		t.Fatalf("JSON-string payload must be refused type-accurately: %s", text)
	}
	resp = f.call(t, "whiteboard.update", map[string]any{"payload": map[string]any{
		"add":    `[{"class":"policy"}]`,
		"answer": []any{map[string]any{"id": 9, "answer": "a"}},
		"extra":  true,
	}})
	text = toolText(t, resp)
	for _, want := range []string{
		"nothing was applied",
		"payload.add: must be an array of objects; got string",
		"payload.answer[0].id: (09) is not in the store",
		"payload.answer[0].answer_quote: required",
		"payload.extra: unknown field",
		"## Payload",
		"whiteboard.update payload: {",
		"close_topics?: [topic],",
		// The rsrc renderer substitutes only {{.Name}} placeholders, so the
		// item-reference example survives rendering.
		"`supersedes {{12}}`",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("refusal missing %q:\n%s", want, text)
		}
	}
	if _, err := os.Stat(f.dir()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a refused first update must write nothing")
	}
	resp = f.call(t, "whiteboard.query", map[string]any{"payload": map[string]any{"status": "done"}})
	if text := toolText(t, resp); !isToolError(resp) || !strings.Contains(text, "payload.status: must be one of") || !strings.Contains(text, "whiteboard.query payload:") {
		t.Fatalf("query refusal must carry the problem and the schema: %s", text)
	}
}

func TestWhiteboardQueryAndNowPathThroughTools(t *testing.T) {
	f := newWhiteboardFixture(t)
	mustWrite(t, f.root, "tmp/now.html", `<svg viewBox="0 0 10 10"><text>diagram-marker</text></svg>`)
	f.update(t, `{"add":[{"class":"technical","topic":"t","title":"settle me"}],"now_path":"tmp/now.html"}`)
	pagePath := filepath.Join(f.dir(), f.replyID()+".html")
	page, _ := os.ReadFile(pagePath)
	if now, item := strings.Index(string(page), "diagram-marker"), strings.Index(string(page), "settle me"); now < 0 || now > item {
		t.Fatalf("Now must render above the topics on the render that changed it (now %d, item %d)", now, item)
	}
	os.Remove(filepath.Join(f.root, "tmp", "now.html"))
	f.update(t, `{"answer":[{"id":1,"answer":"ok","answer_quote":"(01) ok"}]}`)
	page, _ = os.ReadFile(pagePath)
	if now, item := strings.Index(string(page), "diagram-marker"), strings.Index(string(page), "settle me"); now < 0 || now > item {
		t.Fatalf("now_path content must survive the file's deletion and stay above the topics once unchanged (now %d, item %d)", now, item)
	}
	text := toolText(t, f.call(t, "whiteboard.query", map[string]any{"payload": map[string]any{"id": 1}}))
	if !strings.Contains(text, "(01) [answered] [technical] t: settle me") || !strings.Contains(text, `answer_quote: "(01) ok"`) {
		t.Fatalf("query output:\n%s", text)
	}
	if text := toolText(t, f.call(t, "whiteboard.query", map[string]any{})); !strings.Contains(text, "(01)") {
		t.Fatalf("query with no payload must list every item:\n%s", text)
	}
}

func TestWhiteboardNowStaysFreshAcrossImplicitRenders(t *testing.T) {
	f := newWhiteboardFixture(t)
	pagePath := filepath.Join(f.dir(), f.replyID()+".html")
	nowTag := func() string {
		t.Helper()
		page, err := os.ReadFile(pagePath)
		if err != nil {
			t.Fatalf("read page: %v", err)
		}
		return regexp.MustCompile(`<details class="now[^>]*>`).FindString(string(page))
	}
	const fresh, stale = `<details class="now" id="wb-now" open>`, `<details class="now stale" id="wb-now">`
	f.update(t, `{"add":[{"class":"technical","topic":"t","title":"x"}],"now":"<p>n</p>"}`)
	if got := nowTag(); got != fresh {
		t.Fatalf("after the update that set Now: %q, want %q", got, fresh)
	}
	for _, step := range []struct {
		tool string
		args map[string]any
	}{
		{"todo.add", map[string]any{"key": "a", "title": "write tests"}},
		{"agenda.set", map[string]any{"key": "notes", "value": map[string]any{"x": 1}}},
	} {
		if resp := f.call(t, step.tool, step.args); isToolError(resp) {
			t.Fatalf("%s failed: %s", step.tool, resp)
		}
		if got := nowTag(); got != fresh {
			t.Fatalf("after %s: %q, want %q (an implicit render must not make Now stale)", step.tool, got, fresh)
		}
	}
	f.update(t, `{"answer":[{"id":1,"answer":"ok","answer_quote":"(01) ok"}]}`)
	if got := nowTag(); got != stale {
		t.Fatalf("after an update without now: %q, want %q", got, stale)
	}
}

func TestWhiteboardTodoAndAgendaWritesRenderFullPageOnly(t *testing.T) {
	for _, knob := range []string{"off", "on"} {
		t.Run("knob "+knob, func(t *testing.T) {
			f := newWhiteboardFixture(t)
			if resp := f.call(t, "config.tune", map[string]any{"key": "workflow.whiteboard_enabled", "value": knob}); isToolError(resp) {
				t.Fatalf("config.tune: %s", resp)
			}
			f.call(t, "agenda.set", map[string]any{"key": "plan", "value": map[string]any{"step": 1}})
			if _, err := os.Stat(f.dir()); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("writes before the board exists must not render")
			}
			f.update(t, `{}`)
			embedsBefore := len(f.embeds(t, f.dir()))
			steps := []struct {
				tool string
				args map[string]any
			}{
				{"todo.add", map[string]any{"key": "a", "title": "write tests"}},
				{"todo.check", map[string]any{"key": "a", "status": "done"}},
				{"agenda.set", map[string]any{"key": "notes", "value": map[string]any{"x": 1}}},
				{"agenda.clear", map[string]any{"key": "notes"}},
			}
			for i, step := range steps {
				if resp := f.call(t, step.tool, step.args); isToolError(resp) {
					t.Fatalf("%s failed: %s", step.tool, resp)
				}
				want := "wbStamp(\"" + string(rune('2'+i)) + "\");\n"
				if got := f.stamp(t); got != want {
					t.Fatalf("after %s stamp = %q, want %q", step.tool, got, want)
				}
			}
			page, _ := os.ReadFile(filepath.Join(f.dir(), f.replyID()+".html"))
			if !strings.Contains(string(page), "write tests") {
				t.Fatalf("the full page must show the current todos")
			}
			if got := len(f.embeds(t, f.dir())); got != embedsBefore {
				t.Fatalf("implicit renders must write no embed: %d -> %d", embedsBefore, got)
			}
			// A failed write does not re-render.
			f.call(t, "todo.check", map[string]any{"key": "missing", "status": "done"})
			if got := f.stamp(t); got != "wbStamp(\"5\");\n" {
				t.Fatalf("a failed todo write must not re-render: %q", got)
			}
		})
	}
}

func TestWhiteboardToolsRefuseSpawnedKeys(t *testing.T) {
	f := newWhiteboardFixture(t)
	worker, err := f.s.sessions.mint(f.root, roleLead, f.key)
	if err != nil {
		t.Fatal(err)
	}
	delegate, err := f.s.sessions.mint(f.root, roleDelegate, f.key)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{worker, delegate} {
		for _, tool := range []string{"whiteboard.update", "whiteboard.query"} {
			resp := f.call(t, tool, map[string]any{"session_key": key, "payload": map[string]any{}})
			if !isToolError(resp) || !strings.Contains(resp, "not available to a spawned session") {
				t.Fatalf("%s with a spawned key must be refused: %s", tool, resp)
			}
		}
	}
	// The top-level lead works with the knob at its builtin off.
	f.update(t, `{}`)
}

// The whiteboard is a top-level-lead channel, so with the knob on a worker
// key's workflow_manual keeps the knob-off text while its lead's manual
// carries the section.
func TestWhiteboardManualVariantsSkipSpawnedKeys(t *testing.T) {
	f := newWhiteboardFixture(t)
	f.call(t, "config.tune", map[string]any{"key": "workflow.whiteboard_enabled", "value": "on"})
	// A worker's key is lead-scoped with a parent; leaf scopes cannot call
	// workflow_manual at all.
	worker, err := f.s.sessions.mint(f.root, roleLead, f.key)
	if err != nil {
		t.Fatal(err)
	}
	if lead := toolText(t, f.call(t, "workflow_manual", map[string]any{"session_key": f.key})); !strings.Contains(lead, "### Whiteboard") || strings.Contains(lead, lineDiscussionNoCounter) {
		t.Fatalf("knob on: the top-level lead's manual must carry the whiteboard variants")
	}
	manual := toolText(t, f.call(t, "workflow_manual", map[string]any{"session_key": worker}))
	if strings.Contains(manual, "### Whiteboard") || !strings.Contains(manual, lineDiscussionNoCounter) {
		t.Fatalf("knob on: a spawned key's manual must keep the knob-off text:\n%s", manual)
	}
}

func TestWhiteboardKnobWriteDefaultsAndCatalog(t *testing.T) {
	f := newWhiteboardFixture(t)
	if wsconfig.GlobalOnly(wsconfig.ItemWorkflowWhiteboardEnabled) {
		t.Fatalf("workflow.whiteboard_enabled must resolve through every scope")
	}
	entry := registryEntryByKey(wsconfig.ItemWorkflowWhiteboardEnabled)
	if !entry.HarnessBucketed() || !entry.RequiresLeadAuthority || !entry.RepoScoped() {
		t.Fatalf("whiteboard entry must be harness-bucketed, lead-only, and repo-scoped: %+v", entry)
	}
	if got, want := entry.RepoKey(), "workflow.whiteboard_enabled.<claude|codex|pi|all>"; got != want {
		t.Fatalf("RepoKey = %q, want %q", got, want)
	}
	// agents.tier is harness-applicable but keeps its own structured writer.
	tier := registryEntryByKey("agents.tier")
	if tier.HarnessBucketed() || tier.RepoKey() != "agents.tier" {
		t.Fatalf("agents.tier must not take the bucket path: bucketed=%v repo key %q", tier.HarnessBucketed(), tier.RepoKey())
	}

	catalog := parseTuningCatalogResponse(t, f.call(t, "config.list", map[string]any{"format": "json"}))
	knob := requireTuningKnob(t, catalog, "workflow.whiteboard_enabled")
	assertFieldEnum(t, knob.ValueFields, "value", []string{"on", "off"})
	assertFieldEnum(t, knob.SelectorFields, "harness", []string{"claude", "codex", "pi", "*"})
	assertFieldEnum(t, knob.SelectorFields, "scope", wsconfig.ScopeSchemaEnum())
	if got := mustMarshalJSON(t, knob.Current); !strings.Contains(got, `"value":"off"`) || !strings.Contains(got, `"scope":"builtin"`) {
		t.Fatalf("catalog current value must be the builtin off: %s", got)
	}
	if !knob.RepoScope || knob.RepoKey != "workflow.whiteboard_enabled.<claude|codex|pi|all>" {
		t.Fatalf("catalog must offer the repo scope under the bucket keys: %+v", knob)
	}

	// No scope and no harness: global scope, the session's harness bucket.
	resp := toolText(t, f.call(t, "config.tune", map[string]any{"key": "workflow.whiteboard_enabled", "value": "on"}))
	if !strings.Contains(resp, "workflow.whiteboard_enabled.claude: on [scope:global]") || strings.Contains(resp, "shadowed") {
		t.Fatalf("a scope-less, harness-less write must land in the global claude bucket: %s", resp)
	}
	if v := readStoredOverride(t, wsconfig.ScopeGlobal, f.root, "workflow.whiteboard_enabled.claude"); v != "on" {
		t.Fatalf("stored global claude bucket = %q", v)
	}
	catalog = parseTuningCatalogResponse(t, f.call(t, "config.list", map[string]any{"format": "json"}))
	if got := mustMarshalJSON(t, requireTuningKnob(t, catalog, "workflow.whiteboard_enabled").Current); !strings.Contains(got, `"value":"on"`) || !strings.Contains(got, `"scope":"global"`) {
		t.Fatalf("catalog current value must reflect the write: %s", got)
	}
	if got := toolText(t, f.call(t, "config.get", map[string]any{"key": "workflow.whiteboard_enabled"})); !strings.Contains(got, "workflow.whiteboard_enabled: on [scope:global]") {
		t.Fatalf("config.get must read the session's harness bucket: %s", got)
	}
	resp = toolText(t, f.call(t, "config.tune", map[string]any{"key": "workflow.whiteboard_enabled", "reset": true}))
	if !strings.Contains(resp, "workflow.whiteboard_enabled.claude: reset [scope:global]") || !strings.Contains(resp, "effective workflow.whiteboard_enabled: off [scope:builtin]") {
		t.Fatalf("a scope-less reset must clear the global claude bucket and echo global: %s", resp)
	}
	if v := readStoredOverride(t, wsconfig.ScopeGlobal, f.root, "workflow.whiteboard_enabled.claude"); v != "" {
		t.Fatalf("reset must remove the global claude bucket, got %q", v)
	}

	// A write naming another harness lands in its bucket and is reported as
	// not applying to this session.
	resp = toolText(t, f.call(t, "config.tune", map[string]any{"key": "workflow.whiteboard_enabled", "value": "on", "harness": "codex"}))
	if !strings.Contains(resp, "workflow.whiteboard_enabled.codex: on [scope:global]") || !strings.Contains(resp, "shadowed: effective workflow.whiteboard_enabled: off [scope:builtin]") {
		t.Fatalf("a codex-bucket write from a claude session: %s", resp)
	}
	resp = toolText(t, f.call(t, "config.tune", map[string]any{"key": "workflow.whiteboard_enabled", "value": "on", "harness": "*", "scope": "project"}))
	if !strings.Contains(resp, "workflow.whiteboard_enabled.all: on [scope:project]") {
		t.Fatalf("harness * must store the all bucket: %s", resp)
	}
}

func TestWhiteboardKnobWriteRequiresLeadKeyAtEveryScope(t *testing.T) {
	f := newWhiteboardFixture(t)
	delegate, err := f.s.sessions.mint(f.root, roleDelegate, f.key)
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range wsconfig.ScopeSchemaEnum() {
		for _, key := range []string{"", delegate} {
			resp := f.call(t, "config.tune", map[string]any{"key": "workflow.whiteboard_enabled", "value": "on", "scope": scope, "session_key": key})
			if !isToolError(resp) {
				t.Fatalf("scope %s, key %q: a write without a lead session_key must be refused: %s", scope, key, resp)
			}
		}
	}
}

// readStoredOverride reads one scope's stored override for key without the
// layered fallback; "" when absent.
func readStoredOverride(t *testing.T, scope wsconfig.Scope, root, key string) string {
	t.Helper()
	var opts wsconfig.Options
	if scope == wsconfig.ScopeRepo {
		opts.RepoRoot = root
	}
	r := wsconfig.NewResolver(opts, nil, nil, nil)
	rv, err := r.Get("", key)
	if err != nil {
		t.Fatal(err)
	}
	if rv.Scope != scope {
		return ""
	}
	return rv.Value
}

func (f *whiteboardFixture) tune(t *testing.T, args map[string]any) {
	t.Helper()
	args["key"] = "workflow.whiteboard_enabled"
	if resp := f.call(t, "config.tune", args); isToolError(resp) {
		t.Fatalf("config.tune(%v): %s", args, resp)
	}
}

func (f *whiteboardFixture) manualHasWhiteboard(t *testing.T, key string) bool {
	t.Helper()
	return strings.Contains(toolText(t, f.call(t, "workflow_manual", map[string]any{"session_key": key})), "### Whiteboard")
}

func TestWhiteboardKnobResolvesPerHarness(t *testing.T) {
	f := newWhiteboardFixture(t)
	f.tune(t, map[string]any{"value": "on", "harness": "claude"})
	if !f.manualHasWhiteboard(t, f.key) {
		t.Fatalf("a global claude on must render the section in a claude session")
	}
	f.s.sessionHarness = "codex"
	if f.manualHasWhiteboard(t, f.key) {
		t.Fatalf("a global claude on must not render the section in a codex session")
	}
	// The all bucket applies when the harness bucket is unset.
	f.tune(t, map[string]any{"value": "on", "harness": "*"})
	if !f.manualHasWhiteboard(t, f.key) {
		t.Fatalf("the all bucket must apply to a codex session with no codex value")
	}
	f.tune(t, map[string]any{"value": "off", "harness": "codex"})
	if f.manualHasWhiteboard(t, f.key) {
		t.Fatalf("the codex bucket must beat the all bucket")
	}
}

func TestWhiteboardKnobBucketMajorPrecedence(t *testing.T) {
	f := newWhiteboardFixture(t)
	// A member's global claude value beats a committed repo all value.
	f.tune(t, map[string]any{"value": "on", "harness": "*", "scope": "repo"})
	f.tune(t, map[string]any{"value": "off", "harness": "claude"})
	if f.manualHasWhiteboard(t, f.key) {
		t.Fatalf("global claude off must beat repo all on")
	}
	f.tune(t, map[string]any{"value": "on", "harness": "claude"})
	f.tune(t, map[string]any{"value": "off", "harness": "*", "scope": "repo"})
	if !f.manualHasWhiteboard(t, f.key) {
		t.Fatalf("global claude on must beat repo all off")
	}
	// Within one bucket the ordinary scope order holds: repo beats global.
	f.tune(t, map[string]any{"value": "off", "harness": "claude", "scope": "repo"})
	if f.manualHasWhiteboard(t, f.key) {
		t.Fatalf("repo claude off must beat global claude on")
	}
	// Resetting a bucket falls back to the next value in order.
	f.tune(t, map[string]any{"reset": true, "harness": "claude", "scope": "repo"})
	if !f.manualHasWhiteboard(t, f.key) {
		t.Fatalf("after the repo claude reset, global claude on must apply")
	}
	f.tune(t, map[string]any{"reset": true, "harness": "claude"})
	if f.manualHasWhiteboard(t, f.key) {
		t.Fatalf("after the global claude reset, repo all off must apply")
	}
}

func TestWhiteboardKnobLegacyBareKeyReadsAsAll(t *testing.T) {
	f := newWhiteboardFixture(t)
	// A value stored before the knob was bucketed sits under the bare key.
	global := wsconfig.NewResolver(wsconfig.Options{}, nil, nil, nil)
	if err := global.Set(wsconfig.ItemWorkflowWhiteboardEnabled, "on", wsconfig.SetOptions{ExplicitScope: wsconfig.ScopeGlobal}); err != nil {
		t.Fatal(err)
	}
	if !f.manualHasWhiteboard(t, f.key) {
		t.Fatalf("a legacy global bare on must render the section")
	}
	f.s.sessionHarness = "codex"
	if !f.manualHasWhiteboard(t, f.key) {
		t.Fatalf("a legacy bare value applies to every harness")
	}
	f.s.sessionHarness = "claude"
	// A repo all off beats the global bare on: .all is tried before the bare key.
	f.tune(t, map[string]any{"value": "off", "harness": "*", "scope": "repo"})
	if f.manualHasWhiteboard(t, f.key) {
		t.Fatalf("a repo all off must beat a global bare on")
	}
	f.tune(t, map[string]any{"reset": true, "harness": "*", "scope": "repo"})
	f.tune(t, map[string]any{"value": "off", "harness": "claude"})
	if f.manualHasWhiteboard(t, f.key) {
		t.Fatalf("a claude bucket off must beat a global bare on")
	}
}

func TestWhiteboardKnobSessionScope(t *testing.T) {
	f := newWhiteboardFixture(t)
	other, _ := parseLoginResponse(t, callLogin(t, f.s, 920000, f.root, nil))
	f.tune(t, map[string]any{"value": "on", "scope": "session"})
	if !f.manualHasWhiteboard(t, f.key) {
		t.Fatalf("a session on must render the section for that session")
	}
	if f.manualHasWhiteboard(t, other) {
		t.Fatalf("a session on must not reach another lead session")
	}
	// The playbook variants agree with the manual for the same session.
	read := func(key, name string) string {
		t.Helper()
		return toolText(t, f.call(t, "playbook.read", map[string]any{"name": name, "session_key": key}))
	}
	capture := withNamespace(whiteboardCaptureLine)
	bullet := withNamespace(lineTicketQueueBoard)
	if !strings.Contains(read(f.key, "lead-discuss"), capture) || !strings.Contains(read(f.key, "lead-ticket"), bullet) {
		t.Fatalf("lead-discuss and lead-ticket must carry the knob-on variants for the session that set it")
	}
	if !strings.Contains(read(f.key, "lead-workflow-manual"), "### Whiteboard") {
		t.Fatalf("playbook.read of the manual must agree with workflow_manual")
	}
	if strings.Contains(read(other, "lead-discuss"), capture) || strings.Contains(read(other, "lead-ticket"), bullet) {
		t.Fatalf("another session must render the knob-off variants")
	}
	// Resetting the session value falls back to the next scope.
	f.tune(t, map[string]any{"value": "on", "harness": "*"})
	f.tune(t, map[string]any{"value": "off", "scope": "session"})
	if f.manualHasWhiteboard(t, f.key) {
		t.Fatalf("a session off must beat a global all on")
	}
	f.tune(t, map[string]any{"reset": true, "scope": "session"})
	if !f.manualHasWhiteboard(t, f.key) {
		t.Fatalf("after the session reset, the global all on must apply")
	}
}

func TestWhiteboardKnobRepoScopeReachesFreshBootstrapManual(t *testing.T) {
	f := newWhiteboardFixture(t)
	f.tune(t, map[string]any{"value": "on", "harness": "*", "scope": "repo"})
	if _, err := os.Stat(filepath.Join(f.root, ".ws-workflow", "config.json")); err != nil {
		t.Fatalf("repo write must land in the worktree: %v", err)
	}
	fresh := toolText(t, f.call(t, "workflow_manual", map[string]any{"session_key": freshBootstrapKey, "root": f.root}))
	if !strings.Contains(fresh, "### Whiteboard") {
		t.Fatalf("a repo-scope on must render the section in a FRESH-with-root bootstrap manual")
	}
	// FRESH without a root has no repository to read the repo scope from.
	if bare := toolText(t, f.call(t, "workflow_manual", map[string]any{"session_key": freshBootstrapKey})); strings.Contains(bare, "### Whiteboard") {
		t.Fatalf("a FRESH render without a root must not read the repo scope")
	}
}

func TestWhiteboardGarbageCollection(t *testing.T) {
	f := newWhiteboardFixture(t)
	manual := func() {
		t.Helper()
		if resp := f.call(t, "workflow_manual", map[string]any{}); isToolError(resp) {
			t.Fatalf("workflow_manual: %s", resp)
		}
	}
	manual()
	if _, err := os.Stat(f.dir()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("GC must not create the whiteboard folder")
	}
	f.update(t, `{}`)
	live := f.embeds(t, f.dir())
	orphans := []string{"deadorphanid.html", "deadorphanid.stamp.js", "deadorphanid-0123abcd.html"}
	for _, name := range orphans {
		mustWrite(t, f.dir(), name, "x")
	}
	mustWrite(t, f.dir(), "notes.txt", "kept: not a whiteboard file")
	manual()
	for _, name := range orphans {
		if _, err := os.Stat(filepath.Join(f.dir(), name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("orphan %s must be deleted", name)
		}
	}
	for _, name := range append(live, f.replyID()+".html", f.replyID()+".stamp.js", "notes.txt", whiteboardGCMarker) {
		if _, err := os.Stat(filepath.Join(f.dir(), name)); err != nil {
			t.Fatalf("%s must be kept: %v", name, err)
		}
	}
	// At most once per day.
	mustWrite(t, f.dir(), "laterorphan.html", "x")
	manual()
	if _, err := os.Stat(filepath.Join(f.dir(), "laterorphan.html")); err != nil {
		t.Fatalf("a second scan within a day must not run: %v", err)
	}
	old := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(filepath.Join(f.dir(), whiteboardGCMarker), old, old); err != nil {
		t.Fatal(err)
	}
	manual()
	if _, err := os.Stat(filepath.Join(f.dir(), "laterorphan.html")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the next day's scan must delete the orphan")
	}
}

func TestWhiteboardWorkflowTextFollowsKnob(t *testing.T) {
	f := newWhiteboardFixture(t)
	read := func(name string) string {
		t.Helper()
		return toolText(t, f.call(t, "playbook.read", map[string]any{"name": name}))
	}
	manualOff := read("lead-workflow-manual")
	discussOff := read("lead-discuss")
	ticketOff := read("lead-ticket")
	if strings.Contains(strings.ToLower(manualOff), "whiteboard") {
		t.Fatalf("knob off: the manual must carry no whiteboard text")
	}
	if !strings.Contains(manualOff, lineDiscussionNoCounter) || !strings.Contains(discussOff, lineDiscussNotPersisted) || strings.Contains(ticketOff, "id_floor") {
		t.Fatalf("knob off: the shipped rules must render unchanged")
	}

	f.call(t, "config.tune", map[string]any{"key": "workflow.whiteboard_enabled", "value": "on"})
	manualOn := read("lead-workflow-manual")
	for _, want := range []string{
		"### Whiteboard",
		"The whiteboard is your primary channel to the user.",
		"#### Payload",
		"whiteboard.update payload: {",
		"write `{{12}}` in `title`",
		"Whiteboard: ai-docs/whiteboards/<reply-id>.html - (102), (103)",
		"- Discussion IDs come from the whiteboard store",
	} {
		if !strings.Contains(manualOn, want) {
			t.Fatalf("knob on: manual missing %q", want)
		}
	}
	if strings.Contains(manualOn, lineDiscussionNoCounter) || strings.Contains(manualOn, "\n# Whiteboard") {
		t.Fatalf("knob on: the counter rule must be replaced and the playbook title dropped")
	}
	if injected := toolText(t, f.call(t, "workflow_manual", map[string]any{})); !strings.Contains(injected, "### Whiteboard") || !strings.Contains(injected, "#### Payload") {
		t.Fatalf("knob on: the workflow_manual tool must carry the whiteboard section too")
	}
	capture := withNamespace(whiteboardCaptureLine)
	discussOn := read("lead-discuss")
	if !strings.Contains(discussOn, capture) || strings.Contains(discussOn, "nothing here is persisted") {
		t.Fatalf("knob on: lead-discuss must carry the capture line instead:\n%s", discussOn)
	}
	ticketOn := read("lead-ticket")
	bullet := withNamespace(lineTicketQueueBoard)
	idx := strings.Index(ticketOn, bullet)
	if idx < 0 || idx > strings.Index(ticketOn, "### Trace a change before writing it") || idx < strings.Index(ticketOn, "### Queue state") {
		t.Fatalf("knob on: lead-ticket must carry the queue step inside Queue state:\n%s", ticketOn)
	}
	for _, want := range []string{"`id_floor` set to the queue's last-ID line", "the query does not return"} {
		if !strings.Contains(bullet, want) {
			t.Fatalf("queue step missing %q", want)
		}
	}
	// The whiteboard playbook carries the same capture line word for word.
	if playbook := read("whiteboard"); !strings.Contains(playbook, capture) {
		t.Fatalf("whiteboard playbook must carry the capture line %q", capture)
	}
}

func TestWhiteboardPlaybookStatesPointerAndEmbedDirWithoutNamingAHost(t *testing.T) {
	f := newWhiteboardFixture(t)
	text := toolText(t, f.call(t, "playbook.read", map[string]any{"name": "whiteboard"}))
	for _, want := range []string{
		"Whiteboard: ai-docs/whiteboards/<reply-id>.html - (102), (103)",
		"When the host's instructions name a folder its embedded files must live in,",
		"pass that folder as `embed_dir` on your first update after the session starts",
		"embed the bare file name the update returns",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("whiteboard playbook missing %q:\n%s", want, text)
		}
	}
	lower := strings.ToLower(text)
	for _, host := range []string{"orca", "orca_chat", "scratch_dir", "claude", "codex"} {
		if strings.Contains(lower, host) {
			t.Fatalf("the whiteboard playbook must name no host, found %q", host)
		}
	}
}

// TestConfigGetPromptKeyResolvesHarnessBucketFirst pins config.get for a
// prompt.<point> key: the session harness's bucket wins over .all whatever
// scope holds each, and the result names the scope that answered.
func TestConfigGetPromptKeyResolvesHarnessBucketFirst(t *testing.T) {
	f := newWhiteboardFixture(t)
	const key = "prompt.UserPreferenceSection"
	tune := func(harness, scope, value string) {
		t.Helper()
		resp := f.call(t, "config.tune", map[string]any{"key": key, "harness": harness, "scope": scope, "value": value})
		if isToolError(resp) {
			t.Fatalf("config.tune(%s, %s): %s", harness, scope, resp)
		}
	}
	get := func() configGetResult {
		t.Helper()
		resp := f.call(t, "config.get", map[string]any{"key": key, "format": "json"})
		if isToolError(resp) {
			t.Fatalf("config.get: %s", resp)
		}
		var result configGetResult
		if err := json.Unmarshal([]byte(toolText(t, resp)), &result); err != nil {
			t.Fatalf("config.get json: %v\n%s", err, toolText(t, resp))
		}
		return result
	}

	tune("*", "session", "all-text")
	if got := get(); got.Value != "all-text" || got.Scope != "session" {
		t.Fatalf("only .all set: %+v", got)
	}
	// The claude bucket at a broader scope still beats .all at session.
	tune("claude", "global", "claude-text")
	if got := get(); got.Value != "claude-text" || got.Scope != "global" {
		t.Fatalf("claude bucket must win over .all: %+v", got)
	}
	// Another harness's bucket does not apply to this claude session.
	tune("codex", "session", "codex-text")
	if got := get(); got.Value != "claude-text" || got.Scope != "global" {
		t.Fatalf("codex bucket must not apply: %+v", got)
	}
}

// TestConfigListOverridesResolveBucketedKnobForSessionHarness pins the scoped
// overrides view: the bare workflow.whiteboard_enabled row shows the value
// effective for this harness (matching the catalog's current), while the
// per-bucket rows keep showing where the stored values live.
func TestConfigListOverridesResolveBucketedKnobForSessionHarness(t *testing.T) {
	f := newWhiteboardFixture(t)
	if resp := f.call(t, "config.tune", map[string]any{"key": "workflow.whiteboard_enabled", "value": "on"}); isToolError(resp) {
		t.Fatalf("config.tune: %s", resp)
	}
	if resp := f.call(t, "config.tune", map[string]any{"key": "workflow.whiteboard_enabled", "value": "off", "harness": "codex", "scope": "project"}); isToolError(resp) {
		t.Fatalf("config.tune codex: %s", resp)
	}
	resp := f.call(t, "config.list", map[string]any{"format": "json"})
	var view struct {
		ResolvedOverrides []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
			Scope string `json:"scope"`
		} `json:"resolved_overrides"`
	}
	if err := json.Unmarshal([]byte(toolText(t, resp)), &view); err != nil {
		t.Fatalf("config.list json: %v", err)
	}
	rows := map[string][2]string{}
	for _, item := range view.ResolvedOverrides {
		rows[item.Key] = [2]string{item.Value, item.Scope}
	}
	if got := rows["workflow.whiteboard_enabled"]; got != [2]string{"on", "global"} {
		t.Fatalf("bare row must show the claude-effective value, got %v", got)
	}
	if got := rows["workflow.whiteboard_enabled.claude"]; got != [2]string{"on", "global"} {
		t.Fatalf("claude bucket row = %v", got)
	}
	if got := rows["workflow.whiteboard_enabled.codex"]; got != [2]string{"off", "project"} {
		t.Fatalf("codex bucket row = %v", got)
	}
}
