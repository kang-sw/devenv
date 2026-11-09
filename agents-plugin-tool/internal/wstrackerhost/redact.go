package wstrackerhost

import (
	"regexp"
	"strings"
)

// remoteCredentials matches the userinfo of an http(s) URL, where a token
// or password sits; an ssh user (git@host) carries none. The userinfo runs
// to the last '@' before the host's path, since a password may hold an
// unencoded '@' (user:p@ss@host).
var remoteCredentials = regexp.MustCompile(`(?i)(https?://)[^/\s]*@`)

// remoteQuery matches the query string of an http(s) URL, where a token can
// sit too (private_token=, access_token=): group 1 is everything up to the
// "?", group 2 the query, which ends at a fragment, whitespace, or quote.
var remoteQuery = regexp.MustCompile(`(?i)(https?://[^\s?#'"]*\?)([^\s#'"]*)`)

// RedactCredentials hides the userinfo and every query value of each
// http(s) URL in s, so a token in a store's remote URL never reaches an
// agent's context or a browser. Query names stay, so the reader can tell
// what was there.
func RedactCredentials(s string) string {
	s = remoteCredentials.ReplaceAllString(s, "${1}***@")
	return remoteQuery.ReplaceAllStringFunc(s, func(url string) string {
		m := remoteQuery.FindStringSubmatch(url)
		params := strings.Split(m[2], "&")
		for i, p := range params {
			if name, _, ok := strings.Cut(p, "="); ok {
				params[i] = name + "=***"
			} else if p != "" {
				params[i] = "***"
			}
		}
		return m[1] + strings.Join(params, "&")
	})
}

// Redact redacts the view's remote URL, sync error, and wiki warnings in
// place; a nil view is left alone.
func (v *StoreView) Redact() {
	if v == nil {
		return
	}
	if v.Store.Remote != nil {
		r := RedactCredentials(*v.Store.Remote)
		v.Store.Remote = &r
	}
	v.Sync.Error = RedactCredentials(v.Sync.Error)
	for i, w := range v.Sync.WikiWarnings {
		v.Sync.WikiWarnings[i] = RedactCredentials(w)
	}
}
