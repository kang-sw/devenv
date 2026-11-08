package wsdashboard

import "net/http"

// themeCookie is the browser cookie holding the gear menu's theme choice:
// "light" or "dark". No cookie means auto, the page following the OS. The
// server reads it so a page renders its root data-theme from the first byte,
// with no flash of the wrong theme; the page script writes it.
const themeCookie = "ws_dashboard_theme"

// requestTheme is the theme a request pins: "light" or "dark" when the cookie
// says so, else "" (auto). Any other cookie value is auto.
func requestTheme(r *http.Request) string {
	c, err := r.Cookie(themeCookie)
	if err != nil {
		return ""
	}
	switch c.Value {
	case "light", "dark":
		return c.Value
	}
	return ""
}
