package wsi18n

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The key sweep: every key a template, script, or Go call site names must be
// in the English table (a key missing from it would render as the key), every
// other table may hold only keys English has, and every English key is used
// somewhere (a table is not a dumping ground). The call forms it scans are
// the only forms the sources use: {{t "k"}}, {{tn "k" n}}, {{tcode "k" x}} in
// templates, wsT("k") and wsTn("k", n) in scripts, and wsi18n.T/Plural(lang,
// "k") in Go. A key built at run time cannot be seen, so none is built.
var (
	tmplKey = regexp.MustCompile(`\{\{-?\s*\(?(?:t|tn|tcode)\s+"([^"]+)"`)
	// A key may also open a parenthesised call inside a pipeline.
	tmplKeyNested = regexp.MustCompile(`\((?:t|tn|tcode)\s+"([^"]+)"`)
	// Every client key starts with ClientPrefix, so a quoted literal that
	// does is a use, whatever expression (a conditional) picks it.
	jsKey = regexp.MustCompile(`"(js\.[a-z0-9.]+)"`)
	goKey = regexp.MustCompile(`wsi18n\.(?:T|Plural)\(\s*[A-Za-z_.]+,\s*"([^"]+)"`)
	// Plural keys name the base; the tables hold ".one" and ".other".
	pluralCall = regexp.MustCompile(`(?:\{\{-?\s*tn\s+|\bwsTn\(\s*|wsi18n\.Plural\(\s*[A-Za-z_.]+,\s*)"([^"]+)"`)
)

// sourceRoots are the directories whose templates, scripts, and Go files the
// sweep reads, relative to this package.
var sourceRoots = []string{"../wsdashboard", "../wswhiteboard", "../mcp"}

func usedKeys(t *testing.T) (keys, plural map[string]string) {
	t.Helper()
	keys, plural = map[string]string{}, map[string]string{}
	for _, root := range sourceRoots {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return err
			}
			name := info.Name()
			if strings.HasSuffix(name, "_test.go") {
				return nil
			}
			var res []*regexp.Regexp
			switch filepath.Ext(name) {
			case ".tmpl":
				res = []*regexp.Regexp{tmplKey, tmplKeyNested}
			case ".js":
				res = []*regexp.Regexp{jsKey}
			case ".go":
				res = []*regexp.Regexp{goKey}
			default:
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, re := range res {
				for _, m := range re.FindAllStringSubmatch(string(raw), -1) {
					keys[m[1]] = path
				}
			}
			for _, m := range pluralCall.FindAllStringSubmatch(string(raw), -1) {
				plural[m[1]] = path
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	// A plural call names the base; expand it to the two forms.
	for base, path := range plural {
		delete(keys, base)
		keys[base+".one"] = path
		keys[base+".other"] = path
	}
	return keys, plural
}

func TestEveryUsedKeyIsInTheEnglishTable(t *testing.T) {
	keys, _ := usedKeys(t)
	if len(keys) < 100 {
		t.Fatalf("the sweep found only %d keys: its patterns or roots are stale", len(keys))
	}
	en := Table(Default)
	var missing []string
	for k, path := range keys {
		if _, ok := en[k]; !ok {
			missing = append(missing, k+" ("+path+")")
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("keys used but missing from tables/en.json:\n%s", strings.Join(missing, "\n"))
	}
}

func TestEveryEnglishKeyIsUsed(t *testing.T) {
	keys, _ := usedKeys(t)
	var unused []string
	for k := range Table(Default) {
		if k == nameKey {
			continue
		}
		if _, ok := keys[k]; !ok {
			unused = append(unused, k)
		}
	}
	sort.Strings(unused)
	if len(unused) > 0 {
		t.Fatalf("keys in tables/en.json that no template, script, or Go call site uses:\n%s", strings.Join(unused, "\n"))
	}
}

func TestOtherTablesHoldOnlyEnglishKeysAndNames(t *testing.T) {
	en := Table(Default)
	for _, l := range Languages() {
		if l.Code == Default {
			continue
		}
		table := Table(l.Code)
		if table[nameKey] == "" {
			t.Errorf("%s: no %s, so the gear menu cannot name it", l.Code, nameKey)
		}
		for k, v := range table {
			if _, ok := en[k]; !ok {
				t.Errorf("%s: key %q is not in the English table", l.Code, k)
			}
			if strings.TrimSpace(v) == "" {
				t.Errorf("%s: key %q is empty", l.Code, k)
			}
		}
	}
}

// A translation must keep every placeholder its English text has, and add
// none, or a call site's arguments land in the wrong place.
func TestPlaceholdersMatchEnglish(t *testing.T) {
	ph := regexp.MustCompile(`\{[0-9]+\}`)
	set := func(s string) string {
		found := ph.FindAllString(s, -1)
		sort.Strings(found)
		return strings.Join(found, ",")
	}
	en := Table(Default)
	for _, l := range Languages() {
		if l.Code == Default {
			continue
		}
		for k, v := range Table(l.Code) {
			if set(v) != set(en[k]) {
				t.Errorf("%s: %q has placeholders [%s], English has [%s]", l.Code, k, set(v), set(en[k]))
			}
		}
	}
}

// A count key has both forms in every table that has either.
func TestPluralKeysComeInPairs(t *testing.T) {
	for _, l := range Languages() {
		table := Table(l.Code)
		for k := range table {
			if base, ok := strings.CutSuffix(k, ".one"); ok {
				if _, ok := table[base+".other"]; !ok {
					t.Errorf("%s: %q has no .other form", l.Code, k)
				}
			}
			if base, ok := strings.CutSuffix(k, ".other"); ok {
				if _, ok := table[base+".one"]; !ok {
					t.Errorf("%s: %q has no .one form", l.Code, k)
				}
			}
		}
	}
}
