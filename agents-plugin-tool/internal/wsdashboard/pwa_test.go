package wsdashboard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"strings"
	"testing"
)

// The manifest an installing browser reads: served as a manifest, scoped to
// the whole origin so a session page stays in the app window, standalone,
// and naming icons that are themselves served as PNGs of their stated size.
func TestPWAManifestAndIconsAreServed(t *testing.T) {
	f := startPageFixture(t)
	resp, err := testClient().Get(f.base + pwaPrefix + "manifest.webmanifest")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/manifest+json" {
		t.Fatalf("manifest = %d %q, want 200 application/manifest+json", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	var m struct {
		Icons []struct{ Src, Sizes, Type, Purpose string }
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("manifest is not JSON: %v", err)
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	str := func(key string) string {
		var s string
		_ = json.Unmarshal(fields[key], &s)
		return s
	}
	for key, want := range map[string]string{"id": "/", "start_url": "/", "scope": "/", "display": "standalone", "theme_color": themeColorDark, "background_color": themeColorDark} {
		if got := str(key); got != want {
			t.Errorf("manifest %s = %q, want %q", key, got, want)
		}
	}
	if str("name") == "" || str("short_name") == "" {
		t.Errorf("manifest lacks a name: %s", raw)
	}
	sizes := map[string]bool{}
	for _, icon := range m.Icons {
		resp, err := testClient().Get(f.base + icon.Src)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/png" || icon.Type != "image/png" {
			t.Errorf("icon %s = %d %q (manifest type %q), want 200 image/png", icon.Src, resp.StatusCode, resp.Header.Get("Content-Type"), icon.Type)
			continue
		}
		cfg, err := png.DecodeConfig(bytes.NewReader(body))
		if err != nil {
			t.Errorf("icon %s is not a PNG: %v", icon.Src, err)
			continue
		}
		if got := fmt.Sprintf("%dx%d", cfg.Width, cfg.Height); got != icon.Sizes {
			t.Errorf("icon %s is %s, manifest says %s", icon.Src, got, icon.Sizes)
		}
		sizes[icon.Sizes+" "+icon.Purpose] = true
	}
	for _, want := range []string{"192x192 any", "512x512 any", "512x512 maskable"} {
		if !sizes[want] {
			t.Errorf("manifest lacks a %s icon: %v", want, sizes)
		}
	}

	if code, _ := get(t, f.base+pwaPrefix+"nope.png"); code != http.StatusNotFound {
		t.Errorf("unknown app asset = %d, want 404", code)
	}
	resp, err = testClient().Post(f.base+pwaPrefix+"manifest.webmanifest", "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST manifest = %d, want 405", resp.StatusCode)
	}
}

// Every dashboard page links the manifest (with credentials, for a LAN
// viewer's cookie) and sets theme-color: each scheme its own colour under
// auto, both the pinned theme's colour when the cookie pins one.
func TestPagesLinkTheAppManifest(t *testing.T) {
	f := startPageFixture(t)
	link := `<link rel="manifest" href="/pwa/manifest.webmanifest" crossorigin="use-credentials">`
	meta := func(scheme, color string) string {
		return `<meta name="theme-color" media="(prefers-color-scheme: ` + scheme + `)" content="` + color + `"`
	}
	for _, page := range []struct{ name, path string }{
		{"session list", "/"},
		{"board-first session", "/s/boardsession"},
		{"adapter session", "/s/livesession"},
		{"finding page", "/s/notyetthere"},
	} {
		for _, c := range []struct{ cookie, light, dark string }{
			{"", themeColorLight, themeColorDark},
			{"light", themeColorLight, themeColorLight},
			{"dark", themeColorDark, themeColorDark},
		} {
			_, body := getTheme(t, f.base+page.path, c.cookie)
			for _, want := range []string{link, meta("light", c.light), meta("dark", c.dark)} {
				if !strings.Contains(body, want) {
					t.Errorf("%s (theme %q) lacks %s", page.name, c.cookie, want)
				}
			}
		}
	}
}
