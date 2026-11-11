package wsdashboard

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/websocket"

	"github.com/kang-sw/devenv/internal/wsholder"
)

// The terminal routes of one session page (terminals.go holds the
// terminals themselves):
//
//   - GET  /s/<reply-id>/terminals answers terminalList: the terminals
//     opened from this session, the live ones whose opening session is gone
//     from the index (orphans, which the drawer offers to attach or stop),
//     and the folders the "+ Terminal" popover offers.
//   - POST /s/<reply-id>/terminals {"folder": "worktree" | "root" | "path",
//     "path": <absolute folder>, "rows": n, "cols": n} opens a terminal in
//     the session's own root (the default), its repository's main root, or
//     the typed folder, and answers 201 with its TerminalInfo. A folder
//     that cannot be used answers 400 with one of the terminalFolder codes
//     as its text, which the popover shows inline.
//   - POST /s/<reply-id>/terminals/<id>/close ends the terminal and its
//     shell, and answers 204; the page asks the user first.
//   - GET  /s/<reply-id>/terminals/<id>/ws?from=<sequence> upgrades to the
//     terminal's WebSocket. Any known terminal answers under any known
//     session, so an orphan attaches on the page the user is viewing.
//
// A terminal is a shell on the host, so every one of them answers only a
// browser on this machine (localViewer; a LAN viewer, token or not, gets
// 404, as if the routes did not exist) that is this daemon's own page: a
// POST and the WebSocket handshake carry the daemon's Origin (sameOrigin),
// and the list, a GET a browser sends no Origin with, is refused when it
// carries a foreign Origin or a Sec-Fetch-Site other than same-origin.
//
// The WebSocket's frames are JSON text, each {"t": <type>, ...}.
//
// Daemon to page:
//
//	{"t":"hello","state":"running"|"exited","exit_code":n,"signal":n,
//	 "next":n,"rows":n,"cols":n}   first frame: the shell's state, one past
//	                               the last sequence written, the PTY size
//	{"t":"out","seq":n,"data":"<base64>"}        one record of PTY output
//	{"t":"exit","seq":n,"exit_code":n,"signal":n} the shell ended
//	{"t":"gap","from":a,"to":b}   the records a..b-1 left the holder's ring
//	                              before this page read them
//	{"t":"closed"}                the terminal is gone; the daemon closes
//	{"t":"ping"}                  idle frame; the page answers {"t":"pong"}
//
// Records arrive in sequence order without holes, except across a gap. The
// page keeps the last sequence it took and connects again with from set to
// the one after it; from=0 (or none) asks for everything the ring holds. A
// connection the daemon drops without "closed" (a viewer too far behind,
// a holder it lost) is one to connect again. While the daemon is not
// attached to the terminal's holder, or has not finished looking for the
// terminals a predecessor left, the routes answer 503.
//
// Page to daemon:
//
//	{"t":"in","data":"<text>"}    keystrokes, as UTF-8 text
//	{"t":"in","bin":"<base64>"}   keystrokes that are not text
//	{"t":"size","rows":n,"cols":n} the size the page's card fits
//	{"t":"focus"}                 the terminal took the keyboard focus
//	{"t":"pong"}
//
// The PTY takes the size of the viewer that last typed or focused; until
// one has, the last size any viewer sent. A size equal to the PTY's is not
// sent to the holder.

const (
	// terminalBodyLimit bounds a terminal POST body.
	terminalBodyLimit = 16 << 10
	// terminalFrameMax bounds one frame the page sends: a paste goes in one.
	terminalFrameMax = 1 << 20
)

// The codes a refused folder answers with.
const (
	terminalFolderRelative = "not_absolute"
	terminalFolderMissing  = "not_found"
	terminalFolderNotDir   = "not_a_folder"
)

//go:embed assets/xterm/xterm.js
var xtermJS []byte

//go:embed assets/xterm/addon-fit.js
var xtermFitJS []byte

//go:embed assets/xterm/xterm.css
var xtermCSS string

// xtermLicense embeds the emulator's licenses beside it, so a build that
// carries the scripts carries them.
//
//go:embed assets/xterm/LICENSE assets/xterm/LICENSE.addon-fit
var xtermLicense embed.FS

var xtermOnce = sync.OnceValues(func() (string, []byte) {
	data := make([]byte, 0, len(xtermJS)+len(xtermFitJS)+1)
	data = append(append(append(data, xtermJS...), '\n'), xtermFitJS...)
	sum := sha256.Sum256(data)
	return "xterm-" + hex.EncodeToString(sum[:6]) + ".js", data
})

// XtermAsset is the vendored terminal emulator (assets/xterm/README.md):
// the script a session page loads from beside itself when its first
// terminal card opens, and its name, which carries a hash of the bytes.
func XtermAsset() (name string, data []byte) { return xtermOnce() }

// serveXterm serves the emulator script; like the mermaid script, its name
// never changes under its content.
func serveXterm(w http.ResponseWriter) {
	_, data := XtermAsset()
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	_, _ = w.Write(data)
}

// termSymbolsWOFF2 is the symbols-only Nerd Font (assets/nerdfont/README.md)
// the terminal card lists after the page's monospace stack, so a prompt's
// powerline separators and icons draw instead of empty boxes.
//
//go:embed assets/nerdfont/SymbolsNerdFontMono-Regular.woff2
var termSymbolsWOFF2 []byte

// termSymbolsLicense embeds the font's license beside it.
//
//go:embed assets/nerdfont/LICENSE
var termSymbolsLicense embed.FS

// termSymbolsFamily is the family the font is declared under. terminal.js
// names it in the emulator's font list: change the two together.
const termSymbolsFamily = "WsNerdSymbols"

var termSymbolsName = sync.OnceValue(func() string {
	sum := sha256.Sum256(termSymbolsWOFF2)
	return "nerd-symbols-" + hex.EncodeToString(sum[:6]) + ".woff2"
})

// TermSymbolsAsset is the terminal's symbols font: the file a page that
// offers terminals loads from beside itself when a terminal first draws,
// and its name, which carries a hash of the bytes.
func TermSymbolsAsset() (name string, data []byte) { return termSymbolsName(), termSymbolsWOFF2 }

// serveTermSymbols serves the symbols font; its name never changes under
// its content.
func serveTermSymbols(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "font/woff2")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	_, _ = w.Write(termSymbolsWOFF2)
}

// xtermStyle is the emulator's stylesheet with the symbols font's face,
// which a page that offers terminals inlines. The browser fetches a face
// only when text first uses its family, so the board never waits for it.
func xtermStyle() template.CSS {
	return template.CSS(xtermCSS + "\n@font-face { font-family: \"" + termSymbolsFamily + "\"; src: url(\"/s/" + termSymbolsName() + "\") format(\"woff2\"); font-display: swap; }\n")
}

// terminalFolders are the folders the "+ Terminal" popover offers for a
// session.
type terminalFolders struct {
	// Worktree is the session's own root.
	Worktree string `json:"worktree"`
	// Root is the main root of the repository Worktree is a linked
	// worktree of; empty when it is not one.
	Root string `json:"root,omitempty"`
}

// terminalList is the list route's answer.
type terminalList struct {
	Terminals []TerminalInfo  `json:"terminals"`
	Orphans   []TerminalInfo  `json:"orphans"`
	Folders   terminalFolders `json:"folders"`
}

// terminalCreate is the create route's body. Unknown fields are ignored.
type terminalCreate struct {
	Folder string `json:"folder"`
	Path   string `json:"path"`
	Rows   int    `json:"rows"`
	Cols   int    `json:"cols"`
}

// offersTerminals reports whether r's viewer is offered terminals: the
// daemon runs them and the browser is on this machine.
func (d *Daemon) offersTerminals(r *http.Request) bool {
	return d.terminals != nil && localViewer(r)
}

// terminalSession runs the terminal routes' shared checks on a request for
// the session replyID. On a refusal it has answered the request and ok is
// false.
func (d *Daemon) terminalSession(w http.ResponseWriter, r *http.Request, replyID string) (s Session, ok bool) {
	if !d.offersTerminals(r) {
		http.NotFound(w, r)
		return Session{}, false
	}
	if (r.Method == http.MethodGet || r.Method == http.MethodHead) && !wsUpgrade(r) {
		// A browser sends no Origin with a same-origin GET.
		site := r.Header.Get("Sec-Fetch-Site")
		if (r.Header.Get("Origin") != "" && !sameOrigin(r)) || (site != "" && site != "same-origin" && site != "none") {
			http.Error(w, "cross-origin request refused", http.StatusForbidden)
			return Session{}, false
		}
	} else if !sameOrigin(r) {
		http.Error(w, "cross-origin request refused", http.StatusForbidden)
		return Session{}, false
	}
	if d.index != nil && ValidReplyID(replyID) {
		s, ok = d.index.Session(replyID)
	}
	if !ok {
		http.Error(w, "unknown session", http.StatusNotFound)
		return Session{}, false
	}
	if !d.terminals.ready() {
		http.Error(w, "the daemon is still finding its terminals", http.StatusServiceUnavailable)
		return Session{}, false
	}
	return s, true
}

// handleTerminals serves the terminal routes of one session; rest is the
// path after "terminals".
func (d *Daemon) handleTerminals(w http.ResponseWriter, r *http.Request, replyID, rest string) {
	s, ok := d.terminalSession(w, r, replyID)
	if !ok {
		return
	}
	rest = strings.TrimPrefix(rest, "/")
	if rest == "" {
		switch r.Method {
		case http.MethodGet, http.MethodHead:
			d.listTerminals(w, s)
		case http.MethodPost:
			d.createTerminal(w, r, s)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}
	id, action, _ := strings.Cut(rest, "/")
	if !wsholder.ValidID(id) || (action != "close" && action != "ws") {
		http.NotFound(w, r)
		return
	}
	if action == "close" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		switch known, err := d.terminals.close(id); {
		case !known:
			http.Error(w, "unknown terminal", http.StatusNotFound)
		case err != nil:
			http.Error(w, err.Error(), http.StatusConflict)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !wsUpgrade(r) {
		http.Error(w, "this route is a WebSocket", http.StatusUpgradeRequired)
		return
	}
	from, err := strconv.ParseUint(r.URL.Query().Get("from"), 10, 64)
	if err != nil && r.URL.Query().Get("from") != "" {
		http.Error(w, "from must be a sequence number", http.StatusBadRequest)
		return
	}
	t := d.terminals.get(id)
	if t == nil {
		http.Error(w, "unknown terminal", http.StatusNotFound)
		return
	}
	if t.lost() {
		// The page tries again; the manager's rescan may attach again.
		http.Error(w, errTerminalLost.Error(), http.StatusServiceUnavailable)
		return
	}
	if _, ok := w.(http.Hijacker); !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	d.serveTerminal(w, r, t, from)
}

func (d *Daemon) listTerminals(w http.ResponseWriter, s Session) {
	out := terminalList{Terminals: []TerminalInfo{}, Orphans: []TerminalInfo{}, Folders: terminalFolders{Worktree: s.Project}}
	if root := worktreeMain(s.Project); root != "" && root != s.Project {
		out.Folders.Root = root
	}
	for _, t := range d.terminals.list() {
		if t.Session == s.ReplyID {
			out.Terminals = append(out.Terminals, t)
		} else if _, live := d.index.Session(t.Session); !live {
			out.Orphans = append(out.Orphans, t)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(out)
}

// terminalFolder resolves a create request's folder for the session s. A
// folder that cannot be used returns its refusal code.
func terminalFolder(s Session, req terminalCreate) (dir, refusal string) {
	switch req.Folder {
	case "", "worktree":
		dir = s.Project
	case "root":
		if dir = worktreeMain(s.Project); dir == "" {
			dir = s.Project
		}
	case "path":
		dir = strings.TrimSpace(req.Path)
		if dir == "~" || strings.HasPrefix(dir, "~/") || strings.HasPrefix(dir, `~\`) {
			if home, err := os.UserHomeDir(); err == nil {
				dir = filepath.Join(home, dir[1:])
			}
		}
		if !filepath.IsAbs(dir) {
			return "", terminalFolderRelative
		}
		dir = filepath.Clean(dir)
	default:
		return "", terminalFolderRelative
	}
	info, err := os.Stat(dir)
	switch {
	case dir == "" || err != nil:
		return "", terminalFolderMissing
	case !info.IsDir():
		return "", terminalFolderNotDir
	}
	return dir, ""
}

func (d *Daemon) createTerminal(w http.ResponseWriter, r *http.Request, s Session) {
	var req terminalCreate
	if err := json.NewDecoder(io.LimitReader(r.Body, terminalBodyLimit)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "malformed request", http.StatusBadRequest)
		return
	}
	dir, refusal := terminalFolder(s, req)
	if refusal != "" {
		http.Error(w, refusal, http.StatusBadRequest)
		return
	}
	info, err := d.terminals.create(s.ReplyID, dir, d.shell(), req.Rows, req.Cols)
	if err != nil {
		fmt.Fprintf(d.log, "dashboard: open terminal in %s: %v\n", dir, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, info)
}

// termInput is one frame the page sends.
type termInput struct {
	T    string `json:"t"`
	Data string `json:"data"`
	Bin  []byte `json:"bin"`
	Rows int    `json:"rows"`
	Cols int    `json:"cols"`
}

// serveTerminal upgrades r and runs one viewer of t on the connection: a
// reader that applies the page's frames, and this goroutine, which writes
// the frames the terminal queues. The caller has run the route's checks.
func (d *Daemon) serveTerminal(w http.ResponseWriter, r *http.Request, t *terminal, from uint64) {
	window := wsReadWindow * d.keepAlive
	websocket.Server{Handler: func(ws *websocket.Conn) {
		ws.MaxPayloadBytes = terminalFrameMax
		v := t.join(from)
		if v == nil {
			_ = websocket.Message.Send(ws, `{"t":"closed"}`)
			_ = ws.Close()
			return
		}
		readDone := make(chan struct{})
		go func() {
			defer close(readDone)
			for {
				_ = ws.SetReadDeadline(time.Now().Add(window))
				var raw string
				if err := websocket.Message.Receive(ws, &raw); err != nil {
					return
				}
				var in termInput
				if json.Unmarshal([]byte(raw), &in) != nil {
					continue
				}
				switch in.T {
				case "in":
					data := in.Bin
					if in.Data != "" {
						data = []byte(in.Data)
					}
					// A refused write (the shell has exited) is not the
					// connection's end: the page learns of the exit from
					// the stream. A holder the daemon has lost sends
					// nothing more, so the connection ends and the page
					// asks again.
					if errors.Is(t.input(v, data), errTerminalLost) {
						return
					}
				case "size":
					if in.Rows > 0 && in.Cols > 0 {
						t.resize(v, in.Rows, in.Cols)
					}
				case "focus":
					t.focus(v)
				}
			}
		}()
		keepAlive := time.NewTicker(d.keepAlive)
		defer keepAlive.Stop()
		send := func(frame string) error {
			_ = ws.SetWriteDeadline(time.Now().Add(wsWriteTimeout))
			return websocket.Message.Send(ws, frame)
		}
	write:
		for {
			select {
			case <-v.wake:
				frames, last := v.take()
				for _, frame := range frames {
					if send(string(frame)) != nil {
						break write
					}
				}
				if last {
					break write
				}
			case <-keepAlive.C:
				// An idle viewer learns here that the holder is gone
				// (the sync ends the terminal, which queues "closed") or
				// that the daemon lost it (the connection ends).
				d.terminals.sync()
				if t.lost() || send(`{"t":"ping"}`) != nil {
					break write
				}
			case <-v.gone:
				break write
			case <-readDone:
				break write
			case <-d.stop:
				break write
			}
		}
		t.leave(v)
		_ = ws.Close()
		<-readDone
	}}.ServeHTTP(w, r)
}
