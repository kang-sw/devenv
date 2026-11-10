package wsdashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/websocket"

	"github.com/kang-sw/devenv/internal/wsholder"
)

// termRequest runs one request through the daemon's handler from peer,
// addressed to host, with the extra headers.
func termRequest(d *Daemon, peer, host, method, target, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.RemoteAddr = peer
	req.Host = host
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	ctx, cancel := context.WithCancel(req.Context())
	cancel()
	rec := httptest.NewRecorder()
	d.Handler().ServeHTTP(rec, req.WithContext(ctx))
	return rec
}

// terminalRoutes is a request for each terminal route; none of them starts
// a shell (the folder is refused, the terminal unknown).
var terminalRoutes = []struct{ method, path, body string }{
	{http.MethodGet, "/s/livesession/terminals", ""},
	{http.MethodPost, "/s/livesession/terminals", `{"folder":"path","path":"not/absolute"}`},
	{http.MethodPost, "/s/livesession/terminals/hguardtest/close", ""},
	{http.MethodGet, "/s/livesession/terminals/hguardtest/ws", ""},
}

// TestTerminalRoutesAnswerOnlyTheDaemonsOwnLocalPage pins the terminal
// routes' gate, the system-open route's: a browser on this machine that
// sends the daemon's own Origin reaches each route, a LAN viewer holding
// the token gets 404 as if the routes did not exist, and a request from
// this machine that names another origin gets 403, the WebSocket handshake
// included.
func TestTerminalRoutesAnswerOnlyTheDaemonsOwnLocalPage(t *testing.T) {
	manager := wsholder.New(wsholder.Options{Dir: filepath.Join(t.TempDir(), wsholder.DirName)})
	f := startPageFixture(t, lanMode, withHolders(manager))
	token := readLANToken(t, f.dir)
	port := strconv.Itoa(f.d.Endpoint().Port)
	local, lan := "127.0.0.1:"+port, "192.168.7.20:"+port
	own := map[string]string{"Origin": "http://" + local}

	reached := map[string]int{
		"GET /s/livesession/terminals":                   http.StatusOK,
		"POST /s/livesession/terminals":                  http.StatusBadRequest,
		"POST /s/livesession/terminals/hguardtest/close": http.StatusNotFound,
		"GET /s/livesession/terminals/hguardtest/ws":     http.StatusUpgradeRequired,
	}
	for _, route := range terminalRoutes {
		name := route.method + " " + route.path
		headers := own
		if route.method == http.MethodGet {
			// A browser sends no Origin with a same-origin GET.
			headers = map[string]string{"Sec-Fetch-Site": "same-origin"}
		}
		rec := termRequest(f.d, "127.0.0.1:50000", local, route.method, route.path, route.body, headers)
		if rec.Code != reached[name] || strings.Contains(rec.Body.String(), "404 page not found") {
			t.Errorf("%s from this machine's own page = %d (%s), want %d from the route", name, rec.Code, strings.TrimSpace(rec.Body.String()), reached[name])
		}
		for _, v := range []struct {
			what, peer, host string
			headers          map[string]string
		}{
			{"a LAN peer with the token", "192.168.7.30:50000", lan, map[string]string{"Origin": "http://" + lan, "Cookie": lanTokenCookie + "=" + token}},
			{"this machine by its LAN address", "127.0.0.1:50000", lan, map[string]string{"Origin": "http://" + lan, "Cookie": lanTokenCookie + "=" + token}},
		} {
			rec := termRequest(f.d, v.peer, v.host, route.method, route.path, route.body, v.headers)
			if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "404 page not found") {
				t.Errorf("%s as %s = %d (%s), want the 404 of a route that is not there", name, v.what, rec.Code, strings.TrimSpace(rec.Body.String()))
			}
		}
		for what, headers := range map[string]map[string]string{
			"another origin":     {"Origin": "http://evil.example"},
			"a cross-site fetch": {"Sec-Fetch-Site": "cross-site"},
			"an opaque origin":   {"Origin": "null"},
		} {
			if route.method == http.MethodPost && headers["Origin"] == "" {
				continue // a POST without the daemon's Origin is the next case
			}
			if rec := termRequest(f.d, "127.0.0.1:50000", local, route.method, route.path, route.body, headers); rec.Code != http.StatusForbidden {
				t.Errorf("%s from %s = %d (%s), want 403", name, what, rec.Code, strings.TrimSpace(rec.Body.String()))
			}
		}
		if route.method == http.MethodPost {
			if rec := termRequest(f.d, "127.0.0.1:50000", local, route.method, route.path, route.body, nil); rec.Code != http.StatusForbidden {
				t.Errorf("%s without an Origin = %d, want 403", name, rec.Code)
			}
		}
	}
	if rec := termRequest(f.d, "127.0.0.1:50000", local, http.MethodGet, "/s/nosuchsession/terminals", "", nil); rec.Code != http.StatusNotFound {
		t.Errorf("the list of an unknown session = %d, want 404", rec.Code)
	}

	// The handshake itself: a real upgrade request.
	const ws = "/s/livesession/terminals/hguardtest/ws"
	for _, c := range []struct {
		what    string
		host    string
		headers map[string]string
		want    int
	}{
		{"the daemon's own Origin, an unknown terminal", local, own, http.StatusNotFound},
		{"another origin", local, map[string]string{"Origin": "http://evil.example"}, http.StatusForbidden},
		{"no Origin", local, nil, http.StatusForbidden},
		{"the LAN address with the token", lan, map[string]string{"Origin": "http://" + lan, "Cookie": lanTokenCookie + "=" + token}, http.StatusNotFound},
	} {
		if got := handshake(t, f.base, ws, c.host, c.headers); got != c.want {
			t.Errorf("WebSocket handshake with %s = %d, want %d", c.what, got, c.want)
		}
	}
}

// TestTerminalsAreOfferedToALocalViewerOnly pins what the page carries: a
// browser on this machine gets the emulator's name and stylesheet when the
// daemon can host terminals, a LAN viewer and a daemon without a holder
// manager get neither, and the emulator script is served under its
// content-hash name with its licenses embedded beside it.
func TestTerminalsAreOfferedToALocalViewerOnly(t *testing.T) {
	name, data := XtermAsset()
	if !strings.HasPrefix(name, "xterm-") || !strings.HasSuffix(name, ".js") || !bytes.Contains(data, []byte("Terminal")) || !bytes.Contains(data, []byte("FitAddon")) {
		t.Fatalf("XtermAsset = %q (%d bytes), want the emulator and its fit addon under a hash name", name, len(data))
	}
	for _, file := range []string{"assets/xterm/LICENSE", "assets/xterm/LICENSE.addon-fit"} {
		if raw, err := xtermLicense.ReadFile(file); err != nil || !strings.Contains(string(raw), "The xterm.js authors") || !strings.Contains(string(raw), "Permission is hereby granted, free of charge") {
			t.Errorf("%s is not the emulator's MIT license: %v", file, err)
		}
	}
	manager := wsholder.New(wsholder.Options{Dir: filepath.Join(t.TempDir(), wsholder.DirName)})
	f := startPageFixture(t, lanMode, withHolders(manager))
	port := strconv.Itoa(f.d.Endpoint().Port)
	local := viewer{peer: "127.0.0.1:50000", host: "127.0.0.1:" + port}
	lan := viewer{peer: "192.168.7.30:50000", host: "192.168.7.20:" + port, cookie: readLANToken(t, f.dir)}
	attr := ` data-term="` + name + `"`
	const style = ".xterm .xterm-viewport"
	if page := serve(f.d, local, http.MethodGet, "/s/livesession", "").Body.String(); !strings.Contains(page, attr) || !strings.Contains(page, style) {
		t.Errorf("a local viewer's page lacks the terminal offer (%s) or the emulator's stylesheet", attr)
	}
	if page := serve(f.d, lan, http.MethodGet, "/s/livesession", "").Body.String(); strings.Contains(page, "data-term=") || strings.Contains(page, style) || !strings.Contains(page, `data-rid="livesession"`) {
		t.Error("a LAN viewer's page offers terminals")
	}
	rec := serve(f.d, local, http.MethodGet, "/s/"+name, "")
	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), data) || !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/javascript") {
		t.Errorf("GET /s/%s = %d (%d bytes, %q), want the script, cached for good", name, rec.Code, rec.Body.Len(), rec.Header().Get("Cache-Control"))
	}

	plain := startPageFixture(t, withHolders(newFakeHolders()))
	plainLocal := viewer{peer: "127.0.0.1:50000", host: "127.0.0.1:" + strconv.Itoa(plain.d.Endpoint().Port)}
	if page := serve(plain.d, plainLocal, http.MethodGet, "/s/livesession", "").Body.String(); strings.Contains(page, "data-term=") {
		t.Error("a daemon whose holder host cannot run terminals offers them")
	}
	if rec := serve(plain.d, plainLocal, http.MethodGet, "/s/livesession/terminals", ""); rec.Code != http.StatusNotFound {
		t.Errorf("the terminal list without a terminal host = %d, want 404", rec.Code)
	}
}

// TestPickShell pins the shell rule: $SHELL then /bin/sh on POSIX; pwsh,
// powershell, %COMSPEC%, then cmd.exe on Windows; and a TERM that names no
// capable terminal becomes xterm-256color while a capable one is kept.
func TestPickShell(t *testing.T) {
	runnable := func(names ...string) func(string) (string, bool) {
		return func(name string) (string, bool) {
			for _, n := range names {
				if n == name {
					return `C:\bin\` + name, true
				}
			}
			return "", false
		}
	}
	posix := func(name string) (string, bool) { return name, name == "/bin/zsh" }
	for _, c := range []struct {
		what    string
		goos    string
		environ []string
		look    func(string) (string, bool)
		shell   string
		term    string
	}{
		{"POSIX with a runnable $SHELL", "linux", []string{"SHELL=/bin/zsh", "TERM=xterm-kitty"}, posix, "/bin/zsh", "xterm-kitty"},
		{"POSIX without $SHELL", "linux", []string{"TERM=dumb"}, posix, "/bin/sh", terminalTerm},
		{"POSIX with a $SHELL that cannot be run", "darwin", []string{"SHELL=/opt/gone/fish", "TERM="}, posix, "/bin/sh", terminalTerm},
		{"POSIX without TERM", "linux", []string{"SHELL=/bin/zsh"}, posix, "/bin/zsh", terminalTerm},
		{"Windows with pwsh", "windows", []string{"ComSpec=C:\\Windows\\cmd.exe"}, runnable("pwsh.exe", "powershell.exe"), `C:\bin\pwsh.exe`, terminalTerm},
		{"Windows with powershell only", "windows", []string{"ComSpec=C:\\Windows\\cmd.exe"}, runnable("powershell.exe"), `C:\bin\powershell.exe`, terminalTerm},
		{"Windows with neither", "windows", []string{"ComSpec=C:\\Windows\\cmd.exe", "Term=dumb"}, runnable(), `C:\Windows\cmd.exe`, terminalTerm},
		{"Windows with nothing", "windows", []string{"=C:=C:\\work"}, runnable(), "cmd.exe", terminalTerm},
	} {
		got := pickShell(c.goos, c.environ, c.look)
		if len(got.Argv) != 1 || got.Argv[0] != c.shell {
			t.Errorf("%s: shell = %v, want %s", c.what, got.Argv, c.shell)
		}
		if got.Env["TERM"] != c.term {
			t.Errorf("%s: TERM = %q, want %q", c.what, got.Env["TERM"], c.term)
		}
		for k := range got.Env {
			if k == "" || (k != "TERM" && strings.EqualFold(k, "TERM")) {
				t.Errorf("%s: the environment holds the name %q", c.what, k)
			}
		}
	}
}

// TestTerminalFolder pins the folder a create request resolves to: the
// session's own root by default, the main root of a linked worktree's
// repository for "root", a typed absolute folder for "path", and a refusal
// code for a path that is relative, missing, or a file.
func TestTerminalFolder(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	main, linked, file := filepath.Join(base, "main"), filepath.Join(base, "linked"), filepath.Join(base, "file.txt")
	gitdir := filepath.Join(main, ".git", "worktrees", "linked")
	for _, dir := range []string{gitdir, linked} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for name, body := range map[string]string{filepath.Join(linked, ".git"): "gitdir: " + gitdir + "\n", file: "x"} {
		if err := os.WriteFile(name, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	inLinked := Session{Registration: Registration{ReplyID: "s", Project: linked}}
	inMain := Session{Registration: Registration{ReplyID: "s", Project: main}}
	for _, c := range []struct {
		what    string
		s       Session
		req     terminalCreate
		dir     string
		refusal string
	}{
		{"the default", inLinked, terminalCreate{}, linked, ""},
		{"the worktree", inLinked, terminalCreate{Folder: "worktree"}, linked, ""},
		{"the repository root of a linked worktree", inLinked, terminalCreate{Folder: "root"}, main, ""},
		{"the repository root of a main checkout", inMain, terminalCreate{Folder: "root"}, main, ""},
		{"a typed folder", inLinked, terminalCreate{Folder: "path", Path: "  " + main + string(filepath.Separator) + " "}, main, ""},
		{"a relative path", inLinked, terminalCreate{Folder: "path", Path: "main"}, "", terminalFolderRelative},
		{"an empty path", inLinked, terminalCreate{Folder: "path"}, "", terminalFolderRelative},
		{"a missing folder", inLinked, terminalCreate{Folder: "path", Path: filepath.Join(base, "gone")}, "", terminalFolderMissing},
		{"a file", inLinked, terminalCreate{Folder: "path", Path: file}, "", terminalFolderNotDir},
		{"a session whose root is gone", Session{Registration: Registration{Project: filepath.Join(base, "gone")}}, terminalCreate{}, "", terminalFolderMissing},
		{"an unknown choice", inLinked, terminalCreate{Folder: "elsewhere"}, "", terminalFolderRelative},
	} {
		if dir, refusal := terminalFolder(c.s, c.req); dir != c.dir || refusal != c.refusal {
			t.Errorf("%s: terminalFolder = %q, %q; want %q, %q", c.what, dir, refusal, c.dir, c.refusal)
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		if dir, refusal := terminalFolder(inLinked, terminalCreate{Folder: "path", Path: "~"}); refusal != "" || dir != filepath.Clean(home) {
			t.Errorf("~ resolves to %q (%q), want the home folder %q", dir, refusal, home)
		}
	}
}

// --- terminals on real holders -------------------------------------------------

// termFixture is a daemon whose terminals run /bin/sh on real holders in
// holdersDir. The test is skipped where no holder can run one.
type termFixture struct {
	*pageFixture
	manager *wsholder.Manager
}

func termTestShell() terminalShell {
	return terminalShell{Argv: []string{"/bin/sh"}, Env: map[string]string{"PATH": os.Getenv("PATH"), "TERM": terminalTerm, "PS1": "$ ", "HOME": os.TempDir()}}
}

func termHoldersDir(t *testing.T) (dir, python string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the test shell is /bin/sh")
	}
	python = holderPython(t)
	dir = filepath.Join(t.TempDir(), wsholder.DirName)
	// Whatever the test leaves running is stopped by a last manager.
	t.Cleanup(func() {
		last := wsholder.New(wsholder.Options{Dir: dir, Python: python})
		defer last.Close()
		last.Scan()
		for _, h := range last.List().Holders {
			_ = last.Stop(h.ID)
		}
	})
	return dir, python
}

func startTermFixture(t *testing.T, holdersDir, python string, ringBytes int) *termFixture {
	t.Helper()
	manager := wsholder.New(wsholder.Options{Dir: holdersDir, Python: python, RingBytes: ringBytes})
	f := startPageFixture(t, withHolders(manager), func(o *DaemonOptions) { o.terminalShell = termTestShell })
	return &termFixture{pageFixture: f, manager: manager}
}

var termHTTP = &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 60 * time.Second}

// do sends one request to a terminal route as the daemon's own page.
func (f *termFixture) do(t *testing.T, method, path, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, f.base+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if method == http.MethodPost {
		req.Header.Set("Origin", f.base)
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := termHTTP.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

func (f *termFixture) create(t *testing.T, body string) TerminalInfo {
	t.Helper()
	code, answer := f.do(t, http.MethodPost, "/s/livesession/terminals", body)
	var info TerminalInfo
	if code != http.StatusCreated || json.Unmarshal([]byte(answer), &info) != nil || !wsholder.ValidID(info.ID) {
		t.Fatalf("create terminal = %d (%s), want 201 with the terminal", code, strings.TrimSpace(answer))
	}
	return info
}

func (f *termFixture) list(t *testing.T, replyID string) terminalList {
	t.Helper()
	code, answer := f.do(t, http.MethodGet, "/s/"+replyID+"/terminals", "")
	var list terminalList
	if code != http.StatusOK || json.Unmarshal([]byte(answer), &list) != nil {
		t.Fatalf("list terminals = %d (%s)", code, strings.TrimSpace(answer))
	}
	return list
}

// termClient is one page's WebSocket to a terminal: everything it was sent.
type termClient struct {
	conn *websocket.Conn

	mu     sync.Mutex
	hello  *termFrame
	seqs   []uint64
	recs   [][]byte // the data of each record in seqs
	gaps   []termFrame
	out    bytes.Buffer
	exit   *termFrame
	closed bool
	ended  bool
}

// open connects to the terminal id from sequence from, as the page does.
func (f *termFixture) open(t *testing.T, id string, from uint64) *termClient {
	t.Helper()
	url := "ws" + strings.TrimPrefix(f.base, "http") + "/s/livesession/terminals/" + id + "/ws?from=" + strconv.FormatUint(from, 10)
	cfg, err := websocket.NewConfig(url, f.base)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := websocket.DialConfig(cfg)
	if err != nil {
		t.Fatalf("terminal websocket: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	c := &termClient{conn: conn}
	go func() {
		for {
			var raw string
			if err := websocket.Message.Receive(conn, &raw); err != nil {
				c.mu.Lock()
				c.ended = true
				c.mu.Unlock()
				return
			}
			var frame termFrame
			if json.Unmarshal([]byte(raw), &frame) != nil {
				continue
			}
			c.mu.Lock()
			switch frame.T {
			case "hello":
				c.hello = &frame
			case "out":
				c.seqs, c.recs = append(c.seqs, frame.Seq), append(c.recs, frame.Data)
				c.out.Write(frame.Data)
			case "exit":
				c.seqs, c.recs = append(c.seqs, frame.Seq), append(c.recs, nil)
				c.exit = &frame
			case "gap":
				c.gaps = append(c.gaps, frame)
			case "closed":
				c.closed = true
			case "ping":
				_ = websocket.Message.Send(conn, `{"t":"pong"}`)
			}
			c.mu.Unlock()
		}
	}()
	return c
}

func (c *termClient) send(t *testing.T, frame string) {
	t.Helper()
	if err := websocket.Message.Send(c.conn, frame); err != nil {
		t.Fatalf("send %s: %v", frame, err)
	}
}

// typeLine types a shell command line.
func (c *termClient) typeLine(t *testing.T, line string) {
	t.Helper()
	raw, _ := json.Marshal(map[string]string{"t": "in", "data": line + "\n"})
	c.send(t, string(raw))
}

func (c *termClient) text() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.out.String()
}

func (c *termClient) sequences() []uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]uint64(nil), c.seqs...)
}

func (c *termClient) last() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.seqs) == 0 {
		return 0
	}
	return c.seqs[len(c.seqs)-1]
}

// termWait waits for cond, longer than waitFor: a holder is a process to
// start.
func termWait(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// waitText waits until the terminal has printed want.
func (c *termClient) waitText(t *testing.T, want string) {
	t.Helper()
	termWait(t, "the terminal to print "+want, func() bool { return strings.Contains(c.text(), want) })
}

// assertContiguous fails unless the client took the records from first on,
// each once and in order.
func (c *termClient) assertContiguous(t *testing.T, who string, first uint64) {
	t.Helper()
	seqs := c.sequences()
	if len(seqs) == 0 || seqs[0] != first {
		t.Errorf("%s: sequences begin %v, want them to begin at %d", who, head(seqs, 5), first)
		return
	}
	for i, s := range seqs {
		if s != first+uint64(i) {
			t.Errorf("%s: sequence %d at position %d, want %d: a record was repeated or skipped (%v...)", who, s, i, first+uint64(i), head(seqs, i+2))
			return
		}
	}
}

func head(seqs []uint64, n int) []uint64 {
	if len(seqs) > n {
		return seqs[:n]
	}
	return seqs
}

// TestTerminalRoundTripsInputOutputResizeAndExit runs a shell through the
// routes: create starts it in the asked folder at the asked size, the
// WebSocket carries keystrokes in and output out in sequence, a size frame
// reaches the PTY, the shell's exit arrives with its status, the list
// reports all of it, and close ends the holder and tells the viewer.
func TestTerminalRoundTripsInputOutputResizeAndExit(t *testing.T) {
	holdersDir, python := termHoldersDir(t)
	f := startTermFixture(t, holdersDir, python, 0)
	work, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if code, answer := f.do(t, http.MethodPost, "/s/livesession/terminals", `{"folder":"path","path":"`+filepath.Join(work, "gone")+`"}`); code != http.StatusBadRequest || strings.TrimSpace(answer) != terminalFolderMissing {
		t.Fatalf("create in a missing folder = %d (%s), want 400 %s", code, strings.TrimSpace(answer), terminalFolderMissing)
	}
	if list := f.list(t, "livesession"); len(list.Terminals) != 0 || list.Folders.Worktree != f.project {
		t.Fatalf("list before any terminal = %+v, want none and the session's root %s", list, f.project)
	}

	body, _ := json.Marshal(terminalCreate{Folder: "path", Path: work, Rows: 20, Cols: 90})
	info := f.create(t, string(body))
	if info.Session != "livesession" || info.Cwd != work || info.Shell != "/bin/sh" || info.State != "running" || info.StartedAt == 0 {
		t.Fatalf("created terminal = %+v", info)
	}
	if list := f.list(t, "livesession"); len(list.Terminals) != 1 || list.Terminals[0].ID != info.ID || len(list.Orphans) != 0 {
		t.Fatalf("list = %+v, want the one terminal", list)
	}
	if list := f.list(t, "boardsession"); len(list.Terminals) != 0 || len(list.Orphans) != 0 {
		t.Errorf("another live session's list = %+v, want nothing: the terminal belongs to the page that opened it", list)
	}
	var session terminalSession
	for _, h := range f.manager.List().Holders {
		if h.ID == info.ID {
			_ = json.Unmarshal(h.Session, &session)
		}
	}
	if session.Kind != terminalKind || session.ReplyID != "livesession" || session.Cwd != work {
		t.Errorf("the holder's session description = %+v, want the terminal marker, the opening session, and the folder", session)
	}

	c := f.open(t, info.ID, 0)
	c.typeLine(t, "stty size; pwd; echo ready-$((1+1))")
	c.waitText(t, "ready-2")
	if out := c.text(); !strings.Contains(out, "20 90") || !strings.Contains(out, work) {
		t.Errorf("output = %q, want the PTY's 20 by 90 and the folder %s", out, work)
	}
	c.mu.Lock()
	hello := c.hello
	c.mu.Unlock()
	if hello == nil || hello.State != "running" || hello.Rows != 20 || hello.Cols != 90 {
		t.Errorf("hello = %+v, want a running shell at 20 by 90", hello)
	}

	c.send(t, `{"t":"size","rows":31,"cols":101}`)
	c.typeLine(t, "stty size; echo again-$((2+2))")
	c.waitText(t, "again-4")
	if !strings.Contains(c.text(), "31 101") {
		t.Errorf("output after the size frame = %q, want 31 by 101", c.text())
	}

	c.typeLine(t, "exit 3")
	termWait(t, "the exit frame", func() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.exit != nil })
	c.mu.Lock()
	exit := c.exit
	c.mu.Unlock()
	if exit.ExitCode == nil || *exit.ExitCode != 3 {
		t.Errorf("exit frame = %+v, want exit code 3", exit)
	}
	c.assertContiguous(t, "the viewer", 1)
	if list := f.list(t, "livesession"); len(list.Terminals) != 1 || list.Terminals[0].State != "exited" || list.Terminals[0].ExitCode == nil || *list.Terminals[0].ExitCode != 3 {
		t.Errorf("list after the exit = %+v, want the terminal exited with 3", list.Terminals)
	}
	late := f.open(t, info.ID, 0)
	termWait(t, "a late viewer to learn of the exit", func() bool {
		late.mu.Lock()
		defer late.mu.Unlock()
		return late.hello != nil && late.hello.State == "exited" && late.exit != nil
	})

	if code, answer := f.do(t, http.MethodPost, "/s/livesession/terminals/"+info.ID+"/close", ""); code != http.StatusNoContent {
		t.Fatalf("close = %d (%s), want 204", code, strings.TrimSpace(answer))
	}
	termWait(t, "the viewer to be told the terminal closed", func() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.closed && c.ended })
	if list := f.list(t, "livesession"); len(list.Terminals) != 0 {
		t.Errorf("list after close = %+v, want none", list.Terminals)
	}
	if _, err := os.Stat(filepath.Join(holdersDir, info.ID)); !os.IsNotExist(err) {
		t.Errorf("the terminal's holder directory still exists after close: %v", err)
	}
	if code, _ := f.do(t, http.MethodPost, "/s/livesession/terminals/"+info.ID+"/close", ""); code != http.StatusNotFound {
		t.Errorf("closing a closed terminal = %d, want 404", code)
	}
}

// TestTerminalLateJoinerReplaysFromItsOwnSequence pins the fan-out: a
// viewer that joins later takes the holder's replay from the sequence it
// asked for (everything, or the one after its last), and a viewer already
// there takes no record twice while the stream replays for the other.
func TestTerminalLateJoinerReplaysFromItsOwnSequence(t *testing.T) {
	holdersDir, python := termHoldersDir(t)
	f := startTermFixture(t, holdersDir, python, 0)
	info := f.create(t, `{}`)
	if info.Cwd != f.project {
		t.Errorf("a terminal opened without a folder runs in %s, want the session's root %s", info.Cwd, f.project)
	}

	a := f.open(t, info.ID, 0)
	a.typeLine(t, "echo one-$((1+0))")
	a.waitText(t, "one-1")

	b := f.open(t, info.ID, 0)
	b.waitText(t, "one-1")
	a.typeLine(t, "echo two-$((1+1))")
	a.waitText(t, "two-2")
	b.waitText(t, "two-2")

	// A page that kept everything but the last two records connects again.
	from := a.last() - 1
	c := f.open(t, info.ID, from)
	termWait(t, "the reconnecting viewer's backfill", func() bool { return c.last() >= from+1 })
	a.typeLine(t, "echo three-$((1+2))")
	for _, v := range []*termClient{a, b, c} {
		v.waitText(t, "three-3")
	}
	termWait(t, "every viewer to reach the same record", func() bool { return a.last() == b.last() && b.last() == c.last() })

	a.assertContiguous(t, "the first viewer", 1)
	b.assertContiguous(t, "the late joiner", 1)
	c.assertContiguous(t, "the reconnecting viewer", from)
	for who, v := range map[string]*termClient{"the first viewer": a, "the late joiner": b, "the reconnecting viewer": c} {
		v.mu.Lock()
		if len(v.gaps) != 0 {
			t.Errorf("%s was told of a gap %+v; the ring held every record", who, v.gaps)
		}
		v.mu.Unlock()
	}
	// The reconnecting viewer took the records from its sequence on, and
	// nothing from before it.
	a.mu.Lock()
	var want bytes.Buffer
	for i, seq := range a.seqs {
		if seq >= from {
			want.Write(a.recs[i])
		}
	}
	a.mu.Unlock()
	if got := c.text(); got != want.String() || got == a.text() {
		t.Errorf("the reconnecting viewer was sent %q, want the records from %d on: %q", got, from, want.String())
	}
}

// TestTerminalSurvivesADaemonRestartAndReportsAGap runs one shell under two
// daemons in turn. The second finds the terminal from its holder's
// descriptor with the session it was opened from, a page that reconnects
// with the sequence after its last is told which records left the holder's
// ring meanwhile and then takes the rest in order, and a page that asks for
// everything starts at the ring's first record without being told of a gap.
func TestTerminalSurvivesADaemonRestartAndReportsAGap(t *testing.T) {
	holdersDir, python := termHoldersDir(t)
	f1 := startTermFixture(t, holdersDir, python, 2048)
	info := f1.create(t, `{}`)
	a := f1.open(t, info.ID, 0)
	a.typeLine(t, "echo one-$((1+0))")
	a.waitText(t, "one-1")
	kept := a.last()
	// Far more output than the ring holds, written while no daemon runs.
	a.typeLine(t, "sleep 1; i=0; while [ $i -lt 600 ]; do echo line-$i; i=$((i+1)); done; echo fin-$((40+2))")
	termWait(t, "the command line's echo", func() bool { return a.last() > kept })
	kept = a.last()
	a.conn.Close()
	f1.d.Close()

	f2 := startTermFixture(t, holdersDir, python, 2048)
	var listed TerminalInfo
	termWait(t, "the second daemon to list the terminal", func() bool {
		list := f2.list(t, "livesession")
		if len(list.Terminals) != 1 {
			return false
		}
		listed = list.Terminals[0]
		return listed.State == "running"
	})
	if listed.ID != info.ID || listed.Cwd != info.Cwd || listed.Session != "livesession" || listed.StartedAt != info.StartedAt {
		t.Fatalf("the second daemon lists %+v, want the terminal %+v the first opened", listed, info)
	}
	watch := f2.open(t, info.ID, 0)
	watch.waitText(t, "fin-42")

	b := f2.open(t, info.ID, kept+1)
	b.waitText(t, "fin-42")
	b.mu.Lock()
	gaps := append([]termFrame(nil), b.gaps...)
	b.mu.Unlock()
	if len(gaps) != 1 || gaps[0].From != kept+1 || gaps[0].To <= kept+1 {
		t.Fatalf("the reconnecting page's gaps = %+v, want one from %d on", gaps, kept+1)
	}
	b.assertContiguous(t, "the reconnecting page", gaps[0].To)
	if strings.Contains(b.text(), "line-0\r\n") {
		t.Errorf("the reconnecting page was sent a record that had left the ring")
	}

	fresh := f2.open(t, info.ID, 0)
	fresh.waitText(t, "fin-42")
	fresh.mu.Lock()
	freshGaps, first := len(fresh.gaps), uint64(0)
	if len(fresh.seqs) > 0 {
		first = fresh.seqs[0]
	}
	fresh.mu.Unlock()
	if freshGaps != 0 || first <= 1 {
		t.Errorf("a page that asked for everything: %d gaps, first sequence %d; want no gap and the ring's first record", freshGaps, first)
	}
	fresh.assertContiguous(t, "the fresh page", first)

	// The shell is the same one: it still answers.
	b.typeLine(t, "echo back-$((3+4))")
	for _, v := range []*termClient{watch, b, fresh} {
		v.waitText(t, "back-7")
	}
}

// TestTerminalTakesTheSizeOfTheViewerThatLastTypedOrFocused pins the size
// rule with two viewers: until one has typed or focused, the last size
// sent is the PTY's; after that only the active viewer's size counts, and
// typing or focusing makes a viewer active and applies the size it sent.
func TestTerminalTakesTheSizeOfTheViewerThatLastTypedOrFocused(t *testing.T) {
	holdersDir, python := termHoldersDir(t)
	f := startTermFixture(t, holdersDir, python, 0)
	info := f.create(t, `{}`)
	a, b := f.open(t, info.ID, 0), f.open(t, info.ID, 0)
	size := func(c *termClient, mark string) string {
		t.Helper()
		c.typeLine(t, "echo "+mark+"=$(stty size)=")
		// The PTY echoes the typed line too; the answer is the one with
		// digits between the marks.
		answer := regexp.MustCompile(mark + `=(\d+ \d+)=`)
		var got string
		termWait(t, "the size after "+mark, func() bool {
			m := answer.FindStringSubmatch(c.text())
			if m == nil {
				return false
			}
			got = m[1]
			return true
		})
		return got
	}
	// Nobody is active yet: each size sent is applied, the last one stays.
	a.send(t, `{"t":"size","rows":20,"cols":90}`)
	b.send(t, `{"t":"size","rows":25,"cols":100}`)
	// Typing makes a the active viewer, with the size it sent.
	if got := size(a, "first"); got != "20 90" {
		t.Errorf("after a typed, the PTY is %q, want a's 20 90", got)
	}
	// b is not active: its new size waits.
	b.send(t, `{"t":"size","rows":26,"cols":101}`)
	if got := size(a, "second"); got != "20 90" {
		t.Errorf("after the inactive viewer resized, the PTY is %q, want a's 20 90 still", got)
	}
	// Focus alone makes b active.
	b.send(t, `{"t":"focus"}`)
	termWait(t, "b's focus to set the PTY's size", func() bool {
		return strings.Contains(f.do2(t, info.ID), `"rows":26`)
	})
	// a's card changes size while b is active: nothing happens until a
	// types again.
	a.send(t, `{"t":"size","rows":22,"cols":92}`)
	if got := size(b, "third"); got != "26 101" {
		t.Errorf("while b is active, the PTY is %q, want b's 26 101", got)
	}
	if got := size(a, "fourth"); got != "22 92" {
		t.Errorf("after a typed again, the PTY is %q, want a's 22 92", got)
	}
}

// do2 is the hello a new viewer of terminal id gets, as JSON: the PTY's
// size as the daemon holds it.
func (f *termFixture) do2(t *testing.T, id string) string {
	t.Helper()
	c := f.open(t, id, ^uint64(0)>>1)
	defer c.conn.Close()
	var raw []byte
	termWait(t, "a hello frame", func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.hello == nil {
			return false
		}
		raw, _ = json.Marshal(c.hello)
		return true
	})
	return string(raw)
}

// slowScanHost is a holder manager whose scans wait to be released.
type slowScanHost struct {
	*wsholder.Manager
	release chan struct{}
}

func (h *slowScanHost) Scan() {
	<-h.release
	h.Manager.Scan()
}

// TestTerminalRoutesWaitForTheFirstScan pins what a page is told while a
// new daemon has not yet found the terminals its predecessor left: 503, not
// a list without them, which would have the page drop its cards.
func TestTerminalRoutesWaitForTheFirstScan(t *testing.T) {
	host := &slowScanHost{Manager: wsholder.New(wsholder.Options{Dir: filepath.Join(t.TempDir(), wsholder.DirName)}), release: make(chan struct{})}
	f := startPageFixture(t, withHolders(host))
	f.d.terminals.scanWait = 50 * time.Millisecond
	local := "127.0.0.1:" + strconv.Itoa(f.d.Endpoint().Port)
	list := func() int {
		return termRequest(f.d, "127.0.0.1:50000", local, http.MethodGet, "/s/livesession/terminals", "", nil).Code
	}
	if code := list(); code != http.StatusServiceUnavailable {
		t.Errorf("the list before the first scan = %d, want 503", code)
	}
	if code := handshake(t, f.base, "/s/livesession/terminals/hguardtest/ws", local, map[string]string{"Origin": "http://" + local}); code != http.StatusServiceUnavailable {
		t.Errorf("the WebSocket handshake before the first scan = %d, want 503", code)
	}
	close(host.release)
	waitFor(t, "the list after the first scan", func() bool { return list() == http.StatusOK })
}

// TestTerminalViewerLearnsOfALostHolder pins what a viewer sees when the
// terminal's holder dies under the daemon: its connection ends on the next
// keystroke without "closed", so the page asks again, and the terminal no
// longer upgrades (503 while the manager still lists the holder, 404 once
// it has forgotten it).
func TestTerminalViewerLearnsOfALostHolder(t *testing.T) {
	holdersDir, python := termHoldersDir(t)
	f := startTermFixture(t, holdersDir, python, 0)
	info := f.create(t, `{}`)
	c := f.open(t, info.ID, 0)
	c.typeLine(t, "echo up-$((1+1))")
	c.waitText(t, "up-2")
	var pid int
	for _, h := range f.manager.List().Holders {
		if h.ID == info.ID {
			pid = h.PID
		}
	}
	proc, err := os.FindProcess(pid)
	if pid <= 0 || err != nil {
		t.Fatalf("the terminal's holder has no process: pid %d, %v", pid, err)
	}
	if err := proc.Kill(); err != nil {
		t.Fatal(err)
	}
	termWait(t, "the manager to lose the holder", func() bool { return f.manager.Client(info.ID) == nil })
	// The page may have a keystroke on its way; the write fails or is
	// refused, and either ends the connection.
	_ = websocket.Message.Send(c.conn, `{"t":"in","data":"x"}`)
	termWait(t, "the viewer's connection to end", func() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.ended })
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	local := "127.0.0.1:" + strconv.Itoa(f.d.Endpoint().Port)
	code := handshake(t, f.base, "/s/livesession/terminals/"+info.ID+"/ws", local, map[string]string{"Origin": "http://" + local})
	if code != http.StatusServiceUnavailable && code != http.StatusNotFound {
		t.Errorf("the handshake for a lost terminal = %d, want 503 or 404", code)
	}
	if closed && code != http.StatusNotFound {
		t.Errorf("the viewer was told the terminal closed while the daemon still lists it (handshake %d)", code)
	}
}
