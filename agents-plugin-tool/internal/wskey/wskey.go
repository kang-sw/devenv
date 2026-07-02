// Package wskey generates LLM-friendly word-chain session keys from the embedded
// EFF large wordlist. It is a pure string-generator with no auth policy; callers
// own uniqueness enforcement and any association of a key with a root or role.
package wskey

import (
	"crypto/rand"
	_ "embed"
	"errors"
	"fmt"
	"math/big"
	"strings"
)

//go:embed eff_large_wordlist.txt
var wordlistText string

// wordPool holds the parsed word pool, initialized once at package load.
var wordPool []string

func init() {
	lines := strings.Split(wordlistText, "\n")
	pool := make([]string, 0, len(lines))
	for _, line := range lines {
		word := strings.TrimSpace(line)
		if word != "" {
			pool = append(pool, word)
		}
	}
	wordPool = pool
}

// WordCount returns the number of words in the embedded pool. Exposed for tests.
func WordCount() int {
	return len(wordPool)
}

// Words returns a copy of the word pool. Exposed for tests.
func Words() []string {
	out := make([]string, len(wordPool))
	copy(out, wordPool)
	return out
}

// Generate mints a single word-chain session key: 4 random words from the
// embedded EFF large wordlist followed by a zero-padded 2-digit numeric suffix,
// all joined by hyphens (e.g. "amber-tide-fox-river-42"). The key format is
// opaque to callers; do not parse it.
func Generate() (string, error) {
	poolSize := big.NewInt(int64(len(wordPool)))
	var words [4]string
	for i := range words {
		n, err := rand.Int(rand.Reader, poolSize)
		if err != nil {
			return "", fmt.Errorf("wskey: crypto/rand failed: %w", err)
		}
		words[i] = wordPool[n.Int64()]
	}
	suffix, err := rand.Int(rand.Reader, big.NewInt(100))
	if err != nil {
		return "", fmt.Errorf("wskey: crypto/rand failed: %w", err)
	}
	return fmt.Sprintf("%s-%s-%s-%s-%02d", words[0], words[1], words[2], words[3], suffix.Int64()), nil
}

// GenerateUnique mints a key that the provided exists predicate does not report
// as taken. It re-rolls on collision up to maxAttempts times and returns an error
// if a free key cannot be found. The predicate is called outside any external
// lock; callers that need atomic check-and-insert must do so independently after
// receiving the key.
func GenerateUnique(exists func(string) bool) (string, error) {
	const maxAttempts = 64
	for i := 0; i < maxAttempts; i++ {
		key, err := Generate()
		if err != nil {
			return "", err
		}
		if !exists(key) {
			return key, nil
		}
	}
	return "", errors.New("wskey: could not mint a unique key after 64 attempts")
}
