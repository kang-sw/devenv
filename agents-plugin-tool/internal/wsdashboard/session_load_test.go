package wsdashboard

import (
	"io"
	"net/http"
	"os/exec"
	"strings"
	"testing"
)

func TestFreshSessionLoadClient(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; run node --test testdata/session-load.test.cjs locally")
	}
	out, err := exec.Command(node, "--test", "testdata/session-load.test.cjs").CombinedOutput()
	if err != nil {
		t.Fatalf("fresh session load: %v\n%s", err, out)
	}
	t.Log(string(out))
}

func TestFreshSessionScriptAsset(t *testing.T) {
	a, err := loadDashboardAssets()
	if err != nil {
		t.Fatal(err)
	}
	f := startPageFixture(t)
	res, err := testClient().Get(f.base + "/s/" + a.data.ScriptFile)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK || string(body) != string(a.data.Script) {
		t.Fatal("content-addressed script does not serve the exact ordered source")
	}
	for key, want := range map[string]string{
		"Cache-Control":          "public, max-age=31536000, immutable",
		"Content-Type":           "text/javascript; charset=utf-8",
		"X-Content-Type-Options": "nosniff",
	} {
		if got := res.Header.Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if code, _ := get(t, f.base+"/s/dashboard-unknown.js"); code != http.StatusNotFound {
		t.Errorf("unknown script hash answered %d", code)
	}
	if code, page := get(t, f.base+"/s/livesession?frame=1"); code != http.StatusOK ||
		!strings.Contains(page, `<script src="/s/`+a.data.ScriptFile+`"></script>`) || strings.Contains(page, string(a.data.Script)) {
		t.Fatal("fresh frame must reference, not inline, the shared main script")
	}
	changed, err := buildDashboardAssets(func(name string) ([]byte, error) {
		data, err := embeddedDashboardAsset(name)
		if name == "dashboard-lifecycle.js" {
			data = append(append([]byte(nil), data...), []byte("\n// cache identity test\n")...)
		}
		return data, err
	})
	if err != nil {
		t.Fatal(err)
	}
	if changed.data.ScriptFile == a.data.ScriptFile {
		t.Fatal("script bytes changed without changing immutable asset URL")
	}
	t.Logf("cached main script: %d raw bytes per warm frame removed from HTML", len(body))
}
