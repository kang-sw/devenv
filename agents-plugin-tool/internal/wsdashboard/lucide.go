package wsdashboard

import (
	"embed"
	"fmt"
	"html/template"
	"regexp"
	"sort"
	"strings"
)

// The vendored Lucide icons (assets/lucide: the upstream files, unchanged,
// with the package's license). The session page inlines them as one sprite,
// a symbol l-<name> per icon, which the card framework and the file
// explorer draw by reference; the dashboard's own hand-drawn sprite
// (sprite.svg, symbols i-<name>) stays what the rest of the page uses.
//
//go:embed assets/lucide/icons/*.svg
var lucideFS embed.FS

// lucideLicense embeds the icons' license beside them, so a build that
// carries the icons carries it.
//
//go:embed assets/lucide/LICENSE
var lucideLicense embed.FS

var (
	lucideBody  = regexp.MustCompile(`(?s)<svg\b[^>]*>(.*)</svg>`)
	lucideSpace = regexp.MustCompile(`\s*\n\s*`)
	lucideName  = regexp.MustCompile(`^[a-z0-9-]+$`)
)

// lucideIcons returns the vendored icons' names, sorted, and each icon's
// drawing (the children of its svg element).
func lucideIcons() ([]string, map[string]string, error) {
	const dir = "assets/lucide/icons"
	entries, err := lucideFS.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}
	var names []string
	body := map[string]string{}
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".svg")
		if !lucideName.MatchString(name) {
			return nil, nil, fmt.Errorf("lucide icon %q: not an icon name", e.Name())
		}
		raw, err := lucideFS.ReadFile(dir + "/" + e.Name())
		if err != nil {
			return nil, nil, err
		}
		m := lucideBody.FindSubmatch(raw)
		if m == nil {
			return nil, nil, fmt.Errorf("lucide icon %q: no svg element", e.Name())
		}
		names = append(names, name)
		body[name] = strings.TrimSpace(lucideSpace.ReplaceAllString(string(m[1]), ""))
	}
	sort.Strings(names)
	return names, body, nil
}

// lucideSprite is the inline sprite of the vendored icons. Every Lucide
// icon is drawn on a 24-unit grid with a round 2-unit stroke; the
// stylesheet's .li rule carries the stroke.
func lucideSprite() (template.HTML, error) {
	names, body, err := lucideIcons()
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(`<svg width="0" height="0" style="position:absolute" aria-hidden="true">`)
	for _, name := range names {
		b.WriteString(`<symbol id="l-` + name + `" viewBox="0 0 24 24">` + body[name] + `</symbol>`)
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String()), nil
}
