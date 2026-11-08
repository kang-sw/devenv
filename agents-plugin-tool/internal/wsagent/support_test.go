package wsagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/claudeagents"
	"github.com/kang-sw/devenv/internal/wsdashboard"
)

// stubLookPath makes every executable resolve except those in missing.
func stubLookPath(t *testing.T, missing ...string) {
	t.Helper()
	gone := map[string]bool{}
	for _, name := range missing {
		gone[name] = true
	}
	old := lookPath
	lookPath = func(name string) (string, error) {
		if gone[name] {
			return "", exec.ErrNotFound
		}
		return "/usr/local/bin/" + filepath.Base(name), nil
	}
	t.Cleanup(func() { lookPath = old })
}

// supportHarness is a runner with fake claude and pi backends, two tracker
// stores, and a Support over the runner's directory.
type supportHarness struct {
	*harness
	claude, pi *fakeBackend
	stores     map[string]string
	support    *Support
	now        time.Time
}

func newSupportHarness(t *testing.T) *supportHarness {
	t.Helper()
	h := newHarness(t, nil)
	sh := &supportHarness{
		harness: h,
		claude:  &fakeBackend{exe: "fake-claude"},
		pi:      &fakeBackend{exe: "fake-pi"},
		stores:  map[string]string{"alpha": t.TempDir(), "beta": t.TempDir()},
		now:     time.Now(),
	}
	h.config.Backends = map[string]Backend{BackendClaude: sh.claude, BackendPi: sh.pi}
	stubLookPath(t)
	sh.restart()
	return sh
}

func (sh *supportHarness) storeRoot(id string) (string, error) {
	if root, ok := sh.stores[id]; ok {
		return root, nil
	}
	return "", fmt.Errorf("no registered store %q", id)
}

// restart replaces the runner and the Support with fresh ones over the
// same directory, as a daemon restart does.
func (sh *supportHarness) restart() {
	sh.runner.Close()
	sh.start()
	sh.support = NewSupport(SupportOptions{Runner: sh.runner, Dir: sh.dir, StoreRoot: sh.storeRoot, Now: func() time.Time { return sh.now }})
}

func (sh *supportHarness) send(store, text string) State {
	sh.t.Helper()
	st, err := sh.support.Send(store, text)
	if err != nil {
		sh.t.Fatalf("Send(%s): %v", store, err)
	}
	return st
}

func (sh *supportHarness) storeConv(store string) *State {
	sh.t.Helper()
	ans, err := sh.support.Store(store)
	if err != nil {
		sh.t.Fatalf("Store(%s): %v", store, err)
	}
	return ans.Conversation
}

func (sh *supportHarness) choose(store string, req selectRequest) SelectAnswer {
	sh.t.Helper()
	ans, err := sh.support.Select(store, req)
	if err != nil {
		sh.t.Fatalf("Select(%s): %v", store, err)
	}
	return ans
}

func strp(s string) *string { return &s }

func TestSupportKeepsOneConversationPerStore(t *testing.T) {
	sh := newSupportHarness(t)
	if c := sh.storeConv("alpha"); c != nil {
		t.Fatalf("a store with no conversation reports %+v", c)
	}
	first := sh.send("alpha", "hello")
	if first.Profile != SupportProfileClaude || first.Backend != BackendClaude || first.Root != sh.stores["alpha"] {
		t.Fatalf("first conversation = %+v, want the claude support profile at the store root", first)
	}
	p := sh.claude.proc(0)
	if got := p.nextSent(t); got != "hello" {
		t.Fatalf("first turn = %q", got)
	}
	p.reply("hi")
	eventually(t, "turn end", func() bool { return sh.state(first.ID).Status == StatusIdle })
	second := sh.send("alpha", "again")
	if second.ID != first.ID {
		t.Fatalf("second send opened %s, want the store's conversation %s", second.ID, first.ID)
	}
	if got := p.nextSent(t); got != "again" {
		t.Fatalf("second turn = %q", got)
	}
	p.reply("ok")

	sh.restart()
	c := sh.storeConv("alpha")
	if c == nil || c.ID != first.ID {
		t.Fatalf("after a restart the store's conversation = %+v, want %s", c, first.ID)
	}
	if st := sh.send("alpha", "after restart"); st.ID != first.ID {
		t.Fatalf("send after restart opened %s, want %s", st.ID, first.ID)
	}
	if got := sh.claude.proc(1).nextSent(t); got != "after restart" {
		t.Fatalf("resumed turn = %q", got)
	}
	if c := sh.storeConv("beta"); c != nil {
		t.Fatalf("another store shares the conversation: %+v", c)
	}
}

func TestSupportNewConversationClosesTheOld(t *testing.T) {
	sh := newSupportHarness(t)
	old := sh.send("alpha", "hello")
	fresh, err := sh.support.New("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if fresh.ID == old.ID || fresh.Status != StatusIdle {
		t.Fatalf("new conversation = %+v (old %s)", fresh, old.ID)
	}
	if st := sh.state(old.ID); st.Status != StatusClosed {
		t.Fatalf("old conversation status %s, want closed", st.Status)
	}
	if c := sh.storeConv("alpha"); c == nil || c.ID != fresh.ID {
		t.Fatalf("store maps %+v, want %s", c, fresh.ID)
	}
	if sh.claude.startCount() != 1 {
		t.Fatalf("new must not send: %d process starts", sh.claude.startCount())
	}
	// New with no open conversation just opens one.
	if st, err := sh.support.New("beta"); err != nil || st.Root != sh.stores["beta"] {
		t.Fatalf("New(beta) = %+v, %v", st, err)
	}
}

func TestSupportHarnessSwitchReplacesTheConversation(t *testing.T) {
	sh := newSupportHarness(t)
	old := sh.send("alpha", "hello")
	ans := sh.choose("alpha", selectRequest{Harness: strp(BackendPi), Model: strp("openrouter/~anthropic/claude-fable-latest")})
	if ans.Conversation == nil || ans.Conversation.ID == old.ID {
		t.Fatalf("harness switch answered %+v", ans.Conversation)
	}
	if c := ans.Conversation; c.Profile != SupportProfilePi || c.Backend != BackendPi || c.Model != "openrouter/~anthropic/claude-fable-latest" || c.Effort != "" {
		t.Fatalf("switched conversation = %+v", c)
	}
	if want := (Selection{Harness: BackendPi, Model: "openrouter/~anthropic/claude-fable-latest"}); ans.Selection != want {
		t.Fatalf("selection = %+v, want %+v", ans.Selection, want)
	}
	if st := sh.state(old.ID); st.Status != StatusClosed {
		t.Fatalf("old conversation status %s, want closed", st.Status)
	}
	if c := sh.storeConv("alpha"); c == nil || c.ID != ans.Conversation.ID {
		t.Fatalf("store maps %+v", c)
	}
	// A switch resets model and effort the request does not give.
	back := sh.choose("alpha", selectRequest{Harness: strp(BackendClaude)})
	if back.Selection != (Selection{Harness: BackendClaude}) || back.Conversation.Model != "" || back.Conversation.Backend != BackendClaude {
		t.Fatalf("switch back = %+v", back)
	}
}

func TestSupportModelAndEffortChangeKeepTheConversation(t *testing.T) {
	sh := newSupportHarness(t)
	conv := sh.send("alpha", "hello")
	ans := sh.choose("alpha", selectRequest{Model: strp("sonnet"), Effort: strp("high")})
	if ans.Conversation == nil || ans.Conversation.ID != conv.ID {
		t.Fatalf("model change answered %+v, want conversation %s", ans.Conversation, conv.ID)
	}
	rec, err := ReadRecord(sh.dir, conv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Model != "sonnet" || rec.Effort != "high" || rec.ClosedAt != "" {
		t.Fatalf("record = model %q effort %q closed %q", rec.Model, rec.Effort, rec.ClosedAt)
	}
	if want := (Selection{Harness: BackendClaude, Model: "sonnet", Effort: "high"}); ans.Selection != want {
		t.Fatalf("selection = %+v, want %+v", ans.Selection, want)
	}
	// Naming the conversation's own harness is no switch.
	same := sh.choose("alpha", selectRequest{Harness: strp(BackendClaude), Effort: strp("low")})
	if same.Conversation.ID != conv.ID || same.Conversation.Model != "sonnet" || same.Conversation.Effort != "low" {
		t.Fatalf("same-harness select = %+v", same.Conversation)
	}
	if _, err := sh.support.Select("alpha", selectRequest{Effort: strp("NOT A WORD")}); err == nil {
		t.Fatal("a malformed effort must be refused")
	}
}

func TestSupportLastChoiceIsTheDefaultAndPersists(t *testing.T) {
	sh := newSupportHarness(t)
	if ans, _ := sh.support.Store("alpha"); ans.Selection != (Selection{Harness: BackendClaude}) {
		t.Fatalf("default selection = %+v, want claude with defaults", ans.Selection)
	}
	ans := sh.choose("alpha", selectRequest{Harness: strp(BackendPi), Effort: strp("high")})
	if ans.Conversation != nil {
		t.Fatalf("select with no conversation opened %+v", ans.Conversation)
	}
	if len(sh.runner.List()) != 0 {
		t.Fatalf("select with no conversation must only save the choice")
	}
	conv := sh.send("beta", "hello")
	if conv.Backend != BackendPi || conv.Profile != SupportProfilePi || conv.Effort != "high" {
		t.Fatalf("another store's first conversation = %+v, want the last choice", conv)
	}

	info, err := os.Stat(filepath.Join(sh.dir, supportFileName))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("support.json mode %v, want 0600", info.Mode().Perm())
	}

	sh.restart()
	want := Selection{Harness: BackendPi, Effort: "high"}
	if got := sh.support.Choices(context.Background()).Last; got != want {
		t.Fatalf("last choice after restart = %+v, want %+v", got, want)
	}
	if c := sh.storeConv("beta"); c == nil || c.ID != conv.ID {
		t.Fatalf("beta's conversation after restart = %+v", c)
	}
}

func TestSupportDefaultSkipsAnUnavailableHarness(t *testing.T) {
	sh := newSupportHarness(t)
	stubLookPath(t, "fake-claude")
	if ans, _ := sh.support.Store("alpha"); ans.Selection != (Selection{Harness: BackendPi}) {
		t.Fatalf("default with claude missing = %+v, want pi", ans.Selection)
	}
	if _, err := sh.support.Select("alpha", selectRequest{Harness: strp(BackendClaude)}); err == nil {
		t.Fatal("an unavailable harness must be refused")
	}
	if _, err := sh.support.Select("alpha", selectRequest{Harness: strp("codex")}); err == nil {
		t.Fatal("an unknown harness must be refused")
	}
	stubLookPath(t, "fake-claude", "fake-pi")
	var re *requestError
	if _, err := sh.support.Send("alpha", "hello"); !errors.As(err, &re) || re.status != http.StatusServiceUnavailable {
		t.Fatalf("send with no harness = %v, want 503", err)
	}
}

const piListing = `provider      model                                               context  max-out  thinking  images
openai-codex  gpt-5.3-codex-spark                                 128K     128K     yes       no
openai-codex  gpt-plain                                           128K     16K      no        no
openrouter    ~anthropic/claude-fable-latest                      1M       128K     yes       yes
openrouter    ~anthropic/claude-fable-latest                      1M       128K     yes       yes
`

// listingStub replaces runListing; it counts calls.
type listingStub struct {
	mu    sync.Mutex
	calls int
	args  []string
	out   string
	err   error
}

func stubListing(t *testing.T, out string, err error) *listingStub {
	t.Helper()
	s := &listingStub{out: out, err: err}
	old := runListing
	runListing = func(_ context.Context, command string, args, _ []string) ([]byte, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.calls++
		s.args = append([]string{command}, args...)
		return []byte(s.out), s.err
	}
	t.Cleanup(func() { runListing = old })
	return s
}

func (s *listingStub) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// realChoices is a Support over the real claude and pi backends.
func realChoices(t *testing.T, now *time.Time) *Support {
	t.Helper()
	r := NewRunner(Options{
		Dir:      t.TempDir(),
		Backends: map[string]Backend{BackendClaude: ClaudeBackend{Command: "claude-x"}, BackendPi: PiBackend{Command: "pi-x"}},
		Config:   testConfig(t),
	})
	return NewSupport(SupportOptions{Runner: r, Dir: t.TempDir(), StoreRoot: func(string) (string, error) { return "", errors.New("none") }, Now: func() time.Time { return *now }})
}

func harnessByName(t *testing.T, ans ChoicesAnswer, name string) SupportHarness {
	t.Helper()
	h, ok := findHarness(ans.Harnesses, name)
	if !ok {
		t.Fatalf("no %s harness in %+v", name, ans.Harnesses)
	}
	return h
}

func TestSupportChoicesListEachHarness(t *testing.T) {
	stubLookPath(t)
	listing := stubListing(t, piListing, nil)
	now := time.Now()
	s := realChoices(t, &now)
	ans := s.Choices(context.Background())
	if len(ans.Harnesses) != 2 || ans.Harnesses[0].Harness != BackendClaude || ans.Harnesses[1].Harness != BackendPi {
		t.Fatalf("harnesses = %+v, want claude then pi", ans.Harnesses)
	}
	if ans.Last != (Selection{Harness: BackendClaude}) {
		t.Fatalf("last = %+v", ans.Last)
	}

	claude := harnessByName(t, ans, BackendClaude)
	var ids []string
	for _, m := range claude.Models {
		ids = append(ids, m.ID)
	}
	if !reflect.DeepEqual(ids, []string{"opus", "sonnet", "haiku", "fable"}) {
		t.Fatalf("claude models = %v, want exactly the version-less aliases", ids)
	}
	if !reflect.DeepEqual(claude.Efforts, claudeagents.Levels) || claude.Profile != SupportProfileClaude || !claude.Available {
		t.Fatalf("claude harness = %+v", claude)
	}

	pi := harnessByName(t, ans, BackendPi)
	if !pi.Available || pi.Error != "" || pi.Profile != SupportProfilePi {
		t.Fatalf("pi harness = %+v", pi)
	}
	if !reflect.DeepEqual(pi.Efforts, []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}) {
		t.Fatalf("pi efforts = %v", pi.Efforts)
	}
	raw, _ := json.Marshal(pi.Models)
	if want := `[{"id":"openai-codex/gpt-5.3-codex-spark","thinking":true},{"id":"openai-codex/gpt-plain","thinking":false},{"id":"openrouter/~anthropic/claude-fable-latest","thinking":true}]`; string(raw) != want {
		t.Fatalf("pi models = %s\nwant %s", raw, want)
	}
	if want := []string{"/usr/local/bin/pi-x", "--offline", "--no-approve", "--list-models"}; !reflect.DeepEqual(listing.args, want) {
		t.Fatalf("listing ran %v, want %v", listing.args, want)
	}

	// The listing is cached for a while.
	s.Choices(context.Background())
	if listing.count() != 1 {
		t.Fatalf("listing ran %d times within the cache window", listing.count())
	}
	now = now.Add(6 * time.Minute)
	s.Choices(context.Background())
	if listing.count() != 2 {
		t.Fatalf("listing ran %d times after the cache window", listing.count())
	}
}

func TestSupportChoicesMarkAMissingBinaryUnavailable(t *testing.T) {
	stubLookPath(t, "pi-x")
	listing := stubListing(t, piListing, nil)
	now := time.Now()
	ans := realChoices(t, &now).Choices(context.Background())
	pi := harnessByName(t, ans, BackendPi)
	if pi.Available || pi.Error == "" || len(pi.Models) != 0 {
		t.Fatalf("pi with no binary = %+v, want unavailable", pi)
	}
	if listing.count() != 0 {
		t.Fatal("an unavailable harness must not be listed")
	}
	if claude := harnessByName(t, ans, BackendClaude); !claude.Available || len(claude.Models) != 4 {
		t.Fatalf("claude = %+v", claude)
	}
}

func TestSupportChoicesListingErrorIsPerHarness(t *testing.T) {
	stubLookPath(t)
	listing := stubListing(t, "", errors.New("exit status 1"))
	now := time.Now()
	s := realChoices(t, &now)
	ans := s.Choices(context.Background())
	pi := harnessByName(t, ans, BackendPi)
	if !pi.Available || !strings.Contains(pi.Error, "exit status 1") || len(pi.Models) != 0 || len(pi.Efforts) != 7 {
		t.Fatalf("pi with a failed listing = %+v", pi)
	}
	if claude := harnessByName(t, ans, BackendClaude); claude.Error != "" || len(claude.Models) != 4 {
		t.Fatalf("claude = %+v", claude)
	}
	// A failure is not cached.
	listing.mu.Lock()
	listing.err, listing.out = nil, piListing
	listing.mu.Unlock()
	if pi := harnessByName(t, s.Choices(context.Background()), BackendPi); pi.Error != "" || len(pi.Models) != 3 {
		t.Fatalf("pi after the listing recovered = %+v", pi)
	}
}

func TestParsePiModelsRefusesAnUnknownTable(t *testing.T) {
	if _, err := parsePiModels([]byte("No models available.\n")); err == nil {
		t.Fatal("output with no model table must be an error")
	}
	if models, err := parsePiModels(nil); err != nil || len(models) != 0 {
		t.Fatalf("empty output = %v, %v", models, err)
	}
}

func TestModelPatternAcceptsTilde(t *testing.T) {
	for _, ok := range []string{"openrouter/~anthropic/claude-fable-latest", "opus", "anthropic/claude-x:high", "m[1m]"} {
		if !modelPattern.MatchString(ok) {
			t.Errorf("modelPattern refuses %q", ok)
		}
	}
	for _, bad := range []string{"", "a b", "$(x)", "a;b"} {
		if modelPattern.MatchString(bad) {
			t.Errorf("modelPattern accepts %q", bad)
		}
	}
}

// supportDaemon serves a supportHarness's Support and runner from a real
// dashboard daemon.
func supportDaemon(t *testing.T) (*supportHarness, *daemonHarness) {
	t.Helper()
	sh := newSupportHarness(t)
	sh.runner.Close()
	sh.runner = NewRunner(sh.config)
	sh.support = NewSupport(SupportOptions{Runner: sh.runner, Dir: sh.dir, StoreRoot: sh.storeRoot})
	d, err := wsdashboard.StartDaemon(wsdashboard.DaemonOptions{Dir: sh.dir, Port: 0, Sessions: leadSessions{}, Agents: sh.runner, Support: sh.support, PollInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	return sh, &daemonHarness{harness: sh.harness, base: fmt.Sprintf("http://127.0.0.1:%d", d.Endpoint().Port)}
}

func TestSupportRoutesEndToEnd(t *testing.T) {
	sh, d := supportDaemon(t)

	r := d.do(http.MethodGet, "/support/choices", "", "", nil)
	var choices ChoicesAnswer
	if r.status != http.StatusOK || json.Unmarshal([]byte(r.body), &choices) != nil || len(choices.Harnesses) != 2 {
		t.Fatalf("choices: %d %s", r.status, r.body)
	}
	r = d.do(http.MethodGet, "/support/stores/alpha", "", "", nil)
	if r.status != http.StatusOK || !strings.Contains(r.body, `"conversation":null`) || !strings.Contains(r.body, `"store":"alpha"`) {
		t.Fatalf("store before any send: %d %s", r.status, r.body)
	}
	r = d.post("/support/stores/alpha/send", map[string]string{"text": "hello"})
	if r.status != http.StatusOK {
		t.Fatalf("send: %d %s", r.status, r.body)
	}
	id := decodeState(t, r).ID
	if strings.Contains(r.body, "fake-agent-key") {
		t.Fatalf("a support route revealed the ws session key: %s", r.body)
	}
	if got := sh.claude.proc(0).nextSent(t); got != "hello" {
		t.Fatalf("turn = %q", got)
	}
	var store StoreAnswer
	r = d.do(http.MethodGet, "/support/stores/alpha", "", "", nil)
	if json.Unmarshal([]byte(r.body), &store) != nil || store.Conversation == nil || store.Conversation.ID != id {
		t.Fatalf("store after send: %s", r.body)
	}
	r = d.post("/support/stores/alpha/select", map[string]string{"harness": BackendPi})
	var sel SelectAnswer
	if r.status != http.StatusOK || json.Unmarshal([]byte(r.body), &sel) != nil || sel.Conversation == nil || sel.Conversation.Backend != BackendPi {
		t.Fatalf("select: %d %s", r.status, r.body)
	}
	r = d.post("/support/stores/alpha/new", nil)
	if r.status != http.StatusOK || decodeState(t, r).ID == sel.Conversation.ID {
		t.Fatalf("new: %d %s", r.status, r.body)
	}
	if r := d.post("/support/stores/alpha/select", map[string]string{"harness": "codex"}); r.status != http.StatusBadRequest {
		t.Fatalf("unknown harness: %d %s", r.status, r.body)
	}

	// An unknown store is 404 on every route.
	if r := d.do(http.MethodGet, "/support/stores/nope", "", "", nil); r.status != http.StatusNotFound {
		t.Fatalf("unknown store GET: %d %s", r.status, r.body)
	}
	for _, action := range []string{"send", "new", "select"} {
		if r := d.post("/support/stores/nope/"+action, map[string]string{"text": "x"}); r.status != http.StatusNotFound {
			t.Fatalf("unknown store %s: %d %s", action, r.status, r.body)
		}
	}
}

func TestSupportWritesRefuseOtherOrigins(t *testing.T) {
	_, d := supportDaemon(t)
	body := map[string]string{"text": "hello"}
	if r := d.do(http.MethodPost, "/support/stores/alpha/send", "http://evil.example", "application/json", body); r.status != http.StatusForbidden {
		t.Fatalf("foreign origin: %d %s", r.status, r.body)
	}
	if r := d.do(http.MethodPost, "/support/stores/alpha/send", "", "text/plain", body); r.status != http.StatusForbidden {
		t.Fatalf("non-JSON content type: %d %s", r.status, r.body)
	}
	if r := d.do(http.MethodPost, "/support/stores/alpha/select", "", "text/plain", map[string]string{"harness": BackendPi}); r.status != http.StatusForbidden {
		t.Fatalf("non-JSON select: %d %s", r.status, r.body)
	}
	if r := d.do(http.MethodPost, "/support/stores/alpha/send", d.base, "application/json", body); r.status != http.StatusOK {
		t.Fatalf("own origin: %d %s", r.status, r.body)
	}
}
