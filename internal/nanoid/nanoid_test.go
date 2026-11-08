package nanoid_test

import (
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/nanoid"
)

func TestAlphabet_MatchesExpectedCharset(t *testing.T) {
	// The absence of "-" and ":" is load-bearing for a spawn ack's file name
	// and flag.
	const want = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	if nanoid.Alphabet != want {
		t.Errorf("Alphabet = %q, want %q", nanoid.Alphabet, want)
	}
	for _, forbidden := range []rune{'.', ':', '-', ' '} {
		if strings.ContainsRune(nanoid.Alphabet, forbidden) {
			t.Errorf("Alphabet contains forbidden rune %q; the spawn ack's file name and flag require its absence", forbidden)
		}
	}
}

func TestIsIDShaped(t *testing.T) {
	t.Run("it accepts every rune of the alphabet, and an id of any width up to the bound", func(t *testing.T) {
		for _, r := range nanoid.Alphabet {
			if !nanoid.IsIDShaped(string(r)) {
				t.Errorf("IsIDShaped(%q) = false, want every rune of the alphabet", r)
			}
		}
		for _, id := range []string{"tok123", "b1abcd", strings.Repeat("a", 32)} {
			if !nanoid.IsIDShaped(id) {
				t.Errorf("IsIDShaped(%q) = false, want true", id)
			}
		}
	})

	t.Run("it rejects an empty id, one over the bound, and one carrying a character outside the alphabet", func(t *testing.T) {
		for _, id := range []string{"", strings.Repeat("a", 33), "a-b", "a:b", "a.b", "../a", "a/b", "a b", "a_b", "é"} {
			if nanoid.IsIDShaped(id) {
				t.Errorf("IsIDShaped(%q) = true, want false", id)
			}
		}
	})

	t.Run("every id a generator mints is one", func(t *testing.T) {
		id, err := nanoid.NewGenerator()()
		if err != nil || !nanoid.IsIDShaped(id) {
			t.Errorf("minted %q, %v; want an id", id, err)
		}
	})
}

func TestNewGenerator(t *testing.T) {
	t.Run("it mints a distinct id per call", func(t *testing.T) {
		gen := nanoid.NewGenerator()
		seen := map[string]struct{}{}
		for range 200 {
			id, err := gen()
			if err != nil {
				t.Fatalf("generate id: %v", err)
			}
			if _, dup := seen[id]; dup {
				t.Fatalf("generator minted %q twice in 200 calls", id)
			}
			seen[id] = struct{}{}
		}
	})
}
