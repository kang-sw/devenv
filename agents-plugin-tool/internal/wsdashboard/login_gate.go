package wsdashboard

import (
	"bytes"
	"embed"
	"html/template"
	"net/http"
	"strings"

	"github.com/kang-sw/devenv/internal/wsi18n"
)

// The gate is self-contained: a cookieless page must not fetch protected assets.
//
//go:embed assets/login-gate.tmpl assets/login-gate.css assets/login-gate.js
var loginGateFS embed.FS

func (d *Daemon) serveLoginGate(w http.ResponseWriter, r *http.Request, invalid bool) {
	d.serveLoginGateState(w, r, invalid, false)
}

func (d *Daemon) serveLoginGateState(w http.ResponseWriter, r *http.Request, invalid, rateLimited bool) {
	langCookie, _ := r.Cookie(wsi18n.CookieName)
	pref := ""
	if langCookie != nil {
		pref = langCookie.Value
	}
	lang := wsi18n.Resolve(pref, r.Header.Get("Accept-Language"))
	get := func(name string) string {
		raw, _ := loginGateFS.ReadFile("assets/login-gate." + name)
		return string(raw)
	}
	t := template.Must(template.New("gate").Funcs(template.FuncMap{
		"t": func(key string) string { return wsi18n.T(lang, key) },
	}).Parse(get("tmpl")))
	// Only a validated local path is serialized, never a credential or session data.
	path := loginNext(r)
	data := struct {
		Lang, Theme, Next string
		Invalid           bool
		RateLimited       bool
		Clipboard         bool
		CSS               template.CSS
		Script            template.JS
		Logo              template.HTML
	}{Lang: lang, Theme: requestTheme(r), Next: path, Invalid: invalid,
		Clipboard: localViewer(r) && d.clipboardAvailable(), RateLimited: rateLimited,
		CSS: template.CSS(get("css")), Script: template.JS(get("js"))}
	logo, _ := dashboardFS.ReadFile("assets/brand/werkschaft-light.svg")
	data.Logo = template.HTML(strings.Replace(string(logo), `stroke="#1E2235"`, `stroke="currentColor"`, 1))
	var out bytes.Buffer
	if err := t.Execute(&out, data); err != nil {
		http.Error(w, "login page unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; connect-src 'self'; base-uri 'none'; form-action 'self'; frame-ancestors 'self'")
	status := http.StatusUnauthorized
	if rateLimited {
		status = http.StatusTooManyRequests
	}
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(out.Bytes())
	}
}
