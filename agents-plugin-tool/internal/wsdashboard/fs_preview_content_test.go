package wsdashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFSPreviewHighlightedLines(t *testing.T) {
	f := startPageFixture(t, lanMode)
	local, _ := fsViewers(t, f)
	for _, tc := range []struct {
		name, text string
		highlight  bool
	}{
		{"main.go", "package main\n\n// <script> is source, not markup\nfunc main() { println(42) }\n", true},
		{"exact.go", "package main\n" + strings.Repeat(" ", hlMaxBytes-len("package main\n")), true},
		{"large.go", "package main\n" + strings.Repeat(" ", hlMaxBytes), false},
		{"unknown.wsunknown", "package main\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writeFile(t, filepath.Join(f.project, tc.name), tc.text)
			rec := serve(f.d, local, http.MethodGet, "/s/livesession/fs/preview?path="+tc.name, "")
			var a fsPreviewAnswer
			if err := json.Unmarshal(rec.Body.Bytes(), &a); err != nil {
				t.Fatal(err)
			}
			if rec.Code != http.StatusOK || a.Kind != "text" || a.Text != tc.text || (len(a.HighlightedLines) > 0) != tc.highlight {
				t.Fatalf("preview: status %d kind %q, text matches %v, highlighted %v", rec.Code, a.Kind, a.Text == tc.text, len(a.HighlightedLines) > 0)
			}
			if !tc.highlight {
				if strings.Contains(rec.Body.String(), `"highlighted_lines"`) {
					t.Fatal("plain fallback must omit the optional field")
				}
				return
			}
			var lines []string
			colored := false
			for _, line := range a.HighlightedLines {
				var s strings.Builder
				for _, p := range line {
					s.WriteString(p.Text)
					colored = colored || p.Class == "k"
				}
				lines = append(lines, s.String())
			}
			if strings.Join(lines, "\n") != tc.text || !colored {
				t.Fatal("highlighting lost source bytes or keyword colors")
			}
		})
	}
	// A lexer may normalize CRLF; never publish tokens that change source.
	text := "package main\r\n// comment\r\n"
	if got := fsHighlightedLines("crlf.go", []byte(text)); got != nil {
		var lines []string
		for _, line := range got {
			var s strings.Builder
			for _, p := range line {
				s.WriteString(p.Text)
			}
			lines = append(lines, s.String())
		}
		if strings.Join(lines, "\n") != text {
			t.Fatal("CRLF was changed")
		}
	}
}

func TestFSPDFPreviewRawAndGuards(t *testing.T) {
	f := startPageFixture(t, lanMode)
	local, lan := fsViewers(t, f)
	const pdf = "%PDF-1.4\nPDF-INSIDE\n%%EOF\n"
	writeFile(t, filepath.Join(f.project, "report.PDF"), pdf)
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "secret.pdf"), "%PDF-1.4\nPDF-OUTSIDE")
	for _, v := range []viewer{local, lan} {
		code, a := fsPreview(t, f, v, "path=report.PDF")
		if code != http.StatusOK || a.Kind != "pdf" || a.Src != "/s/livesession/fs/raw?path=report.PDF" || a.Frame != "" {
			t.Fatalf("%s PDF preview = %d %+v", v.name, code, a)
		}
		rec := serve(f.d, v, http.MethodGet, a.Src, "")
		h := rec.Header()
		if rec.Code != http.StatusOK || rec.Body.String() != pdf || h.Get("Content-Type") != "application/pdf" ||
			h.Get("Content-Security-Policy") != "default-src 'none'; object-src 'self'; frame-src 'self'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'; frame-ancestors 'self'" ||
			h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Referrer-Policy") != "no-referrer" || h.Get("Cache-Control") != "no-cache" || h.Get("Accept-Ranges") != "bytes" {
			t.Errorf("%s PDF raw = %d %v", v.name, rec.Code, h)
		}
		for _, path := range []string{filepath.Join(outside, "secret.pdf"), "../secret.pdf", "missing.pdf"} {
			for _, route := range []string{"preview", "raw"} {
				rec := serve(f.d, v, http.MethodGet, "/s/livesession/fs/"+route+"?path="+url.QueryEscape(path), "")
				want := http.StatusNotFound
				if v.name == local.name && path == filepath.Join(outside, "secret.pdf") {
					want = http.StatusOK
				}
				if rec.Code != want || (want != http.StatusOK && strings.Contains(rec.Body.String(), "PDF-")) {
					t.Errorf("%s %s %q = %d, want %d", v.name, route, path, rec.Code, want)
				}
				if want == http.StatusOK && route == "raw" && !strings.Contains(rec.Body.String(), "PDF-OUTSIDE") {
					t.Errorf("local outside PDF bytes missing: %s", rec.Body.String())
				}
			}
		}
	}
	// The existing explicit-root contract stays local and same-origin only.
	query := "root=" + url.QueryEscape(outside) + "&path=secret.pdf"
	if code, a := fsPreview(t, f, local, query); code != http.StatusOK || a.Kind != "pdf" || !strings.Contains(a.Src, "&root=") {
		t.Fatalf("local explicit-root PDF = %d %+v", code, a)
	} else {
		rec := serve(f.d, local, http.MethodGet, a.Src, "")
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/pdf" ||
			rec.Header().Get("Cross-Origin-Resource-Policy") != "same-origin" {
			t.Errorf("local explicit-root raw = %d %v", rec.Code, rec.Header())
		}
	}
	for _, route := range []string{"preview", "raw"} {
		if rec := serve(f.d, lan, http.MethodGet, "/s/livesession/fs/"+route+"?"+query, ""); rec.Code != http.StatusNotFound {
			t.Errorf("LAN explicit root %s = %d", route, rec.Code)
		}
		req := httptest.NewRequest(http.MethodGet, "/s/livesession/fs/"+route+"?"+query, nil)
		req.Host, req.RemoteAddr = local.host, local.peer
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		rec := httptest.NewRecorder()
		localhostTestHandler(t, f.d).ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("cross-site explicit root %s = %d", route, rec.Code)
		}
	}
	// The browser viewer uses byte ranges, just like video.
	req := httptest.NewRequest(http.MethodGet, "/s/livesession/fs/raw?path=report.PDF", nil)
	req.Host, req.RemoteAddr = local.host, local.peer
	req.Header.Set("Range", "bytes=0-7")
	rec := httptest.NewRecorder()
	localhostTestHandler(t, f.d).ServeHTTP(rec, req)
	if rec.Code != http.StatusPartialContent || rec.Body.String() != pdf[:8] || rec.Header().Get("Content-Type") != "application/pdf" {
		t.Errorf("PDF range = %d %q %v", rec.Code, rec.Body.String(), rec.Header())
	}
	t.Run("symlinks", func(t *testing.T) {
		for name, target := range map[string]string{
			"alias.pdf":  filepath.Join(f.project, "report.PDF"),
			"escape.pdf": filepath.Join(outside, "secret.pdf"),
			"outlink":    outside,
		} {
			if err := os.Symlink(target, filepath.Join(f.project, name)); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
		}
		for _, v := range []viewer{local, lan} {
			for _, path := range []string{"alias.pdf", "escape.pdf", "outlink/secret.pdf"} {
				for _, route := range []string{"preview", "raw"} {
					rec := serve(f.d, v, http.MethodGet, "/s/livesession/fs/"+route+"?path="+url.QueryEscape(path), "")
					want := http.StatusNotFound
					if v.name == local.name {
						want = http.StatusOK
					}
					if rec.Code != want || (want == http.StatusNotFound && strings.Contains(rec.Body.String(), "PDF-")) {
						t.Errorf("%s symlink %s %s = %d, want %d", v.name, route, path, rec.Code, want)
					}
					if want == http.StatusOK && route == "raw" && !strings.Contains(rec.Body.String(), "PDF-") {
						t.Errorf("local PDF symlink bytes missing: %s", rec.Body.String())
					}
				}
			}
		}
	})
}

func TestFSRawExistingMediaHeaders(t *testing.T) {
	f := startPageFixture(t, lanMode)
	local, _ := fsViewers(t, f)
	for name, ctype := range map[string]string{"a.png": "image/png", "a.svg": "image/svg+xml", "a.mp4": "video/mp4"} {
		writeFile(t, filepath.Join(f.project, name), "existing-media")
		rec := serve(f.d, local, http.MethodGet, "/s/livesession/fs/raw?path="+name, "")
		h := rec.Header()
		if rec.Code != http.StatusOK || h.Get("Content-Type") != ctype ||
			h.Get("Content-Security-Policy") != "default-src 'none'; style-src 'unsafe-inline'; sandbox" ||
			h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Cache-Control") != "no-cache" || h.Get("Referrer-Policy") != "no-referrer" {
			t.Errorf("%s headers changed: %d %v", name, rec.Code, h)
		}
	}
}
