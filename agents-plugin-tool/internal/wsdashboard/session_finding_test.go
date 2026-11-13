package wsdashboard

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// A page asked for before its session registered gets the finding page (404,
// the spinner, the retry and give-up window), and once the session is
// registered the same URL serves the real page, which is what the finding
// page's poll waits for. A malformed reply-id stays a bare 404.
func TestSessionPageFindingSpinnerUntilRegistered(t *testing.T) {
	f := startPageFixture(t)
	code, body := get(t, f.base+"/s/newlead")
	if code != http.StatusNotFound {
		t.Fatalf("GET /s/newlead before registration = %d, want 404", code)
	}
	for _, want := range []string{"Finding session…", `data-retry="1500"`, `data-giveup="10000"`, `data-notfound="Unknown session"`, "location.reload()"} {
		if !strings.Contains(body, want) {
			t.Errorf("finding page lacks %q", want)
		}
	}
	if _, body := getLang(t, f.base+"/s/newlead", "ko", ""); !strings.Contains(body, "세션을 찾는 중…") {
		t.Errorf("finding page is not localized: %s", body)
	}
	if code, body := get(t, f.base+"/s/bad%20id"); code != http.StatusNotFound || strings.Contains(body, "Finding") {
		t.Errorf("malformed id = %d, want a bare 404", code)
	}

	register(t, f.dir, "newlead", f.project, time.Now())
	f.src.setPage("newlead", PageContent{HasBoard: true, Seq: 1, Board: boardRegions(t, "new lead board")})
	waitFor(t, "the finding poll to be served the page", func() bool {
		code, body := get(t, f.base+"/s/newlead")
		return code == http.StatusOK && !strings.Contains(body, "Finding session")
	})
}
