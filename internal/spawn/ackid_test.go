package spawn

import (
	"errors"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/nanoid"
)

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

func TestNewSpawnID_GeneratesPathSafeIDAndPropagatesError(t *testing.T) {
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

func TestNewSpawnID_RejectsNonPathSafeGeneratedID(t *testing.T) {
	for _, bad := range []string{"has-hyphen", "has.dot", "has:colon", "has space"} {
		t.Run(bad, func(t *testing.T) {
			id, err := NewSpawnID(queuedGenerator(bad))
			if err == nil {
				t.Fatalf("NewSpawnID(%q) returned nil error, want a not-path-safe error", bad)
			}
			if id != "" {
				t.Errorf("NewSpawnID(%q) id = %q, want empty", bad, id)
			}
		})
	}

	t.Run("empty id from generator", func(t *testing.T) {
		id, err := NewSpawnID(queuedGenerator(""))
		if err == nil {
			t.Fatalf("NewSpawnID(\"\") returned nil error, want a not-path-safe error")
		}
		if id != "" {
			t.Errorf("NewSpawnID(\"\") id = %q, want empty", id)
		}
	})
}

func TestAckFileName_FormatsAndRoundTrips(t *testing.T) {
	name, err := ackFileName("b1abcd", "t2wxyz")
	if want := "b1abcd-t2wxyz"; err != nil || name != want {
		t.Fatalf("ackFileName = %q, %v; want %q", name, err, want)
	}

	batch, token, ok := parseAckFileName(name)
	if !ok || batch != "b1abcd" || token != "t2wxyz" {
		t.Errorf("parseAckFileName(%q) = (%q, %q, %v), want (b1abcd, t2wxyz, true)", name, batch, token, ok)
	}
}

func TestAckFileName_RefusesWhatNoBurstMinted(t *testing.T) {
	for _, pair := range [][2]string{
		{"", "t1"}, {"b1", ""}, {"../b1", "t1"}, {"b1", "t1/x"}, {"b-1", "t1"}, {"b1", "t.1"},
		{strings.Repeat("b", 33), "t1"},
	} {
		if name, err := ackFileName(pair[0], pair[1]); err == nil {
			t.Errorf("ackFileName(%q, %q) = %q, want a refusal", pair[0], pair[1], name)
		}
	}
}

func TestParseAckFileName_RejectsForeignOrDelimiterless(t *testing.T) {
	for _, input := range []string{"onlyonepart", "-t2wxyz", "b1abcd-", "-", "b1-t2-t3", ".b1-t2", "b1-t2.tmp", ""} {
		t.Run(input, func(t *testing.T) {
			if batch, token, ok := parseAckFileName(input); ok || batch != "" || token != "" {
				t.Errorf("parseAckFileName(%q) = (%q, %q, %v), want a reject", input, batch, token, ok)
			}
		})
	}
}

func TestFormatSpawnAckFlag_FormatsAndRoundTrips(t *testing.T) {
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

func TestParseSpawnAckFlag_RejectsMissingColonOrWhatNoBurstMinted(t *testing.T) {
	for _, bad := range []string{"nocolon", ":t1", "b1:", ":", "", "../b1:t1", "b1:t1/x", "b-1:t1", "b1:t1:t2"} {
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
	gen := nanoid.NewGenerator()

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
		if strings.IndexFunc(id, func(r rune) bool { return !strings.ContainsRune(nanoid.Alphabet, r) }) >= 0 {
			t.Errorf("NewSpawnID(real) produced a non-path-safe id %q", id)
		}
	}
}
