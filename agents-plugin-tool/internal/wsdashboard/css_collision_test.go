package wsdashboard

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The dashboard page inlines several module stylesheets into one document, so
// a class defined in two of them is one rule set to the browser: the
// whiteboard's .tk badge once restyled the tracker's .tk root into a 17px
// pill. Each module stylesheet declares its prefix(es) in a header comment
// ("class prefix: ..."); a NEW class takes its module's prefix. Existing
// classes are not renamed, so intentional overlaps are allowlisted below.

// root, when set, is the module's scope class: a selector that starts with it
// only matches inside the module's own subtree, so only the first compound
// (the root and its variants) is a page-global name.
var cssModules = []struct {
	name, path, prefix, root string
}{
	{"dashboard", "assets/dashboard.css", "d", ""},
	{"memo", "assets/memo.css", "memo", ""},
	{"tracker", "assets/tracker.css", "tk", "tk"},
	{"whiteboard", "../wswhiteboard/assets/page.css", "wb", ""},
}

// allowedCSSOverlaps maps "class module,module" (modules sorted) to the reason
// those modules may both define it. A new module joining the class is a new
// collision and fails.
var allowedCSSOverlaps = map[string]string{
	"rtext dashboard,whiteboard":        "overview clamps the existing whiteboard recap, scoped to data-overview",
	"trecap dashboard,whiteboard":       "overview clamps the existing whiteboard recap, scoped to data-overview",
	"tnote dashboard,whiteboard":        "overview hides existing whiteboard note fields without changing their state",
	"tnote-toggle dashboard,whiteboard": "overview fades existing whiteboard head actions, scoped to data-overview",
	"achip dashboard,tracker":           "pre-existing dashboard and tracker overlap (host styles the tracker mount), grandfathered",
	"ack tracker,whiteboard":            "pre-existing tracker and whiteboard overlap, grandfathered",
	"agent dashboard,whiteboard":        "dashboard extends the whiteboard board vocabulary it is inlined after",
	"amk dashboard,tracker":             "pre-existing dashboard and tracker overlap (host styles the tracker mount), grandfathered",
	"ap dashboard,tracker":              "pre-existing dashboard and tracker overlap (host styles the tracker mount), grandfathered",
	"arow dashboard,whiteboard":         "dashboard extends the whiteboard board vocabulary it is inlined after",
	"arr dashboard,whiteboard":          "dashboard extends the whiteboard board vocabulary it is inlined after",
	"body dashboard,whiteboard":         "dashboard extends the whiteboard board vocabulary it is inlined after",
	"btn dashboard,tracker,whiteboard":  "shared host atom restyled per module",
	"card dashboard,whiteboard":         "dashboard extends the whiteboard board vocabulary it is inlined after",
	"cbar dashboard,whiteboard":         "dashboard extends the whiteboard board vocabulary it is inlined after",
	"cbody dashboard,whiteboard":        "dashboard extends the whiteboard board vocabulary it is inlined after",
	"chead dashboard,whiteboard":        "dashboard extends the whiteboard board vocabulary it is inlined after",
	"chip dashboard,tracker,whiteboard": "shared host atom restyled per module",
	"ckrow dashboard,whiteboard":        "dashboard extends the whiteboard board vocabulary it is inlined after",
	"counts dashboard,whiteboard":       "dashboard extends the whiteboard board vocabulary it is inlined after",
	"danger dashboard,tracker":          "pre-existing dashboard and tracker overlap (host styles the tracker mount), grandfathered",
	"dash dashboard,whiteboard":         "dashboard extends the whiteboard board vocabulary it is inlined after",
	"del dashboard,whiteboard":          "dashboard extends the whiteboard board vocabulary it is inlined after",
	"dn dashboard,whiteboard":           "dashboard extends the whiteboard board vocabulary it is inlined after",
	"doc-body dashboard,whiteboard":     "dashboard extends the whiteboard board vocabulary it is inlined after",
	"doc-frame dashboard,whiteboard":    "dashboard extends the whiteboard board vocabulary it is inlined after",
	"done dashboard,whiteboard":         "dashboard extends the whiteboard board vocabulary it is inlined after",
	"drag dashboard,tracker,whiteboard": "shared host atom restyled per module",
	"err dashboard,tracker":             "pre-existing dashboard and tracker overlap (host styles the tracker mount), grandfathered",
	"entry-panel dashboard,whiteboard":  "dashboard extends the whiteboard board vocabulary it is inlined after",
	"flash dashboard,whiteboard":        "dashboard extends the whiteboard board vocabulary it is inlined after",
	"framed dashboard,whiteboard":       "dashboard extends the whiteboard board vocabulary it is inlined after",
	"go dashboard,tracker":              "pre-existing dashboard and tracker overlap (host styles the tracker mount), grandfathered",
	"head dashboard,whiteboard":         "dashboard extends the whiteboard board vocabulary it is inlined after",
	"i dashboard,tracker,whiteboard":    "shared host atom restyled per module",
	"ia dashboard,tracker":              "pre-existing dashboard and tracker overlap (host styles the tracker mount), grandfathered",
	"idcopy dashboard,tracker":          "pre-existing dashboard and tracker overlap (host styles the tracker mount), grandfathered",
	"k dashboard,whiteboard":            "dashboard extends the whiteboard board vocabulary it is inlined after",
	"lbl dashboard,tracker,whiteboard":  "shared host atom restyled per module",
	"live dashboard,whiteboard":         "dashboard extends the whiteboard board vocabulary it is inlined after",
	"ln dashboard,whiteboard":           "dashboard extends the whiteboard board vocabulary it is inlined after",
	"md dashboard,whiteboard":           "dashboard extends the whiteboard board vocabulary it is inlined after",
	"mh dashboard,tracker":              "pre-existing dashboard and tracker overlap (host styles the tracker mount), grandfathered",
	"mini dashboard,whiteboard":         "dashboard extends the whiteboard board vocabulary it is inlined after",
	"mk dashboard,tracker":              "pre-existing dashboard and tracker overlap (host styles the tracker mount), grandfathered",
	"more dashboard,whiteboard":         "dashboard extends the whiteboard board vocabulary it is inlined after",
	"msep dashboard,tracker":            "pre-existing dashboard and tracker overlap (host styles the tracker mount), grandfathered",
	"msg dashboard,whiteboard":          "dashboard extends the whiteboard board vocabulary it is inlined after",
	"nm dashboard,whiteboard":           "dashboard extends the whiteboard board vocabulary it is inlined after",
	"no dashboard,whiteboard":           "dashboard extends the whiteboard board vocabulary it is inlined after",
	"none dashboard,whiteboard":         "dashboard extends the whiteboard board vocabulary it is inlined after",
	"note dashboard,whiteboard":         "dashboard extends the whiteboard board vocabulary it is inlined after",
	"nt dashboard,whiteboard":           "dashboard extends the whiteboard board vocabulary it is inlined after",
	"off dashboard,tracker":             "pre-existing dashboard and tracker overlap (host styles the tracker mount), grandfathered",
	"ok dashboard,whiteboard":           "dashboard extends the whiteboard board vocabulary it is inlined after",
	"on dashboard,tracker,whiteboard":   "shared host atom restyled per module",
	"open dashboard,tracker,whiteboard": "shared host atom restyled per module",
	"opt dashboard,whiteboard":          "dashboard extends the whiteboard board vocabulary it is inlined after",
	"panel dashboard,whiteboard":        "dashboard extends the whiteboard board vocabulary it is inlined after",
	"pick tracker,whiteboard":           "pre-existing tracker and whiteboard overlap, grandfathered",
	"quiet dashboard,whiteboard":        "dashboard extends the whiteboard board vocabulary it is inlined after",
	"rail dashboard,memo":               "memo pad hooks the dashboard rail state",
	"rail-open dashboard,memo":          "memo pad hooks the dashboard rail state",
	"reply dashboard,whiteboard":        "dashboard extends the whiteboard board vocabulary it is inlined after",
	"rl dashboard,whiteboard":           "dashboard extends the whiteboard board vocabulary it is inlined after",
	"rscrim dashboard,memo":             "memo pad hooks the dashboard rail state",
	"sel dashboard,tracker":             "pre-existing dashboard and tracker overlap (host styles the tracker mount), grandfathered",
	"srow dashboard,whiteboard":         "dashboard extends the whiteboard board vocabulary it is inlined after",
	"stext dashboard,whiteboard":        "dashboard extends the whiteboard board vocabulary it is inlined after",
	"sub dashboard,whiteboard":          "dashboard extends the whiteboard board vocabulary it is inlined after",
	"tbar dashboard,whiteboard":         "dashboard extends the whiteboard board vocabulary it is inlined after",
	"tf dashboard,tracker":              "pre-existing dashboard and tracker overlap (host styles the tracker mount), grandfathered",
	"title dashboard,whiteboard":        "dashboard extends the whiteboard board vocabulary it is inlined after",
	"tk dashboard,tracker":              "pre-existing dashboard and tracker overlap (host styles the tracker mount), grandfathered",
	"tk-paged dashboard,tracker":        "pre-existing dashboard and tracker overlap (host styles the tracker mount), grandfathered",
	"tk-spage dashboard,tracker":        "pre-existing dashboard and tracker overlap (host styles the tracker mount), grandfathered",
	"top dashboard,whiteboard":          "dashboard extends the whiteboard board vocabulary it is inlined after",
	"tophead dashboard,whiteboard":      "dashboard extends the whiteboard board vocabulary it is inlined after",
	"topic dashboard,whiteboard":        "dashboard extends the whiteboard board vocabulary it is inlined after",
	"tree dashboard,whiteboard":         "dashboard extends the whiteboard board vocabulary it is inlined after",
	"tsum dashboard,whiteboard":         "dashboard extends the whiteboard board vocabulary it is inlined after",
	"up dashboard,whiteboard":           "dashboard extends the whiteboard board vocabulary it is inlined after",
	"wb-rail dashboard,whiteboard":      "dashboard extends the whiteboard board vocabulary it is inlined after",
	"wb-stuck dashboard,whiteboard":     "dashboard extends the whiteboard board vocabulary it is inlined after",
	"why dashboard,whiteboard":          "dashboard extends the whiteboard board vocabulary it is inlined after",
	"wstoast dashboard,tracker":         "pre-existing dashboard and tracker overlap (host styles the tracker mount), grandfathered",
}

var (
	cssComment   = regexp.MustCompile(`(?s)/\*.*?\*/`)
	cssString    = regexp.MustCompile(`"[^"]*"|'[^']*'`)
	cssClassName = regexp.MustCompile(`\.(-?[_a-zA-Z][\w-]*)`)
)

// cssClasses returns every class named in a selector prelude of css,
// skipping declaration bodies, at-rule preludes (@keyframes, @media params)
// and attribute strings.
func cssClasses(css, root string) map[string]bool {
	css = cssComment.ReplaceAllString(css, "")
	out := map[string]bool{}
	var prelude strings.Builder
	depth := 0 // rule-block nesting; selectors live at any depth for @media
	type frame struct{ selector bool }
	var stack []frame
	for i := 0; i < len(css); i++ {
		c := css[i]
		switch c {
		case '{':
			p := strings.TrimSpace(prelude.String())
			prelude.Reset()
			sel := !strings.HasPrefix(p, "@")
			if len(stack) > 0 && stack[len(stack)-1].selector {
				sel = false // declaration block of a nested rule
			}
			if sel {
				p = cssString.ReplaceAllString(p, "")
				for _, one := range strings.Split(p, ",") {
					one = strings.TrimSpace(one)
					if root != "" && (one == "."+root || strings.HasPrefix(one, "."+root+".") || strings.HasPrefix(one, "."+root+" ")) {
						one = strings.Fields(one)[0] // scoped: only the root compound is global
					}
					for _, m := range cssClassName.FindAllStringSubmatch(one, -1) {
						out[m[1]] = true
					}
				}
			}
			// A nested block under a selector rule is a declaration body.
			stack = append(stack, frame{selector: sel || (len(stack) > 0 && stack[len(stack)-1].selector)})
			depth++
		case '}':
			if depth > 0 {
				depth--
				stack = stack[:len(stack)-1]
			}
			prelude.Reset()
		case ';':
			if len(stack) == 0 || !stack[len(stack)-1].selector {
				prelude.Reset()
			}
		default:
			if len(stack) == 0 || !stack[len(stack)-1].selector {
				prelude.WriteByte(c)
			}
		}
	}
	return out
}

func TestCSSClassesDoNotCollideAcrossModules(t *testing.T) {
	owners := map[string][]string{}
	for _, m := range cssModules {
		b, err := os.ReadFile(m.path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "class prefix: "+m.prefix) {
			t.Errorf("%s declares no %q header line; add a comment 'class prefix: %s' naming its new-class prefix", m.path, "class prefix: "+m.prefix, m.prefix)
		}
		for c := range cssClasses(string(b), m.root) {
			owners[c] = append(owners[c], m.name)
		}
	}
	var bad []string
	for c, mods := range owners {
		if len(mods) < 2 {
			continue
		}
		sort.Strings(mods)
		if _, ok := allowedCSSOverlaps[c+" "+strings.Join(mods, ",")]; ok {
			continue
		}
		bad = append(bad, "."+c+" in "+strings.Join(mods, ", "))
	}
	sort.Strings(bad)
	for _, b := range bad {
		t.Errorf("class defined in two module stylesheets: %s. They share one page, so the rules merge; rename the newer class with its module's prefix (dashboard d-, memo memo-, tracker tk-, whiteboard wb), or allowlist it with a reason", b)
	}
}
