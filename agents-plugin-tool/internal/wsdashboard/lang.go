package wsdashboard

import (
	"net/http"

	"github.com/kang-sw/devenv/internal/wsi18n"
)

// requestLang is the language a request's page renders in and the language
// choice behind it: the gear menu's cookie when it names an available
// language (pref is that code), else the browser's Accept-Language (pref is
// empty: the page follows the browser), else English. The server reads the
// choice so a page, its stream, and its history pages all render in one
// language from the first byte, with no switch after load.
func requestLang(r *http.Request) (lang, pref string) {
	cookie := ""
	if c, err := r.Cookie(wsi18n.CookieName); err == nil {
		cookie = c.Value
	}
	return wsi18n.Resolve(cookie, r.Header.Get("Accept-Language")), wsi18n.Pref(cookie)
}
