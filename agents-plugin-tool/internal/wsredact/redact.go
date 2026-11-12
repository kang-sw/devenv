// Package wsredact masks credentials in short display text before a browser
// or an agent sees it. Every rule replaces the secret part of a match with
// Mask and leaves the surrounding text, so a reader can still tell what was
// there.
//
// It is best-effort, not a guarantee: the rules catch well-known token shapes
// and secret-named values, and a secret in any other shape passes through.
// A false positive only hides a harmless value, so the rules lean toward
// masking where a shape is distinctive.
package wsredact

import (
	"regexp"
	"strings"
)

// Mask replaces each secret part.
const Mask = "***"

// urlUserinfo matches the userinfo of an http(s) URL, where a token or
// password sits; an ssh user (git@host) carries none. The userinfo runs to
// the last '@' before the host's path, since a password may hold an
// unencoded '@' (user:p@ss@host).
var urlUserinfo = regexp.MustCompile(`(?i)(https?://)[^/\s]*@`)

// urlQuery matches the query string of an http(s) URL, where a token can sit
// too (private_token=, access_token=): group 1 is everything up to the "?",
// group 2 the query, which ends at a fragment, whitespace, or quote.
var urlQuery = regexp.MustCompile(`(?i)(https?://[^\s?#'"]*\?)([^\s#'"]*)`)

// URLCredentials hides the userinfo and every query value of each http(s)
// URL in s. Query names stay.
func URLCredentials(s string) string {
	s = urlUserinfo.ReplaceAllString(s, "${1}"+Mask+"@")
	return urlQuery.ReplaceAllStringFunc(s, func(url string) string {
		m := urlQuery.FindStringSubmatch(url)
		params := strings.Split(m[2], "&")
		for i, p := range params {
			if name, _, ok := strings.Cut(p, "="); ok {
				params[i] = name + "=" + Mask
			} else if p != "" {
				params[i] = Mask
			}
		}
		return m[1] + strings.Join(params, "&")
	})
}

// secretKey is a secret-named key: one of the names, case-insensitive,
// optionally after word_ or word- prefixes (db_password, GITHUB_TOKEN,
// aws_secret_access_key). A longer word that only starts with a name
// (tokenizer, tokens, authorization) is not one.
const secretKey = `(?:[A-Za-z0-9]+[_-])*(?:password|passwd|pwd|secret|token|api[_-]?key|access[_-]?key|private[_-]?key|client[_-]?secret|auth)`

// secretValue is the value after a secret key: the inside of a double- or
// single-quoted string (to its closing quote, or to the end when cut), or a
// bare run up to whitespace or a delimiter.
const secretValue = `(?:"((?:[^"\\]|\\.)+)|'([^']+)|([^\s"',;&]+))`

// rules apply in order. A rule with capture groups masks each group that
// matched; one without masks its whole match.
var rules = []*regexp.Regexp{
	// Well-known token shapes, a subset of gitleaks' (MIT) rules. Minimum
	// lengths are loose where a prefix is distinctive, so a token an
	// adapter's preview cap cut short is still masked.
	regexp.MustCompile(`\b(?:AKIA|ASIA)[A-Z0-9]{12,}\b`),                   // AWS access key id
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{8,}`),                      // GitHub token
	regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{8,}`),                    // GitHub fine-grained token
	regexp.MustCompile(`\bglpat-[A-Za-z0-9_-]{8,}`),                        // GitLab token
	regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{8,}`),                    // Slack token
	regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_-]{8,}`),                       // Anthropic key
	regexp.MustCompile(`\bsk-(?:proj-)?[A-Za-z0-9_-]{20,}`),                // OpenAI key
	regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{16,}`),                         // Google API key
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}(?:\.[A-Za-z0-9_-]*){0,2}`), // JWT, or its cut head
	// A PEM private key block: its body to the END line, or to the end of
	// the text when the preview cut it.
	regexp.MustCompile(`(?s)-----BEGIN[A-Z0-9 ]*PRIVATE KEY-----(.+?)(?:-----END[A-Z0-9 ]*PRIVATE KEY-----|$)`),
	// An Authorization header's credentials.
	regexp.MustCompile(`(?i)\bauthorization["']?\s*[:=]\s*["']?(?:bearer|basic|token)\s+([^\s"',;]+)`),
	// key=value, key: value, "key": "value".
	regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9_.-])["']?` + secretKey + `["']?\s*[:=]\s*` + secretValue),
	// --flag value, --flag=value; a bare value starting with "-" is the next
	// flag, not a value.
	regexp.MustCompile(`(?i)(?:^|\s)--` + secretKey + `(?:=|\s+)(?:"((?:[^"\\]|\\.)+)|'([^']+)|([^\s"'-][^\s"']*))`),
}

// Secrets masks every credential the rules recognize in s: URL userinfo and
// query values, well-known token shapes, PEM private key bodies,
// Authorization credentials, and the values of secret-named keys and flags.
func Secrets(s string) string {
	s = URLCredentials(s)
	for _, re := range rules {
		s = maskMatches(re, s)
	}
	return s
}

// maskMatches replaces each of re's capture groups that matched in s with
// Mask, or each whole match when re has no groups.
func maskMatches(re *regexp.Regexp, s string) string {
	matches := re.FindAllStringSubmatchIndex(s, -1)
	if matches == nil {
		return s
	}
	var b strings.Builder
	last := 0
	for _, m := range matches {
		spans := [][2]int{{m[0], m[1]}}
		if len(m) > 2 {
			spans = spans[:0]
			for g := 2; g < len(m); g += 2 {
				if m[g] >= 0 && m[g+1] > m[g] {
					spans = append(spans, [2]int{m[g], m[g+1]})
				}
			}
		}
		for _, sp := range spans {
			b.WriteString(s[last:sp[0]])
			b.WriteString(Mask)
			last = sp[1]
		}
	}
	b.WriteString(s[last:])
	return b.String()
}
