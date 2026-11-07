package wsdashboard

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
)

// The system-open route: POST /s/<reply-id>/open {"target": <url>} opens
// target in the host machine's default browser, for the link menu's "open in
// system browser" item. It runs a command on the host, so the server enforces
// what the page only reflects by showing the item to a loopback viewer:
//
//   - the request comes from a browser on this machine (localViewer); a LAN
//     viewer, token or not, gets 404, as if the route did not exist;
//   - it carries an Origin equal to the daemon's own (sameOrigin), as the
//     write routes do, which stops a cross-site page's post;
//   - target is, strictly, an absolute http(s) URL with no user info, or the
//     path of this session's own file view (/s/<reply-id>/file?path=...),
//     which is opened on the daemon's loopback address the request named;
//     it is printable ASCII with no space, quote, or backslash, as a browser
//     serializes a link's href;
//   - the OS open command runs directly, never through a shell, with target
//     as one argument that starts with "http" and so never reads as an
//     option. On Windows that is rundll32's FileProtocolHandler, not
//     "cmd /c start", where a "&" in a query would start a second command.
//
// It answers 204 once the command started; whether the browser then opened
// is not observed.

const (
	// openBodyLimit bounds the route's body; openTargetMax bounds target.
	openBodyLimit = 16 << 10
	openTargetMax = 8 << 10
)

// openRequest is the system-open route's body. Unknown fields are ignored.
type openRequest struct {
	Target string `json:"target"`
}

// openTarget validates a system-open target for the session replyID on the
// daemon addressed as host, and returns the URL to hand the OS.
func openTarget(raw, replyID, host string) (string, bool) {
	if raw == "" || len(raw) > openTargetMax {
		return "", false
	}
	for i := 0; i < len(raw); i++ {
		if c := raw[i]; c <= ' ' || c >= 0x7f || c == '"' || c == '\\' {
			return "", false
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Opaque != "" {
		return "", false
	}
	switch u.Scheme {
	case "http", "https":
		if u.Host == "" {
			return "", false
		}
		return u.String(), true
	case "":
		if u.Host != "" || u.Path != "/s/"+replyID+"/file" || u.Fragment != "" || !ValidReplyID(replyID) {
			return "", false
		}
		q, err := url.ParseQuery(u.RawQuery)
		if err != nil || len(q) != 1 || len(q["path"]) != 1 || q.Get("path") == "" {
			return "", false
		}
		return (&url.URL{Scheme: "http", Host: host, Path: u.Path, RawQuery: u.RawQuery}).String(), true
	}
	return "", false
}

// systemOpen opens target in the default browser with the host OS's open
// command, run with no shell; the command is reaped in the background.
func systemOpen(target string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", target)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// handleSystemOpen serves the system-open route of one session.
func (d *Daemon) handleSystemOpen(w http.ResponseWriter, r *http.Request, replyID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !localViewer(r) {
		http.NotFound(w, r)
		return
	}
	if !sameOrigin(r) {
		http.Error(w, "cross-origin request refused", http.StatusForbidden)
		return
	}
	if d.index == nil || !ValidReplyID(replyID) {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	if _, ok := d.index.Session(replyID); !ok {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	var req openRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, openBodyLimit)).Decode(&req); err != nil {
		http.Error(w, "malformed request", http.StatusBadRequest)
		return
	}
	target, ok := openTarget(req.Target, replyID, r.Host)
	if !ok {
		http.Error(w, "target must be an http(s) URL or this session's file view", http.StatusBadRequest)
		return
	}
	if err := d.openSystem(target); err != nil {
		fmt.Fprintf(d.log, "dashboard: system open: %v\n", err)
		http.Error(w, "the system browser could not be started", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
