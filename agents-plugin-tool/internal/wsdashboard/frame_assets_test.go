package wsdashboard

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsi18n"
)

// frameDependencies expands only prerequisites, never the main script. Tests
// that inspect module hooks/order deliberately follow the frame's new URLs.
// TestFramedSessionAssets separately verifies their HTTP and placement contract.
func frameDependencies(t *testing.T, page string) string {
	t.Helper()
	a, err := loadDashboardAssets()
	if err != nil {
		t.Fatal(err)
	}
	for name, file := range a.files {
		if name == a.data.ScriptFile {
			continue
		}
		page = strings.ReplaceAll(page, `<script src="/s/`+name+`"></script>`, "<script>"+file.body+"</script>")
		page = strings.ReplaceAll(page, `<link rel="stylesheet" href="/s/`+name+`">`, "<style>"+file.body+"</style>")
	}
	return page
}

func readBody(t *testing.T, res *http.Response) string {
	t.Helper()
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestFramedSessionAssets(t *testing.T) {
	f := startPageFixture(t)
	a, err := loadDashboardAssets()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(a.files[a.data.FrameScriptFile].body, string(a.data.ConfirmScript)+"\n;\n") {
		t.Fatal("frame shortcuts must load after the confirmation dialog's Escape handler")
	}
	for _, lang := range wsi18n.Languages() {
		t.Run(lang.Code, func(t *testing.T) {
			req, _ := http.NewRequest("GET", f.base+"/s/livesession?frame=1", nil)
			req.Header.Set("Accept-Language", lang.Code)
			res, err := testClient().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			page := readBody(t, res)
			if strings.Contains(page, "<script>") || strings.Contains(page, "<style>") {
				t.Fatal("framed document still contains inline executable/style assets")
			}
			// Topic srcdocs use fixed public assets because their opaque
			// origin sends no cookie. Every referenced frame hash stays public.
			for _, name := range regexp.MustCompile(`wb-frame-[0-9a-f]+\.(?:js|css)`).FindAllString(page, -1) {
				got := authServe(f.d, authRequest("GET", "/s/"+name, ""))
				if got.Code != 200 || got.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
					t.Fatalf("cookieless frame asset %s = %d", name, got.Code)
				}
			}
			if !strings.Contains(page, `<symbol id="i-`) || !strings.Contains(page, `<symbol id="l-`) {
				t.Fatal("same-document sprites must remain inline")
			}
			want := []string{a.data.FrameI18nFiles[lang.Code], a.data.FrameScriptFile, a.data.FrameWebFile, a.data.FrameSelectFile, a.data.ScriptFile}
			at := -1
			for _, name := range want {
				i := strings.Index(page, `<script src="/s/`+name+`"></script>`)
				if i <= at {
					t.Fatalf("script %s missing or out of order", name)
				}
				at = i
			}
		})
	}
	// All resources, including optional terminal and every language table, use
	// exactly the main-script caching/content identity and authorization rules.
	for name, file := range a.files {
		res, err := testClient().Get(f.base + "/s/" + name)
		if err != nil {
			t.Fatal(err)
		}
		body := readBody(t, res)
		if res.StatusCode != 200 || body != file.body || res.Header.Get("Content-Type") != file.ctype || res.Header.Get("Cache-Control") != "public, max-age=31536000, immutable" || res.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("immutable asset %s: status=%d headers=%v", name, res.StatusCode, res.Header)
		}
		if !regexp.MustCompile(`^dashboard-[0-9a-f]{24}\.(js|css)$`).MatchString(name) {
			t.Fatal(name)
		}
		local := authRequest("GET", "/s/"+name, "")
		local.Header.Set("Sec-Fetch-Mode", "cors")
		if got := authServe(f.d, local); got.Code != 401 {
			t.Fatalf("cookieless asset %s = %d", name, got.Code)
		}
		local.AddCookie(authCookie(t, f.d))
		if got := authServe(f.d, local); got.Code != 200 {
			t.Fatalf("native-cookie asset %s = %d", name, got.Code)
		}
	}
	if code, _ := get(t, f.base+"/s/dashboard-unknown.css"); code != 404 {
		t.Fatal(code)
	}
	// LAN uses the same gated URLs, not publicAsset's cookieless frame exemption.
	lan := startPageFixture(t, func(o *DaemonOptions) { o.LAN = true })
	token, err := lan.d.lanToken()
	if err != nil {
		t.Fatal(err)
	}
	for name := range a.files {
		r := authRequest("GET", "/s/"+name, "")
		r.Host, r.RemoteAddr = fmt.Sprintf("192.0.2.1:%d", lan.d.endpoint.Port), "192.0.2.2:12345"
		r.Header.Set("Sec-Fetch-Mode", "cors")
		if got := authServe(lan.d, r); got.Code == 200 {
			t.Fatalf("unauthenticated LAN asset %s", name)
		}
		r.AddCookie(&http.Cookie{Name: lanTokenCookie, Value: token})
		if got := authServe(lan.d, r); got.Code != 200 {
			t.Fatalf("LAN asset %s = %d", name, got.Code)
		}
	}
}
