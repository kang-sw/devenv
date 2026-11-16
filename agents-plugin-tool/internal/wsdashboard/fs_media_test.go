package wsdashboard

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestFSMediaPreviewAndRange(t *testing.T) {
	f := startPageFixture(t, lanMode)
	local, lan := fsViewers(t, f)
	body := strings.Repeat("media", 100)
	for _, tt := range []struct {
		name, kind, ctype string
	}{
		{"track.mp3", "audio", "audio/mpeg"},
		{"track.wav", "audio", "audio/wav"},
		{"track.ogg", "audio", "audio/ogg"},
		{"track.oga", "audio", "audio/ogg"},
		{"track.flac", "audio", "audio/flac"},
		{"track.m4a", "audio", "audio/mp4"},
		{"track.aac", "audio", "audio/aac"},
		{"track.opus", "audio", "audio/ogg"},
		{"clip.MOV", "video", "video/quicktime"},
		{"clip.ogv", "video", "video/ogg"},
		{"clip.m4v", "video", "video/mp4"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			writeFile(t, filepath.Join(f.project, tt.name), body)
			for _, v := range []viewer{local, lan} {
				code, a := fsPreview(t, f, v, "path="+url.QueryEscape(tt.name))
				if code != http.StatusOK || a.Kind != tt.kind || a.Src != fsURL("livesession", "raw", tt.name, "") {
					t.Fatalf("%s: preview = %d %+v", v.name, code, a)
				}
				req := httptest.NewRequest(http.MethodGet, a.Src, nil)
				req.Host, req.RemoteAddr = v.host, v.peer
				if v.cookie != "" {
					req.AddCookie(&http.Cookie{Name: lanTokenCookie, Value: v.cookie})
				}
				req.Header.Set("Range", "bytes=10-19")
				rec := httptest.NewRecorder()
				localhostTestHandler(t, f.d).ServeHTTP(rec, req)
				if rec.Code != http.StatusPartialContent || rec.Header().Get("Content-Type") != tt.ctype ||
					rec.Header().Get("Content-Range") != "bytes 10-19/500" || rec.Body.String() != body[10:20] ||
					rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("Content-Security-Policy") != imageContentSecurityPolicy {
					t.Errorf("%s: raw = %d %v %q", v.name, rec.Code, rec.Header(), rec.Body.String())
				}
			}
		})
	}
}

func TestFSOutsideMediaPreview(t *testing.T) {
	o := startOutsideFixture(t)
	for _, tt := range []struct {
		name, kind, ctype string
	}{
		{"track.mp3", "audio", "audio/mpeg"},
		{"track.ogg", "audio", "audio/ogg"},
		{"track.flac", "audio", "audio/flac"},
		{"track.m4a", "audio", "audio/mp4"},
		{"track.opus", "audio", "audio/ogg"},
		{"clip.mov", "video", "video/quicktime"},
		{"clip.ogv", "video", "video/ogg"},
		{"clip.m4v", "video", "video/mp4"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := o.write(t, o.away, tt.name, "MEDIA")
			a := o.linked(t, path)
			if a.Kind != tt.kind || !strings.HasPrefix(a.Src, "/s/livesession/fs/ocap/") {
				t.Fatalf("outside preview = %+v", a)
			}
			rec := o.read(a.Src)
			if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != tt.ctype || rec.Body.String() != "MEDIA" {
				t.Errorf("outside media = %d %v %q", rec.Code, rec.Header(), rec.Body.String())
			}
			if code, _ := o.open(t, o.lan, path); code != http.StatusNotFound {
				t.Errorf("LAN outside preview = %d, want 404", code)
			}
		})
	}
}
