package state_test

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

var ruleBooted = time.Date(2026, 11, 4, 8, 0, 0, 0, time.UTC)

func bootedAt(at time.Time) func() (time.Time, error) {
	return func() (time.Time, error) { return at, nil }
}

// claims reports whether a claim of name, created after the boot, under
// ClaimRule(dir) takes over held rather than replacing it.
func claims(t *testing.T, dir string, held sessionindex.Entry) bool {
	t.Helper()
	idx := entryIndexOf(t, held)
	if _, err := idx.Claim(held.Name, "", ruleBooted.Add(2*time.Hour), 4242, state.ClaimRule(dir, anyName)); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	got, _ := idx.Lookup(held.Name)
	return got.Label == held.Label
}

// anyName is a name rule refusing none.
func anyName(string) error { return nil }

// entryIndexOf is a version 2 index holding entries as sessions.json would.
func entryIndexOf(t *testing.T, entries ...sessionindex.Entry) sessionindex.Index {
	t.Helper()
	data, err := json.Marshal(map[string]any{"version": sessionindex.SchemaVersion, "sessions": entries})
	if err != nil {
		t.Fatalf("encode sessions.json: %v", err)
	}
	idx, err := sessionindex.Decode(data)
	if err != nil {
		t.Fatalf("decode sessions.json: %v", err)
	}
	return idx
}

func TestClaimRule(t *testing.T) {
	labelled := func(at time.Time) sessionindex.Entry {
		return sessionindex.Entry{Name: forgotten, Label: "api work", CreatedAt: at, PID: 1717}
	}

	for _, tc := range []struct {
		name    string
		created time.Time
		adopts  bool
	}{
		{"an entry created before the boot is adopted", ruleBooted.Add(-time.Hour), true},
		{"an entry created at the boot time read is adopted", ruleBooted, true},
		{"an entry created within the tolerance after the boot time read is adopted", ruleBooted.Add(9 * time.Second), true},
		{"an entry created past the tolerance is of this boot, and replaced", ruleBooted.Add(10 * time.Second), false},
		{"an entry created an hour into this boot is replaced", ruleBooted.Add(time.Hour), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state.SetBootTimeForTest(t, bootedAt(ruleBooted))

			if got := claims(t, t.TempDir(), labelled(tc.created)); got != tc.adopts {
				t.Errorf("adopted = %v, want %v", got, tc.adopts)
			}
		})
	}

	for name, raise := range map[string]func(dir string) error{
		"the session's hydrating flag is raised":      func(dir string) error { return state.Hydrating.Set(dir, forgotten) },
		"the session's resume-pending flag is raised": func(dir string) error { return state.ResumePending.Set(dir, forgotten) },
	} {
		t.Run("an entry of this boot is adopted while "+name, func(t *testing.T) {
			state.SetBootTimeForTest(t, bootedAt(ruleBooted))
			dir := t.TempDir()
			if err := raise(dir); err != nil {
				t.Fatal(err)
			}

			if !claims(t, dir, labelled(ruleBooted.Add(time.Hour))) {
				t.Error("the entry was replaced, want it adopted")
			}
		})
	}

	t.Run("an entry of this boot is replaced while a bootstrap restores, as only the session's own flags adopt", func(t *testing.T) {
		state.SetBootTimeForTest(t, bootedAt(ruleBooted))
		dir := t.TempDir()
		if err := state.Restoring.Set(dir); err != nil {
			t.Fatal(err)
		}

		if claims(t, dir, labelled(ruleBooted.Add(time.Hour))) {
			t.Error("the entry was adopted, want it replaced")
		}
	})

	t.Run("an entry of this boot is replaced while only another session's flag is raised", func(t *testing.T) {
		state.SetBootTimeForTest(t, bootedAt(ruleBooted))
		dir := t.TempDir()
		if err := state.Hydrating.Set(dir, neighbour); err != nil {
			t.Fatal(err)
		}

		if claims(t, dir, labelled(ruleBooted.Add(time.Hour))) {
			t.Error("the entry was adopted, want it replaced")
		}
	})

	t.Run("a flag that can't be looked for counts as raised", func(t *testing.T) {
		state.SetBootTimeForTest(t, bootedAt(ruleBooted))
		blocker := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(blocker, nil, 0o600); err != nil {
			t.Fatal(err)
		}

		if !claims(t, blocker, labelled(ruleBooted.Add(time.Hour))) {
			t.Error("the entry was replaced, want it adopted")
		}
	})

	t.Run("a boot time that can't be read adopts every entry, and warns", func(t *testing.T) {
		state.SetBootTimeForTest(t, func() (time.Time, error) { return time.Time{}, errors.New("no sysctl") })
		sink := logtest.Install(t)

		if !claims(t, t.TempDir(), labelled(ruleBooted.Add(time.Hour))) {
			t.Error("the entry was replaced, want it adopted")
		}
		rec := sink.Records().Matching("sessions", "claim-rule").AtExactLevel(slog.LevelWarn).Only(t, "the fallback's warning")
		if got := rec.AttrString(t, "op"); got != "claim-rule" {
			t.Errorf("op = %q, want claim-rule", got)
		}
		if rec.ErrorAttr(t, "error") == nil {
			t.Error("the warning carries no error")
		}
	})

	t.Run("a boot time that can't be read warns once a process, however many rules are built", func(t *testing.T) {
		state.SetBootTimeForTest(t, func() (time.Time, error) { return time.Time{}, errors.New("no sysctl") })
		sink := logtest.Install(t)

		for range 3 {
			if !claims(t, t.TempDir(), labelled(ruleBooted.Add(time.Hour))) {
				t.Error("the entry was replaced, want it adopted")
			}
		}
		sink.Records().Matching("sessions", "claim-rule").Only(t, "the one warning")
	})

	t.Run("it reads this machine's boot time", func(t *testing.T) {
		if claims(t, t.TempDir(), labelled(time.Now().Add(-time.Minute))) {
			t.Error("an entry from a minute ago was adopted, want it replaced as of this boot")
		}
	})
}

// decision 72: a restore completes at its helper's claim, so a session whose
// hydrating flag is up has a restore no claim but the helper's finishes.
func TestClaimRule_ReadsTheHydratingFlag(t *testing.T) {
	blocked := func(t *testing.T) string {
		blocker := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(blocker, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		return blocker
	}
	for _, tc := range []struct {
		name       string
		stage      func(t *testing.T) string
		unfinished bool
	}{
		{"a session whose hydrating flag is raised has its restore unfinished", raisedFor(state.Hydrating, forgotten), true},
		{"a session whose hydrating flag can't be looked for has its restore unfinished", blocked, true},
		{"a session whose resume-pending flag alone is raised has not", raisedFor(state.ResumePending, forgotten), false},
		{"a session whose neighbour's hydrating flag alone is raised has not", raisedFor(state.Hydrating, neighbour), false},
	} {
		t.Run(tc.name+", which a claim of it reads", func(t *testing.T) {
			for name, boot := range map[string]func() (time.Time, error){
				"judging by the boot": bootedAt(ruleBooted),
				"with no boot time":   func() (time.Time, error) { return time.Time{}, errors.New("no sysctl") },
			} {
				t.Run(name, func(t *testing.T) {
					state.SetBootTimeForTest(t, boot)
					live := ruleBooted.Add(time.Hour)
					idx := entryIndexOf(t, sessionindex.Entry{Name: forgotten, CreatedAt: live, PID: 4242, Owed: true})

					if _, err := idx.Claim(forgotten, "", live, 4242, state.ClaimRule(tc.stage(t), anyName)); err != nil {
						t.Fatalf("Claim: %v", err)
					}

					if got, _ := idx.Lookup(forgotten); got.Owed != tc.unfinished {
						t.Errorf("entry = %+v; want it owed still %v, its restore unfinished", got, tc.unfinished)
					}
				})
			}
		})
	}
}

// decision 74: no writer of sessions.json keeps an entry under a name restore
// could never make again.
func TestClaimRule_RefusesNamesCreateRefuses(t *testing.T) {
	refused := errors.New("invalid session name: ends with '*'")
	refuse := func(name string) error {
		if name == "foo*" {
			return refused
		}
		return nil
	}
	for name, boot := range map[string]func() (time.Time, error){
		"judging by the boot": bootedAt(ruleBooted),
		"with no boot time":   func() (time.Time, error) { return time.Time{}, errors.New("no sysctl") },
	} {
		t.Run("it refuses a name the rule given refuses, "+name, func(t *testing.T) {
			state.SetBootTimeForTest(t, boot)
			idx := entryIndexOf(t)

			_, err := idx.Claim("foo*", "", ruleBooted.Add(time.Hour), 4242, state.ClaimRule(t.TempDir(), refuse))

			if !errors.Is(err, refused) || !errors.Is(err, sessionindex.ErrNameRefused) || idx.Len() != 0 {
				t.Errorf("Claim = %v, index %+v; want the name refused and no entry", err, idx.Entries())
			}
			if _, err := idx.Claim("foo", "", ruleBooted.Add(time.Hour), 4242, state.ClaimRule(t.TempDir(), refuse)); err != nil {
				t.Errorf("Claim of a name it takes = %v", err)
			}
		})
	}

	t.Run("a rule given no name rule refuses every name", func(t *testing.T) {
		state.SetBootTimeForTest(t, bootedAt(ruleBooted))
		idx := entryIndexOf(t)

		if _, err := idx.Claim(forgotten, "", ruleBooted.Add(time.Hour), 4242, state.ClaimRule(t.TempDir(), nil)); !errors.Is(err, sessionindex.ErrNameRefused) {
			t.Errorf("Claim = %v, want the name refused", err)
		}
	})
}

func TestFlagged(t *testing.T) {
	for _, tc := range []struct {
		name  string
		stage func(t *testing.T) string
		want  bool
	}{
		{"a session with neither flag is not", func(t *testing.T) string { return t.TempDir() }, false},
		{"a session whose hydrating flag is raised is", raisedFor(state.Hydrating, forgotten), true},
		{"a session whose resume-pending flag is raised is", raisedFor(state.ResumePending, forgotten), true},
		{"a session whose neighbour's flag alone is raised is not", raisedFor(state.Hydrating, neighbour), false},
		{"a session whose flags can't be looked for is", func(t *testing.T) string {
			blocker := filepath.Join(t.TempDir(), "file")
			if err := os.WriteFile(blocker, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			return blocker
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := state.Flagged(tc.stage(t), forgotten); got != tc.want {
				t.Errorf("Flagged = %v, want %v", got, tc.want)
			}
		})
	}
}

// raisedFor stages a state directory with flag raised for name.
func raisedFor(flag state.SessionFlag, name string) func(t *testing.T) string {
	return func(t *testing.T) string {
		dir := t.TempDir()
		if err := flag.Set(dir, name); err != nil {
			t.Fatal(err)
		}
		return dir
	}
}
