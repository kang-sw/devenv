package wsdashboard

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

type authClipboardFake struct {
	available bool
	err       error
	links     []string
	deadline  bool
}

func (f *authClipboardFake) Available() bool { return f.available }
func (f *authClipboardFake) WriteText(ctx context.Context, text string) error {
	f.links = append(f.links, text)
	_, f.deadline = ctx.Deadline()
	return f.err
}

func clipboardRequest(next string) *http.Request {
	body, _ := json.Marshal(map[string]string{"next": next})
	r := authRequest("POST", "/auth/clipboard", string(body))
	r.Header.Set("Origin", "http://"+r.Host)
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	return r
}

func TestLocalhostAuthClipboardWritesSingleUseLinkWithoutLeakingIt(t *testing.T) {
	d := authFixture(t)
	writer := &authClipboardFake{available: true}
	d.clipboard = writer
	if !d.clipboardAvailable() {
		t.Fatal("injected clipboard unavailable")
	}
	w := authServe(d, clipboardRequest("/s/session?q=1#topic"))
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"ok":true}` || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("clipboard response: %d %v %s", w.Code, w.Header(), w.Body.String())
	}
	if len(w.Result().Cookies()) != 0 || len(writer.links) != 1 || !writer.deadline {
		t.Fatalf("writer/cookies: %+v %v", writer, w.Result().Cookies())
	}
	u, err := url.Parse(writer.links[0])
	if err != nil {
		t.Fatal(err)
	}
	code := u.Query().Get("login")
	if u.Scheme != "http" || u.Host != "127.0.0.1:43210" || u.Path != "/s/session" || u.Query().Get("q") != "1" || u.Fragment != "topic" || len(code) < 32 {
		t.Fatalf("native link = %q", writer.links[0])
	}
	if strings.Contains(w.Body.String(), code) || strings.Contains(w.Header().Get("Location"), code) {
		t.Fatal("credential leaked in response")
	}
	if got := authServe(d, exchangeRequest(writer.links[0], "/s/session?q=1#topic")); got.Code != 200 {
		t.Fatalf("clipboard link exchange = %d %s", got.Code, got.Body.String())
	}
	if got := authServe(d, exchangeRequest(writer.links[0], "/")); got.Code != 401 {
		t.Fatalf("clipboard code reused: %d", got.Code)
	}
}

func TestLocalhostAuthClipboardCallerBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, peer, host, origin, site string
		omitSite                       bool
		want                           int
	}{
		{"local", "127.0.0.1:4", "127.0.0.1:43210", "http://127.0.0.1:43210", "same-origin", false, 200},
		{"missing metadata", "127.0.0.1:4", "127.0.0.1:43210", "http://127.0.0.1:43210", "", true, 200},
		{"LAN peer", "192.0.2.1:4", "127.0.0.1:43210", "http://127.0.0.1:43210", "same-origin", false, 404},
		{"LAN host", "127.0.0.1:4", "192.0.2.1:43210", "http://192.0.2.1:43210", "same-origin", false, 404},
		{"missing origin", "127.0.0.1:4", "127.0.0.1:43210", "", "same-origin", false, 403},
		{"foreign origin", "127.0.0.1:4", "127.0.0.1:43210", "http://evil.example", "same-origin", false, 403},
		{"opaque origin", "127.0.0.1:4", "127.0.0.1:43210", "null", "same-origin", false, 403},
		{"same site", "127.0.0.1:4", "127.0.0.1:43210", "http://127.0.0.1:43210", "same-site", false, 403},
		{"cross site", "127.0.0.1:4", "127.0.0.1:43210", "http://127.0.0.1:43210", "cross-site", false, 403},
		{"none metadata", "127.0.0.1:4", "127.0.0.1:43210", "http://127.0.0.1:43210", "none", false, 403},
		{"empty metadata", "127.0.0.1:4", "127.0.0.1:43210", "http://127.0.0.1:43210", "", false, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := authFixture(t)
			d.lan = true // both peer and Host checks must survive LAN mode
			writer := &authClipboardFake{available: true}
			d.clipboard = writer
			r := clipboardRequest("/")
			r.RemoteAddr, r.Host = tc.peer, tc.host
			r.Header.Set("Origin", tc.origin)
			if tc.omitSite {
				r.Header.Del("Sec-Fetch-Site")
			} else {
				r.Header.Set("Sec-Fetch-Site", tc.site)
			}
			w := authServe(d, r)
			if w.Code != tc.want {
				t.Fatalf("got %d want %d: %s", w.Code, tc.want, w.Body.String())
			}
			if tc.want != 200 && len(writer.links) != 0 {
				t.Fatal("refused caller wrote clipboard")
			}
		})
	}
}

func TestLocalhostAuthClipboardHidesNonLocalHostInLoopbackMode(t *testing.T) {
	d := authFixture(t)
	d.clipboard = &authClipboardFake{available: true}
	r := clipboardRequest("/")
	r.Host = "evil.example"
	r.Header.Set("Origin", "http://evil.example")
	if w := authServe(d, r); w.Code != 404 {
		t.Fatalf("non-local Host in loopback mode = %d want 404", w.Code)
	}
}

func TestLocalhostAuthClipboardNextAndUnavailable(t *testing.T) {
	for _, next := range []string{"", "https://evil.example/", "//evil.example/", "/\\evil", "/%5cevil", "/%2fhost", "/%0a"} {
		d := authFixture(t)
		writer := &authClipboardFake{available: true}
		d.clipboard = writer
		if w := authServe(d, clipboardRequest(next)); w.Code != 400 {
			t.Fatalf("next %q = %d", next, w.Code)
		}
		if len(writer.links) != 0 || len(d.auth.codes) != 0 {
			t.Fatal("invalid next minted a code")
		}
	}
	for _, writer := range []clipboardWriter{nil, &authClipboardFake{}, &authClipboardFake{available: true, err: errors.New("native failure with sensitive diagnostics")}} {
		d := authFixture(t)
		d.clipboard = writer
		w := authServe(d, clipboardRequest("/"))
		if w.Code != 503 || strings.TrimSpace(w.Body.String()) != `{"error":"clipboard_unavailable"}` || len(d.auth.codes) != 0 {
			t.Fatalf("unavailable: %d %s codes=%d", w.Code, w.Body.String(), len(d.auth.codes))
		}
	}
}

func TestLocalhostAuthClipboardRateBudgetIsIndependent(t *testing.T) {
	d := authFixture(t)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	d.auth.now = func() time.Time { return now }
	writer := &authClipboardFake{available: true}
	d.clipboard = writer
	for i := 0; i < clipboardLimit; i++ {
		if w := authServe(d, clipboardRequest("/")); w.Code != 200 {
			t.Fatalf("attempt %d = %d", i, w.Code)
		}
		retry := "2"
		if i == clipboardLimit-1 {
			retry = "42" // the minute budget now dominates the interval
		}
		if w := authServe(d, clipboardRequest("/")); w.Code != 429 || w.Header().Get("Retry-After") != retry {
			t.Fatalf("interval = %d %v", w.Code, w.Header())
		}
		now = now.Add(clipboardInterval)
	}
	if w := authServe(d, clipboardRequest("/")); w.Code != 429 || w.Header().Get("Retry-After") != "40" {
		t.Fatalf("minute limit = %d %v", w.Code, w.Header())
	}
	if len(writer.links) != clipboardLimit || d.auth.failures != 0 {
		t.Fatalf("budgets mixed: clipboard=%d failures=%d", len(writer.links), d.auth.failures)
	}
	// Clipboard limits must not prevent normal exchange of a pasted link.
	if w := authServe(d, exchangeRequest(writer.links[0], "/")); w.Code != 200 {
		t.Fatalf("clipboard limiter blocked exchange: %d", w.Code)
	}
	now = now.Add(40 * time.Second)
	if w := authServe(d, clipboardRequest("/")); w.Code != 200 {
		t.Fatalf("window did not reset: %d", w.Code)
	}
}

func TestLocalhostAuthLimitersAcceptAZeroFixtureClock(t *testing.T) {
	d := authFixture(t)
	d.auth.now = func() time.Time { return time.Time{} }
	if retry := d.clipboardRetry(); retry != 0 {
		t.Fatalf("first clipboard attempt = %d", retry)
	}
	if retry := d.clipboardRetry(); retry != 2 {
		t.Fatalf("zero clock bypassed clipboard interval: %d", retry)
	}
	for i := 0; i < loginFailureLimit; i++ {
		d.consumeLoginCode("invalid")
	}
	if _, retry := d.consumeLoginCode("invalid"); retry != 60 {
		t.Fatalf("zero clock bypassed exchange limiter: %d", retry)
	}
}

func TestLocalhostAuthNativeClipboardCommandSelectionAndFallback(t *testing.T) {
	for _, tc := range []struct {
		goos string
		want []string
	}{
		{"darwin", []string{"/fake/pbcopy"}}, {"windows", []string{"/fake/powershell.exe"}},
		{"linux", []string{"/fake/wl-copy", "/fake/xclip", "/fake/xsel"}}, {"other", nil},
	} {
		commands := clipboardCommands(tc.goos, func(name string) (string, error) { return "/fake/" + name, nil })
		var got []string
		for _, c := range commands {
			got = append(got, c.path)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s: %v want %v", tc.goos, got, tc.want)
		}
		if tc.goos == "windows" {
			script := strings.Join(commands[0].args, " ")
			for _, required := range []string{"-NoProfile", "-NonInteractive", "-STA", "UTF8Encoding", "In.ReadToEnd()", "Set-Clipboard -Value $text"} {
				if !strings.Contains(script, required) {
					t.Fatalf("missing %q in %q", required, script)
				}
			}
		}
	}
	commands := clipboardCommands("linux", func(name string) (string, error) {
		if name == "wl-copy" {
			return "", errors.New("absent")
		}
		return name, nil
	})
	var called []string
	text := "한글 🦀 ; $notAScript"
	writer := &nativeClipboardWriter{commands: commands, run: func(_ context.Context, c clipboardCommand, stdin string) error {
		if stdin != text || strings.Contains(strings.Join(c.args, " "), text) {
			t.Fatal("text was changed or interpolated into argv")
		}
		called = append(called, c.path)
		if c.path == "xclip" {
			if !reflect.DeepEqual(c.args, []string{"-selection", "clipboard"}) {
				t.Fatal(c.args)
			}
			return errors.New("display unavailable")
		}
		if !reflect.DeepEqual(c.args, []string{"-b"}) {
			t.Fatal(c.args)
		}
		return nil
	}}
	if !writer.Available() {
		t.Fatal("available commands were ignored")
	}
	if err := writer.WriteText(context.Background(), text); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(called, []string{"xclip", "xsel"}) {
		t.Fatal(called)
	}
	missing := &nativeClipboardWriter{}
	if missing.Available() || missing.WriteText(context.Background(), text) == nil {
		t.Fatal("no command reported available")
	}
}
