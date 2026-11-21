package mcp

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func whiteboardFolder(root, key string) wswhiteboard.FolderStore {
	return wswhiteboard.FolderStore{Dir: whiteboardBoardDir(root, key)}
}

func newTestBoardOwner(t *testing.T, root, version string) *Server {
	t.Helper()
	boards := NewWhiteboardStores(nil)
	t.Cleanup(func() {
		if err := boards.Close(); err != nil {
			t.Errorf("close boards: %v", err)
		}
	})
	return NewServerWithOptions(ServerOptions{Root: root, Version: version, BoardOwner: true, Boards: boards})
}

func restartTestBoardOwner(t *testing.T, old *Server, root, version string) *Server {
	t.Helper()
	if err := old.sessions.boards.Close(); err != nil {
		t.Fatalf("close boards for restart: %v", err)
	}
	return newTestBoardOwner(t, root, version)
}

// whiteboardFixture is one lead session in a fresh git repository, with the
// shipped rsrc tree so refusals and manual renders read the real playbook.
type whiteboardFixture struct {
	source *DashboardSessionSource
	s      *Server
	key    string
	root   string
	id     int
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
	s := newTestBoardOwner(t, root, "test")
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

func (f *whiteboardFixture) update(t *testing.T, commands string) string {
	t.Helper()
	resp := f.call(t, "whiteboard.update", map[string]any{"commands": commands})
	if isToolError(resp) {
		t.Fatalf("whiteboard.update(%s) failed: %s", commands, toolText(t, resp))
	}
	return toolText(t, resp)
}

// embedDirCommand is the wb:embed-dir command for dir; attribute values are
// never decoded, so the path is written as is.
func embedDirCommand(dir string) string {
	return `<wb:embed-dir path="` + dir + `"/>`
}

// collapseSpace folds every whitespace run to one space, so an assertion on
// playbook prose does not depend on where the source wraps a line.
func collapseSpace(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func (f *whiteboardFixture) dir() string {
	return filepath.Join(f.root, "ai-docs", "whiteboards")
}

func (f *whiteboardFixture) replyID() string {
	return wsmailbox.ReplyID(f.key)
}

func (f *whiteboardFixture) pageDir() string {
	return filepath.Join(f.dir(), f.replyID())
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
		switch tool.Name {
		case "whiteboard.update":
			if strings.Join(names, ",") != "commands,session_key" {
				t.Fatalf("%s advertises %v, want only session_key and commands", tool.Name, names)
			}
			if commands, _ := props["commands"].(map[string]any); commands["type"] != "string" {
				t.Fatalf("%s commands must be a string: %v", tool.Name, commands)
			}
		case "whiteboard.query":
			if strings.Join(names, ",") != "payload,session_key" {
				t.Fatalf("%s advertises %v, want only session_key and payload", tool.Name, names)
			}
			if payload, _ := props["payload"].(map[string]any); payload["type"] != "object" || payload["properties"] != nil {
				t.Fatalf("%s payload must be an opaque object: %v", tool.Name, payload)
			}
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

func TestWhiteboardUpdateWritesNoEmbedSnapshots(t *testing.T) {
	f := newWhiteboardFixture(t)
	for i := 0; i < 2; i++ {
		text := f.update(t, ``)
		if strings.Contains(text, "embed:") {
			t.Fatalf("update returned an embed line: %s", text)
		}
	}
	if names := f.embeds(t, f.dir()); len(names) != 0 {
		t.Fatalf("update wrote embed snapshots: %v", names)
	}
}

func TestWhiteboardEmbedDirIsDeprecatedNoop(t *testing.T) {
	f := newWhiteboardFixture(t)
	host := t.TempDir()
	f.update(t, ``)
	before, ok := f.s.sessions.readState(f.key, partAll)
	if !ok {
		t.Fatal("read state before deprecated command")
	}
	text := f.update(t, embedDirCommand(host))
	if !strings.Contains(text, "info: wb:embed-dir is deprecated and ignored; embed HTML snapshots are no longer rendered.") || strings.Contains(text, "embed:") {
		t.Fatalf("deprecated command result: %s", text)
	}
	after, ok := f.s.sessions.readState(f.key, partAll)
	if !ok {
		t.Fatal("read state after deprecated command")
	}
	before.Whiteboard.Seq = after.Whiteboard.Seq
	if !reflect.DeepEqual(before.Whiteboard, after.Whiteboard) {
		t.Fatalf("deprecated command stored state: before=%+v after=%+v", before.Whiteboard, after.Whiteboard)
	}
	if entries, err := os.ReadDir(host); err != nil || len(entries) != 0 {
		t.Fatalf("deprecated command wrote host files: %v, %v", entries, err)
	}
}

func TestWhiteboardExcludeFailureLeavesNoCommittedBoard(t *testing.T) {
	f := newWhiteboardFixture(t)
	exclude := filepath.Join(f.root, ".git", "info", "exclude")
	os.Remove(exclude)
	if err := os.MkdirAll(exclude, 0o755); err != nil {
		t.Fatal(err)
	}
	if resp := f.call(t, "whiteboard.update", map[string]any{"commands": ""}); !isToolError(resp) {
		t.Fatalf("an unwritable exclude file must fail the first render: %s", resp)
	}
	assertWhiteboardUncommittedForTest(t, f.pageDir())
	os.Remove(exclude)
	f.update(t, ``)
	if n := excludeEntries(t, f.root); n != 1 {
		t.Fatalf("the retried render must register the exclude: %d entries", n)
	}
}

func TestWhiteboardRefusalsCarryProblemsAndPlaybookPointer(t *testing.T) {
	f := newWhiteboardFixture(t)
	resp := f.call(t, "whiteboard.update", map[string]any{"commands": `<wb:answer id="9" value="a">a</wb:answer>
<wb:topic name="t"><wb:add class="policy" color="red"><wb:title>x</wb:title></wb:add></wb:topic>
<wb:mystery/>`})
	text := toolText(t, resp)
	if !isToolError(resp) {
		t.Fatalf("a bad batch must be refused: %s", text)
	}
	for _, want := range []string{
		"nothing was applied",
		"command 1 <wb:answer> (line 1) (09): not in the store",
		"command 2 <wb:add> (line 2): unknown attribute color",
		"command 3 <wb:mystery> (line 3): unknown command",
		"Syntax and commands: the `whiteboard` playbook (ws/playbook.read).",
	} {
		if !strings.Contains(collapseSpace(text), want) {
			t.Fatalf("update refusal missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "## Command syntax") || strings.Contains(text, "## Commands") || strings.Contains(text, "## Query") {
		t.Fatalf("the update refusal must not repeat playbook sections:\n%s", text)
	}
	assertWhiteboardUncommittedForTest(t, f.pageDir())

	resp = f.call(t, "whiteboard.query", map[string]any{"payload": map[string]any{"status": "done"}})
	text = toolText(t, resp)
	if !isToolError(resp) || !strings.Contains(text, "payload.status: must be one of") {
		t.Fatalf("query refusal must carry the problem: %s", text)
	}
	if !strings.Contains(text, "## Query") || !strings.Contains(text, "whiteboard.query payload:") || strings.Contains(text, "## Commands") || strings.Contains(text, "## Command syntax") {
		t.Fatalf("query refusal must repeat the Query section and not the Commands section: %s", text)
	}
	// Every problem is listed in one refusal, with the grammar.
	resp = f.call(t, "whiteboard.query", map[string]any{"payload": map[string]any{"depth": "deep", "fields": []any{"answer", "bogus"}, "extra": 1}})
	text = toolText(t, resp)
	for _, want := range []string{"payload.depth: must be one of topics, items, summary, full", "payload.fields[1]: must be one of", "payload.extra: unknown field", "whiteboard.query payload:"} {
		if !isToolError(resp) || !strings.Contains(text, want) {
			t.Fatalf("query refusal missing %q: %s", want, text)
		}
	}
}

// The retired payload argument goes through the full tools/call path, whose
// central precheck would otherwise refuse it as an undeclared argument with
// no migration hint (handlerOwnedArguments defers it to the handler).
func TestWhiteboardUpdateRefusesTheRetiredPayloadArgument(t *testing.T) {
	f := newWhiteboardFixture(t)
	for name, payload := range map[string]any{
		"object": map[string]any{"add": []any{map[string]any{"class": "policy", "topic": "t", "title": "x"}}},
		"empty":  map[string]any{},
		"string": `{"add":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			resp := f.call(t, "whiteboard.update", map[string]any{"payload": payload})
			text := toolText(t, resp)
			if !isToolError(resp) {
				t.Fatalf("a payload argument must be refused: %s", text)
			}
			for _, want := range []string{
				"payload: no longer accepted; whiteboard.update takes commands",
				"nothing was applied",
				"Syntax and commands: the `whiteboard` playbook (ws/playbook.read).",
			} {
				if !strings.Contains(text, want) {
					t.Fatalf("payload refusal missing %q:\n%s", want, text)
				}
			}
			if strings.Contains(text, "unknown argument") {
				t.Fatalf("the refusal must come from the handler, not the precheck: %s", text)
			}
		})
	}
	// Carrying both is still a refusal, and nothing renders.
	resp := f.call(t, "whiteboard.update", map[string]any{"payload": map[string]any{}, "commands": ""})
	if !isToolError(resp) {
		t.Fatalf("payload beside commands must be refused: %s", resp)
	}
	if _, err := os.Stat(f.dir()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a refused payload call must write nothing")
	}
	// An argument the handler does not own is still refused by the precheck.
	if resp := f.call(t, "whiteboard.update", map[string]any{"commands": "", "bogus": 1}); !isToolError(resp) {
		t.Fatalf("an undeclared argument must still be refused: %s", resp)
	}
}

func TestWhiteboardUpdateRefusesNonStringCommands(t *testing.T) {
	f := newWhiteboardFixture(t)
	for name, commands := range map[string]any{
		"object": map[string]any{"add": []any{}},
		"array":  []any{"<wb:floor id=\"1\"/>"},
		"number": 7,
		"bool":   true,
	} {
		t.Run(name, func(t *testing.T) {
			resp := f.call(t, "whiteboard.update", map[string]any{"commands": commands})
			text := toolText(t, resp)
			if !isToolError(resp) || !strings.Contains(text, "commands: must be a string of wb: commands") || !strings.Contains(text, "Syntax and commands: the `whiteboard` playbook (ws/playbook.read).") {
				t.Fatalf("a non-string commands must be refused with the playbook pointer: %s", text)
			}
		})
	}
	if _, err := os.Stat(f.dir()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a refused call must write nothing")
	}
}

func TestWhiteboardQueryAndTopicBodyThroughTools(t *testing.T) {
	f := newWhiteboardFixture(t)
	f.update(t, `<wb:topic name="t"><wb:add class="technical"><wb:title>settle me</wb:title></wb:add></wb:topic>
<wb:topic name="t"><wb:body>
background body-marker line
</wb:body></wb:topic>`)
	page := f.servedPage(t)
	if body, item := strings.Index(string(page), "body-marker"), strings.Index(string(page), "settle me"); body < 0 || item < 0 || body < item {
		t.Fatalf("a legacy body panel must render after the earlier item in write order (body %d, item %d)", body, item)
	}
	if strings.Contains(string(page), `id="wb-now"`) || strings.Contains(string(page), `class="now`) {
		t.Fatalf("the page must carry no Now slot")
	}
	f.update(t, `<wb:answer id="1" value="ok">(01) ok</wb:answer>`)
	// Once answered, the prior body is retained in lazy topic history.
	history, ok := f.source.ReadTopicHistory(wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root}, "", []string{"t"}, "")
	if !ok || len(history) != 1 || !strings.Contains(string(history[0].HTML), "body-marker") {
		t.Fatal("the body must survive in served history after an unrelated update")
	}
	// An id query defaults to depth summary: the settled item's answer only.
	text := toolText(t, f.call(t, "whiteboard.query", map[string]any{"payload": map[string]any{"id": 1}}))
	if !strings.Contains(text, "topic \"t\" [open]") || !strings.Contains(text, "\n  (01) [answered] [technical] settle me\n    answer: ok\n") || strings.Contains(text, "answer_quote") {
		t.Fatalf("query output:\n%s", text)
	}
	text = toolText(t, f.call(t, "whiteboard.query", map[string]any{"payload": map[string]any{"id": 1, "depth": "full"}}))
	if !strings.Contains(text, `answer_quote: "(01) ok"`) {
		t.Fatalf("query output at depth full:\n%s", text)
	}
	if strings.Contains(text, "topic body:") {
		t.Fatalf("a query without topic must not print a body:\n%s", text)
	}
	text = toolText(t, f.call(t, "whiteboard.query", map[string]any{"payload": map[string]any{"topic": "t", "body": true}}))
	if !strings.Contains(text, "topic body:") || !strings.Contains(text, "background body-marker line") || !strings.Contains(text, "settle me") {
		t.Fatalf("query with topic must print the body and the items:\n%s", text)
	}
	// Discovery exposes metadata and system history, not item detail/body.
	if text := toolText(t, f.call(t, "whiteboard.query", map[string]any{})); !strings.Contains(text, "topic \"t\" [open]") || strings.Contains(text, "topic body:") || strings.Contains(text, "background body-marker") || strings.Contains(text, "  (01) [answered]") {
		t.Fatalf("query with no payload must stay metadata-only:\n%s", text)
	}
	// wb:edit changes the stored body, and the next query shows the change.
	f.update(t, `<wb:topic name="t"><wb:edit><wb:old>body-marker</wb:old><wb:new>edited-marker</wb:new></wb:edit></wb:topic>`)
	text = toolText(t, f.call(t, "whiteboard.query", map[string]any{"payload": map[string]any{"topic": "t", "body": true}}))
	if !strings.Contains(text, "background edited-marker line") || strings.Contains(text, "body-marker") {
		t.Fatalf("query after wb:edit:\n%s", text)
	}
}

// No update result carries a now_file line, and the retired Now file is no
// longer part of the board's files.
func TestWhiteboardUpdateResultHasNoNowFile(t *testing.T) {
	f := newWhiteboardFixture(t)
	text := f.update(t, ``)
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "now_file:") {
			t.Fatalf("update result must carry no now_file line:\n%s", text)
		}
	}
	if retiredEmbedFile.MatchString("abc.now.md") {
		t.Fatalf("the file pattern must not match .now.md")
	}
}

// The stale-topic nudge reaches the update result as an info line, once per
// hour, and never the pages written to disk.
func TestWhiteboardUpdateResultCarriesStaleTopicNudge(t *testing.T) {
	f := newWhiteboardFixture(t)
	var bodies strings.Builder
	for _, name := range []string{"alpha", "beta", "gamma", "delta", "epsilon"} {
		bodies.WriteString(`<wb:topic name="` + name + `"><wb:body>notes on ` + name + "</wb:body></wb:topic>\n")
	}
	const warning = `discussing_without_open_items: "alpha", "beta", "delta", "epsilon", "gamma"`
	if text := f.update(t, bodies.String()); resultLine(t, text, "info: ") != warning {
		t.Fatalf("fresh topics must retain the structural warning without a stale nudge:\n%s", text)
	}
	// Seed an aged board under a fresh key: committed entry timestamps are
	// immutable, and aging only current content no longer makes a topic stale.
	key, err := f.s.sessions.mint(f.root, roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	f.key = key
	old := time.Now().Add(-2 * time.Hour).UTC()
	if err := f.s.sessions.mutateRecord(f.key, partWhiteboard, func(rec *sessionRecord) error {
		_, problems := wswhiteboard.Apply(rec.Whiteboard, bodies.String(), wswhiteboard.Env{Now: old})
		if len(problems) > 0 {
			return errors.New(strings.Join(problems, "; "))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	text := f.update(t, ``)
	want := "5 of 5 open topics unchanged for 60+ min (alpha, beta, gamma, delta, epsilon); if finished, close them with <wb:close/>."
	if got := resultLine(t, text, "info: "); got != want+" "+warning {
		t.Fatalf("info line = %q\nwant        %q", got, want+" "+warning)
	}
	if strings.Contains(f.servedPage(t), "unchanged for") {
		t.Fatal("served page renders the response-only nudge")
	}
	if text := f.update(t, ``); resultLine(t, text, "info: ") != warning {
		t.Fatalf("a second update within the hour must retain only the structural warning:\n%s", text)
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
			resp := f.call(t, tool, map[string]any{"session_key": key})
			if !isToolError(resp) || !strings.Contains(resp, "not available to a spawned session") {
				t.Fatalf("%s with a spawned key must be refused: %s", tool, resp)
			}
		}
	}
	// The top-level lead works with the knob at its builtin off.
	f.update(t, ``)
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
	if got, want := entry.RepoKey(), "workflow.whiteboard_enabled.<claude|codex|pi|agy|all>"; got != want {
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
	assertFieldEnum(t, knob.SelectorFields, "harness", []string{"claude", "codex", "pi", "agy", "*"})
	assertFieldEnum(t, knob.SelectorFields, "scope", wsconfig.ScopeSchemaEnum())
	if got := mustMarshalJSON(t, knob.Current); !strings.Contains(got, `"value":"off"`) || !strings.Contains(got, `"scope":"builtin"`) {
		t.Fatalf("catalog current value must be the builtin off: %s", got)
	}
	if !knob.RepoScope || knob.RepoKey != "workflow.whiteboard_enabled.<claude|codex|pi|agy|all>" {
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
	capture := withNamespace(whiteboardCaptureLine, RuntimeNamespace())
	bullet := withNamespace(lineTicketQueueBoard, RuntimeNamespace())
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

func TestWhiteboardFirstUpdateTurnsSessionKnobOn(t *testing.T) {
	f := newWhiteboardFixture(t)
	other, _ := parseLoginResponse(t, callLogin(t, f.s, 920000, f.root, nil))
	if f.manualHasWhiteboard(t, f.key) {
		t.Fatalf("precondition: knob resolves off")
	}
	// A query never flips the knob.
	f.call(t, "whiteboard.query", map[string]any{})
	if f.manualHasWhiteboard(t, f.key) {
		t.Fatalf("whiteboard.query must not turn the knob on")
	}
	f.update(t, `<wb:topic name="t"><wb:add class="policy"><wb:title>q?</wb:title></wb:add></wb:topic>`)
	if v, ok := f.s.sessions.getOverride(f.key, "workflow.whiteboard_enabled.claude"); !ok || v != "on" {
		t.Fatalf("session claude bucket = %q, %v; want on", v, ok)
	}
	if !f.manualHasWhiteboard(t, f.key) {
		t.Fatalf("the manual after the first update must carry the whiteboard section")
	}
	if f.manualHasWhiteboard(t, other) {
		t.Fatalf("another session must stay off")
	}
}

func TestWhiteboardUpdateLeavesExplicitSessionOff(t *testing.T) {
	f := newWhiteboardFixture(t)
	f.tune(t, map[string]any{"value": "off", "scope": "session"})
	f.update(t, `<wb:topic name="t"><wb:add class="policy"><wb:title>q?</wb:title></wb:add></wb:topic>`)
	if f.manualHasWhiteboard(t, f.key) {
		t.Fatalf("an explicit session off must stand after a whiteboard update")
	}
	// A session all-bucket off stands even when a global harness bucket also says off.
	g := newWhiteboardFixture(t)
	g.tune(t, map[string]any{"value": "off", "harness": "claude"})
	g.tune(t, map[string]any{"value": "off", "harness": "*", "scope": "session"})
	g.update(t, `<wb:topic name="t"><wb:add class="policy"><wb:title>q?</wb:title></wb:add></wb:topic>`)
	if g.manualHasWhiteboard(t, g.key) {
		t.Fatalf("a session all off must stand after a whiteboard update")
	}
}

func TestWhiteboardUpdateOverridesNonSessionOffAtSessionScope(t *testing.T) {
	f := newWhiteboardFixture(t)
	f.tune(t, map[string]any{"value": "off", "harness": "claude"})
	f.update(t, `<wb:topic name="t"><wb:add class="policy"><wb:title>q?</wb:title></wb:add></wb:topic>`)
	if !f.manualHasWhiteboard(t, f.key) {
		t.Fatalf("a global off must be overridden at session scope by the first update")
	}
	if !strings.Contains(toolText(t, f.call(t, "config.list", map[string]any{})), "workflow.whiteboard_enabled") {
		t.Fatalf("config.list still lists the knob")
	}
}

func TestWhiteboardRefusedUpdateDoesNotTurnKnobOn(t *testing.T) {
	f := newWhiteboardFixture(t)
	resp := f.call(t, "whiteboard.update", map[string]any{"commands": `<wb:bogus/>`})
	if !isToolError(resp) {
		t.Fatalf("a bogus command must be refused: %s", resp)
	}
	if _, ok := f.s.sessions.getOverride(f.key, "workflow.whiteboard_enabled.claude"); ok {
		t.Fatalf("a refused batch must not write the session knob")
	}
	if f.manualHasWhiteboard(t, f.key) {
		t.Fatalf("a refused batch must not turn the manual section on")
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
	if !strings.Contains(manualOff, lineDiscussionNoCounter) || !strings.Contains(discussOff, lineDiscussNotPersisted) || strings.Contains(ticketOff, "wb:floor") {
		t.Fatalf("knob off: the shipped rules must render unchanged")
	}

	f.call(t, "config.tune", map[string]any{"key": "workflow.whiteboard_enabled", "value": "on"})
	manualOn := read("lead-workflow-manual")
	for _, want := range []string{
		"### Whiteboard",
		"The whiteboard is your primary channel for what you ask the user.",
		// Explanations moved to chat with the Now slot; the mandate and the
		// playbook's each-turn rule must agree on it.
		"While the user is watching the board (the playbook's Each turn rule says when), progress and explanations go on it too and the chat does not repeat what the board holds; otherwise the chat carries them as usual.",

		"- Discussion IDs come from the whiteboard store",
	} {
		if !strings.Contains(collapseSpace(manualOn), want) {
			t.Fatalf("knob on: manual missing %q", want)
		}
	}
	if strings.Contains(manualOn, lineDiscussionNoCounter) || strings.Contains(manualOn, "\n# Whiteboard") {
		t.Fatalf("knob on: the counter rule must be replaced and the playbook title dropped")
	}
	injected := toolText(t, f.call(t, "workflow_manual", map[string]any{}))
	if !strings.Contains(injected, "### Whiteboard") || !strings.Contains(injected, "#### Commands") {
		t.Fatalf("knob on: the workflow_manual tool must carry the whiteboard section too")
	}
	// The shared command syntax renders exactly once, ahead of the tools'
	// sections, and only while a command-language tool is on.
	syntaxHeadings := func(text string) int {
		n := 0
		for _, line := range strings.Split(text, "\n") {
			if strings.HasSuffix(line, "# Command syntax") {
				n++
			}
		}
		return n
	}
	if n := syntaxHeadings(manualOff); n != 0 {
		t.Fatalf("knob off: the manual must carry no command syntax section (%d headings)", n)
	}
	for name, text := range map[string]string{"playbook.read": manualOn, "workflow_manual": injected} {
		if n := syntaxHeadings(text); n != 1 {
			t.Fatalf("knob on, %s: the command syntax section must appear exactly once, found %d", name, n)
		}
		if !strings.Contains(text, "\n### Command syntax\n") || strings.Index(text, "### Command syntax") > strings.Index(text, "### Whiteboard") {
			t.Fatalf("knob on, %s: the syntax section must sit at ### ahead of the whiteboard section", name)
		}
	}
	capture := withNamespace(whiteboardCaptureLine, RuntimeNamespace())
	discussOn := read("lead-discuss")
	if !strings.Contains(discussOn, capture) || strings.Contains(discussOn, "nothing here is persisted") {
		t.Fatalf("knob on: lead-discuss must carry the capture line instead:\n%s", discussOn)
	}
	ticketOn := read("lead-ticket")
	if ticketOn == ticketOff || strings.Contains(ticketOn, "<!-- ws:override:") {
		t.Fatal("lead-ticket's whiteboard extension must resolve and change with the knob")
	}
}

func TestWhiteboardPlaybookNamesNoHost(t *testing.T) {
	f := newWhiteboardFixture(t)
	text := toolText(t, f.call(t, "playbook.read", map[string]any{"name": "whiteboard"}))
	if strings.TrimSpace(text) == "" {
		t.Fatal("whiteboard playbook is empty")
	}
	lower := strings.ToLower(text)
	for _, host := range []string{"orca", "orca_chat", "scratch_dir", "claude", "codex"} {
		if strings.Contains(lower, host) {
			t.Fatalf("the whiteboard playbook must name no host, found %q", host)
		}
	}
}

// The whiteboard playbook read alone states the whole input format: it ends
// with the shared command syntax section, which is no playbook of its own.
func TestWhiteboardPlaybookReadAppendsCommandSyntax(t *testing.T) {
	f := newWhiteboardFixture(t)
	text := toolText(t, f.call(t, "playbook.read", map[string]any{"name": "whiteboard"}))
	if strings.Count(text, "## Command syntax") != 1 {
		t.Fatalf("the whiteboard playbook must end with the command syntax section once:\n%s", text)
	}
	if resp := f.call(t, "playbook.read", map[string]any{"name": "command-syntax"}); !isToolError(resp) && !strings.Contains(toolText(t, resp), "not found") {
		t.Fatalf("the command syntax part must not read as a playbook: %s", toolText(t, resp))
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
