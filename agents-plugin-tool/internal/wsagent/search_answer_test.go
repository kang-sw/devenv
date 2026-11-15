package wsagent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wslive"
)

type answerHarness struct {
	*harness
	s          *SearchAnswers
	claude, pi *fakeBackend
}

func newAnswerHarness(t *testing.T) *answerHarness {
	t.Helper()
	stubLookPath(t)
	h := newHarness(t, nil)
	a := &answerHarness{harness: h, claude: &fakeBackend{exe: "fake-claude"}, pi: &fakeBackend{exe: "fake-pi"}}
	h.runner.opts.Backends = map[string]Backend{BackendClaude: a.claude, BackendPi: a.pi}
	a.s = NewSearchAnswers(SearchAnswerOptions{Runner: h.runner, Session: func(id string) (string, []byte, error) {
		if id == "missing" {
			return "", nil, os.ErrNotExist
		}
		return h.root, []byte(`{"items":[{"id":852}],"topics":[]}`), nil
	}})
	t.Cleanup(a.s.Close)
	return a
}

func awaitAnswer(t *testing.T, a *searchAnswer) answerView {
	t.Helper()
	select {
	case <-a.done:
		return a.snapshot()
	case <-time.After(5 * time.Second):
		t.Fatal("answer did not reap its process")
		return answerView{}
	}
}

func answerRequest(s http.Handler, method, path, body, origin, peer, host string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://"+host+path, strings.NewReader(body))
	r.RemoteAddr = peer
	r.Header.Set("Content-Type", "application/json")
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func TestSearchAnswerHarnessSelection(t *testing.T) {
	for _, tc := range []struct {
		name    string
		missing []string
		want    string
	}{
		{"claude-first", nil, BackendClaude}, {"pi-only", []string{"fake-claude"}, BackendPi}, {"none", []string{"fake-claude", "fake-pi"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newAnswerHarness(t)
			stubLookPath(t, tc.missing...)
			w := answerRequest(h.s, "POST", SearchAnswerRoutePrefix+"session", `{"question":"where?"}`, "http://localhost:1234", "127.0.0.1:55", "localhost:1234")
			if tc.want == "" {
				if w.Code != 503 || !strings.Contains(w.Body.String(), "no agent harness") {
					t.Fatalf("missing harness: %d %s", w.Code, w.Body)
				}
				return
			}
			if w.Code != 201 {
				t.Fatalf("start: %d %s", w.Code, w.Body)
			}
			b := h.claude
			if tc.want == BackendPi {
				b = h.pi
			}
			p := b.waitProc(t, 0)
			p.nextSent(t)
			p.reply("found")
			if v := awaitAnswer(t, h.s.bySession["session"]); v.Status != TurnDone || v.Text != "found" || !p.isClosed() {
				t.Fatalf("answer: %+v, closed %v", v, p.isClosed())
			}
		})
	}
}

func TestSearchAnswerOverrideCeiling(t *testing.T) {
	for _, backend := range []string{BackendClaude, BackendPi} {
		p := Profile{Backend: backend, Tools: searchAnswerTools(backend)[:1], Model: "custom-model", Effort: "low", Instructions: "custom.md"}
		if err := checkSearchAnswerProfile(ResolvedProfile{Name: SearchAnswerProfileClaude, Definition: p}); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name   string
		change func(*Profile)
	}{
		{"shell", func(p *Profile) { p.Tools = []string{"Bash"} }},
		{"write", func(p *Profile) { p.Tools = []string{"Write"} }},
		{"network", func(p *Profile) { p.Tools = []string{"WebFetch"} }},
		{"rule", func(p *Profile) { p.Tools = []string{"Read(//** )"} }},
		{"ws_tools", func(p *Profile) { p.WSTools = []string{"whiteboard.query"} }},
		{"mcp_servers", func(p *Profile) { p.MCPServers = map[string]json.RawMessage{"x": json.RawMessage(`{"command":"x"}`)} }},
		{"plugins", func(p *Profile) { p.Plugins = []string{"x"} }},
		{"skills", func(p *Profile) { p.Skills = true }},
		{"lead", func(p *Profile) { p.Lead = true }},
		{"permission_mode", func(p *Profile) { p.PermissionMode = "bypassPermissions" }},
		{"list_in_index", func(p *Profile) { p.ListInIndex = true }},
		{"idle_timeout_seconds", func(p *Profile) { n := 0; p.IdleTimeoutSeconds = &n }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := Profile{Backend: BackendClaude}
			tc.change(&p)
			err := checkSearchAnswerProfile(ResolvedProfile{Name: SearchAnswerProfileClaude, Scope: wsconfig.ScopeProject, Source: "override.json", Definition: p})
			if err == nil || !strings.Contains(err.Error(), ProfilePrefix+SearchAnswerProfileClaude) || !strings.Contains(err.Error(), "override.json") {
				t.Fatalf("override not named/refused: %v", err)
			}
		})
	}
}

func TestSearchAnswerOverrideLaunchFrozen(t *testing.T) {
	h := newAnswerHarness(t)
	path := filepath.Join(h.root, ".ws-workflow", "config.json")
	// Whole-value repo override may select Pi even under the Claude key.
	writeStructured(t, path, map[string]string{ProfilePrefix + SearchAnswerProfileClaude: `{"backend":"pi","model":"custom","effort":"low","tools":["read"],"instructions":"custom.md"}`})
	if err := os.WriteFile(filepath.Join(h.root, "custom.md"), []byte("Use precise evidence."), 0600); err != nil {
		t.Fatal(err)
	}
	g := newGate(t, func() {})
	h.s.opts.beforeSpawn = g.wait
	a, err := h.s.start("session", "question")
	if err != nil {
		t.Fatal(err)
	}
	g.awaitEntered(t)
	// Replace the winning definition while the service has checked its
	// ceiling but neither create nor startProcess has resolved it yet.
	writeStructured(t, path, map[string]string{ProfilePrefix + SearchAnswerProfileClaude: `{"backend":"pi","model":"custom","effort":"low","tools":["read","bash"],"instructions":"custom.md"}`})
	g.release()
	p := h.pi.waitProc(t, 0)
	p.nextSent(t)
	spec := h.pi.start(0)
	if spec.Profile.Backend != BackendPi || !reflect.DeepEqual(spec.Profile.Tools, []string{"read"}) || spec.Model != "custom" || spec.Effort != "low" {
		t.Fatalf("not the checked override: tools=%v backend=%s model=%s effort=%s", spec.Profile.Tools, spec.Profile.Backend, spec.Model, spec.Effort)
	}
	data, err := os.ReadFile(spec.Instructions)
	if err != nil || !strings.Contains(string(data), "Use precise evidence.") {
		t.Fatalf("instructions: %q %v", data, err)
	}
	p.reply("done")
	awaitAnswer(t, a)
	// Invalid selected override fails the request instead of falling back.
	writeStructured(t, filepath.Join(h.root, ".ws-workflow", "config.json"), map[string]string{ProfilePrefix + SearchAnswerProfileClaude: `{"backend":"pi","tools":["bash"]}`})
	if _, err := h.s.start("session", "again"); err == nil || !strings.Contains(err.Error(), ProfilePrefix+SearchAnswerProfileClaude) {
		t.Fatalf("unsafe override: %v", err)
	}
}

func TestSearchAnswerReadableGrantExcludesRunnerCredentials(t *testing.T) {
	h := newAnswerHarness(t)
	a, err := h.s.start("session", "inspect the board")
	if err != nil {
		t.Fatal(err)
	}
	p := h.claude.waitProc(t, 0)
	prompt := p.nextSent(t)
	spec := h.claude.start(0)
	boardPath := strings.Split(strings.Split(prompt, "Whiteboard snapshot: ")[1], "\n")[0]
	snapshotDir := filepath.Dir(boardPath)
	questionDir := filepath.Dir(snapshotDir)
	if filepath.Base(snapshotDir) != "snapshot" {
		t.Fatalf("snapshot is not in its dedicated readable subtree: %s", snapshotDir)
	}

	var wantRules []string
	for _, tool := range []string{"Read", "Grep", "Glob"} {
		for _, dir := range []string{h.root, snapshotDir} {
			wantRules = append(wantRules, tool+"(//"+strings.TrimPrefix(filepath.ToSlash(dir), "/")+"/**)")
		}
	}
	if !reflect.DeepEqual(spec.Profile.Tools, wantRules) {
		t.Fatalf("read grants must name only the project and snapshot subtree: got %v, want %v", spec.Profile.Tools, wantRules)
	}
	if !strings.HasPrefix(spec.Dir, filepath.Join(questionDir, "runner")+string(filepath.Separator)) {
		t.Fatalf("runner is not a sibling of the readable snapshot subtree: %s", spec.Dir)
	}
	rec, err := readRecord(spec.Dir)
	if err != nil || rec.WSKey == "" {
		t.Fatalf("fixture has no credential-bearing runner record: %v", err)
	}
	for _, readable := range []string{h.root, snapshotDir} {
		err := filepath.WalkDir(readable, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			if entry.Name() == "agent.json" {
				t.Errorf("credential-bearing artifact is inside a read grant: %s", path)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(data), rec.WSKey) {
				t.Errorf("minted session credential is inside a read grant: %s", path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	files, err := os.ReadDir(snapshotDir)
	if err != nil || len(files) != 1 || files[0].Name() != "board.json" {
		t.Fatalf("readable subtree must contain only the board snapshot: %v, %v", files, err)
	}
	p.reply("done")
	awaitAnswer(t, a)
	if _, err := os.Stat(questionDir); !os.IsNotExist(err) {
		t.Fatalf("question cleanup did not remove both sibling trees: %v", err)
	}
}

func TestSearchAnswerLaunchFlagsAndSnapshot(t *testing.T) {
	for _, backend := range []string{BackendClaude, BackendPi} {
		t.Run(backend, func(t *testing.T) {
			h := newAnswerHarness(t)
			b := h.claude
			if backend == BackendPi {
				stubLookPath(t, "fake-claude")
				b = h.pi
			}
			a, err := h.s.start("session", "show [[board:852]]")
			if err != nil {
				t.Fatal(err)
			}
			p := b.waitProc(t, 0)
			prompt := p.nextSent(t)
			spec := b.start(0)
			if spec.workDir() != h.root || spec.WSMCP != nil || spec.Lead != nil || spec.Holder != nil {
				t.Fatalf("launch: %+v", spec)
			}
			line := strings.Split(strings.Split(prompt, "Whiteboard snapshot: ")[1], "\n")[0]
			data, err := os.ReadFile(line)
			if err != nil || !strings.Contains(string(data), `"id":852`) {
				t.Fatalf("snapshot: %q %v", data, err)
			}
			info, _ := os.Stat(line)
			if info.Mode().Perm() != 0400 {
				t.Fatalf("snapshot mode: %v", info.Mode())
			}
			if !strings.Contains(prompt, "[[file:<root-relative path>:<line>]]") {
				t.Fatal("missing citation contract")
			}
			if backend == BackendClaude {
				args, err := claudeArgs(spec)
				if err != nil {
					t.Fatal(err)
				}
				joined := strings.Join(args, "\n")
				for _, pair := range []string{"--tools\nRead,Grep,Glob", "--permission-mode\ndontAsk", "--setting-sources\n", "--strict-mcp-config", "--disable-slash-commands"} {
					if !strings.Contains(joined, pair) {
						t.Errorf("missing %s in %v", pair, args)
					}
				}
				for _, tool := range []string{"Read", "Grep", "Glob"} {
					for _, dir := range []string{h.root, filepath.Dir(line)} {
						want := tool + "(//" + strings.TrimPrefix(filepath.ToSlash(dir), "/") + "/**)"
						if !strings.Contains(joined, want) {
							t.Errorf("missing scoped rule %s", want)
						}
					}
				}
				if err := checkClaudeInit(claudeInit{Tools: []string{"Read", "Grep", "Glob", "Bash"}, PermissionMode: "dontAsk"}, spec); err == nil {
					t.Fatal("extra shell tool accepted")
				}
			} else {
				args := strings.Join(piArgs(spec, ""), "\n")
				for _, want := range []string{"--tools\nread,grep,find,ls", "--no-extensions", "--no-context-files", "--no-skills", "--no-prompt-templates", "--no-approve"} {
					if !strings.Contains(args, want) {
						t.Errorf("missing %s in %s", want, args)
					}
				}
				if err := checkPiTools([]string{"bash"}, piToolSurface(spec)); err == nil {
					t.Fatal("extra shell tool accepted")
				}
			}
			p.reply("answer")
			awaitAnswer(t, a)
			if _, err := os.Stat(filepath.Dir(line)); !os.IsNotExist(err) {
				t.Fatalf("snapshot survived reaping: %v", err)
			}
			if rows := h.runner.List(); len(rows) != 0 {
				t.Fatalf("private answer in public runner: %+v", rows)
			}
			entries, _ := os.ReadDir(filepath.Join(h.dir, "sessions"))
			if len(entries) != 0 {
				t.Fatalf("private answer in public session registry: %v", entries)
			}
		})
	}
}

func TestSearchAnswerLifecycle(t *testing.T) {
	for _, mode := range []string{"success", "failure", "crash", "cancel", "timeout", "shutdown", "start-failure"} {
		t.Run(mode, func(t *testing.T) {
			h := newAnswerHarness(t)
			if mode == "timeout" {
				h.s.opts.timeout = 100 * time.Millisecond
			}
			if mode == "start-failure" {
				h.claude.startErr = errors.New("cannot start fake")
			}
			a, err := h.s.start("session", mode)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "start-failure" {
				if v := awaitAnswer(t, a); v.Status != TurnFailed || !strings.Contains(v.Error, "cannot start fake") {
					t.Fatalf("%+v", v)
				}
				return
			}
			p := h.claude.waitProc(t, 0)
			p.nextSent(t)
			want := TurnDone
			switch mode {
			case "success":
				p.reply("ok")
			case "failure":
				p.events <- Event{TurnEnd: true, TurnError: "fake failed"}
				want = TurnFailed
			case "crash":
				p.exit()
				want = TurnFailed
			case "cancel":
				a.cancel()
				want = "cancelled"
			case "timeout":
				want = "timed_out"
			case "shutdown":
				h.s.Close()
				want = "cancelled"
			}
			if v := awaitAnswer(t, a); v.Status != want {
				t.Fatalf("%s: %+v", mode, v)
			}
			if mode == "crash" {
				// exit closes Events without calling Close: the fake's
				// isClosed flag records Close, not process liveness.
				if _, open := <-p.events; open {
					t.Fatal("crashed process still emits events")
				}
			} else if !p.isClosed() {
				t.Fatal("answer process survived terminal status")
			}
			if entries, _ := os.ReadDir(filepath.Join(h.dir, "search-answers")); len(entries) != 0 {
				t.Fatalf("private files survived: %v", entries)
			}
		})
	}
}

func TestSearchAnswerAdmissionAndSupersede(t *testing.T) {
	h := newAnswerHarness(t)
	a, err := h.s.start("one", "first")
	if err != nil {
		t.Fatal(err)
	}
	p := h.claude.waitProc(t, 0)
	p.nextSent(t)
	second, err := h.s.start("two", "second")
	if err != nil {
		t.Fatal(err)
	}
	p2 := h.claude.waitProc(t, 1)
	p2.nextSent(t)
	if _, err := h.s.start("three", "over cap"); requestStatus(err) != 429 {
		t.Fatalf("cap: %v", err)
	}
	replacement, err := h.s.start("one", "replacement")
	if err != nil {
		t.Fatal(err)
	}
	if v := awaitAnswer(t, a); v.Status != "cancelled" || !p.isClosed() {
		t.Fatalf("supersede did not reap: %+v", v)
	}
	p3 := h.claude.waitProc(t, 2)
	p3.nextSent(t)
	if replacement.snapshot().ID == a.snapshot().ID || h.claude.start(2).SessionID != "" {
		t.Fatal("supersede reused a conversation")
	}
	p2.reply("two done")
	awaitAnswer(t, second)
	third, err := h.s.start("three", "slot freed")
	if err != nil {
		t.Fatal(err)
	}
	h.claude.waitProc(t, 3).nextSent(t)
	third.cancel()
	replacement.cancel()
	awaitAnswer(t, third)
	awaitAnswer(t, replacement)
}

func TestSearchAnswerHTTPAccessAndPrivateStreaming(t *testing.T) {
	h := newAnswerHarness(t)
	a, err := h.s.start("session", "question")
	if err != nil {
		t.Fatal(err)
	}
	p := h.claude.waitProc(t, 0)
	p.nextSent(t)
	path := SearchAnswerRoutePrefix + "session/" + a.snapshot().ID
	for _, tc := range []struct {
		name, peer, host, origin string
		code                     int
	}{
		{"remote", "192.0.2.1:55", "localhost:1234", "http://localhost:1234", 404},
		{"remote-host", "127.0.0.1:55", "192.0.2.1:1234", "http://192.0.2.1:1234", 404},
		{"bad-origin", "127.0.0.1:55", "localhost:1234", "http://evil.test", 403},
	} {
		for _, endpoint := range []struct{ method, path, body string }{{"POST", SearchAnswerRoutePrefix + "session", `{"question":"bad"}`}, {"GET", path, ""}, {"GET", path + "/events", ""}, {"POST", path + "/cancel", `{}`}} {
			w := answerRequest(h.s, endpoint.method, endpoint.path, endpoint.body, tc.origin, tc.peer, tc.host)
			if w.Code != tc.code {
				t.Errorf("%s %s %s: %d", tc.name, endpoint.method, endpoint.path, w.Code)
			}
		}
	}
	for _, body := range []string{`{}`, `{"question":"  "}`, `{"question":"` + strings.Repeat("q", 33<<10) + `"}`} {
		w := answerRequest(h.s, "POST", SearchAnswerRoutePrefix+"session", body, "http://localhost:1234", "127.0.0.1:55", "localhost:1234")
		if w.Code != 400 {
			t.Errorf("bad body: %d", w.Code)
		}
	}
	w := answerRequest(h.s, "POST", SearchAnswerRoutePrefix+"missing", `{"question":"?"}`, "http://localhost:1234", "127.0.0.1:55", "localhost:1234")
	if w.Code != 404 {
		t.Fatalf("missing session: %d %s", w.Code, w.Body)
	}
	// An isolated daemon fixture uses an OS-selected port, never the shared
	// daemon. Its generic runner and session streams must know no answer.
	d, err := wsdashboard.StartDaemon(wsdashboard.DaemonOptions{Dir: h.dir, Port: 0, Agents: h.runner, SearchAnswers: h.s})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	base := fmt.Sprintf("http://127.0.0.1:%d", d.Endpoint().Port)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", base+path+"/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "text/event-stream" || resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("stream headers: %v", resp)
	}
	scan := bufio.NewScanner(resp.Body)
	readRegion := func() map[string]string {
		t.Helper()
		for scan.Scan() {
			if line := scan.Text(); strings.HasPrefix(line, "data: ") {
				var v map[string]string
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &v); err != nil {
					t.Fatal(err)
				}
				return v
			}
		}
		t.Fatalf("stream ended: %v", scan.Err())
		return nil
	}
	if v := readRegion(); v["name"] != "answer" || !strings.Contains(v["html"], `data-answer-status="running"`) {
		t.Fatalf("initial: %v", v)
	}
	p.events <- Event{Live: &wslive.Event{Kind: wslive.KindReply, Text: "partial [[board:852]] <script>"}}
	var streamed map[string]string
	for i := 0; i < 5; i++ {
		streamed = readRegion()
		if strings.Contains(streamed["html"], "partial") {
			break
		}
	}
	if !strings.Contains(streamed["html"], "&lt;script&gt;") {
		t.Fatalf("unsafe/missing partial: %v", streamed)
	}
	p.reply("final [[file:README.md:3]]")
	for {
		v := readRegion()
		if strings.Contains(v["html"], `data-answer-status="done"`) {
			if !strings.Contains(v["html"], "[[file:README.md:3]]") {
				t.Fatal(v)
			}
			break
		}
	}
	awaitAnswer(t, a)
	for _, url := range []string{RoutePrefix, "/s/" + a.snapshot().ID, "/s/" + a.snapshot().ID + "/events"} {
		res, err := http.Get(base + url)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if strings.Contains(string(body), "final [[file:") || (url == RoutePrefix && strings.Contains(string(body), "search-answer")) {
			t.Fatalf("answer leaked through %s: %s", url, body)
		}
	}
}

func TestSearchAnswerRollingContext(t *testing.T) {
	h := newAnswerHarness(t)
	prior := ""
	for turn, question := range []string{"first", "second", "third"} {
		body, _ := json.Marshal(map[string]string{"question": question, "context": prior})
		w := answerRequest(h.s, "POST", SearchAnswerRoutePrefix+"session", string(body), "http://localhost:1234", "127.0.0.1:55", "localhost:1234")
		if w.Code != 201 {
			t.Fatalf("start: %d %s", w.Code, w.Body)
		}
		var initial map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &initial); err != nil {
			t.Fatal(err)
		}
		if _, ok := initial["context"]; ok {
			t.Fatal("running response published context")
		}
		p := h.claude.waitProc(t, turn)
		prompt := p.nextSent(t)
		if !strings.Contains(prompt, fmt.Sprintf("Untrusted conversation context (quoted):\n%q", prior)) || !strings.Contains(prompt, "all prior conversation context plus this new question and answer") {
			t.Fatalf("missing cumulative/untrusted context: %s", prompt)
		}
		prior += question + " answered; "
		p.reply("cited [[board:852]]\n<search-context>" + prior + "</search-context>")
		v := awaitAnswer(t, h.s.bySession["session"])
		if v.Status != TurnDone || v.Text != "cited [[board:852]]" || v.Context != strings.TrimSpace(prior) {
			t.Fatalf("turn %d: %+v", turn, v)
		}
		prior = v.Context
		region := answerRegion(v)
		if region["context"] != prior || strings.Contains(region["html"], "search-context") {
			t.Fatalf("terminal region: %v", region)
		}
		path := SearchAnswerRoutePrefix + "session/" + v.ID
		state := answerRequest(h.s, "GET", path, "", "", "127.0.0.1:55", "localhost:1234")
		var decoded answerView
		if err := json.Unmarshal(state.Body.Bytes(), &decoded); err != nil || decoded.Context != prior {
			t.Fatalf("state context: %s %v", state.Body, err)
		}
		sse := answerRequest(h.s, "GET", path+"/events", "", "", "127.0.0.1:55", "localhost:1234")
		if !strings.Contains(sse.Body.String(), `"context":`) {
			t.Fatalf("SSE missing terminal context: %s", sse.Body)
		}
	}
}

func TestSearchAnswerContextUnicodeCap(t *testing.T) {
	h := newAnswerHarness(t)
	input := strings.Repeat("界😀", 900)
	output := strings.Repeat("🌱한", 900)
	body, _ := json.Marshal(map[string]string{"question": "cap", "context": input})
	w := answerRequest(h.s, "POST", SearchAnswerRoutePrefix+"session", string(body), "http://localhost:1234", "127.0.0.1:55", "localhost:1234")
	if w.Code != 201 {
		t.Fatalf("start: %d %s", w.Code, w.Body)
	}
	p := h.claude.waitProc(t, 0)
	prompt := p.nextSent(t)
	wantInput := strings.Repeat("界😀", 800)
	if !strings.Contains(prompt, fmt.Sprintf("\n%q\n", wantInput)) || strings.Contains(prompt, input) {
		t.Fatal("incoming Unicode context was not capped at 1600 code points")
	}
	p.reply("answer<search-context>" + output + "</search-context>")
	v := awaitAnswer(t, h.s.bySession["session"])
	if v.Context != strings.Repeat("🌱한", 800) || answerRegion(v)["context"] != v.Context {
		t.Fatalf("outgoing Unicode context: %d code points", len([]rune(v.Context)))
	}
}

func TestSearchAnswerContextTerminalOnly(t *testing.T) {
	for _, mode := range []string{"success", "failure", "cancel", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			h := newAnswerHarness(t)
			if mode == "timeout" {
				h.s.opts.timeout = 2 * time.Second
			}
			a, err := h.s.start("session", "question", "previous summary")
			if err != nil {
				t.Fatal(err)
			}
			p := h.claude.waitProc(t, 0)
			p.nextSent(t)
			for _, raw := range []string{"answer <search-con", "answer <search-context>private summary</search-context>"} {
				p.events <- Event{Live: &wslive.Event{Kind: wslive.KindReply, Text: raw}}
				deadline := time.Now().Add(time.Second)
				for strings.TrimSpace(a.snapshot().Text) != "answer" && time.Now().Before(deadline) {
					time.Sleep(10 * time.Millisecond)
				}
				// Wait for the second cumulative snapshot too, not just the
				// unchanged visible answer from the partial-tag snapshot.
				time.Sleep(150 * time.Millisecond)
				v := a.snapshot()
				if strings.TrimSpace(v.Text) != "answer" || v.Context != "" || answerRegion(v)["context"] != "" || strings.Contains(answerRegion(v)["html"], "search-con") {
					t.Fatalf("stream leaked context: %+v", v)
				}
			}
			switch mode {
			case "success":
				p.reply("answer <search-context>private summary</search-context>")
			case "failure":
				p.events <- Event{TurnEnd: true, TurnError: "fake failed"}
			case "cancel":
				a.cancel()
			}
			v := awaitAnswer(t, a)
			if mode == "success" {
				if v.Context != "private summary" {
					t.Fatalf("missing successful context: %+v", v)
				}
			} else if v.Context != "" || answerRegion(v)["context"] != "" {
				t.Fatalf("unsuccessful context promoted: %+v", v)
			}
			if strings.Contains(v.Text, "search-con") || strings.Contains(v.Text, "private summary") {
				t.Fatalf("terminal display leaked context: %+v", v)
			}
		})
	}
}

func TestSearchAnswerContextPartialAndLegacy(t *testing.T) {
	const open = "<search-context>"
	for n := 1; n < len(open); n++ {
		text, summary := splitSearchContext("answer" + open[:n])
		if text != "answer" || summary != "" {
			t.Fatalf("partial opening %d: %q %q", n, text, summary)
		}
	}
	for _, raw := range []string{"answer<search-context>secret", "answer<search-context>secret</search-cont"} {
		if text, summary := splitSearchContext(raw); text != "answer" || summary != "" {
			t.Fatalf("incomplete context: %q %q", text, summary)
		}
	}
	h := newAnswerHarness(t)
	w := answerRequest(h.s, "POST", SearchAnswerRoutePrefix+"session", `{"question":"legacy"}`, "http://localhost:1234", "127.0.0.1:55", "localhost:1234")
	if w.Code != 201 {
		t.Fatalf("legacy request: %d %s", w.Code, w.Body)
	}
	p := h.claude.waitProc(t, 0)
	p.nextSent(t)
	p.reply("plain legacy [[board:852]] <script>")
	v := awaitAnswer(t, h.s.bySession["session"])
	raw, _ := json.Marshal(v)
	if v.Text != "plain legacy [[board:852]] <script>" || v.Context != "" || strings.Contains(string(raw), `"context"`) {
		t.Fatalf("legacy response: %s", raw)
	}
	if _, ok := answerRegion(v)["context"]; ok {
		t.Fatal("legacy region gained context")
	}
}

func TestSearchAnswerBuiltinDescriptions(t *testing.T) {
	for _, name := range []string{SearchAnswerProfileClaude, SearchAnswerProfilePi} {
		p, err := ParseProfile(BuiltinProfiles()[ProfilePrefix+name])
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(p.Description, "best effort, not a sandbox") || p.Model != "" || p.Effort != "" {
			t.Fatalf("builtin: %+v", p)
		}
	}
}
