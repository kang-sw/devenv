package wsdashboard

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// The dashboard page inlines several module stylesheets into one document, so
// a class defined in two of them is one rule set to the browser: the
// whiteboard's .tk badge once restyled the tracker's .tk root into a 17px
// pill. Each module stylesheet declares its prefix(es) in a header comment
// ("class prefix: ..."); a NEW class takes its module's prefix. Existing
// classes are not renamed, so intentional overlaps are allowlisted below.

// root, when set, is a module-local scope class: a selector that starts with it
// only matches inside the module's own subtree, so only the first compound
// (the root and its variants) is a page-global name.
var cssModules = []struct {
	name, path, prefix, root string
}{
	{"dashboard", "assets/dashboard.css", "d", ""},
	{"memo", "assets/memo.css", "memo", ""},
	{"tracker", "assets/tracker.css", "tk", "tk"},
	{"whiteboard", "../wswhiteboard/assets/page.css", "wb", "dw"},
}

// allowedCSSOverlaps maps "class module,module" (modules sorted) to the reason
// those modules may both define it. A new module joining the class is a new
// collision and fails.
var allowedCSSOverlaps = map[string]string{
	"cat dashboard,whiteboard":          "wide drawer restyles existing ticket atoms only under #p-tickets",
	"csub dashboard,whiteboard":         "wide drawer restyles existing ticket atoms only under #p-tickets",
	"ctitle dashboard,whiteboard":       "wide drawer restyles existing ticket atoms only under #p-tickets",
	"kcard dashboard,whiteboard":        "wide drawer restyles existing ticket atoms only under #p-tickets",
	"lane dashboard,whiteboard":         "wide drawer restyles existing ticket atoms only under #p-tickets",
	"lanes dashboard,whiteboard":        "wide drawer restyles existing ticket atoms only under #p-tickets",
	"wsc-backing dashboard,memo":        "both pages mount the shared capsules.js editor; memo.css scopes its copy under .memo-pad",
	"wsc-capsule dashboard,memo":        "both pages mount the shared capsules.js editor; memo.css scopes its copy under .memo-pad",
	"wsc-editor dashboard,memo":         "both pages mount the shared capsules.js editor; memo.css scopes its copy under .memo-pad",
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

// The header's allowlisted .top remains valid, but a sticky pill wrapper
// must never carry that bare class: both stylesheets share the same DOM.
func TestWhiteboardPillEdgesCannotMatchHeaderTop(t *testing.T) {
	read := func(name string) string {
		t.Helper()
		b, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	assets, err := wswhiteboard.PageAssets()
	if err != nil {
		t.Fatal(err)
	}
	css := string(assets.CSS)
	pill := read("../wswhiteboard/assets/pill.js")
	rail := read("../wswhiteboard/assets/rail.js")
	if !regexp.MustCompile(`(?m)^\.top\s*\{`).MatchString(read("assets/dashboard.css")) {
		t.Fatal("fixture no longer includes the dashboard's bare .top collision source")
	}
	for _, modifier := range []string{"wb-oq-top", "wb-oq-bot"} {
		// Exact class tokens, not suffixes, determine a .top selector match.
		for _, class := range strings.Fields("oq-wrap " + modifier) {
			if class == "top" || class == "bot" {
				t.Fatalf("pill modifier %s can match a bare header edge class", modifier)
			}
		}
		if !strings.Contains(css, ".oq-wrap."+modifier+" {") || !strings.Contains(css, ".oq-wrap."+modifier+" .oq-pill {") ||
			!strings.Contains(rail, `host.querySelector(".oq-wrap.`+modifier+`")`) || !strings.Contains(pill, `"`+modifier+`"`) {
			t.Errorf("pill construction, rail offsets and CSS disagree on %s", modifier)
		}
	}
	for _, fragment := range []string{`var modifier = edge === "top" ? "wb-oq-top" : "wb-oq-bot";`,
		`host.querySelector(".oq-wrap." + modifier)`, `w.className = "oq-wrap " + modifier;`} {
		if !strings.Contains(pill, fragment) {
			t.Errorf("pill wrapper does not use isolated edge modifier: %s", fragment)
		}
	}
	for name, source := range map[string]string{"css": css, "pill": pill, "rail": rail} {
		if regexp.MustCompile(`\.oq-wrap\.(?:top|bot)\b`).MatchString(source) || strings.Contains(source, `w.className = "oq-wrap " + edge`) {
			t.Errorf("%s retains an unnamespaced pill edge", name)
		}
	}
}

func TestMemoUsesSharedEditorStyles(t *testing.T) {
	dashboard, err := dashboardTestAsset("assets/dashboard.css")
	if err != nil {
		t.Fatal(err)
	}
	memo, err := dashboardTestAsset("assets/memo.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`.wsc-backing { display: none !important; }`,
		`.wsc-editor:empty::before`, `.wsc-editor:focus`, `.wsc-editor[aria-disabled="true"]`,
		`.wsc-capsule { display: inline; white-space: normal; box-decoration-break: clone; -webkit-box-decoration-break: clone; overflow-wrap: anywhere;`,
	} {
		if !strings.Contains(string(dashboard), want) {
			t.Errorf("missing shared editor atom %q", want)
		}
	}
	for _, want := range []string{
		`.memo-pad textarea, .memo-pad textarea + [contenteditable] {`,
		`.memo-pad textarea + [contenteditable] { max-height: 240px; overflow: auto; }`,
		`.memo-pad [hidden], .memo-pad textarea[hidden] + [contenteditable] { display: none; }`,
	} {
		if !strings.Contains(string(memo), want) {
			t.Errorf("missing memo editor layout %q", want)
		}
	}
	if strings.Contains(string(memo), ".wsc-") {
		t.Error("memo duplicates shared editor atom ownership")
	}
}

func TestCSSClassesDoNotCollideAcrossModules(t *testing.T) {
	// The narrow exceptions below must not hide other selectors using the
	// same names; those still participate in the collision scan.
	probe := cssClasses(`.oq-pill { color: red } .board-updated { color: red } .dw .pop { color: red }`, "")
	for _, class := range []string{"oq-pill", "board-updated", "dw", "pop"} {
		if !probe[class] {
			t.Errorf("collision scan misses unscoped .%s", class)
		}
	}
	owners := map[string][]string{}
	for _, m := range cssModules {
		var b []byte
		var err error
		if m.name == "whiteboard" {
			var assets wswhiteboard.SharedAssets
			assets, err = wswhiteboard.PageAssets()
			b = []byte(assets.CSS)
		} else {
			b, err = dashboardTestAsset(m.path)
		}
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "class prefix: "+m.prefix) {
			t.Errorf("%s declares no %q header line; add a comment 'class prefix: %s' naming its new-class prefix", m.path, "class prefix: "+m.prefix, m.prefix)
		}
		css := string(b)
		if m.name == "dashboard" {
			// This exact cross-module rule is intentional: it pauses the
			// whiteboard animation. Do not exempt other uses of these classes.
			for _, exception := range []struct{ rule, fragment string }{
				{`:root[data-animation-paused] :is(.spin, .pulse, .pibadge, .caret, .ibadge.busy .istop, .rail .drkwork, .rail .hi .rrun, .rail .hi.perm, .rail .sbn, .dskel i, .tk .tk-qspin, .oq-pill.board-updated) { animation-play-state: paused; }`, `.oq-pill.board-updated`},
			} {
				if strings.Count(css, exception.rule) != 1 {
					t.Errorf("dashboard intentional cross-module rule missing or repeated: %s", exception.rule)
				}
				clean := strings.Replace(exception.rule, exception.fragment, "", 1)
				css = strings.Replace(css, exception.rule, clean, 1)
			}
		}
		for c := range cssClasses(css, m.root) {
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
