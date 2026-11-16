package wsdashboard

import (
	"net/http"
	"testing"
)

func TestLocalhostAuthForeignNavigationsDoNotSpendExchangeBudget(t *testing.T) {
	for _, tc := range []struct {
		name, site, origin, referer string
	}{
		{"cross-site", "cross-site", "", "http://foreign.example/"},
		{"same-site other port", "same-site", "", "http://127.0.0.1:1/"},
		{"none", "none", "", ""},
		{"none foreign Origin", "none", "http://foreign.example", ""},
		{"none foreign Referer", "none", "", "http://foreign.example/"},
		{"missing metadata", "", "", ""},
		{"mismatched Origin", "same-origin", "http://foreign.example", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := authFixture(t)
			_, code := authLink(t, d, "/")
			for i := 0; i < 3*loginFailureLimit; i++ {
				r := authRequest(http.MethodGet, "/?login=wrong", "")
				r.Header.Set("Sec-Fetch-Mode", "navigate")
				if tc.site != "" {
					r.Header.Set("Sec-Fetch-Site", tc.site)
				}
				if tc.origin != "" {
					r.Header.Set("Origin", tc.origin)
				}
				if tc.referer != "" {
					r.Header.Set("Referer", tc.referer)
				}
				if w := authServe(d, r); w.Code != http.StatusUnauthorized {
					t.Fatalf("foreign navigation %d = %d", i, w.Code)
				}
			}
			if d.auth.failures != 0 {
				t.Fatalf("foreign navigations spent %d failures", d.auth.failures)
			}
			if w := authServe(d, exchangeRequest("wrong", "/")); w.Code != 401 || d.auth.failures != 1 {
				t.Fatalf("own-origin failure no longer counts: %d, failures=%d", w.Code, d.auth.failures)
			}
			if w := authServe(d, exchangeRequest(code, "/")); w.Code != http.StatusOK {
				t.Fatalf("foreign navigation flood blocked paste: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestLocalhostAuthSameOriginNavigationSharesExchangeBudget(t *testing.T) {
	d := authFixture(t)
	for i := 0; i < loginFailureLimit; i++ {
		r := authRequest(http.MethodGet, "/?login=wrong", "")
		r.Header.Set("Sec-Fetch-Mode", "navigate")
		r.Header.Set("Sec-Fetch-Site", "same-origin")
		if w := authServe(d, r); w.Code != http.StatusUnauthorized {
			t.Fatalf("own-origin navigation %d = %d", i, w.Code)
		}
	}
	_, code := authLink(t, d, "/")
	if w := authServe(d, exchangeRequest(code, "/")); w.Code != http.StatusTooManyRequests {
		t.Fatalf("same-origin navigation failures bypassed budget: %d", w.Code)
	}
}

func TestLocalhostAuthHEADDoesNotConsumeLoginCode(t *testing.T) {
	for _, authenticated := range []bool{false, true} {
		for _, metadata := range []bool{false, true} {
			t.Run(map[bool]string{false: "cookieless", true: "authenticated"}[authenticated]+"/"+map[bool]string{false: "Accept", true: "navigate"}[metadata], func(t *testing.T) {
				d := authFixture(t)
				d.agents = &authAgentRouteProbe{}
				link, _ := authLink(t, d, "/agents?panel=1")
				r := authRequest(http.MethodHead, link, "")
				r.Header.Set("Accept", "text/html")
				if metadata {
					r.Header.Set("Sec-Fetch-Mode", "navigate")
					r.Header.Set("Sec-Fetch-Site", "same-origin")
				}
				if authenticated {
					r.AddCookie(authCookie(t, d))
				}
				w := authServe(d, r)
				want := http.StatusUnauthorized
				if authenticated {
					want = http.StatusNoContent
				}
				if w.Code != want || len(w.Result().Cookies()) != 0 || w.Header().Get("Location") != "" || w.Body.Len() != 0 {
					t.Fatalf("HEAD exchanged or redirected: %d %v %s", w.Code, w.Header(), w.Body.String())
				}
				if len(d.auth.codes) != 1 || d.auth.failures != 0 {
					t.Fatal("HEAD mutated login codes or failure budget")
				}
				r.Method = http.MethodGet
				if w = authServe(d, r); w.Code != http.StatusSeeOther || len(w.Result().Cookies()) != 1 || len(d.auth.codes) != 0 {
					t.Fatalf("GET could not exchange the previewed code: %d %v", w.Code, w.Header())
				}
			})
		}
	}
}
