package wsdoc

import (
	"regexp"
	"sort"
	"strings"
)

const snippetContextRunes = 48

var docQueryTokenRE = regexp.MustCompile(`[a-z0-9]+`)

type MatchEvidence struct {
	Line         int      `json:"line"`
	MatchedTerms []string `json:"matched_terms"`
	Snippet      string   `json:"snippet"`
}

type docQueryCandidate struct {
	Path     string
	Fields   []string
	BodyText string
}

type docQueryMatch struct {
	Score   int
	Matches []MatchEvidence
	Terms   []string
}

func matchDocumentQuery(query string, candidate docQueryCandidate) (docQueryMatch, bool) {
	terms := queryTerms(query)
	if len(terms) == 0 {
		return docQueryMatch{}, false
	}
	fieldText := strings.Join(candidate.Fields, "\n")
	allText := fieldText + "\n" + candidate.BodyText
	allTokens := tokenCounts(allText)
	matchedTerms := []string{}
	for _, term := range terms {
		if allTokens[term] > 0 {
			matchedTerms = append(matchedTerms, term)
		}
	}
	if len(matchedTerms) == 0 || (len(terms) > 1 && len(matchedTerms) < 2) {
		return docQueryMatch{}, false
	}
	fieldTokens := tokenCounts(fieldText)
	score := 0
	for _, term := range matchedTerms {
		score += allTokens[term]
		if fieldTokens[term] > 0 {
			score += 3
		}
	}
	matches := lineMatches(candidate.BodyText, matchedTerms)
	if len(matches) == 0 && fieldText != "" {
		matches = fieldLineMatches(fieldText, matchedTerms)
	}
	return docQueryMatch{Score: score, Matches: matches, Terms: matchedTerms}, true
}

func queryTerms(query string) []string {
	tokens := docQueryTokenRE.FindAllString(strings.ToLower(query), -1)
	seen := map[string]bool{}
	terms := []string{}
	for _, token := range tokens {
		if len(token) < 3 || seen[token] {
			continue
		}
		seen[token] = true
		terms = append(terms, token)
	}
	return terms
}

func tokenCounts(text string) map[string]int {
	counts := map[string]int{}
	for _, token := range docQueryTokenRE.FindAllString(strings.ToLower(text), -1) {
		counts[token]++
	}
	return counts
}

func lineMatches(text string, terms []string) []MatchEvidence {
	out := []MatchEvidence{}
	for i, line := range strings.Split(text, "\n") {
		matched := matchedTermsInText(line, terms)
		if len(matched) == 0 {
			continue
		}
		out = append(out, MatchEvidence{Line: i + 1, MatchedTerms: matched, Snippet: compactSnippet(line, matched)})
	}
	return out
}

func fieldLineMatches(text string, terms []string) []MatchEvidence {
	out := []MatchEvidence{}
	for _, line := range strings.Split(text, "\n") {
		matched := matchedTermsInText(line, terms)
		if len(matched) == 0 {
			continue
		}
		out = append(out, MatchEvidence{Line: 0, MatchedTerms: matched, Snippet: compactSnippet(line, matched)})
	}
	return out
}

func matchedTermsInText(text string, terms []string) []string {
	counts := tokenCounts(text)
	matched := []string{}
	for _, term := range terms {
		if counts[term] > 0 {
			matched = append(matched, term)
		}
	}
	return matched
}

func compactSnippet(line string, terms []string) string {
	trimmed := strings.Join(strings.Fields(line), " ")
	if len([]rune(trimmed)) <= snippetContextRunes*2 {
		return trimmed
	}
	lower := strings.ToLower(trimmed)
	idx := -1
	for _, term := range terms {
		if pos := strings.Index(lower, term); pos >= 0 && (idx < 0 || pos < idx) {
			idx = pos
		}
	}
	if idx < 0 {
		idx = 0
	}
	runes := []rune(trimmed)
	start := idx - snippetContextRunes
	if start < 0 {
		start = 0
	}
	end := idx + snippetContextRunes
	if end > len(runes) {
		end = len(runes)
	}
	snippet := string(runes[start:end])
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(runes) {
		snippet += "..."
	}
	return snippet
}

func sortMatchesByLine(matches []MatchEvidence) {
	sort.Slice(matches, func(i, j int) bool { return matches[i].Line < matches[j].Line })
}
