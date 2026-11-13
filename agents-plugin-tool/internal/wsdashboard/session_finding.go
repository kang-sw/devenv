package wsdashboard

import (
	"html/template"
	"net/http"
	"strings"

	"github.com/kang-sw/devenv/internal/wsi18n"
)

// A session page asked for right after the session was created (the new
// session form navigates the moment the spawn answers 201) can precede the
// session's registration, which its ws-mcp writes a moment later. The page
// route answers a well-formed but unknown reply-id with this page rather
// than a bare 404: a "Finding session" spinner that re-asks the same URL
// every findingRetryMs and reloads once it is served, then settles on the
// not-found message after findingGiveUpMs so a session that never appears
// does not spin forever. The status stays 404: the session is unknown now.
const (
	findingRetryMs  = 1500
	findingGiveUpMs = 10000
)

var findingTmpl = template.Must(template.New("finding").Parse(`<!DOCTYPE html>
<html data-theme="{{.Theme}}" data-lang="{{.Lang}}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Finding}}</title>
<style>
:root{color-scheme:light dark;--bg:#fff;--fg:#1f2328;--dim:#656d76}
@media(prefers-color-scheme:dark){:root:not([data-theme=light]){--bg:#0d1117;--fg:#e6edf3;--dim:#8b949e}}
:root[data-theme=dark]{color-scheme:dark;--bg:#0d1117;--fg:#e6edf3;--dim:#8b949e}
:root[data-theme=light]{color-scheme:light}
html,body{height:100%;margin:0}
body{display:flex;align-items:center;justify-content:center;background:var(--bg);color:var(--fg);font:15px system-ui,sans-serif}
#finding{display:flex;align-items:center;gap:12px;color:var(--dim)}
.spin{width:18px;height:18px;border:2px solid var(--dim);border-top-color:transparent;border-radius:50%;animation:s 1s linear infinite}
@keyframes s{to{transform:rotate(360deg)}}
@media(prefers-reduced-motion:reduce){.spin{animation-duration:3s}}
#finding.gone .spin{display:none}
</style>
</head>
<body>
<div id="finding" role="status" aria-live="polite" data-retry="{{.RetryMs}}" data-giveup="{{.GiveUpMs}}" data-notfound="{{.NotFound}}"><span class="spin"></span><span id="findmsg">{{.Finding}}</span></div>
<script>
(function () {
  var box = document.getElementById("finding"), msg = document.getElementById("findmsg");
  var retry = +box.getAttribute("data-retry"), giveUp = +box.getAttribute("data-giveup"), t0 = Date.now();
  function tick() {
    if (Date.now() - t0 >= giveUp) {
      box.className = "gone";
      msg.textContent = box.getAttribute("data-notfound");
      document.title = msg.textContent;
      return;
    }
    fetch(location.href, { credentials: "same-origin", cache: "no-store" }).then(function (r) {
      if (r.status === 200) { location.reload(); return; }
      setTimeout(tick, retry);
    }, function () { setTimeout(tick, retry); });
  }
  setTimeout(tick, retry);
})();
</script>
</body>
</html>
`))

// serveFindingSession answers a session page for a well-formed reply-id the
// daemon does not hold yet with the finding page, and asks the index to read
// that session's registration now so the next poll need not wait for the
// polling backstop.
func (d *Daemon) serveFindingSession(w http.ResponseWriter, r *http.Request, replyID, lang string) {
	if d.index != nil {
		d.index.Reread(replyID)
	}
	var b strings.Builder
	_ = findingTmpl.Execute(&b, map[string]any{
		"Theme":    requestTheme(r),
		"Lang":     lang,
		"Finding":  wsi18n.T(lang, "session.finding"),
		"NotFound": wsi18n.T(lang, "session.notfound"),
		"RetryMs":  findingRetryMs,
		"GiveUpMs": findingGiveUpMs,
	})
	writeHTMLStatus(w, http.StatusNotFound, b.String())
}
