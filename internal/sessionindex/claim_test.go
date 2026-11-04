package sessionindex_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/sessionindex"
)

var (
	thisBoot        = time.Date(2026, 11, 4, 8, 0, 0, 0, time.UTC)
	beforeBoot      = thisBoot.Add(-26 * time.Hour)
	earlierThisBoot = thisBoot.Add(time.Hour)
	liveCreation    = thisBoot.Add(2 * time.Hour)
)

// ruleRestoring judges by thisBoot, the sessions named in restoring reporting
// as restoring.
func ruleRestoring(restoring ...string) sessionindex.Rule {
	return sessionindex.NewRule(thisBoot, func(name string) bool { return slices.Contains(restoring, name) })
}

// claimFor claims the session's entry under ruleRestoring, as a fixture for a
// test about something else, failing it on a refusal.
func claimFor(t *testing.T, idx *sessionindex.Index, name, dir string, created time.Time, pid int) {
	t.Helper()
	if _, err := idx.Claim(name, dir, created, pid, ruleRestoring()); err != nil {
		t.Fatalf("Claim(%s): %v", name, err)
	}
}

// keeping is a's entry holding everything a claim must not lose, created at
// at, running pid.
func keeping(at time.Time, pid int) sessionindex.Entry {
	return sessionindex.Entry{
		Name: "a", Label: "Old", Dir: "/kept", Cwd: "/was/here",
		CreatedAt: at, PID: pid, ScrollbackFile: "scrollback/a-1793000000-1717.bin",
	}
}

// owed is e marked owed a restore.
func owed(e sessionindex.Entry) sessionindex.Entry {
	e.Owed = true
	return e
}

func withDir(e sessionindex.Entry, dir string) sessionindex.Entry {
	e.Dir = dir
	return e
}

func seedIndex(t *testing.T, dir string, entries ...sessionindex.Entry) {
	t.Helper()
	data, err := sessionindex.Encode(sessionindex.IndexOf(entries))
	if err != nil {
		t.Fatalf("encode seed: %v", err)
	}
	writeIndex(t, dir, string(data))
}

// sameEntry compares entries field by field, creation times as instants.
func sameEntry(got, want sessionindex.Entry) bool {
	if !got.CreatedAt.Equal(want.CreatedAt) {
		return false
	}
	got.CreatedAt, want.CreatedAt = time.Time{}, time.Time{}
	return got == want
}

func sameEntries(got, want []sessionindex.Entry) bool {
	return slices.EqualFunc(got, want, sameEntry)
}

func TestClaim(t *testing.T) {
	judged := ruleRestoring()
	fresh := sessionindex.Entry{Name: "a", Dir: "/a", CreatedAt: liveCreation, PID: 4242}

	for _, tc := range []struct {
		name     string
		held     sessionindex.Entry
		rule     sessionindex.Rule
		created  time.Time
		pid      int
		want     sessionindex.Entry
		replaced bool
		refused  error
	}{
		{
			name: "an entry of the same creation keeps everything",
			held: keeping(liveCreation, 4242), rule: judged, created: liveCreation, pid: 4242,
			want: keeping(liveCreation, 4242),
		},
		{
			name: "an entry of the same creation with no dir takes the one given",
			held: withDir(keeping(liveCreation, 4242), ""), rule: judged, created: liveCreation, pid: 4242,
			want: withDir(keeping(liveCreation, 4242), "/a"),
		},
		{
			name: "an entry from the same time recording no pid is the same creation, and takes the pid",
			held: keeping(liveCreation, 0), rule: judged, created: liveCreation, pid: 4242,
			want: keeping(liveCreation, 4242),
		},
		{
			name: "an entry from the same time claimed with no pid is the same creation, keeping its pid",
			held: keeping(liveCreation, 4242), rule: judged, created: liveCreation,
			want: keeping(liveCreation, 4242),
		},
		{
			name: "a creation time recorded in another zone is the same creation",
			held: keeping(liveCreation, 4242), rule: judged, created: liveCreation.In(time.FixedZone("x", 3600)), pid: 4242,
			want: keeping(liveCreation, 4242),
		},
		{
			name: "an entry whose creation was never recorded is adopted, keeping everything",
			held: keeping(time.Time{}, 0), rule: judged, created: liveCreation, pid: 4242,
			want: keeping(liveCreation, 4242),
		},
		{
			name: "an entry saved before this boot is adopted, keeping everything",
			held: keeping(beforeBoot, 1717), rule: judged, created: liveCreation, pid: 4242,
			want: keeping(liveCreation, 4242),
		},
		{
			name: "an entry saved a second before this boot is adopted",
			held: keeping(thisBoot.Add(-time.Second), 1717), rule: judged, created: liveCreation, pid: 4242,
			want: keeping(liveCreation, 4242),
		},
		{
			name: "an adopted entry with no dir takes the one given",
			held: withDir(keeping(beforeBoot, 1717), ""), rule: judged, created: liveCreation, pid: 4242,
			want: withDir(keeping(liveCreation, 4242), "/a"),
		},
		{
			name: "an entry of this boot whose session is restoring is adopted, keeping everything",
			held: keeping(earlierThisBoot, 1717), rule: ruleRestoring("a"), created: liveCreation, pid: 4242,
			want: keeping(liveCreation, 4242),
		},
		{
			name: "under the zero rule an entry of this boot is adopted, keeping everything",
			held: keeping(earlierThisBoot, 1717), rule: sessionindex.Rule{}, created: liveCreation, pid: 4242,
			want: keeping(liveCreation, 4242),
		},
		{
			name: "an entry of this boot claimed knowing no creation is refused, and kept untouched",
			held: keeping(earlierThisBoot, 1717), rule: judged,
			want: keeping(earlierThisBoot, 1717), refused: sessionindex.ErrNoCreation,
		},
		{
			name: "an entry saved before this boot claimed knowing no creation is refused, and kept untouched",
			held: keeping(beforeBoot, 1717), rule: judged,
			want: keeping(beforeBoot, 1717), refused: sessionindex.ErrNoCreation,
		},
		{
			name: "an entry recording no creation or dir claimed knowing no creation is refused, and kept untouched",
			held: withDir(keeping(time.Time{}, 0), ""), rule: judged,
			want: withDir(keeping(time.Time{}, 0), ""), refused: sessionindex.ErrNoCreation,
		},
		{
			name: "an entry under a name the rule refuses is refused, and kept untouched",
			held: keeping(earlierThisBoot, 1717), rule: judged.RefusingNames(refusing("a")), created: liveCreation, pid: 4242,
			want: keeping(earlierThisBoot, 1717), refused: sessionindex.ErrNameRefused,
		},
		{
			name: "an owed entry of the same creation is owed no more, keeping everything else",
			held: owed(keeping(liveCreation, 4242)), rule: judged, created: liveCreation, pid: 4242,
			want: keeping(liveCreation, 4242),
		},
		{
			name: "an owed entry of an earlier creation this boot is adopted, keeping everything, and owed no more",
			held: owed(keeping(earlierThisBoot, 1717)), rule: judged, created: liveCreation, pid: 4242,
			want: keeping(liveCreation, 4242),
		},
		{
			name: "an owed entry claimed knowing no creation is refused, and stays owed",
			held: owed(keeping(earlierThisBoot, 1717)), rule: judged,
			want: owed(keeping(earlierThisBoot, 1717)), refused: sessionindex.ErrNoCreation,
		},
		{
			name: "an owed entry of the same creation stays owed while its session is hydrating",
			held: owed(keeping(liveCreation, 4242)), rule: judged.ReadingHydrating(hydrating("a")), created: liveCreation, pid: 4242,
			want: owed(keeping(liveCreation, 4242)),
		},
		{
			name: "an entry saved before this boot is adopted while its session is hydrating, and owed a restore",
			held: keeping(beforeBoot, 1717), rule: judged.ReadingHydrating(hydrating("a")), created: liveCreation, pid: 4242,
			want: owed(keeping(liveCreation, 4242)),
		},
		{
			name: "an owed entry is adopted while its session is hydrating, and stays owed",
			held: owed(keeping(beforeBoot, 1717)), rule: judged.ReadingHydrating(hydrating("a")), created: liveCreation, pid: 4242,
			want: owed(keeping(liveCreation, 4242)),
		},
		{
			name: "an owed entry is adopted, and owed no more, while only another session is hydrating",
			held: owed(keeping(beforeBoot, 1717)), rule: judged.ReadingHydrating(hydrating("b")), created: liveCreation, pid: 4242,
			want: keeping(liveCreation, 4242),
		},
		{
			name: "an entry of an earlier creation this boot is replaced whole",
			held: keeping(earlierThisBoot, 1717), rule: judged, created: liveCreation, pid: 4242,
			want: fresh, replaced: true,
		},
		{
			name: "an entry created at this boot's instant is of this boot, and replaced whole",
			held: keeping(thisBoot, 1717), rule: judged, created: liveCreation, pid: 4242,
			want: fresh, replaced: true,
		},
		{
			name: "an entry from the same time with another pid is another creation, replaced whole",
			held: keeping(liveCreation, 1717), rule: judged, created: liveCreation, pid: 4242,
			want: fresh, replaced: true,
		},
		{
			name: "an entry from another time this boot with the same pid is another creation, replaced whole",
			held: keeping(earlierThisBoot, 4242), rule: judged, created: liveCreation, pid: 4242,
			want: fresh, replaced: true,
		},
		{
			name: "an entry of this boot is replaced whole while only another session is restoring",
			held: keeping(earlierThisBoot, 1717), rule: ruleRestoring("b"), created: liveCreation, pid: 4242,
			want: fresh, replaced: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			neighbour := sessionindex.Entry{Name: "b", Label: "Beta", CreatedAt: earlierThisBoot}
			seedIndex(t, dir, tc.held, neighbour)

			var changed bool
			var claimErr error
			outcome, err := sessionindex.Update(dir, "cli", func(idx *sessionindex.Index) error {
				changed, claimErr = idx.Claim("a", "/a", tc.created, tc.pid, tc.rule)
				return nil
			})
			if err != nil {
				t.Fatalf("Update: %v", err)
			}
			if (claimErr == nil) != (tc.refused == nil) || !errors.Is(claimErr, tc.refused) {
				t.Errorf("Claim error = %v, want %v", claimErr, tc.refused)
			}
			if want := !sameEntry(tc.held, tc.want); changed != want {
				t.Errorf("Claim = %v, want %v: it reports whether it changed the entry", changed, want)
			}

			idx, err := sessionindex.Read(dir)
			if err != nil {
				t.Fatalf("Read: %v", err)
			}
			if !sameEntries(idx.Entries(), []sessionindex.Entry{tc.want, neighbour}) {
				t.Errorf("sessions = %+v, want a as %+v beside b untouched", idx.Entries(), tc.want)
			}
			var wantReplaced []sessionindex.Entry
			if tc.replaced {
				wantReplaced = []sessionindex.Entry{tc.held}
			}
			if !sameEntries(outcome.Replaced, wantReplaced) || len(outcome.Removed) != 0 {
				t.Errorf("outcome = %+v, want replaced %+v and nothing removed", outcome, wantReplaced)
			}
		})
	}

	t.Run("a session with no entry gets one, recording its creation", func(t *testing.T) {
		idx := sessionindex.IndexOf([]sessionindex.Entry{{Name: "b"}})

		if changed, err := idx.Claim("a", "/a", liveCreation, 4242, judged); !changed || err != nil {
			t.Errorf("Claim = %v, %v; want true, nil for the entry it made", changed, err)
		}

		if got, ok := idx.Lookup("a"); !ok || !sameEntry(got, fresh) || idx.Len() != 2 {
			t.Errorf("sessions = %+v, want b and a as %+v", idx.Entries(), fresh)
		}
	})

	// decisions 32 and 74: a mint, rename, hook set or saver claim never
	// records an entry no restore could make again, nor one no drop could
	// remove.
	for name, c := range map[string]struct {
		name    string
		created time.Time
		rule    sessionindex.Rule
		refused error
	}{
		"knowing no creation":           {name: "a", rule: judged, refused: sessionindex.ErrNoCreation},
		"under a name the rule refuses": {name: "foo*", created: liveCreation, rule: judged.RefusingNames(refusing("foo*")), refused: sessionindex.ErrNameRefused},
	} {
		t.Run("a session with no entry claimed "+name+" is refused, and gets none", func(t *testing.T) {
			idx := sessionindex.IndexOf([]sessionindex.Entry{{Name: "b"}})

			changed, err := idx.Claim(c.name, "/a", c.created, 4242, c.rule)

			if changed || !errors.Is(err, c.refused) || !strings.Contains(err.Error(), c.name) {
				t.Errorf("Claim = %v, %v; want false and a refusal naming %s, %v", changed, err, c.name, c.refused)
			}
			if idx.Len() != 1 {
				t.Errorf("sessions = %+v, want b alone", idx.Entries())
			}
		})
	}

	t.Run("a refused name carries the name rule's own reason", func(t *testing.T) {
		idx := sessionindex.IndexOf(nil)

		_, err := idx.Claim("foo*", "/a", liveCreation, 4242, judged.RefusingNames(refusing("foo*")))

		if !errors.Is(err, errRefusedName) {
			t.Errorf("Claim = %v, want it to wrap the name rule's %v", err, errRefusedName)
		}
	})

	t.Run("a session whose entry this Update removed gets none, and the claim reports no change", func(t *testing.T) {
		idx := sessionindex.IndexOf([]sessionindex.Entry{keeping(liveCreation, 4242)})
		idx.Remove("a")

		if changed, err := idx.Claim("a", "/a", liveCreation, 4242, judged); changed || err != nil {
			t.Errorf("Claim = %v, %v; want false, nil for the entry it could not make", changed, err)
		}
		if _, ok := idx.Lookup("a"); ok {
			t.Errorf("sessions = %+v, want a's entry still removed", idx.Entries())
		}
	})
}

// errRefusedName is the name rule refusing's refusal.
var errRefusedName = errors.New("invalid session name")

// refusing is a name rule refusing the names given.
func refusing(names ...string) func(string) error {
	return func(name string) error {
		if slices.Contains(names, name) {
			return errRefusedName
		}
		return nil
	}
}

// hydrating reports the names given as sessions whose restore is unfinished.
func hydrating(names ...string) func(string) bool {
	return func(name string) bool { return slices.Contains(names, name) }
}

func TestLookupLive(t *testing.T) {
	judged := ruleRestoring()

	for _, tc := range []struct {
		name    string
		held    sessionindex.Entry
		rule    sessionindex.Rule
		created time.Time
		pid     int
		wantOK  bool
	}{
		{"the same creation finds the entry", keeping(liveCreation, 4242), judged, liveCreation, 4242, true},
		{"the same creation in another zone finds the entry", keeping(liveCreation, 4242), judged, liveCreation.In(time.FixedZone("BST", 3600)), 4242, true},
		{"the same time from a live session reporting no pid finds the entry", keeping(liveCreation, 4242), judged, liveCreation, 0, true},
		{"the same time on an entry recording no pid finds it", keeping(liveCreation, 0), judged, liveCreation, 4242, true},
		{"an entry whose creation was never recorded is found, as a claim would adopt it", keeping(time.Time{}, 0), judged, liveCreation, 4242, true},
		{"an entry saved before this boot is found, as a claim would adopt it", keeping(beforeBoot, 1717), judged, liveCreation, 4242, true},
		{"an entry of this boot whose session is restoring is found", keeping(earlierThisBoot, 1717), ruleRestoring("a"), liveCreation, 4242, true},
		{"under the zero rule an entry of this boot is found", keeping(earlierThisBoot, 1717), sessionindex.Rule{}, liveCreation, 4242, true},
		{"an earlier creation this boot finds none", keeping(earlierThisBoot, 1717), judged, liveCreation, 4242, false},
		{"the same time with another pid finds none", keeping(liveCreation, 1717), judged, liveCreation, 4242, false},
		{"a live session reporting no creation finds none", keeping(liveCreation, 4242), judged, time.Time{}, 4242, false},
		{"a live session reporting no creation finds none, even of an entry saved before this boot", keeping(beforeBoot, 1717), judged, time.Time{}, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			idx := sessionindex.IndexOf([]sessionindex.Entry{tc.held})

			got, ok := idx.LookupLive("a", tc.created, tc.pid, tc.rule)

			want := sessionindex.Entry{}
			if tc.wantOK {
				want = tc.held
			}
			if ok != tc.wantOK || !sameEntry(got, want) {
				t.Errorf("LookupLive = %+v, %v; want %+v, %v", got, ok, want, tc.wantOK)
			}
		})
	}

	t.Run("no entry finds none", func(t *testing.T) {
		idx := sessionindex.IndexOf([]sessionindex.Entry{{Name: "b", CreatedAt: liveCreation}})
		if got, ok := idx.LookupLive("a", liveCreation, 0, judged); ok {
			t.Errorf("LookupLive = %+v, true; want none", got)
		}
	})

	for name, label := range map[string]string{"an unprintable label": "api\nwork", "a label of only whitespace": "   "} {
		t.Run(name+" is dropped, the rest kept, and left in the index", func(t *testing.T) {
			held := keeping(liveCreation, 4242)
			held.Label = label
			idx := sessionindex.IndexOf([]sessionindex.Entry{held})

			got, ok := idx.LookupLive("a", liveCreation, 4242, judged)

			want := held
			want.Label = ""
			if !ok || !sameEntry(got, want) {
				t.Errorf("LookupLive = %+v, %v; want %+v", got, ok, want)
			}
			if idx.Entries()[0].Label != label {
				t.Errorf("Label = %q, want the file's label untouched", idx.Entries()[0].Label)
			}
		})
	}
}

func TestRestored(t *testing.T) {
	saved := keeping(beforeBoot, 1717)

	// decision 72: an entry recording a creation of this boot no longer may
	// predate it, so the mark alone keeps it from a drop until the session's
	// helper finishes the restore.
	t.Run("it records the new creation, owed until its helper finishes, and keeps everything else", func(t *testing.T) {
		idx := sessionindex.IndexOf([]sessionindex.Entry{saved, {Name: "b", Label: "Beta"}})

		if !idx.Restored(saved, liveCreation, 4242) {
			t.Fatal("Restored = false, want the creation recorded")
		}

		if got, _ := idx.Lookup("a"); !sameEntry(got, owed(keeping(liveCreation, 4242))) {
			t.Errorf("entry = %+v, want %+v", got, owed(keeping(liveCreation, 4242)))
		}
		if got, _ := idx.Lookup("b"); got != (sessionindex.Entry{Name: "b", Label: "Beta"}) {
			t.Errorf("b = %+v, want it untouched", got)
		}
	})

	t.Run("it records the creation over whatever creation restore read, with no rule to consult", func(t *testing.T) {
		read := keeping(earlierThisBoot, 1717)
		idx := sessionindex.IndexOf([]sessionindex.Entry{read})

		if !idx.Restored(read, liveCreation, 4242) {
			t.Fatal("Restored = false, want the creation recorded")
		}
		if got, _ := idx.Lookup("a"); !sameEntry(got, owed(keeping(liveCreation, 4242))) {
			t.Errorf("entry = %+v, want %+v", got, owed(keeping(liveCreation, 4242)))
		}
	})

	t.Run("an entry already recording the creation reports it recorded", func(t *testing.T) {
		idx := sessionindex.IndexOf([]sessionindex.Entry{keeping(liveCreation, 4242)})

		if !idx.Restored(saved, liveCreation, 4242) {
			t.Error("Restored = false, want the creation reported recorded")
		}
	})

	// A raw recreation of the name, adopted by a claim while restore waited
	// for the lock, is the live session now: restore's dead creation written
	// over it would have the next claim replace the entry, and forget what it
	// kept.
	for name, c := range map[string]struct{ read, since sessionindex.Entry }{
		"another creation":                   {saved, keeping(earlierThisBoot, 7)},
		"the same time under another pid":    {saved, keeping(beforeBoot, 7)},
		"another time under the same pid":    {saved, keeping(earlierThisBoot, 1717)},
		"a creation where restore read none": {keeping(time.Time{}, 0), keeping(earlierThisBoot, 7)},
	} {
		t.Run("an entry taken over for "+name+" since restore read it is left as it is", func(t *testing.T) {
			idx := sessionindex.IndexOf([]sessionindex.Entry{c.since})

			if idx.Restored(c.read, liveCreation, 4242) {
				t.Error("Restored = true, want false: the entry records a creation restore didn't read")
			}
			if got, _ := idx.Lookup("a"); !sameEntry(got, c.since) {
				t.Errorf("entry = %+v, want it untouched, %+v", got, c.since)
			}
		})
	}

	for name, c := range map[string]struct{ read, since sessionindex.Entry }{
		"one that never recorded a creation":         {keeping(time.Time{}, 0), keeping(time.Time{}, 0)},
		"the creation read, its pid filled in since": {keeping(beforeBoot, 0), keeping(beforeBoot, 1717)},
	} {
		t.Run("it records the creation over "+name, func(t *testing.T) {
			idx := sessionindex.IndexOf([]sessionindex.Entry{c.since})

			if !idx.Restored(c.read, liveCreation, 4242) {
				t.Fatal("Restored = false, want the creation recorded")
			}
			if got, _ := idx.Lookup("a"); !sameEntry(got, owed(keeping(liveCreation, 4242))) {
				t.Errorf("entry = %+v, want %+v", got, owed(keeping(liveCreation, 4242)))
			}
		})
	}

	t.Run("an owed entry it records the creation of stays owed", func(t *testing.T) {
		idx := sessionindex.IndexOf([]sessionindex.Entry{owed(saved)})

		if !idx.Restored(owed(saved), liveCreation, 4242) {
			t.Fatal("Restored = false, want the creation recorded")
		}
		if got, _ := idx.Lookup("a"); !sameEntry(got, owed(keeping(liveCreation, 4242))) {
			t.Errorf("entry = %+v, want %+v", got, owed(keeping(liveCreation, 4242)))
		}
	})

	// The helper's claim can land before restore's record of the creation,
	// and has finished the restore when it replayed.
	for name, held := range map[string]sessionindex.Entry{
		"owed":         owed(keeping(liveCreation, 4242)),
		"owed nothing": keeping(liveCreation, 4242),
	} {
		t.Run("an entry already recording the creation, "+name+", is left as it is", func(t *testing.T) {
			idx := sessionindex.IndexOf([]sessionindex.Entry{held})

			if !idx.Restored(saved, liveCreation, 4242) {
				t.Error("Restored = false, want the creation reported recorded")
			}
			if got, _ := idx.Lookup("a"); !sameEntry(got, held) {
				t.Errorf("entry = %+v, want it untouched, %+v", got, held)
			}
		})
	}

	// The claim rule adopts an owed entry whatever creation it records, so
	// the mark is what lets the saver take over a session restore made but
	// couldn't read the creation of.
	t.Run("an owed entry restored with no creation keeps its mark", func(t *testing.T) {
		held := owed(keeping(earlierThisBoot, 1717))
		idx := sessionindex.IndexOf([]sessionindex.Entry{held})

		if idx.Restored(held, time.Time{}, 4242) {
			t.Error("Restored = true, want false: nothing was recorded")
		}
		if got, _ := idx.Lookup("a"); !sameEntry(got, held) {
			t.Errorf("entry = %+v, want it untouched and owed, %+v", got, held)
		}
	})

	t.Run("an owed entry taken over for another creation since restore read it keeps its mark", func(t *testing.T) {
		since := owed(keeping(earlierThisBoot, 7))
		idx := sessionindex.IndexOf([]sessionindex.Entry{since})

		idx.Restored(saved, liveCreation, 4242)

		if got, _ := idx.Lookup("a"); !sameEntry(got, since) {
			t.Errorf("entry = %+v, want it untouched, %+v", got, since)
		}
	})

	t.Run("a zero creation records nothing, and says so", func(t *testing.T) {
		idx := sessionindex.IndexOf([]sessionindex.Entry{saved})

		if idx.Restored(saved, time.Time{}, 4242) {
			t.Error("Restored = true, want false: nothing was recorded")
		}
		if got, _ := idx.Lookup("a"); !sameEntry(got, saved) {
			t.Errorf("entry = %+v, want it untouched", got)
		}
	})

	t.Run("a session with no entry gets none, and says so", func(t *testing.T) {
		idx := sessionindex.IndexOf([]sessionindex.Entry{{Name: "b"}})

		if idx.Restored(saved, liveCreation, 4242) {
			t.Error("Restored = true, want false: there was no entry")
		}
		if idx.Len() != 1 {
			t.Errorf("sessions = %+v, want only b", idx.Entries())
		}
	})
}

// decision 72: a restore completes at its helper's claim, and only there.
func TestHydrated(t *testing.T) {
	t.Run("the owed entry recording the creation is owed no more", func(t *testing.T) {
		dir := t.TempDir()
		seedIndex(t, dir, owed(keeping(liveCreation, 4242)), owed(sessionindex.Entry{Name: "b", CreatedAt: liveCreation, PID: 4242}))
		sink := logtest.Install(t)

		var changed bool
		if _, err := sessionindex.Update(dir, "internal", func(idx *sessionindex.Index) error {
			changed = idx.Hydrated("a", liveCreation, 4242)
			return nil
		}); err != nil {
			t.Fatal(err)
		}

		idx, err := sessionindex.Read(dir)
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := idx.Lookup("a"); !changed || !sameEntry(got, keeping(liveCreation, 4242)) {
			t.Errorf("Hydrated = %v, entry %+v; want true and %+v", changed, got, keeping(liveCreation, 4242))
		}
		if got, _ := idx.Lookup("b"); !got.Owed {
			t.Errorf("b = %+v, want it owed still", got)
		}
		rec := sink.Records().Matching("sessions", "clear-owed").Only(t, "the repayment")
		if got := rec.AttrString(t, "detail"); got != "its helper finished its restore" {
			t.Errorf("detail = %q, want the helper's finish named", got)
		}
	})

	for name, held := range map[string]sessionindex.Entry{
		"another creation":           owed(keeping(earlierThisBoot, 4242)),
		"the same time, another pid": owed(keeping(liveCreation, 1717)),
		"an entry owed nothing":      keeping(liveCreation, 4242),
	} {
		t.Run("it changes nothing in "+name, func(t *testing.T) {
			idx := sessionindex.IndexOf([]sessionindex.Entry{held})

			if idx.Hydrated("a", liveCreation, 4242) {
				t.Error("Hydrated = true, want false")
			}
			if got, _ := idx.Lookup("a"); !sameEntry(got, held) {
				t.Errorf("entry = %+v, want it untouched, %+v", got, held)
			}
		})
	}

	t.Run("a session with no entry gets none", func(t *testing.T) {
		idx := sessionindex.IndexOf(nil)

		if idx.Hydrated("a", liveCreation, 4242) || idx.Len() != 0 {
			t.Errorf("Hydrated changed %+v, want nothing", idx.Entries())
		}
	})
}

func TestRule_Hydrating(t *testing.T) {
	if (sessionindex.Rule{}).Hydrating("a") {
		t.Error("the zero rule reads a as hydrating, want no session")
	}
	rule := ruleRestoring().ReadingHydrating(hydrating("a"))
	if !rule.Hydrating("a") || rule.Hydrating("b") {
		t.Errorf("Hydrating(a), Hydrating(b) = %v, %v; want true, false", rule.Hydrating("a"), rule.Hydrating("b"))
	}
}

func TestSameCreation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		held    sessionindex.Entry
		created time.Time
		pid     int
		want    bool
	}{
		{"the same time and pid", keeping(liveCreation, 4242), liveCreation, 4242, true},
		{"the same time in another zone", keeping(liveCreation, 4242), liveCreation.In(time.FixedZone("BST", 3600)), 4242, true},
		{"the same time with no pid given", keeping(liveCreation, 4242), liveCreation, 0, true},
		{"the same time on an entry recording no pid", keeping(liveCreation, 0), liveCreation, 4242, true},
		{"the same time with another pid", keeping(liveCreation, 1717), liveCreation, 4242, false},
		{"another time with the same pid", keeping(earlierThisBoot, 4242), liveCreation, 4242, false},
		{"an entry recording no creation", keeping(time.Time{}, 0), time.Time{}, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.held.SameCreation(tc.created, tc.pid); got != tc.want {
				t.Errorf("SameCreation = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRule_MayPredateBoot(t *testing.T) {
	for _, tc := range []struct {
		name string
		held sessionindex.Entry
		rule sessionindex.Rule
		want bool
	}{
		{"an entry recording no creation may", keeping(time.Time{}, 0), ruleRestoring(), true},
		{"an entry saved before this boot does", keeping(beforeBoot, 1717), ruleRestoring(), true},
		{"an entry saved a second before this boot does", keeping(thisBoot.Add(-time.Second), 1717), ruleRestoring(), true},
		{"under the zero rule an entry of this boot may", keeping(earlierThisBoot, 1717), sessionindex.Rule{}, true},
		{"an entry created at this boot's instant does not", keeping(thisBoot, 1717), ruleRestoring(), false},
		{"an entry of this boot does not", keeping(earlierThisBoot, 1717), ruleRestoring(), false},
		{"an entry of this boot whose session is restoring does not", keeping(earlierThisBoot, 1717), ruleRestoring("a"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.rule.MayPredateBoot(tc.held); got != tc.want {
				t.Errorf("MayPredateBoot = %v, want %v", got, tc.want)
			}
		})
	}
}
