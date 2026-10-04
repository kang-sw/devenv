package wskey

import (
	"regexp"
	"strings"
	"testing"
)

// keyPattern is the expected format: 3 lowercase-word segments.
var keyPattern = regexp.MustCompile(`^[a-z]+(-[a-z]+){2}$`)

// TestGenerateFormat verifies that minted keys match the opaque word-chain format
// contract: three hyphen-separated lowercase words, e.g. "amber-tide-fox".
func TestGenerateFormat(t *testing.T) {
	for i := 0; i < 20; i++ {
		key, err := Generate()
		if err != nil {
			t.Fatalf("Generate() error: %v", err)
		}
		if !keyPattern.MatchString(key) {
			t.Fatalf("Generate() key %q does not match expected pattern ^[a-z]+(-[a-z]+){2}$", key)
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
// candidates and returns a key the predicate does not mark taken. Determinism is
// driven by the predicate, not the RNG: the predicate rejects the first
// rejectCount fresh candidates (and always rejects already-seen candidates),
// then accepts the next one.
func TestGenerateUniqueReRolls(t *testing.T) {
	const rejectCount = 5

	seen := make(map[string]bool)
	var rejected int
	predicate := func(candidate string) bool {
		if seen[candidate] {
			return true // always reject already-seen candidates to avoid false-pass on hash collision
		}
		if rejected < rejectCount {
			seen[candidate] = true
			rejected++
			return true // reject first rejectCount fresh candidates
		}
		return false // accept the first unseen candidate beyond rejectCount
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
	// The predicate must have been called at least rejectCount+1 times:
	// rejectCount rejections and at least one acceptance.
	if rejected != rejectCount {
		t.Fatalf("predicate rejected %d candidates, want exactly %d", rejected, rejectCount)
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

// TestDeriveDeterministic verifies that Derive is a pure function of its
// inputs: the same seed and word count always produce the same key.
func TestDeriveDeterministic(t *testing.T) {
	a := Derive("260900-feat-x", 3)
	b := Derive("260900-feat-x", 3)
	if a != b {
		t.Fatalf("Derive() not deterministic: %q != %q", a, b)
	}
}

// TestDeriveDifferentSeedsDiffer verifies that a representative set of
// distinct seeds produce distinct keys, catching a broken/constant
// derivation. This is not a strict collision guarantee.
func TestDeriveDifferentSeedsDiffer(t *testing.T) {
	seeds := []string{
		"260900-feat-x",
		"260900-feat-y",
		"260901-bug-z",
		"alpha",
		"beta",
	}
	seen := make(map[string]string, len(seeds))
	for _, seed := range seeds {
		key := Derive(seed, 3)
		if other, ok := seen[key]; ok {
			t.Fatalf("Derive(%q) and Derive(%q) both produced %q", seed, other, key)
		}
		seen[key] = seed
	}
}

// TestDeriveLengthBound verifies the derived key (words=3, <=5-char sub-pool)
// never exceeds the worst-case bound of 5+1+5+1+5=17 characters.
func TestDeriveLengthBound(t *testing.T) {
	seeds := []string{"a", "b", "c", "260900-feat-x", "some-longer-seed-value"}
	for _, seed := range seeds {
		key := Derive(seed, 3)
		if len(key) > 17 {
			t.Fatalf("Derive(%q) = %q, length %d exceeds worst-case bound 17", seed, key, len(key))
		}
		if !keyPattern.MatchString(key) {
			t.Fatalf("Derive(%q) = %q does not match expected pattern ^[a-z]+(-[a-z]+){2}$", seed, key)
		}
	}
}

// TestDeriveFull pins full-pool selection while preserving short-pool output.
func TestDeriveFull(t *testing.T) {
	const want = "headless-routine-remodeler-passage"
	for i := 0; i < 2; i++ {
		if got := DeriveFull("amber-tide-fox", 4); got != want {
			t.Fatalf("DeriveFull() = %q, want %q", got, want)
		}
	}
	pool := make(map[string]bool)
	for _, word := range Words() {
		pool[word] = true
		if len(word) > 9 {
			t.Fatalf("word %q exceeds reply-ID word length bound", word)
		}
	}
	for _, seed := range []string{"amber-tide-fox", "other-session-key", "260900-feat-x"} {
		key := DeriveFull(seed, 4)
		words := strings.Split(key, "-")
		if len(words) != 4 || len(strings.Join(words, "")) > 36 {
			t.Fatalf("DeriveFull(%q) = %q violates four-word/36-letter bound", seed, key)
		}
		for _, word := range words {
			if !pool[word] {
				t.Fatalf("derived word %q is not in full pool", word)
			}
		}
	}
	// The pinned vector includes >5-letter words; using the short pool cannot pass.
	if got := Derive("amber-tide-fox", 3); got != "bash-blunt-tamer" {
		t.Fatalf("existing Derive output changed: %q", got)
	}
}

// TestDeriveShortSubPoolCount pins the collision-space claim: exactly 1476 of
// the 7772 embedded words are <=5 characters.
func TestDeriveShortSubPoolCount(t *testing.T) {
	var count int
	for _, w := range Words() {
		if len(w) <= 5 {
			count++
		}
	}
	if count != 1476 {
		t.Fatalf("expected 1476 words with length <= 5, got %d", count)
	}
}
