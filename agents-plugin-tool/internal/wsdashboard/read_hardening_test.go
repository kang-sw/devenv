package wsdashboard

import (
	"encoding/json"
	"html"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func TestNudgeBrowserOriginGuard(t *testing.T) {
	wake := &countingMailWake{}
	f := startPageFixture(t, func(o *DaemonOptions) { o.MailWake = wake })
	body, err := json.Marshal(NudgeBody{ReplyID: "livesession", Project: f.project})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, origin, site string
		originPresent      bool
		want               int
	}{
		{name: "non-browser", want: http.StatusNoContent},
		{name: "same-origin", origin: f.base, originPresent: true, site: "same-origin", want: http.StatusNoContent},
		{name: "same-origin Origin only", origin: f.base, originPresent: true, want: http.StatusNoContent},
		{name: "same-origin metadata only", site: "same-origin", want: http.StatusNoContent},
		{name: "user navigation metadata", site: "none", want: http.StatusNoContent},
		{name: "foreign Origin", origin: "https://foreign.invalid", originPresent: true, want: http.StatusForbidden},
		{name: "opaque Origin", origin: "null", originPresent: true, want: http.StatusForbidden},
		{name: "empty Origin", originPresent: true, want: http.StatusForbidden},
		{name: "cross-site without Origin", site: "cross-site", want: http.StatusForbidden},
		{name: "same-site without Origin", site: "same-site", want: http.StatusForbidden},
		{name: "cross-site with own Origin", origin: f.base, originPresent: true, site: "cross-site", want: http.StatusForbidden},
		{name: "foreign Origin with own metadata", origin: "https://foreign.invalid", originPresent: true, site: "same-origin", want: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, f.base+"/nudge", strings.NewReader(string(body)))
			if tc.originPresent {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.site != "" {
				req.Header.Set("Sec-Fetch-Site", tc.site)
			}
			_, before := wake.counts()
			statsBefore := f.d.stats.Nudges.Load()
			rec := httptest.NewRecorder()
			f.d.Handler().ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("nudge = %d %s, want %d", rec.Code, rec.Body.String(), tc.want)
			}
			_, after := wake.counts()
			wantDelta := 0
			if tc.want == http.StatusNoContent {
				wantDelta = 1
			}
			if after-before != wantDelta || f.d.stats.Nudges.Load()-statsBefore != int64(wantDelta) {
				t.Fatalf("nudge side effects: mail delta %d, stats delta %d, want %d", after-before, f.d.stats.Nudges.Load()-statsBefore, wantDelta)
			}
		})
	}
}

func TestSessionReadCrossSiteHardening(t *testing.T) {
	f := startPageFixture(t, lanMode)
	local, lan := fsViewers(t, f)
	writeFile(t, filepath.Join(f.project, "shot.png"), "PNG-INSIDE")
	writeFile(t, filepath.Join(f.project, "notes.txt"), "TEXT-INSIDE")
	writeFile(t, filepath.Join(f.project, "notes.md"), "# Notes")
	writeFile(t, filepath.Join(f.project, "page.html"), "<h1>Page</h1>")
	writeFile(t, filepath.Join(f.project, "binary.bin"), "\x00\x01")
	for _, v := range []viewer{local, lan} {
		for _, target := range []string{
			"/s/livesession/image?path=shot.png",
			"/s/livesession/file?path=shot.png",
			"/s/livesession/file?path=notes.txt",
			"/s/livesession/file?path=notes.md",
			"/s/livesession/file?path=page.html",
			"/s/livesession/file?path=binary.bin",
		} {
			for _, site := range []string{"", "same-origin", "none", "cross-site", "same-site"} {
				t.Run(v.name+target+"/"+site, func(t *testing.T) {
					req := httptest.NewRequest(http.MethodGet, target, nil)
					req.RemoteAddr, req.Host = v.peer, v.host
					if v.cookie != "" {
						req.AddCookie(&http.Cookie{Name: lanTokenCookie, Value: v.cookie})
					}
					if site != "" {
						req.Header.Set("Sec-Fetch-Site", site)
						req.Header.Set("Origin", "http://"+v.host)
					}
					if v.localCookie {
						addLocalhostTestCookie(t, f.d, req)
					}
					rec := httptest.NewRecorder()
					f.d.Handler().ServeHTTP(rec, req)
					if site == "cross-site" || site == "same-site" {
						if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "INSIDE") {
							t.Fatalf("foreign load = %d %q, want 404", rec.Code, rec.Body.String())
						}
						return
					}
					want := http.StatusOK
					if strings.HasSuffix(target, "page.html") {
						want = http.StatusFound
					}
					if rec.Code != want {
						t.Fatalf("read = %d %s, want %d", rec.Code, rec.Body.String(), want)
					}
					if rec.Header().Get("Cross-Origin-Resource-Policy") != "same-origin" || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
						t.Fatalf("read headers = %v", rec.Header())
					}
					assertNoCORS(t, rec.Header())
				})
			}
		}
	}
}

func TestWriteJSONNosniff(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusAccepted, http.StatusBadRequest} {
		rec := httptest.NewRecorder()
		writeJSON(rec, status, map[string]bool{"ok": true})
		res := rec.Result()
		if res.StatusCode != status || res.Header.Get("Content-Type") != "application/json" || res.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("JSON status/headers = %d %v", res.StatusCode, res.Header)
		}
	}
}

func TestReadHardeningKeepsSandboxCapabilityImages(t *testing.T) {
	f := startPageFixture(t, lanMode)
	local, lan := fsViewers(t, f)
	bare := lan
	bare.name, bare.cookie = "LAN without cookie", ""
	writeFile(t, filepath.Join(f.project, "shot.png"), "PNG-INSIDE")
	for _, target := range []string{
		f.d.fsCapabilityURL("livesession", f.project, "shot.png"),
		f.d.fsViewURL("livesession", f.project, "shot.png"),
	} {
		for _, v := range []viewer{local, bare} {
			rec := capFetch(f, v, target, capShape{origin: "null", site: "cross-site", mode: "no-cors", dest: "image"})
			if rec.Code != http.StatusOK || rec.Body.String() != "PNG-INSIDE" {
				t.Fatalf("opaque capability image as %s = %d %s", v.name, rec.Code, rec.Body.String())
			}
			// CORP same-origin would block this opaque-origin no-cors load.
			if rec.Header().Get("Cross-Origin-Resource-Policy") != "" || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("capability image headers = %v", rec.Header())
			}
			for _, shape := range []capShape{
				{origin: "https://foreign.invalid", site: "cross-site", mode: "no-cors", dest: "image"},
				{origin: "null", site: "cross-site", mode: "cors", dest: "empty"},
			} {
				if refused := capFetch(f, v, target, shape); refused.Code != http.StatusNotFound {
					t.Errorf("capability refusal = %d, want 404", refused.Code)
				}
			}
		}
	}
}

func TestReadHardeningRelaysLargeTopicImages(t *testing.T) {
	f := startPageFixture(t, lanMode)
	writeFile(t, filepath.Join(f.project, "small.png"), "SMALL")
	large := strings.Repeat("x", imageDataMax+1)
	writeFile(t, filepath.Join(f.project, "large.png"), large)
	if ImageDataURL(f.project, "large.png") != "" {
		t.Fatal("large image should exceed inline limit")
	}
	st := &wswhiteboard.State{Seq: 1, Topics: []wswhiteboard.TopicDoc{{Name: "Images", Changed: 1,
		Body: "![small](small.png) ![big](large.png) <img src=\"large.png\" alt=\"raw\">"}}}
	view := wswhiteboard.BuildView(st, wswhiteboard.ViewInput{
		Layout: wswhiteboard.LayoutFull, ReplyID: "livesession",
		FileText:    func(p string) string { return p },
		ImageSrc:    func(p string) string { return ImageSrc(f.project, "livesession", p) },
		DocImageSrc: func(p string) string { return ImageDataURL(f.project, p) },
		FileActions: FileActions(f.project, "livesession"),
	})
	regions, err := wswhiteboard.RenderRegions(view)
	if err != nil {
		t.Fatal(err)
	}
	board := html.UnescapeString(string(regions.Board))
	src := ImageSrc(f.project, "livesession", "large.png")
	for _, want := range []string{"data:image/png;base64,U01BTEw=", `data-relay="` + src + `" data-sandbox=`, `<img data-relay="` + src + `" alt="raw">`} {
		if !strings.Contains(board, want) {
			t.Errorf("render lacks %q", want)
		}
	}
	if strings.Contains(board, `src="`+src+`"`) {
		t.Fatal("large image must not load directly from the opaque frame")
	}
	// The host's same-origin fetch, including a LAN host with its cookie,
	// can still retrieve the image to pass back to the sandbox as data.
	local, lan := fsViewers(t, f)
	for _, v := range []viewer{local, lan} {
		req := httptest.NewRequest(http.MethodGet, src, nil)
		req.RemoteAddr, req.Host = v.peer, v.host
		req.Header.Set("Origin", "http://"+v.host)
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		if v.cookie != "" {
			req.AddCookie(&http.Cookie{Name: lanTokenCookie, Value: v.cookie})
		}
		if v.localCookie {
			addLocalhostTestCookie(t, f.d, req)
		}
		rec := httptest.NewRecorder()
		f.d.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || rec.Body.String() != large {
			t.Fatalf("host image relay as %s = %d", v.name, rec.Code)
		}
	}
}
