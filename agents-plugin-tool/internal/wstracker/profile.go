package wstracker

import (
	"sort"
	"strings"
	"unicode"
)

// Member profile ((391)-(393) in the mockup README): org (one free-text
// string, may be empty), family (may be empty: a single-name person), and
// given (required). The display name and the avatar are derived from it,
// never stored. Both rules mirror engine.js displayName and avatar.

// Profile is a member's org, family name, and given name.
type Profile struct {
	Org    string `json:"org"`
	Family string `json:"family"`
	Given  string `json:"given"`
}

// MemberProfile is the payload of member.profile. A member writes their own
// profile (Member empty), an admin may name any member. Only the fields
// present are written, each as its own register, so concurrent changes to
// different fields both stand; a present Given cannot be empty. Writers use
// a raw map when a field must be absent rather than empty.
type MemberProfile struct {
	Member  string            `json:"member,omitempty"`
	Profile map[string]string `json:"profile"`
}

// TypeMemberProfile changes a member's profile.
const TypeMemberProfile = "member.profile"

var profileFields = []string{"org", "family", "given"}

var profileLabel = map[string]string{"org": "org", "family": "family name", "given": "given name"}

func (p *Profile) set(field, v string) {
	switch field {
	case "org":
		p.Org = v
	case "family":
		p.Family = v
	case "given":
		p.Given = v
	}
}

// pstr is a trimmed string field; anything but a string is empty.
func pstr(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

// profileOf is the stored profile of a roles-file member or a join payload.
// A member written before profiles (a bare name, or nothing) keeps one given
// name: the name, else the id.
func profileOf(m obj, id string) Profile {
	pr, _ := m["profile"].(map[string]any)
	if pstr(pr["given"]) != "" {
		return Profile{Org: pstr(pr["org"]), Family: pstr(pr["family"]), Given: pstr(pr["given"])}
	}
	out := Profile{Org: pstr(pr["org"]), Family: pstr(pr["family"]), Given: pstr(m["name"])}
	if out.Given == "" {
		out.Given = id
	}
	return out
}

func isCJKRune(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hangul, unicode.Hiragana, unicode.Katakana)
}

func (p Profile) cjk() bool {
	for _, r := range p.Family + p.Given {
		if isCJKRune(r) {
			return true
		}
	}
	return false
}

// DisplayName reads family + given with no space when either holds Hangul,
// a CJK ideograph, or kana ("홍길동"); any other name reads "given family"
// ("Ada Park"); a single name is the given name alone.
func (p Profile) DisplayName() string {
	switch {
	case p.Family == "":
		return p.Given
	case p.cjk():
		return p.Family + p.Given
	}
	return p.Given + " " + p.Family
}

// Avatar is the short text in a member's avatar circle: for a CJK name the
// given name up to two characters ("길동"); otherwise the first letter of
// given and of family, upper-cased ("AP"); for a single name its first two
// letters.
func (p Profile) Avatar() string {
	g, f := []rune(p.Given), []rune(p.Family)
	if p.cjk() {
		return string(g[:min(2, len(g))])
	}
	var out []rune
	if len(f) > 0 {
		out = append(out, g[:min(1, len(g))]...)
		out = append(out, f[:1]...)
	} else {
		out = g[:min(2, len(g))]
	}
	return strings.ToUpper(string(out))
}

// checkProfile judges a profile write (engine.js checkProfile): an object of
// strings among org, family, and given. full (a join) needs given; a later
// write carries only the fields it changes, and a given it carries cannot be
// empty. Unknown or mistyped fields are reported in sorted order (the
// engine reports them in payload order; a payload with one fault agrees).
func checkProfile(v any, full bool) rejection {
	pr, ok := v.(map[string]any)
	if !ok {
		return "profile must be an object {org, family, given}"
	}
	keys := make([]string, 0, len(pr))
	for k := range pr {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !contains(profileFields, k) {
			return rejection(`profile has no field "` + k + `" (org, family, given)`)
		}
		if _, ok := pr[k].(string); !ok {
			return rejection("profile." + k + " must be a string")
		}
	}
	g, hasGiven := pr["given"]
	if full && !hasGiven {
		return "profile.given required"
	}
	if hasGiven && strings.TrimSpace(g.(string)) == "" {
		return "profile.given cannot be empty"
	}
	if len(pr) == 0 {
		return "profile has none of org, family, given"
	}
	return ""
}
