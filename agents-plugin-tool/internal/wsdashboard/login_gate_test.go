package wsdashboard

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsi18n"
)

func TestLoginGateSourcesUseShortBoldLabels(t *testing.T) {
	for _, lang := range wsi18n.Languages() {
		html := string(loginSources(lang.Code))
		if strings.Count(html, "<b>") != 3 || strings.Count(html, "</b>") != 3 || strings.Contains(html, "{0}") || strings.Contains(html, "{1}") || strings.Contains(html, "{2}") {
			t.Fatalf("%s: invalid source-label formatting: %s", lang.Code, html)
		}
	}
	want := "링크 받는 곳: 위의 <b>클립보드에 복사</b> 버튼, 로그인된 대시보드의 <b>설정</b> ⚙, 또는 데스크톱 앱의 <b>연결</b> 메뉴"
	if got := string(loginSources("ko")); got != want {
		t.Fatalf("Korean footer = %s, want %s", got, want)
	}
}

func TestLoginGateNextPreservesWindowsPathQuery(t *testing.T) {
	d := authFixture(t)
	next := "/s/session/file?path=C%3A%5CWorkspace%5Cfile.md&view=1"
	_, code := authLink(t, d, next)
	w := authServe(d, exchangeRequest(code, next))
	var body struct {
		Next string `json:"next"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || w.Code != 200 || body.Next != next {
		t.Fatalf("Windows path query lost: %d %s (%v)", w.Code, w.Body.String(), err)
	}
}

func TestLoginGateClipboardButtonOnlyForAvailableLocalWriter(t *testing.T) {
	for _, tc := range []struct {
		name, peer, host  string
		available, button bool
	}{
		{"loopback", "127.0.0.1:4", "127.0.0.1:43210", true, true},
		{"localhost", "127.0.0.1:4", "localhost:43210", true, true},
		{"IPv6", "[::1]:4", "[::1]:43210", true, true},
		{"no writer", "127.0.0.1:4", "127.0.0.1:43210", false, false},
		{"LAN peer", "192.0.2.1:4", "127.0.0.1:43210", true, false},
		{"LAN Host", "127.0.0.1:4", "192.0.2.1:43210", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := authFixture(t)
			d.lan = true
			writer := &authClipboardFake{available: tc.available}
			d.clipboard = writer
			r := authRequest("GET", "/s/session?panel=1", "")
			r.RemoteAddr, r.Host = tc.peer, tc.host
			r.Header.Set("Accept", "text/html")
			w := authServe(d, r)
			if w.Code != 401 {
				t.Fatalf("cookieless navigation = %d", w.Code)
			}
			if got := strings.Contains(w.Body.String(), `id="lg-copy"`); got != tc.button {
				t.Fatalf("native copy button = %v, want %v", got, tc.button)
			}
			if len(writer.links) != 0 {
				t.Fatal("rendering the gate wrote the native clipboard")
			}
		})
	}
}
