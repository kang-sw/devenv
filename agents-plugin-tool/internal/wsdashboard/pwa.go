package wsdashboard

import (
	"embed"
	"html/template"
	"net/http"
	"strings"
)

// The dashboard installs as an app (a PWA): every dashboard page links the
// web app manifest under pwaPrefix, whose scope is the whole origin, so a
// session page opened from the installed app's index stays in its window.
// There is no service worker: Chromium (Edge and Chrome) no longer requires
// one to offer the install, and the pages are live data that a cache must
// never answer. An installed app is bound to its origin, port included: after
// a dashboard.port change, or a daemon that fell back to another port
// (DaemonOptions.Port), the app still opens the old origin and fails to load;
// installing again from the new origin makes a second app.

//go:embed assets/pwa/manifest.webmanifest assets/pwa/icon-192.png assets/pwa/icon-512.png assets/pwa/icon-maskable-512.png
var pwaFS embed.FS

const pwaPrefix = "/pwa/"

// pwaTypes is every file under pwaPrefix and its content type.
var pwaTypes = map[string]string{
	"manifest.webmanifest":  "application/manifest+json",
	"icon-192.png":          "image/png",
	"icon-512.png":          "image/png",
	"icon-maskable-512.png": "image/png",
}

// The pages' background colours (the whiteboard sheet's --wb-bg), which the
// theme-color metas give the installed app's title bar. The manifest's own
// theme_color and background_color are the dark one.
const (
	themeColorLight = "#f3f3f0"
	themeColorDark  = "#141518"
)

// handlePWA serves the manifest and its icons. They hold nothing about any
// session, but stay behind the access guard like every other route: the page
// links the manifest with credentials, so a LAN viewer's cookie travels.
func handlePWA(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, pwaPrefix)
	ctype, ok := pwaTypes[name]
	if !ok {
		http.NotFound(w, r)
		return
	}
	if !readMethod(w, r) {
		return
	}
	data, err := pwaFS.ReadFile("assets/pwa/" + name)
	if err != nil {
		http.Error(w, "missing asset", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
}

// pwaHead is a page head's manifest link and theme-color metas. Each meta
// carries its scheme's colour (data-c), so the page script can repaint both
// when the theme changes without a reload: a pinned theme (light or dark)
// sets both to its colour, auto lets each follow the OS scheme.
func pwaHead(theme string) template.HTML {
	light, dark := themeColorLight, themeColorDark
	switch theme {
	case "light":
		dark = themeColorLight
	case "dark":
		light = themeColorDark
	}
	return template.HTML(`<link rel="manifest" href="` + pwaPrefix + `manifest.webmanifest" crossorigin="use-credentials">` +
		`<meta name="theme-color" media="(prefers-color-scheme: light)" content="` + light + `" data-scheme="light" data-c="` + themeColorLight + `">` +
		`<meta name="theme-color" media="(prefers-color-scheme: dark)" content="` + dark + `" data-scheme="dark" data-c="` + themeColorDark + `">`)
}
