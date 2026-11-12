package wstrackerhost

import "github.com/kang-sw/devenv/internal/wsredact"

// RedactCredentials hides the userinfo and every query value of each
// http(s) URL in s, so a token in a store's remote URL never reaches an
// agent's context or a browser. Query names stay, so the reader can tell
// what was there. The rule lives in wsredact, which the live dashboard's
// argument previews share.
func RedactCredentials(s string) string {
	return wsredact.URLCredentials(s)
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
