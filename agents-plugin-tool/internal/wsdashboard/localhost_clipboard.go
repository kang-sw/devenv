package wsdashboard

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	clipboardInterval = 2 * time.Second
	clipboardWindow   = time.Minute
	clipboardLimit    = 10
	clipboardTimeout  = 3 * time.Second
)

// clipboardWriter keeps the credential out of the HTTP response: only the
// daemon's native clipboard receives the full, single-use browser login link.
type clipboardWriter interface {
	Available() bool
	WriteText(context.Context, string) error
}

type clipboardCommand struct {
	path string
	args []string
}

type nativeClipboardWriter struct {
	commands []clipboardCommand
	run      func(context.Context, clipboardCommand, string) error
}

func newNativeClipboardWriter() clipboardWriter {
	return &nativeClipboardWriter{commands: clipboardCommands(runtime.GOOS, exec.LookPath), run: runClipboardCommand}
}

// clipboardCommands checks only executables, never touches the clipboard.
// Availability means a native writer exists; a headless/display failure is
// still reported by WriteText without exposing the link to the browser.
func clipboardCommands(goos string, lookPath func(string) (string, error)) []clipboardCommand {
	var candidates []clipboardCommand
	switch goos {
	case "darwin":
		candidates = []clipboardCommand{{path: "pbcopy"}}
	case "windows":
		// Stdin is explicitly decoded as UTF-8, then Set-Clipboard receives
		// a Unicode .NET string. No link text is interpolated into a script,
		// placed in argv, or passed through a legacy console code page.
		candidates = []clipboardCommand{{path: "powershell.exe", args: []string{"-NoProfile", "-NonInteractive", "-STA", "-Command",
			`[Console]::InputEncoding = [System.Text.UTF8Encoding]::new($false); $text = [Console]::In.ReadToEnd(); Set-Clipboard -Value $text`}}}
	case "linux":
		candidates = []clipboardCommand{{path: "wl-copy"}, {path: "xclip", args: []string{"-selection", "clipboard"}}, {path: "xsel", args: []string{"-b"}}}
	}
	var commands []clipboardCommand
	for _, candidate := range candidates {
		if path, err := lookPath(candidate.path); err == nil {
			candidate.path = path
			commands = append(commands, candidate)
		}
	}
	return commands
}

func runClipboardCommand(ctx context.Context, command clipboardCommand, text string) error {
	cmd := exec.CommandContext(ctx, command.path, command.args...)
	cmd.Stdin = strings.NewReader(text)
	// No stdout/stderr capture: platform diagnostics must not log a code.
	return cmd.Run()
}

func (w *nativeClipboardWriter) Available() bool { return len(w.commands) > 0 }

func (w *nativeClipboardWriter) WriteText(ctx context.Context, text string) error {
	for _, command := range w.commands {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := w.run(ctx, command, text); err == nil {
			return nil
		}
	}
	return errors.New("native clipboard unavailable")
}

func (d *Daemon) clipboardAvailable() bool {
	return d.clipboard != nil && d.clipboard.Available()
}

// clipboardRetry reserves an attempt before invoking a platform command. All
// attempts, successful or not, share a small independent per-daemon budget.
func (d *Daemon) clipboardRetry() int {
	a := &d.auth
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.clock()
	if !a.clipboardStarted || !now.Before(a.clipboardWindow.Add(clipboardWindow)) {
		a.clipboardWindow, a.clipboardAttempts = now, 0
	}
	var wait time.Duration
	if a.clipboardAttempts >= clipboardLimit {
		wait = a.clipboardWindow.Add(clipboardWindow).Sub(now)
	}
	if a.clipboardStarted && now.Before(a.clipboardLast.Add(clipboardInterval)) {
		wait = max(wait, a.clipboardLast.Add(clipboardInterval).Sub(now))
	}
	if wait > 0 {
		return max(1, int((wait+time.Second-1)/time.Second))
	}
	a.clipboardLast, a.clipboardStarted = now, true
	a.clipboardAttempts++
	return 0
}

func clipboardUnavailable(w http.ResponseWriter) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "clipboard_unavailable"})
}

func (d *Daemon) discardLoginLink(link string) {
	u, err := url.Parse(link)
	if err != nil {
		return
	}
	code := u.Query().Get("login")
	a := &d.auth
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, candidate := range a.codes {
		if tokenMatches(code, candidate.value) {
			a.codes = append(a.codes[:i], a.codes[i+1:]...)
			return
		}
	}
}

func (d *Daemon) handleAuthClipboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !localViewer(r) {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sites := r.Header.Values("Sec-Fetch-Site")
	if len(r.Header.Values("Origin")) != 1 || !sameOrigin(r) ||
		(len(sites) > 0 && (len(sites) != 1 || sites[0] != "same-origin")) {
		http.Error(w, "forbidden origin", http.StatusForbidden)
		return
	}
	var body struct {
		Next string `json:"next"`
	}
	if !decodeAuthBody(w, r, &body) {
		return
	}
	if !validLoginPath(body.Next) {
		http.Error(w, "invalid next", http.StatusBadRequest)
		return
	}
	if retry := d.clipboardRetry(); retry > 0 {
		authRateLimited(w, retry)
		return
	}
	if !d.clipboardAvailable() {
		clipboardUnavailable(w)
		return
	}
	link, err := d.LoginLink(body.Next)
	if err != nil {
		clipboardUnavailable(w)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), clipboardTimeout)
	err = d.clipboard.WriteText(ctx, link)
	cancel()
	if err != nil {
		// A writer may have partially succeeded. Revoke on any reported
		// failure rather than leaving that unacknowledged code usable.
		d.discardLoginLink(link)
		clipboardUnavailable(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
