package wstrackerhost

import (
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wstracker"
)

func TestRedactsRemoteCredentials(t *testing.T) {
	for in, want := range map[string]string{
		"https://x-access-token:ghp_secret@github.com/o/r.git": "https://***@github.com/o/r.git",
		"fatal: could not read from HTTP://tok@host/r":         "fatal: could not read from HTTP://***@host/r",
		"git@github.com:o/r.git":                               "git@github.com:o/r.git",
		"ssh://git@github.com/o/r.git":                         "ssh://git@github.com/o/r.git",
		"https://github.com/o/r.git":                           "https://github.com/o/r.git",
		"https://user:p@ss@host/x.git":                         "https://***@host/x.git",
		"clone https://u:a@b@c@host/r and https://h/p?q=a@b":   "clone https://***@host/r and https://h/p?q=***",
		"https://host/r.git?private_token=glpat-x&ref=main":    "https://host/r.git?private_token=***&ref=***",
		"fetch 'https://u:p@host/r?access_token=abc' failed":   "fetch 'https://***@host/r?access_token=***' failed",
		"https://host/r.git?sigsecret#frag":                    "https://host/r.git?***#frag",
		"see https://host/docs?q=1 and ssh://h/r?x=1":          "see https://host/docs?q=*** and ssh://h/r?x=1",
	} {
		if got := RedactCredentials(in); got != want {
			t.Fatalf("RedactCredentials(%q) = %q, want %q", in, got, want)
		}
	}
	remote := "https://tok@example.com/r.git"
	v := &StoreView{View: &wstracker.View{}, Store: StoreInfo{Name: "s", Remote: &remote}}
	v.Sync.Error = "push to https://tok@example.com/r.git failed"
	v.Redact()
	if strings.Contains(*v.Store.Remote+v.Sync.Error, "tok") || remote != "https://tok@example.com/r.git" {
		t.Fatalf("the view's remote and error must be redacted without touching the entry: %q %q", *v.Store.Remote, v.Sync.Error)
	}
	(*StoreView)(nil).Redact() // must not panic
	var none StoreView
	none.Redact() // a view with no remote
}
