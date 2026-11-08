package wsdashboard

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsi18n"
	"github.com/kang-sw/devenv/internal/wsinbox"
)

// fakeWorktrees is a WorktreeSource that answers one fixed pool.
type fakeWorktrees struct {
	mu   sync.Mutex
	pool WorktreePool
	err  error
}

func (f *fakeWorktrees) ReadWorktrees(Registration) (WorktreePool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	pool := f.pool
	pool.Worktrees = append([]PooledWorktree(nil), f.pool.Worktrees...)
	return pool, f.err
}

// worktreeFixture is a page fixture whose daemon reads pools from a fake
// over two real directories: a leased worktree of 1500 bytes and an idle
// one of 250.
func worktreeFixture(t *testing.T) (*pageFixture, *fakeWorktrees, PooledWorktree, PooledWorktree) {
	t.Helper()
	poolDir := t.TempDir()
	mk := func(name string, sizes ...int) string {
		dir := filepath.Join(poolDir, name)
		if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
			t.Fatal(err)
		}
		for i, n := range sizes {
			if err := os.WriteFile(filepath.Join(dir, "sub", string(rune('a'+i))), make([]byte, n), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}
	acquired := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC).UnixMilli()
	leased := PooledWorktree{Name: "busy", Path: mk("busy", 1000, 500), Profile: "full", Branch: "impl/x", Leased: true, Holder: "workerrid", Parent: "livesession", AcquiredAt: acquired, ActiveAt: acquired}
	idle := PooledWorktree{Name: "free", Path: mk("free", 250), Profile: "full", Detached: true, ActiveAt: acquired - 3600_000}
	src := &fakeWorktrees{pool: WorktreePool{Pool: poolDir, Worktrees: []PooledWorktree{leased, idle}}}
	f := startPageFixture(t, func(o *DaemonOptions) { o.Worktrees = src })
	return f, src, leased, idle
}

func getPool(t *testing.T, url string) WorktreePool {
	t.Helper()
	code, body := get(t, url)
	if code != http.StatusOK {
		t.Fatalf("GET %s = %d: %s", url, code, body)
	}
	var pool WorktreePool
	if err := json.Unmarshal([]byte(body), &pool); err != nil {
		t.Fatalf("pool answer %q: %v", body, err)
	}
	return pool
}

func TestWorktreesRouteAnswersAtOnceThenWithSizes(t *testing.T) {
	f, _, leased, idle := worktreeFixture(t)
	first := getPool(t, f.base+"/s/livesession/worktrees")
	if !first.Sizing || len(first.Worktrees) != 2 || first.Worktrees[0].Size != nil || first.Worktrees[1].Size != nil {
		t.Fatalf("first read = %+v, want both worktrees, unsized, sizing", first)
	}
	got := getPool(t, f.base+"/s/livesession/worktrees?wait=1")
	if got.Sizing {
		t.Fatalf("a waited read still sizing: %+v", got)
	}
	sizes := map[string]int64{}
	for _, wt := range got.Worktrees {
		if wt.Size == nil {
			t.Fatalf("waited read left %s unsized", wt.Name)
		}
		sizes[wt.Name] = *wt.Size
	}
	if sizes["busy"] != 1500 || sizes["free"] != 250 {
		t.Fatalf("sizes = %v, want busy 1500 and free 250", sizes)
	}
	w0 := got.Worktrees[0]
	w0.Size = nil
	if w0 != leased {
		t.Errorf("leased worktree = %+v, want %+v", w0, leased)
	}
	if got.Worktrees[1].Detached != idle.Detached || got.Worktrees[1].Leased {
		t.Errorf("idle worktree = %+v", got.Worktrees[1])
	}
	// A fresh size answers the next read at once.
	if again := getPool(t, f.base+"/s/livesession/worktrees"); again.Sizing || again.Worktrees[1].Size == nil {
		t.Errorf("a read inside the size TTL = %+v, want cached sizes", again)
	}
}

func TestWorktreesRouteRefusals(t *testing.T) {
	f, src, _, _ := worktreeFixture(t)
	if code, _ := get(t, f.base+"/s/nosuchsession/worktrees"); code != http.StatusNotFound {
		t.Errorf("unknown session = %d, want 404", code)
	}
	src.mu.Lock()
	src.err = errors.New("git exploded")
	src.mu.Unlock()
	if code, body := get(t, f.base+"/s/livesession/worktrees"); code != http.StatusInternalServerError || !strings.Contains(body, "git exploded") {
		t.Errorf("failed read = %d %q, want 500 with the reason", code, body)
	}
	if code, _, _ := postJSON(t, f.base+"/s/livesession/worktrees", f.base, ""); code != http.StatusMethodNotAllowed {
		t.Errorf("POST to the read route = %d, want 405", code)
	}

	bare := startPageFixture(t)
	if code, _ := get(t, bare.base+"/s/livesession/worktrees"); code != http.StatusNotFound {
		t.Errorf("a daemon without a worktree source = %d, want 404", code)
	}
}

func TestWorktreeCleanupHoldsOneMessageWithThePool(t *testing.T) {
	f, _, leased, idle := worktreeFixture(t)
	// Measure first, so the message carries the sizes.
	getPool(t, f.base+"/s/livesession/worktrees?wait=1")

	if code, _, _ := postJSON(t, f.base+"/s/livesession/worktrees/cleanup", "", ""); code != http.StatusForbidden {
		t.Fatalf("cleanup without an Origin = %d, want 403", code)
	}
	if held := f.held(t); len(held) != 0 {
		t.Fatalf("a refused cleanup held %+v", held)
	}
	code, _, raw := postJSON(t, f.base+"/s/livesession/worktrees/cleanup", f.base, "")
	if code != http.StatusOK {
		t.Fatalf("cleanup = %d: %s", code, raw)
	}
	held := f.held(t)
	if len(held) != 1 {
		t.Fatalf("held after cleanup = %+v, want one message", held)
	}
	text := held[0].Entry.Text()
	for _, want := range []string{
		"The user asks to clean up unused worktrees.\n\npool: ",
		"worktree: " + leased.Path + "\n  profile: full\n  branch: impl/x\n  state: leased; holder id:workerrid; parent id:livesession; acquired_at 2026-11-08T12:00:00Z\n",
		"  size: 1.5 kB\n",
		"worktree: " + idle.Path + "\n  profile: full\n  branch: (detached)\n  state: idle\n  last_activity: 2026-11-08T11:00:00Z\n  size: 250 B",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("cleanup message lacks %q:\n%s", want, text)
		}
	}
}

// TestWorktreePoolTextIsBounded: a pool too large for one message lists
// what fits and ends with how many worktrees it left out, and a cleanup of
// it still sends.
func TestWorktreePoolTextIsBounded(t *testing.T) {
	pool := WorktreePool{Pool: "/p"}
	for i := range 2000 {
		pool.Worktrees = append(pool.Worktrees, PooledWorktree{Path: "/p/" + strings.Repeat("w", 40) + strconv.Itoa(i), Branch: "impl/x"})
	}
	text := pool.text(worktreeTextMax)
	if len(text) > worktreeTextMax+64 {
		t.Fatalf("text is %d bytes, bound %d", len(text), worktreeTextMax)
	}
	listed := strings.Count(text, "\nworktree: ")
	if listed == 0 || listed == len(pool.Worktrees) || !strings.HasSuffix(text, "\n... "+strconv.Itoa(len(pool.Worktrees)-listed)+" more worktrees\n") {
		t.Fatalf("listed %d of %d; text ends %q", listed, len(pool.Worktrees), text[len(text)-60:])
	}
	if small := (WorktreePool{Pool: "/p", Worktrees: pool.Worktrees[:2]}).text(worktreeTextMax); strings.Contains(small, "more worktrees") {
		t.Errorf("a small pool is cut: %q", small)
	}

	f, src, _, _ := worktreeFixture(t)
	src.mu.Lock()
	src.pool = pool
	src.mu.Unlock()
	if code, _, raw := postJSON(t, f.base+"/s/livesession/worktrees/cleanup", f.base, ""); code != http.StatusOK {
		t.Fatalf("cleanup of a large pool = %d: %s", code, raw)
	}
}

func TestWorktreeCleanupFollowsThePageLanguage(t *testing.T) {
	f, _, _, _ := worktreeFixture(t)
	req, err := http.NewRequest(http.MethodPost, f.base+"/s/livesession/worktrees/cleanup", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", f.base)
	req.AddCookie(&http.Cookie{Name: wsi18n.CookieName, Value: "ko"})
	resp, err := testClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cleanup = %d: %s", resp.StatusCode, body)
	}
	held := f.held(t)
	if len(held) != 1 || !strings.HasPrefix(held[0].Entry.Text(), wsi18n.T("ko", "worktree.cleanup.ask")+"\n\n") {
		t.Fatalf("held = %+v, want the Korean request line first", held)
	}
}

func TestWorktreeCleanupRefusesASessionWithoutAnAdapter(t *testing.T) {
	f, _, _, _ := worktreeFixture(t)
	if code, _, raw := postJSON(t, f.base+"/s/boardsession/worktrees/cleanup", f.base, ""); code != http.StatusConflict {
		t.Fatalf("cleanup of a session without an adapter = %d (%s), want 409", code, raw)
	}
	held, _ := wsinbox.Held(f.inbox("boardsession"))
	if len(held) != 0 {
		t.Fatalf("a refused cleanup held %+v", held)
	}
}

func TestByteSize(t *testing.T) {
	for n, want := range map[int64]string{0: "0 B", 999: "999 B", 1000: "1.0 kB", 1_500_000: "1.5 MB", 2_340_000_000: "2.3 GB"} {
		if got := byteSize(n); got != want {
			t.Errorf("byteSize(%d) = %q, want %q", n, got, want)
		}
	}
}

// TestWorktreesCardInTheDrawer pins the drawer's pooled-worktree card: on a
// session page of a daemon that reads pools, after the panels and before
// the settings card, its cleanup button live with an adapter and disabled
// with the reason without one; a daemon without a worktree source has no
// card.
func TestWorktreesCardInTheDrawer(t *testing.T) {
	f, _, _, _ := worktreeFixture(t)
	_, live := get(t, f.base+"/s/livesession")
	panels, card, settings := strings.Index(live, `data-region="panels"`), strings.Index(live, `id="p-worktrees"`), strings.Index(live, `id="p-settings"`)
	if panels < 0 || card < panels || settings < card {
		t.Fatalf("drawer order: panels %d, worktrees %d, settings %d", panels, card, settings)
	}
	for _, want := range []string{`id="wtlist"`, `id="wtcount"`, `<button type="button" class="btn wtclean" id="wtclean" title="ask the session to clean up unused worktrees">Request cleanup</button>`} {
		if !strings.Contains(live, want) {
			t.Errorf("live page lacks %q", want)
		}
	}
	_, board := get(t, f.base+"/s/boardsession")
	if !strings.Contains(board, `id="wtclean" disabled title="this session has no live host adapter to ask">`) {
		t.Errorf("board-first page's cleanup button is not disabled with its reason")
	}

	bare := startPageFixture(t)
	if _, page := get(t, bare.base+"/s/livesession"); strings.Contains(page, `id="p-worktrees"`) || !strings.Contains(page, `id="p-settings"`) {
		t.Errorf("a daemon without a worktree source: worktrees card present or settings card missing")
	}
}
