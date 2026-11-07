// Package wsi18n holds the dashboard's per-language translation tables and
// the lookup every page renderer shares. A table is one embedded JSON file,
// tables/<code>.json: a flat object from key to text, with "lang.name" the
// language's own display name. A language is added by adding one table file
// and rebuilding; Languages discovers it, and the gear menu lists it.
//
// Only text a person reads in the browser belongs here: the page chrome the
// dashboard itself draws. Board items, chat, and every agent- or
// person-authored text, and machine-facing strings, never pass through it.
//
// Text with variable parts uses {0}, {1}, ... placeholders. A key that
// depends on a count has a ".one" and an ".other" form (Plural); a language
// without plural forms gives both the same text. A key a non-English table
// lacks falls back to the English table; a key the English table lacks
// renders as the key itself, which keys_test.go keeps from shipping.
package wsi18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"sort"
	"strconv"
	"strings"
	"sync"
)

//go:embed tables/*.json
var tableFS embed.FS

const (
	// Default is the fallback language, and the one a page renders in when
	// no language is asked for.
	Default = "en"
	// CookieName is the browser cookie holding the language choice. The
	// server reads it so a page renders in the chosen language from its
	// first byte; the page script writes it from the gear menu. No cookie
	// means "follow the browser".
	CookieName = "ws_dashboard_lang"
	// ClientPrefix marks the keys the page scripts use. Only these ship to
	// the browser as a table.
	ClientPrefix = "js."

	nameKey = "lang.name"
)

// Language is one available language: its code (the table file's name) and
// the display name its own table gives it.
type Language struct {
	Code string
	Name string
	// Tag is the short code the gear menu shows beside the name.
	Tag string
}

var (
	loadOnce sync.Once
	tables   map[string]map[string]string
	langs    []Language
	// byLower maps a lowercased table code to the code itself, so a cookie
	// or tag in any case finds its table (BCP 47 tags are case-insensitive).
	byLower map[string]string
)

func load() {
	loadOnce.Do(func() {
		entries, err := tableFS.ReadDir("tables")
		if err != nil {
			panic(fmt.Sprintf("wsi18n: read tables: %v", err))
		}
		tables = map[string]map[string]string{}
		byLower = map[string]string{}
		for _, e := range entries {
			code, ok := strings.CutSuffix(e.Name(), ".json")
			if !ok {
				continue
			}
			raw, err := tableFS.ReadFile("tables/" + e.Name())
			if err != nil {
				panic(fmt.Sprintf("wsi18n: read %s: %v", e.Name(), err))
			}
			table := map[string]string{}
			if err := json.Unmarshal(raw, &table); err != nil {
				panic(fmt.Sprintf("wsi18n: parse %s: %v", e.Name(), err))
			}
			tables[code] = table
			byLower[strings.ToLower(code)] = code
		}
		if tables[Default] == nil {
			panic("wsi18n: no " + Default + " table")
		}
		for code, table := range tables {
			name := table[nameKey]
			if name == "" {
				name = code
			}
			langs = append(langs, Language{Code: code, Name: name, Tag: strings.ToUpper(code)})
		}
		sort.Slice(langs, func(i, j int) bool {
			if (langs[i].Code == Default) != (langs[j].Code == Default) {
				return langs[i].Code == Default
			}
			return langs[i].Code < langs[j].Code
		})
	})
}

// Languages lists the available languages, English first, the rest by code.
func Languages() []Language {
	load()
	return append([]Language(nil), langs...)
}

// Supported reports whether code names an available language.
func Supported(code string) bool {
	load()
	_, ok := tables[code]
	return ok
}

// canonical returns the table code a tag in any case names, or "".
func canonical(code string) string {
	load()
	return byLower[strings.ToLower(code)]
}

// matchTag maps one Accept-Language tag to the table code that serves it, or
// "" when none does. Chinese is told apart by script (zh-Hans, zh-Hant) or,
// without one, by region: Taiwan, Hong Kong, and Macau are Traditional and
// every other Chinese tag, bare zh included, is Simplified. Portuguese of
// any region is the Brazilian table. Every other language matches on its
// primary subtag.
func matchTag(tag string) string {
	tag = strings.ToLower(tag)
	if c := canonical(tag); c != "" {
		return c
	}
	parts := strings.Split(tag, "-")
	switch parts[0] {
	case "zh":
		for _, p := range parts[1:] {
			switch p {
			case "hant", "tw", "hk", "mo":
				return canonical("zh-Hant")
			case "hans", "cn", "sg":
				return canonical("zh-Hans")
			}
		}
		return canonical("zh-Hans")
	case "pt":
		return canonical("pt-BR")
	}
	return canonical(parts[0])
}

// Table returns a copy of one language's own table (nil for an unsupported
// code), for tests and tooling.
func Table(code string) map[string]string {
	load()
	t, ok := tables[code]
	if !ok {
		return nil
	}
	out := make(map[string]string, len(t))
	for k, v := range t {
		out[k] = v
	}
	return out
}

// Pref reads the language cookie's value: the code when it names an
// available language, else "" (follow the browser).
func Pref(cookie string) string {
	return canonical(cookie)
}

// Resolve picks the page language: an explicit cookie choice, else the first
// language the Accept-Language header names that a table exists for, else
// Default.
func Resolve(cookie, acceptLanguage string) string {
	if p := Pref(cookie); p != "" {
		return p
	}
	type cand struct {
		code string
		q    float64
	}
	var cands []cand
	for _, part := range strings.Split(acceptLanguage, ",") {
		tag, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" || tag == "*" {
			continue
		}
		q := 1.0
		if v, ok := strings.CutPrefix(strings.TrimSpace(params), "q="); ok {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				q = f
			}
		}
		if q <= 0 {
			continue
		}
		if code := matchTag(tag); code != "" {
			cands = append(cands, cand{code, q})
		}
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].q > cands[j].q })
	if len(cands) > 0 {
		return cands[0].code
	}
	return Default
}

// Lookup returns the text for key in lang, falling back to English; ok is
// false when neither table has it.
func Lookup(lang, key string) (text string, ok bool) {
	load()
	if t, found := tables[lang][key]; found {
		return t, true
	}
	text, ok = tables[Default][key]
	return text, ok
}

// T returns the text for key in lang with {0}, {1}, ... replaced by args. A
// key no table has renders as itself.
func T(lang, key string, args ...any) string {
	text, ok := Lookup(lang, key)
	if !ok {
		return key
	}
	return Fill(text, args...)
}

// Plural returns the ".one" form of key when n is 1, else the ".other" form,
// with {0} the count and args the placeholders after it.
func Plural(lang, key string, n int, args ...any) string {
	suffix := ".other"
	if n == 1 {
		suffix = ".one"
	}
	return T(lang, key+suffix, append([]any{n}, args...)...)
}

// Fill replaces {0}, {1}, ... in text with args.
func Fill(text string, args ...any) string {
	if len(args) == 0 {
		return text
	}
	pairs := make([]string, 0, 2*len(args))
	for i, a := range args {
		pairs = append(pairs, "{"+strconv.Itoa(i)+"}", fmt.Sprint(a))
	}
	return strings.NewReplacer(pairs...).Replace(text)
}

// ClientTable is the table the page scripts use for lang: every ClientPrefix
// key, a missing one taken from English.
func ClientTable(lang string) map[string]string {
	load()
	out := map[string]string{}
	for k, v := range tables[Default] {
		if strings.HasPrefix(k, ClientPrefix) {
			out[k] = v
		}
	}
	for k, v := range tables[lang] {
		if strings.HasPrefix(k, ClientPrefix) {
			out[k] = v
		}
	}
	return out
}

// Script is the inline script that gives a page its client lookup: the
// language, its client table, and wsT and wsTn for the page scripts. It
// escapes the table for a <script> element.
func Script(lang string) template.JS {
	if !Supported(lang) {
		lang = Default
	}
	table, _ := json.Marshal(ClientTable(lang))
	return template.JS("window.wsI18n = { lang: " + strconv.Quote(lang) + ", t: " + string(table) + ` };
window.wsT = function (key) {
  var t = window.wsI18n.t, s = Object.prototype.hasOwnProperty.call(t, key) ? t[key] : key;
  for (var i = 1; i < arguments.length; i++) s = s.split("{" + (i - 1) + "}").join(String(arguments[i]));
  return s;
};
window.wsTn = function (key, n) {
  var a = Array.prototype.slice.call(arguments, 2);
  return window.wsT.apply(null, [key + (n === 1 ? ".one" : ".other"), n].concat(a));
};
`)
}
