package wsdashboard

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"html/template"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// The landing guide's screenshots: real dashboard captures, one per step,
// language (ko, and en for every other language), and theme, which
// testdata/landing-guide.capture.cjs regenerates. guide.json holds each
// image's CSS size. They are served beside the session pages under /s/,
// each name carrying a hash of its bytes, so the landing page inlines only
// their addresses.
//
//go:embed assets/guide/*.webp assets/guide/guide.json
var guideFS embed.FS

// guideShot is one screenshot: its address and CSS size.
type guideShot struct {
	URL  string
	W, H int
}

type guideSet struct {
	// shots is every screenshot by "<lang>-<theme>-<step>".
	shots map[string]guideShot
	// files is every screenshot's bytes by its served name.
	files map[string][]byte
	// id is the hash of every served name, which joins the assets'
	// identity (assetID).
	id string
}

var guideAssets = sync.OnceValue(func() guideSet {
	set := guideSet{shots: map[string]guideShot{}, files: map[string][]byte{}}
	var sizes map[string]struct{ W, H int }
	if raw, err := guideFS.ReadFile("assets/guide/guide.json"); err == nil {
		_ = json.Unmarshal(raw, &sizes)
	}
	entries, _ := guideFS.ReadDir("assets/guide")
	var names []string
	for _, e := range entries {
		file := e.Name()
		if path.Ext(file) != ".webp" {
			continue
		}
		data, err := guideFS.ReadFile("assets/guide/" + file)
		if err != nil {
			continue
		}
		key := strings.TrimSuffix(file, ".webp")
		sum := sha256.Sum256(data)
		name := "guide-" + key + "-" + hex.EncodeToString(sum[:6]) + ".webp"
		set.files[name] = data
		set.shots[key] = guideShot{URL: "/s/" + name, W: sizes[file].W, H: sizes[file].H}
		names = append(names, name)
	}
	sort.Strings(names)
	sum := sha256.Sum256([]byte(strings.Join(names, "\n")))
	set.id = hex.EncodeToString(sum[:8])
	return set
})

// GuideAsset is the landing guide screenshot served as name (a path under
// /s/), if there is one.
func GuideAsset(name string) ([]byte, bool) {
	data, ok := guideAssets().files[name]
	return data, ok
}

// serveGuideAsset serves a guide screenshot; its name never changes under
// its content.
func serveGuideAsset(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", "image/webp")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
}

// guideLang is the screenshot language a page language shows: Korean has
// its own captures, every other language the English ones.
func guideLang(lang string) string {
	if lang == "ko" {
		return "ko"
	}
	return "en"
}

// guideShotHTML is step's screenshot for lang in both themes, the
// stylesheet showing the one for the page's theme (dshot-d on dark,
// dshot-l on light). Both load lazily, so the hidden one is never fetched.
func guideShotHTML(lang, step, alt string) template.HTML {
	set := guideAssets()
	var b strings.Builder
	for _, theme := range []string{"dark", "light"} {
		shot, ok := set.shots[guideLang(lang)+"-"+theme+"-"+step]
		if !ok {
			continue
		}
		b.WriteString(`<img class="dshot-` + theme[:1] + `" src="` + template.HTMLEscapeString(shot.URL) + `"`)
		if shot.W > 0 && shot.H > 0 {
			b.WriteString(` width="` + strconv.Itoa(shot.W) + `" height="` + strconv.Itoa(shot.H) + `"`)
		}
		b.WriteString(` alt="` + template.HTMLEscapeString(alt) + `" loading="lazy" decoding="async">`)
	}
	return template.HTML(b.String())
}
