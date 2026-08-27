// Package wskey generates LLM-friendly word-chain session keys from the embedded
// EFF large wordlist. It is a pure string-generator with no auth policy; callers
// own uniqueness enforcement and any association of a key with a root or role.
package wskey

import (
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"strings"
)

//go:embed eff_large_wordlist.txt
var wordlistText string

// wordPool holds the parsed word pool, initialized once at package load.
var wordPool []string

// shortWordPool holds the subset of wordPool with length <= 5, used by Derive
// to keep deterministic word-key output short.
var shortWordPool []string

func init() {
	lines := strings.Split(wordlistText, "\n")
	pool := make([]string, 0, len(lines))
	short := make([]string, 0, len(lines))
	for _, line := range lines {
		word := strings.TrimSpace(line)
		if word != "" {
			pool = append(pool, word)
			if len(word) <= 5 {
				short = append(short, word)
			}
		}
	}
	wordPool = pool
	shortWordPool = short
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

// Generate mints a single word-chain session key: 3 random words from the
// embedded EFF large wordlist, joined by hyphens (e.g. "amber-tide-fox"). The
// key format is opaque to callers; do not parse it.
func Generate() (string, error) {
	poolSize := big.NewInt(int64(len(wordPool)))
	var words [3]string
	for i := range words {
		n, err := rand.Int(rand.Reader, poolSize)
		if err != nil {
			return "", fmt.Errorf("wskey: crypto/rand failed: %w", err)
		}
		words[i] = wordPool[n.Int64()]
	}
	return fmt.Sprintf("%s-%s-%s", words[0], words[1], words[2]), nil
}

// Derive mints a deterministic word-chain key from seed: it hashes
// "<seed>#<i>" with SHA-256 for each of the requested word count and indexes
// the <=5-char sub-pool with the digest, joining the picks with "-". The same
// seed and word count always produce the same key. Unlike Generate, this is
// not randomized and draws only from the short sub-pool to bound output
// length. The key format is opaque to callers; do not parse it.
func Derive(seed string, words int) string {
	picks := make([]string, words)
	for i := range picks {
		digest := sha256.Sum256([]byte(fmt.Sprintf("%s#%d", seed, i)))
		idx := binary.BigEndian.Uint64(digest[:8]) % uint64(len(shortWordPool))
		picks[i] = shortWordPool[idx]
	}
	return strings.Join(picks, "-")
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
