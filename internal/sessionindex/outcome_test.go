package sessionindex_test

import (
	"errors"
	"testing"

	"github.com/leeovery/portal/internal/sessionindex"
)

func TestUpdate_Outcome(t *testing.T) {
	gone := sessionindex.Entry{Name: "gone", Label: "Gone", CreatedAt: earlierThisBoot, ScrollbackFile: "scrollback/gone-1.bin"}
	recreated := sessionindex.Entry{Name: "recreated", Label: "Old", CreatedAt: earlierThisBoot, PID: 1717, ScrollbackFile: "scrollback/recreated-1.bin"}
	same := sessionindex.Entry{Name: "same", Label: "Same", CreatedAt: liveCreation, PID: 4242}
	saved := sessionindex.Entry{Name: "saved", Label: "Saved", CreatedAt: beforeBoot, PID: 1717}
	unrecorded := sessionindex.Entry{Name: "unrecorded", Label: "Unrecorded"}
	restored := sessionindex.Entry{Name: "restored", Label: "Restored", CreatedAt: beforeBoot}
	held := []sessionindex.Entry{gone, recreated, same, saved, unrecorded, restored}

	update := func(t *testing.T, fn func(*sessionindex.Index) error, entries ...sessionindex.Entry) sessionindex.Outcome {
		t.Helper()
		dir := t.TempDir()
		seedIndex(t, dir, entries...)
		outcome, err := sessionindex.Update(dir, "cli", fn)
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		return outcome
	}

	t.Run("it reports each entry removed and each replaced afresh, as read, and none it kept", func(t *testing.T) {
		outcome := update(t, func(idx *sessionindex.Index) error {
			idx.Remove("gone")
			for _, name := range []string{"recreated", "same", "saved", "unrecorded", "fresh"} {
				idx.Claim(name, "/"+name, liveCreation, 4242, ruleRestoring())
			}
			idx.Restored("restored", liveCreation, 4242)
			_ = idx.SetLabel("saved", "Relabelled")
			return idx.SetLabel("bare", "Bare")
		}, held...)

		if !sameEntries(outcome.Removed, []sessionindex.Entry{gone}) {
			t.Errorf("removed = %+v, want gone as read", outcome.Removed)
		}
		if !sameEntries(outcome.Replaced, []sessionindex.Entry{recreated}) {
			t.Errorf("replaced = %+v, want recreated as read", outcome.Replaced)
		}
	})

	t.Run("a name removed and claimed again reports the entry it read", func(t *testing.T) {
		outcome := update(t, func(idx *sessionindex.Index) error {
			idx.Remove("saved")
			idx.Claim("saved", "/new", liveCreation, 4242, ruleRestoring())
			return nil
		}, held...)

		if got := outcome.Dropped(); !sameEntries(got, []sessionindex.Entry{saved}) {
			t.Errorf("dropped = %+v, want saved as read", got)
		}
	})

	t.Run("a name replaced and then removed reports the entry it read", func(t *testing.T) {
		outcome := update(t, func(idx *sessionindex.Index) error {
			idx.Claim("recreated", "/new", liveCreation, 4242, ruleRestoring())
			idx.Remove("recreated")
			return nil
		}, held...)

		if !sameEntries(outcome.Removed, []sessionindex.Entry{recreated}) || len(outcome.Replaced) != 0 {
			t.Errorf("outcome = %+v, want recreated as read, removed", outcome)
		}
	})

	t.Run("a replaced entry is reported with the scrollback it was read with, not the replacement's", func(t *testing.T) {
		outcome := update(t, func(idx *sessionindex.Index) error {
			idx.Claim("recreated", "/new", liveCreation, 4242, ruleRestoring())
			idx.SetScrollback("recreated", "scrollback/recreated-2.bin")
			return nil
		}, held...)

		if len(outcome.Replaced) != 1 || outcome.Replaced[0].ScrollbackFile != recreated.ScrollbackFile {
			t.Errorf("replaced = %+v, want the entry read, naming %s", outcome.Replaced, recreated.ScrollbackFile)
		}
	})

	t.Run("a name fn adds and removes again reports nothing, as none was read", func(t *testing.T) {
		outcome := update(t, func(idx *sessionindex.Index) error {
			idx.Claim("fresh", "/fresh", liveCreation, 4242, ruleRestoring())
			idx.Remove("fresh")
			return nil
		}, held...)

		if len(outcome.Dropped()) != 0 {
			t.Errorf("outcome = %+v, want empty", outcome)
		}
	})

	for name, fn := range map[string]func(*sessionindex.Index) error{
		"fn errors":          func(idx *sessionindex.Index) error { idx.Remove("gone"); return errors.New("boom") },
		"fn changes nothing": func(idx *sessionindex.Index) error { idx.Remove("absent"); return nil },
		"fn claims an entry of its own creation": func(idx *sessionindex.Index) error {
			idx.Claim("same", "", liveCreation, 4242, ruleRestoring())
			return nil
		},
	} {
		t.Run("it reports nothing when nothing was written: "+name, func(t *testing.T) {
			dir := t.TempDir()
			seedIndex(t, dir, held...)

			outcome, _ := sessionindex.Update(dir, "cli", fn)

			if len(outcome.Dropped()) != 0 {
				t.Errorf("outcome = %+v, want empty", outcome)
			}
		})
	}

	t.Run("it reports nothing when the write fails", func(t *testing.T) {
		dir := unwritableStateDir(t)

		outcome, err := sessionindex.Update(dir, "cli", func(idx *sessionindex.Index) error {
			idx.Claim("a", "/a", liveCreation, 0, ruleRestoring())
			return nil
		})

		if err == nil {
			t.Fatal("Update = nil, want the write failure")
		}
		if len(outcome.Dropped()) != 0 {
			t.Errorf("outcome = %+v, want empty", outcome)
		}
	})
}

func TestOutcome_Dropped(t *testing.T) {
	a, b, c := sessionindex.Entry{Name: "a"}, sessionindex.Entry{Name: "b"}, sessionindex.Entry{Name: "c", CreatedAt: liveCreation}
	outcome := sessionindex.Outcome{Removed: []sessionindex.Entry{a, b}, Replaced: []sessionindex.Entry{c}}

	if got, want := outcome.Dropped(), []sessionindex.Entry{a, b, c}; !sameEntries(got, want) {
		t.Errorf("Dropped = %+v, want %+v", got, want)
	}
	if got := (sessionindex.Outcome{}).Dropped(); len(got) != 0 {
		t.Errorf("Dropped of an empty outcome = %+v, want none", got)
	}
}
