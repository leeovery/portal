package spawn

import (
	"errors"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/session"
)

// queuedGenerator returns each queued id in order, then empty strings once
// exhausted. It never errors — the erroring case is modelled inline.
func queuedGenerator(ids ...string) func() (string, error) {
	i := 0
	return func() (string, error) {
		if i >= len(ids) {
			return "", nil
		}
		id := ids[i]
		i++
		return id, nil
	}
}

func TestNewSpawnID_GeneratesOptionSafeIDAndPropagatesError(t *testing.T) {
	// "it generates a non-empty option-safe id and propagates a generator error to an empty id"
	id, err := NewSpawnID(queuedGenerator("b1abcd"))
	if err != nil {
		t.Fatalf("NewSpawnID(ok) returned error %v, want nil", err)
	}
	if id != "b1abcd" {
		t.Errorf("NewSpawnID(ok) = %q, want %q", id, "b1abcd")
	}

	sentinel := errors.New("generator boom")
	id, err = NewSpawnID(func() (string, error) { return "", sentinel })
	if err == nil {
		t.Fatalf("NewSpawnID(erroring) returned nil error, want non-nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("NewSpawnID(erroring) error = %v, want it to wrap %v", err, sentinel)
	}
	if id != "" {
		t.Errorf("NewSpawnID(erroring) id = %q, want empty (never a partial/blank id)", id)
	}
}

func TestNewSpawnID_RejectsNonOptionSafeGeneratedID(t *testing.T) {
	// "it rejects a generated id containing a hyphen, dot, colon or space"
	for _, bad := range []string{"has-hyphen", "has.dot", "has:colon", "has space"} {
		t.Run(bad, func(t *testing.T) {
			id, err := NewSpawnID(queuedGenerator(bad))
			if err == nil {
				t.Fatalf("NewSpawnID(%q) returned nil error, want a not-option-safe error", bad)
			}
			if id != "" {
				t.Errorf("NewSpawnID(%q) id = %q, want empty", bad, id)
			}
		})
	}

	// A generator returning an empty (yet non-erroring) id is also rejected —
	// the function never treats an empty id as valid.
	t.Run("empty id from generator", func(t *testing.T) {
		id, err := NewSpawnID(queuedGenerator(""))
		if err == nil {
			t.Fatalf("NewSpawnID(\"\") returned nil error, want a not-option-safe error")
		}
		if id != "" {
			t.Errorf("NewSpawnID(\"\") id = %q, want empty", id)
		}
	})
}

func TestSpawnMarkerName_FormatsAndRoundTrips(t *testing.T) {
	// "it formats and round-trips a marker name to (batch, token)"
	name := SpawnMarkerName("b1abcd", "t2wxyz")
	if want := "@portal-spawn-b1abcd-t2wxyz"; name != want {
		t.Fatalf("SpawnMarkerName = %q, want %q", name, want)
	}

	batch, token, ok := ParseSpawnMarkerName(name)
	if !ok {
		t.Fatalf("ParseSpawnMarkerName(%q) ok = false, want true", name)
	}
	if batch != "b1abcd" || token != "t2wxyz" {
		t.Errorf("ParseSpawnMarkerName(%q) = (%q, %q), want (%q, %q)", name, batch, token, "b1abcd", "t2wxyz")
	}
}

func TestParseSpawnMarkerName_RejectsForeignOrDelimiterless(t *testing.T) {
	// "it rejects a foreign-prefixed or delimiter-less marker name on parse"
	tests := []struct {
		name  string
		input string
	}{
		{name: "foreign skeleton prefix", input: "@portal-skeleton-foo"},
		{name: "no delimiter after prefix", input: "@portal-spawn-onlyonepart"},
		{name: "empty batch (leading delimiter)", input: "@portal-spawn--t2wxyz"},
		{name: "empty token (trailing delimiter)", input: "@portal-spawn-b1abcd-"},
		{name: "bare prefix", input: "@portal-spawn-"},
		{name: "unrelated name", input: "@portal-restoring"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			batch, token, ok := ParseSpawnMarkerName(tt.input)
			if ok {
				t.Errorf("ParseSpawnMarkerName(%q) ok = true, want false", tt.input)
			}
			if batch != "" || token != "" {
				t.Errorf("ParseSpawnMarkerName(%q) = (%q, %q), want empty strings on reject", tt.input, batch, token)
			}
		})
	}
}

func TestFormatSpawnAckFlag_FormatsAndRoundTrips(t *testing.T) {
	// "it formats and round-trips the <batch>:<token> ack flag value"
	value := FormatSpawnAckFlag("b1abcd", "t2wxyz")
	if want := "b1abcd:t2wxyz"; value != want {
		t.Fatalf("FormatSpawnAckFlag = %q, want %q", value, want)
	}

	batch, token, ok := ParseSpawnAckFlag(value)
	if !ok {
		t.Fatalf("ParseSpawnAckFlag(%q) ok = false, want true", value)
	}
	if batch != "b1abcd" || token != "t2wxyz" {
		t.Errorf("ParseSpawnAckFlag(%q) = (%q, %q), want (%q, %q)", value, batch, token, "b1abcd", "t2wxyz")
	}
}

func TestParseSpawnAckFlag_RejectsMissingColonOrEmptyPart(t *testing.T) {
	// "it rejects a flag value with a missing colon or an empty batch or token"
	for _, bad := range []string{"nocolon", ":t1", "b1:", ":", ""} {
		t.Run(bad, func(t *testing.T) {
			batch, token, ok := ParseSpawnAckFlag(bad)
			if ok {
				t.Errorf("ParseSpawnAckFlag(%q) ok = true, want false", bad)
			}
			if batch != "" || token != "" {
				t.Errorf("ParseSpawnAckFlag(%q) = (%q, %q), want empty strings on reject", bad, batch, token)
			}
		})
	}
}

func TestNewSpawnID_IndependentRealGeneratorIDs(t *testing.T) {
	// Two independent NewSpawnID calls with the real generator produce two
	// non-empty option-safe strings — no cached id reuse. Content is random,
	// so only charset/non-empty are asserted (never the random value itself).
	gen := session.NewNanoIDGenerator()

	first, err := NewSpawnID(gen)
	if err != nil {
		t.Fatalf("NewSpawnID(real) first call error: %v", err)
	}
	second, err := NewSpawnID(gen)
	if err != nil {
		t.Fatalf("NewSpawnID(real) second call error: %v", err)
	}

	if first == second {
		t.Errorf("NewSpawnID(real) returned the same id twice (%q) — ids must be independent, not cached", first)
	}

	for _, id := range []string{first, second} {
		if id == "" {
			t.Fatalf("NewSpawnID(real) produced an empty id")
		}
		if strings.IndexFunc(id, func(r rune) bool { return !strings.ContainsRune(session.NanoIDAlphabet, r) }) >= 0 {
			t.Errorf("NewSpawnID(real) produced a non-option-safe id %q", id)
		}
	}
}

func TestIsOptionSafeID_GovernedBySharedNanoIDAlphabet(t *testing.T) {
	// The spawn ack-id option-safety check must be governed by exactly the one
	// shared session.NanoIDAlphabet — no more, no less. The whole shared alphabet
	// is option-safe; any char outside it (here the load-bearing "-") is rejected.
	// This pins the ack-id vocabulary to the single shared constant and guards
	// against a silent re-divergence of the charset.
	if !isOptionSafeID(session.NanoIDAlphabet) {
		t.Errorf("isOptionSafeID(NanoIDAlphabet) = false, want true (the whole shared alphabet must be option-safe)")
	}
	if isOptionSafeID(session.NanoIDAlphabet + "-") {
		t.Errorf("isOptionSafeID accepted a hyphen; its charset must be exactly session.NanoIDAlphabet, which excludes %q", "-")
	}
}
