package state_test

import (
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
	idx := sessionindex.Index{Sessions: []sessionindex.Entry{held}}
	idx.Claim(held.Name, "", ruleBooted.Add(2*time.Hour), 4242, state.ClaimRule(dir))
	got, _ := idx.Lookup(held.Name)
	return got.Label == held.Label
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
		"a bootstrap is restoring":                    state.Restoring.Set,
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

	t.Run("it reads this machine's boot time", func(t *testing.T) {
		if claims(t, t.TempDir(), labelled(time.Now().Add(-time.Minute))) {
			t.Error("an entry from a minute ago was adopted, want it replaced as of this boot")
		}
	})
}
