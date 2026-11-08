package wstrackerhost

import "regexp"

// remoteCredentials matches the userinfo of an http(s) URL, where a token
// or password sits; an ssh user (git@host) carries none.
var remoteCredentials = regexp.MustCompile(`(?i)(https?://)[^/@\s]+@`)

// RedactCredentials hides the userinfo of every http(s) URL in s, so a
// token in a store's remote URL never reaches an agent's context or a
// browser.
func RedactCredentials(s string) string {
	return remoteCredentials.ReplaceAllString(s, "${1}***@")
}

// Redact redacts the view's remote URL and sync error in place; a nil
// view is left alone.
func (v *StoreView) Redact() {
	if v == nil {
		return
	}
	if v.Store.Remote != nil {
		r := RedactCredentials(*v.Store.Remote)
		v.Store.Remote = &r
	}
	v.Sync.Error = RedactCredentials(v.Sync.Error)
}
