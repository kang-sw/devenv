package wskey

import (
	"regexp"
	"testing"
)

// keyPattern is the expected format: 4 lowercase-word segments + 2-digit suffix.
var keyPattern = regexp.MustCompile(`^[a-z]+(-[a-z]+){3}-[0-9]{2}$`)

// TestGenerateFormat verifies that minted keys match the opaque word-chain format
// contract: four hyphen-separated lowercase words followed by a 2-digit numeric
// suffix, e.g. "amber-tide-fox-river-42".
func TestGenerateFormat(t *testing.T) {
	for i := 0; i < 20; i++ {
		key, err := Generate()
		if err != nil {
			t.Fatalf("Generate() error: %v", err)
		}
		if !keyPattern.MatchString(key) {
			t.Fatalf("Generate() key %q does not match expected pattern ^[a-z]+(-[a-z]+){3}-[0-9]{2}$", key)
		}
	}
}

// TestWordPoolCount verifies that the embedded wordlist yields exactly 7772
// non-empty, pure-[a-z]+ words.
func TestWordPoolCount(t *testing.T) {
	words := Words()
	if len(words) != 7772 {
		t.Fatalf("expected 7772 words, got %d", len(words))
	}
	pureAlpha := regexp.MustCompile(`^[a-z]+$`)
	for _, w := range words {
		if !pureAlpha.MatchString(w) {
			t.Fatalf("word %q is not a pure [a-z]+ token", w)
		}
	}
}

// TestWordCountHelper verifies that WordCount() returns the same count as
// len(Words()), confirming the pool-level init path.
func TestWordCountHelper(t *testing.T) {
	if n := WordCount(); n != 7772 {
		t.Fatalf("WordCount() = %d, want 7772", n)
	}
}

// TestGenerateUniqueReRolls verifies that GenerateUnique re-rolls past rejected
// candidates and returns a key the predicate does not mark taken. The predicate
// is seeded with a set of "seen" keys grown by the test, not by hooking the RNG.
func TestGenerateUniqueReRolls(t *testing.T) {
	const rejectCount = 5

	seen := make(map[string]bool)
	rejected := 0
	predicate := func(candidate string) bool {
		if rejected < rejectCount {
			seen[candidate] = true
			rejected++
			return true // reject first rejectCount candidates
		}
		return false
	}

	key, err := GenerateUnique(predicate)
	if err != nil {
		t.Fatalf("GenerateUnique() error: %v", err)
	}
	if seen[key] {
		t.Fatalf("GenerateUnique() returned a previously rejected key: %q", key)
	}
	if !keyPattern.MatchString(key) {
		t.Fatalf("GenerateUnique() key %q does not match expected pattern", key)
	}
	if rejected < rejectCount {
		t.Fatalf("predicate was called fewer times than expected: called %d, want >= %d", rejected, rejectCount)
	}
}

// TestGenerateUniqueExhaustionError verifies that GenerateUnique returns an
// error when every candidate is rejected (pathological predicate).
func TestGenerateUniqueExhaustionError(t *testing.T) {
	alwaysTaken := func(string) bool { return true }
	_, err := GenerateUnique(alwaysTaken)
	if err == nil {
		t.Fatal("GenerateUnique should return an error when all candidates are rejected")
	}
}
