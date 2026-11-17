package wsdashboard

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsstate"
)

// secretCacheIO counts the reads and stats wsstate.ReadFileCached made, the
// helper the access guard reads both secret files through. The counters are
// process-wide, so a caller bounds them rather than matching them exactly.
func secretCacheIO() (reads, stats int64) {
	for site, n := range wsstate.FileIOCounters() {
		if !strings.HasPrefix(site, "file_io_site:wsstate/read_cache.go:") {
			continue
		}
		switch {
		case strings.HasSuffix(site, ":read_file"):
			reads += n
		case strings.HasSuffix(site, ":stat"):
			stats += n
		}
	}
	return reads, stats
}

// backdateSecret moves path's mtime past the read cache's racy window, as a
// secret minted on an earlier day is, and returns the stamp it set.
func backdateSecret(t *testing.T, path string, age time.Duration) time.Time {
	t.Helper()
	old := time.Now().Add(-age)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	return old
}

// guardCode runs r through the access guard alone, in front of a handler that
// answers 204, and returns the status.
func guardCode(d *Daemon, r *http.Request) int {
	w := httptest.NewRecorder()
	d.accessGuard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(w, r)
	return w.Code
}

// secretCase is one credential the guard checks per request. refused is the
// status the guard answers when the secret file is unreadable.
type secretCase struct {
	name    string
	file    string
	cred    func(*testing.T, *Daemon) string
	req     func(cred string) *http.Request
	refused int
}

// secretCases are a local viewer's localhost cookie and a LAN viewer's LAN
// token.
func secretCases() []secretCase {
	return []secretCase{
		{
			name: "localhost", file: localhostSecretFile,
			cred: func(t *testing.T, d *Daemon) string { return authCookie(t, d).Value },
			req: func(cred string) *http.Request {
				r := authRequest(http.MethodGet, "/future-route", "")
				r.AddCookie(&http.Cookie{Name: localhostCookieName, Value: cred})
				return r
			},
			refused: http.StatusUnauthorized,
		},
		{
			name: "LAN", file: LANTokenFileName,
			cred: func(t *testing.T, d *Daemon) string {
				t.Helper()
				token, err := d.lanToken()
				if err != nil {
					t.Fatal(err)
				}
				return token
			},
			req: func(cred string) *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/future-route", nil)
				r.RemoteAddr = "192.168.7.30:50000"
				r.Host = "192.168.7.20:43210"
				r.AddCookie(&http.Cookie{Name: lanTokenCookie, Value: cred})
				return r
			},
			refused: http.StatusServiceUnavailable,
		},
	}
}

// TestAccessGuardServesSecretsFromMemory: repeated requests revalidate a
// secret file by one stat each and read it only when it changed, and a
// rewrite or a deletion is seen by the very next request.
func TestAccessGuardServesSecretsFromMemory(t *testing.T) {
	for _, c := range secretCases() {
		t.Run(c.name, func(t *testing.T) {
			d := authFixture(t)
			d.lan = true
			path := filepath.Join(d.dir, c.file)
			cred := c.cred(t, d)
			stamp := backdateSecret(t, path, time.Hour)

			reads, stats := secretCacheIO()
			const requests = 5
			for i := 0; i < requests; i++ {
				if code := guardCode(d, c.req(cred)); code != http.StatusNoContent {
					t.Fatalf("request %d = %d, want 204", i, code)
				}
			}
			gotReads, gotStats := secretCacheIO()
			if n := gotReads - reads; n < 1 || n >= requests {
				t.Fatalf("%d requests made %d cached reads, want the one refill after the backdate", requests, n)
			}
			if n := gotStats - stats; n < requests {
				t.Fatalf("%d requests statted the secret file %d times, want one each", requests, n)
			}

			// Proof by content, independent of the counters: an in-place
			// rewrite that keeps mode, size, mtime and file identity is not
			// read, so the cached credential still passes. Only a rewrite
			// that restores the exact stamp hides this way; the rotations
			// below change it.
			same, err := newToken()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(same+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(path, stamp, stamp); err != nil {
				t.Fatal(err)
			}
			if code := guardCode(d, c.req(cred)); code != http.StatusNoContent {
				t.Fatalf("a request after an unseen in-place rewrite = %d, want 204 from memory", code)
			}

			// A rewrite by temp and rename, past the racy window, as a
			// rotation leaves it: the old credential stops passing at once.
			fresh, err := newToken()
			if err != nil {
				t.Fatal(err)
			}
			if err := writePrivateFile(d.dir, c.file, []byte(fresh+"\n")); err != nil {
				t.Fatal(err)
			}
			backdateSecret(t, path, 30*time.Minute)
			if code := guardCode(d, c.req(cred)); code == http.StatusNoContent {
				t.Fatal("the credential rotated away by a rewrite still passes")
			}
			rotated := c.cred(t, d)
			if rotated == cred {
				t.Fatal("the rewrite did not change the credential")
			}
			if code := guardCode(d, c.req(rotated)); code != http.StatusNoContent {
				t.Fatalf("the rewritten credential = %d, want 204", code)
			}

			// A deletion: the old credential stops passing and a new secret
			// is minted, as before the cache.
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if code := guardCode(d, c.req(rotated)); code == http.StatusNoContent {
				t.Fatal("the credential rotated away by deletion still passes")
			}
			if again := c.cred(t, d); again == rotated {
				t.Fatal("deleting the secret file kept the credential")
			}
		})
	}
}

// TestAccessGuardRefusesAnUnreadableSecret: a secret file that stats but
// cannot be read refuses every request, even one whose credential the cache
// accepted a moment before, and is never replaced by a new mint.
func TestAccessGuardRefusesAnUnreadableSecret(t *testing.T) {
	for _, c := range secretCases() {
		t.Run(c.name, func(t *testing.T) {
			d := authFixture(t)
			d.lan = true
			path := filepath.Join(d.dir, c.file)
			cred := c.cred(t, d)
			backdateSecret(t, path, time.Hour)
			if code := guardCode(d, c.req(cred)); code != http.StatusNoContent {
				t.Fatalf("cached credential = %d, want 204", code)
			}

			// A directory in the file's place stats but cannot be read.
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
			// The error refuses before any mint: no replacement is
			// attempted, not merely one that fails on the directory.
			replaces := wsstate.FileIOCounters()["file_io_atomic_replace"]
			for i := 0; i < 2; i++ {
				if code := guardCode(d, c.req(cred)); code != c.refused {
					t.Fatalf("request %d with an unreadable secret = %d, want %d", i, code, c.refused)
				}
			}
			if n := wsstate.FileIOCounters()["file_io_atomic_replace"] - replaces; n != 0 {
				t.Fatalf("an unreadable secret attempted %d replacements, want none", n)
			}
			if info, err := os.Stat(path); err != nil || !info.IsDir() {
				t.Fatalf("the unreadable secret was replaced: %v", err)
			}
		})
	}
}

// TestPersistedSecretConcurrentFirstUseMintsOnce: requests racing the first
// mint agree on one secret, since the miss path rereads under tokenMu.
func TestPersistedSecretConcurrentFirstUseMintsOnce(t *testing.T) {
	d := authFixture(t)
	const callers = 8
	got := make([]string, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			got[i], _, errs[i] = d.persistedSecret(LANTokenFileName)
		}(i)
	}
	wg.Wait()
	for i := range got {
		if errs[i] != nil {
			t.Fatalf("caller %d: %v", i, errs[i])
		}
		if got[i] == "" || got[i] != got[0] {
			t.Fatalf("caller %d got a different secret than caller 0", i)
		}
	}
}
