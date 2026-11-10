package wsdashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// The file browser's host route: POST /s/<reply-id>/fs/act
// {"path": <path>, "action": "open" | "reveal"[, "root": <abs>]} opens the
// entry at path in the host's default app (open) or shows it selected in
// the host's file manager (reveal), for the explorer's row menu and a
// preview card's "open in the default app" button. It runs a command on the
// host, so it is gated as the system-open route is, once for both actions:
//
//   - the request comes from a browser on this machine (localViewer); a LAN
//     viewer, token or not, gets 404, as if the route did not exist;
//   - it carries an Origin equal to the daemon's own (sameOrigin), which
//     stops a cross-site page's post;
//   - action is exactly "open" or "reveal";
//   - path names an entry inside the browsing root (the session's project
//     root, or the root field under browseRootParam's rules), lexically and
//     once resolved through every symlink; "" names the root itself. The
//     resolved path is what the OS is handed;
//   - the command runs directly, never through a shell, with the path as
//     one argument (hostFileCommand).
//
// It answers 204 once the command started; whether a window then opened is
// not observed.

// fsActRequest is the route's body. Unknown fields are ignored.
type fsActRequest struct {
	Path   string `json:"path"`
	Action string `json:"action"`
	Root   string `json:"root"`
}

// hostFiles is the host the route acts on: its OS, whether it is WSL, the
// conversion of a path to its Windows spelling there, and the command
// start. A test replaces it, so no test opens a window.
type hostFiles struct {
	goos    string
	wsl     bool
	winPath func(path string) (string, error)
	start   func(argv []string) error
}

// defaultHostFiles is the host the daemon runs on.
func defaultHostFiles() *hostFiles {
	return &hostFiles{goos: runtime.GOOS, wsl: runningUnderWSL(), winPath: wslWinPath, start: startHostCommand}
}

// runningUnderWSL reports a Linux daemon inside WSL, whose desktop is
// Windows': the interop marker WSL registers, else its distro variable.
func runningUnderWSL() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	if _, err := os.Stat("/proc/sys/fs/binfmt_misc/WSLInterop"); err == nil {
		return true
	}
	return os.Getenv("WSL_DISTRO_NAME") != ""
}

// wslWinPath converts a WSL path to its Windows spelling with wslpath.
func wslWinPath(path string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "wslpath", "-w", path).Output()
	if err != nil {
		return "", err
	}
	win := strings.TrimRight(string(out), "\r\n")
	if win == "" {
		return "", fmt.Errorf("wslpath printed no path")
	}
	return win, nil
}

// startHostCommand starts argv with no shell and reaps it in the
// background.
func startHostCommand(argv []string) error {
	cmd := exec.Command(argv[0], argv[1:]...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// hostFileCommand is the command that performs action ("open" or "reveal")
// on the absolute path path for a host of the OS goos. Under WSL (wsl) the
// desktop is Windows', so the command is Windows' explorer on winPath, the
// path in its Windows spelling. It is nil for an unknown action.
//
//	open    windows  rundll32 url.dll,FileProtocolHandler <path>
//	        darwin   open <path>
//	        WSL      explorer.exe <winPath>
//	        other    xdg-open <path>
//	reveal  windows  explorer /select,<path>
//	        darwin   open -R <path>
//	        WSL      explorer.exe /select,<winPath>
//	        other    xdg-open <the parent folder>   (no portable "select")
func hostFileCommand(goos string, wsl bool, action, path, winPath string) []string {
	switch action {
	case "open":
		switch {
		case goos == "windows":
			return []string{"rundll32", "url.dll,FileProtocolHandler", path}
		case goos == "darwin":
			return []string{"open", path}
		case wsl:
			return []string{"explorer.exe", winPath}
		}
		return []string{"xdg-open", path}
	case "reveal":
		switch {
		case goos == "windows":
			return []string{"explorer", "/select," + path}
		case goos == "darwin":
			return []string{"open", "-R", path}
		case wsl:
			return []string{"explorer.exe", "/select," + winPath}
		}
		return []string{"xdg-open", filepath.Dir(path)}
	}
	return nil
}

// containedEntry resolves path ("" is root itself) to an existing entry
// inside root, lexically and once resolved through every symlink, and
// returns the resolved path.
func containedEntry(root, path string) (string, bool) {
	full := filepath.Clean(root)
	if path != "" && path != "." {
		rel, ok := relativeInside(root, path)
		if !ok {
			return "", false
		}
		full = filepath.Join(root, rel)
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", false
	}
	real, err := filepath.EvalSymlinks(full)
	if err != nil {
		return "", false
	}
	if r, err := filepath.Rel(realRoot, real); err != nil || (r != "." && !insideRel(r)) {
		return "", false
	}
	if networkPath(real) || !driveOrRootPath(real) {
		return "", false
	}
	if _, err := os.Stat(real); err != nil {
		return "", false
	}
	return real, true
}

// handleFSAct serves the host route of one session.
func (d *Daemon) handleFSAct(w http.ResponseWriter, r *http.Request, replyID string) {
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
	s, ok := d.index.Session(replyID)
	if !ok || s.Project == "" {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	var req fsActRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, openBodyLimit)).Decode(&req); err != nil {
		http.Error(w, "malformed request", http.StatusBadRequest)
		return
	}
	if req.Action != "open" && req.Action != "reveal" {
		http.Error(w, `action must be "open" or "reveal"`, http.StatusBadRequest)
		return
	}
	root := s.Project
	if req.Root != "" {
		if root, ok = browseRootParam(req.Root); !ok {
			http.Error(w, "root must be an absolute folder", http.StatusBadRequest)
			return
		}
	}
	if len(req.Path) > openTargetMax {
		http.Error(w, "path must name an entry inside the root", http.StatusBadRequest)
		return
	}
	target, ok := containedEntry(root, req.Path)
	if !ok {
		http.Error(w, "path must name an entry inside the root", http.StatusBadRequest)
		return
	}
	host := d.hostFiles
	win := ""
	if host.wsl && host.goos != "windows" && host.goos != "darwin" {
		var err error
		if win, err = host.winPath(target); err != nil {
			fmt.Fprintf(d.log, "dashboard: file %s: wslpath: %v\n", req.Action, err)
			http.Error(w, "the path could not be converted for the host", http.StatusInternalServerError)
			return
		}
	}
	if err := host.start(hostFileCommand(host.goos, host.wsl, req.Action, target, win)); err != nil {
		fmt.Fprintf(d.log, "dashboard: file %s: %v\n", req.Action, err)
		http.Error(w, "the host command could not be started", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
