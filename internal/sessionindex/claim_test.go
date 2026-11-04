package sessionindex_test

import (
	"slices"
	"testing"
	"time"

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

// keeping is a's entry holding everything a claim must not lose, created at
// at, running pid.
func keeping(at time.Time, pid int) sessionindex.Entry {
	return sessionindex.Entry{
		Name: "a", Label: "Old", Dir: "/kept", Cwd: "/was/here",
		CreatedAt: at, PID: pid, ScrollbackFile: "scrollback/a-1793000000-1717.bin",
	}
}

func withDir(e sessionindex.Entry, dir string) sessionindex.Entry {
	e.Dir = dir
	return e
}

func seedIndex(t *testing.T, dir string, entries ...sessionindex.Entry) {
	t.Helper()
	data, err := sessionindex.Encode(sessionindex.Index{Sessions: entries})
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
			name: "an entry of this boot claimed knowing no creation is kept untouched",
			held: keeping(earlierThisBoot, 1717), rule: judged,
			want: keeping(earlierThisBoot, 1717),
		},
		{
			name: "an entry saved before this boot claimed knowing no creation is kept untouched",
			held: keeping(beforeBoot, 1717), rule: judged,
			want: keeping(beforeBoot, 1717),
		},
		{
			name: "an entry recording no creation or dir claimed knowing no creation is kept untouched",
			held: withDir(keeping(time.Time{}, 0), ""), rule: judged,
			want: withDir(keeping(time.Time{}, 0), ""),
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

			outcome, err := sessionindex.Update(dir, "cli", func(idx *sessionindex.Index) error {
				idx.Claim("a", "/a", tc.created, tc.pid, tc.rule)
				return nil
			})
			if err != nil {
				t.Fatalf("Update: %v", err)
			}

			idx, err := sessionindex.Read(dir)
			if err != nil {
				t.Fatalf("Read: %v", err)
			}
			if !sameEntries(idx.Sessions, []sessionindex.Entry{tc.want, neighbour}) {
				t.Errorf("sessions = %+v, want a as %+v beside b untouched", idx.Sessions, tc.want)
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

	for _, tc := range []struct {
		name    string
		created time.Time
		want    sessionindex.Entry
	}{
		{"a session with no entry gets one, recording its creation", liveCreation, fresh},
		{"a session with no entry claimed knowing no creation gets one with its dir", time.Time{}, sessionindex.Entry{Name: "a", Dir: "/a"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			idx := sessionindex.Index{Sessions: []sessionindex.Entry{{Name: "b"}}}
			pid := 0
			if !tc.created.IsZero() {
				pid = 4242
			}

			idx.Claim("a", "/a", tc.created, pid, judged)

			if got, ok := idx.Lookup("a"); !ok || !sameEntry(got, tc.want) || len(idx.Sessions) != 2 {
				t.Errorf("sessions = %+v, want b and a as %+v", idx.Sessions, tc.want)
			}
		})
	}
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
			idx := sessionindex.Index{Sessions: []sessionindex.Entry{tc.held}}

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
		idx := sessionindex.Index{Sessions: []sessionindex.Entry{{Name: "b", CreatedAt: liveCreation}}}
		if got, ok := idx.LookupLive("a", liveCreation, 0, judged); ok {
			t.Errorf("LookupLive = %+v, true; want none", got)
		}
	})

	for name, label := range map[string]string{"an unprintable label": "api\nwork", "a label of only whitespace": "   "} {
		t.Run(name+" is dropped, the rest kept, and left in the index", func(t *testing.T) {
			held := keeping(liveCreation, 4242)
			held.Label = label
			idx := sessionindex.Index{Sessions: []sessionindex.Entry{held}}

			got, ok := idx.LookupLive("a", liveCreation, 4242, judged)

			want := held
			want.Label = ""
			if !ok || !sameEntry(got, want) {
				t.Errorf("LookupLive = %+v, %v; want %+v", got, ok, want)
			}
			if idx.Sessions[0].Label != label {
				t.Errorf("Label = %q, want the file's label untouched", idx.Sessions[0].Label)
			}
		})
	}
}

func TestRestored(t *testing.T) {
	saved := keeping(beforeBoot, 1717)

	t.Run("it records the new creation and keeps everything else", func(t *testing.T) {
		idx := sessionindex.Index{Sessions: []sessionindex.Entry{saved, {Name: "b", Label: "Beta"}}}

		if !idx.Restored("a", liveCreation, 4242) {
			t.Fatal("Restored = false, want the creation recorded")
		}

		if got, _ := idx.Lookup("a"); !sameEntry(got, keeping(liveCreation, 4242)) {
			t.Errorf("entry = %+v, want %+v", got, keeping(liveCreation, 4242))
		}
		if got, _ := idx.Lookup("b"); got != (sessionindex.Entry{Name: "b", Label: "Beta"}) {
			t.Errorf("b = %+v, want it untouched", got)
		}
	})

	t.Run("it records the creation whatever the entry's, with no rule to consult", func(t *testing.T) {
		idx := sessionindex.Index{Sessions: []sessionindex.Entry{keeping(earlierThisBoot, 1717)}}

		if !idx.Restored("a", liveCreation, 4242) {
			t.Fatal("Restored = false, want the creation recorded")
		}
		if got, _ := idx.Lookup("a"); !sameEntry(got, keeping(liveCreation, 4242)) {
			t.Errorf("entry = %+v, want %+v", got, keeping(liveCreation, 4242))
		}
	})

	t.Run("an entry already recording the creation reports it recorded", func(t *testing.T) {
		idx := sessionindex.Index{Sessions: []sessionindex.Entry{keeping(liveCreation, 4242)}}

		if !idx.Restored("a", liveCreation, 4242) {
			t.Error("Restored = false, want the creation reported recorded")
		}
	})

	t.Run("a zero creation records nothing, and says so", func(t *testing.T) {
		idx := sessionindex.Index{Sessions: []sessionindex.Entry{saved}}

		if idx.Restored("a", time.Time{}, 4242) {
			t.Error("Restored = true, want false: nothing was recorded")
		}
		if got, _ := idx.Lookup("a"); !sameEntry(got, saved) {
			t.Errorf("entry = %+v, want it untouched", got)
		}
	})

	t.Run("a session with no entry gets none, and says so", func(t *testing.T) {
		idx := sessionindex.Index{Sessions: []sessionindex.Entry{{Name: "b"}}}

		if idx.Restored("a", liveCreation, 4242) {
			t.Error("Restored = true, want false: there was no entry")
		}
		if len(idx.Sessions) != 1 {
			t.Errorf("sessions = %+v, want only b", idx.Sessions)
		}
	})
}
