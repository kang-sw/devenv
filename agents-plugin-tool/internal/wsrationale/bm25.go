package wsrationale

import (
	"math"
	"unicode"
)

const (
	bm25K1 = 1.2
	bm25B  = 0.75
)

// tokenize lowercases and splits on any non-alphanumeric character, dropping
// tokens shorter than 2 characters.
func tokenize(s string) []string {
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) >= 2 {
			out = append(out, string(cur))
		}
		cur = cur[:0]
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur = append(cur, unicode.ToLower(r))
			continue
		}
		flush()
	}
	flush()
	return out
}

// bm25 holds the corpus statistics needed to score any record against a query.
// IDF is computed over the whole scanned corpus, not a filtered subset.
type bm25 struct {
	idf   map[string]float64
	avgdl float64
}

// newBM25 builds corpus statistics over every record's document (its text plus
// its commit subject or ticket title).
func newBM25(records []Record) bm25 {
	n := len(records)
	df := map[string]int{}
	total := 0
	for i := range records {
		toks := records[i].docTokens()
		total += len(toks)
		seen := map[string]bool{}
		for _, t := range toks {
			if !seen[t] {
				seen[t] = true
				df[t]++
			}
		}
	}
	idf := make(map[string]float64, len(df))
	for term, d := range df {
		idf[term] = math.Log(1 + (float64(n)-float64(d)+0.5)/(float64(d)+0.5))
	}
	avgdl := 0.0
	if n > 0 {
		avgdl = float64(total) / float64(n)
	}
	return bm25{idf: idf, avgdl: avgdl}
}

// score returns the BM25 score of one record's document against the query
// tokens.
func (b bm25) score(queryTokens []string, rec Record) float64 {
	if len(queryTokens) == 0 {
		return 0
	}
	docTokens := rec.docTokens()
	if len(docTokens) == 0 {
		return 0
	}
	freq := make(map[string]int, len(docTokens))
	for _, t := range docTokens {
		freq[t]++
	}
	dl := float64(len(docTokens))
	var s float64
	for _, q := range queryTokens {
		f := float64(freq[q])
		if f == 0 {
			continue
		}
		idf := b.idf[q]
		denom := f + bm25K1*(1-bm25B+bm25B*dl/b.avgdl)
		s += idf * (f * (bm25K1 + 1)) / denom
	}
	return s
}
