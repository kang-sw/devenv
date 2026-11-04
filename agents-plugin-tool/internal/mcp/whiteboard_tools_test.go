package mcp

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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
	if now, item := strings.Index(string(page), "diagram-marker"), strings.Index(string(page), "settle me"); now < 0 || now < item {
		t.Fatalf("now_path content must survive the file's deletion and render below the topics once unchanged (now %d, item %d)", now, item)
	}
	text := toolText(t, f.call(t, "whiteboard.query", map[string]any{"payload": map[string]any{"id": 1}}))
	if !strings.Contains(text, "(01) [answered] [technical] t: settle me") || !strings.Contains(text, `answer_quote: "(01) ok"`) {
		t.Fatalf("query output:\n%s", text)
	}
	if text := toolText(t, f.call(t, "whiteboard.query", map[string]any{})); !strings.Contains(text, "(01)") {
		t.Fatalf("query with no payload must list every item:\n%s", text)
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

func TestWhiteboardKnobIsGlobalOnlyAndCataloged(t *testing.T) {
	f := newWhiteboardFixture(t)
	if !wsconfig.GlobalOnly(wsconfig.ItemWorkflowWhiteboardEnabled) {
		t.Fatalf("workflow.whiteboard_enabled must be global-only")
	}
	resp := f.call(t, "config.tune", map[string]any{"key": "workflow.whiteboard_enabled", "value": "on"})
	if !strings.Contains(toolText(t, resp), "workflow.whiteboard_enabled: on [scope:global]") {
		t.Fatalf("knob write must land in global scope: %s", resp)
	}
	resp = f.call(t, "config.tune", map[string]any{"key": "workflow.whiteboard_enabled", "value": "on", "scope": "project"})
	if !isToolError(resp) {
		t.Fatalf("a project-scope write must be refused: %s", resp)
	}
	catalog := parseTuningCatalogResponse(t, f.call(t, "config.list", map[string]any{"format": "json"}))
	knob := requireTuningKnob(t, catalog, "workflow.whiteboard_enabled")
	assertFieldEnum(t, knob.ValueFields, "value", []string{"on", "off"})
	if !strings.Contains(mustMarshalJSON(t, knob.Current), `"value":"on"`) {
		t.Fatalf("catalog current value must reflect the write: %+v", knob.Current)
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
