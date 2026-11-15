package wsagent

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestPermissionHTTPRawBodiesZeroWrites(t *testing.T) {
	h := permissionHarness(t).start()
	st := h.lead(BackendClaude, LeadOptions{})
	for _, target := range []struct{ name, path string }{
		{"create", "/agents/leads/permission"},
		{"settings", "/agents/" + st.ID + "/settings/permission"},
	} {
		for _, body := range []struct{ name, raw string }{
			{"empty", ""}, {"whitespace", " \n\t"}, {"null", "null"},
			{"array", "[]"}, {"string", `"read-only"`}, {"number", "1"}, {"boolean", "true"},
			{"malformed", `{"permission_level":`},
			{"trailing", `{"permission_level":"read-only"} trailing`},
		} {
			t.Run(target.name+"/"+body.name, func(t *testing.T) {
				before := permissionHTTPFiles(t, h.dir)
				keys := h.keys.n
				req := httptest.NewRequest(http.MethodPost, "http://"+localBrowser.host+target.path, strings.NewReader(body.raw))
				req.RemoteAddr, req.Host = localBrowser.remote, localBrowser.host
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Origin", localBrowser.origin)
				res := httptest.NewRecorder()
				h.runner.Handler().ServeHTTP(res, req)
				if res.Code != http.StatusBadRequest {
					t.Fatalf("raw %q: %d %s", body.raw, res.Code, res.Body.String())
				}
				if !reflect.DeepEqual(before, permissionHTTPFiles(t, h.dir)) || h.keys.n != keys || len(h.runner.List()) != 1 || len(h.holders.opList()) != 0 {
					t.Fatalf("raw %q mutated files, keys, conversations or holders", body.raw)
				}
				if got := h.state(st.ID); got.PermissionMode != st.PermissionMode || got.PermissionLevel != st.PermissionLevel || got.Model != st.Model || got.Effort != st.Effort {
					t.Fatalf("partial settings: %+v", got)
				}
			})
		}
	}
}
