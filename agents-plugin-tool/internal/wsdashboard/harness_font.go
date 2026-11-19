package wsdashboard

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"net/http"
	"sync"
)

// harnessFontWOFF2 is Nunito ExtraBold subset to the harness badge labels
// (assets/harness-font/README.md). The page's stylesheet declares it as
// harnessFontFamily; the .hb rules in dashboard.css name it first and fall
// back to a system rounded stack.
//
//go:embed assets/harness-font/Nunito-ExtraBold-subset.woff2
var harnessFontWOFF2 []byte

// harnessFontLicense embeds the font's OFL text beside it.
//
//go:embed assets/harness-font/OFL.txt
var harnessFontLicense embed.FS

const harnessFontFamily = "WsHarnessBadge"

var harnessFontName = sync.OnceValue(func() string {
	sum := sha256.Sum256(harnessFontWOFF2)
	return "harness-badge-" + hex.EncodeToString(sum[:6]) + ".woff2"
})

// harnessFontFace is the @font-face the dashboard stylesheet starts with. The
// browser fetches the file only when a badge first draws.
func harnessFontFace() string {
	return "@font-face { font-family: \"" + harnessFontFamily + "\"; src: url(\"/s/" + harnessFontName() + "\") format(\"woff2\"); font-weight: 800; font-display: swap; }\n"
}

// serveHarnessFont serves the badge font; its name never changes under its
// content.
func serveHarnessFont(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "font/woff2")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	_, _ = w.Write(harnessFontWOFF2)
}
