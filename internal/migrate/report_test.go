package migrate_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/migrate"
)

// The paths the reports name, and how they are shown: below home, from ~.
const (
	home         = "/home/user"
	reportState  = home + "/.config/portal/state"
	reportTmux   = home + "/.config/portal/state-tmux"
	reportBackup = reportTmux + "/hooks.json.tmux"
)

func shown(path string) string {
	if rest, ok := strings.CutPrefix(path, home); ok {
		return "~" + rest
	}
	return path
}

// reportPlan is a plan with something in each of the report's blocks:
// lantern's pane's hook moved; tidepool's two panes, the second's scrollback
// gone; quarry's name taken by a live zmx session; bell's label holding a
// control character; a session with no windows; and hooks keyed by a pane
// address, a tmux pane id, a token of no saved pane and a key of no shape
// Portal wrote.
func reportPlan(t *testing.T) migrate.Plan {
	t.Helper()
	dir := legacyDir(t, []v1Session{
		session("lantern", pane(0, codeDir+"lantern").token("tokLan").scrollback("scrollback/pane-tokLan.bin")),
		session("tidepool · flaky test", pane(0, codeDir+"tidepool").scrollback("scrollback/tidepool__0.0.bin"), pane(1, codeDir+"tidepool/web").scrollback("scrollback/tidepool__0.1.bin")),
		session("quarry", pane(0, codeDir+"quarry")),
		session("bell · ding\a", pane(0, codeDir+"bell")),
		sessionOf("empty"),
	}, map[string]string{
		"scrollback/pane-tokLan.bin":   "lantern\n",
		"scrollback/tidepool__0.0.bin": "tidepool\n",
	})
	return planOf(t, dir, []string{"tokLan", "tidepool · flaky test:0.1", "%3", "tokOld", "notes"}, naming(t, "quarry"))
}

func migratedOutcome(plan migrate.Plan) migrate.Outcome {
	return migrate.Outcome{Course: migrate.Migrated, StateDir: reportState, TmuxDir: reportTmux, Backup: reportBackup, Plan: plan}
}

const wantMigratedReport = `Migrated 4 tmux sessions to 5 sessions. Skipped 1 tmux session.
Moved 1 resume hook. Removed 4, kept in the backup.
Restored 5 sessions. Every saved session is restored.
Each migrated session with a resume hook waits on its resume panel the first time you open it.

Sessions (name ← tmux name):
  lantern ← lantern
  tidepool ← tidepool · flaky test (window 0, pane 0)
  tidepool-2 ← tidepool · flaky test (window 0, pane 1)
  quarry-Gen001 ← quarry
  bell ← bell · ding\a

Generated names:
  quarry-Gen001: the name is taken by "quarry"

Labels dropped:
  bell: label refused: it holds a control or formatting character, such as a newline, tab or escape

Skipped:
  empty: version 1 saved no windows for it

Restored without scrollback:
  tidepool-2: its scrollback file, scrollback/tidepool__0.1.bin, is gone from the tmux version's state

Resume hooks moved:
  tokLan → lantern

Resume hooks removed (kept in the backup):
  %3: a tmux pane id, which hasn't fired since pane tokens
  notes: not a key Portal wrote
  tidepool · flaky test:0.1: a pane address, which hasn't fired since pane tokens
  tokOld: the token of a pane the tmux version didn't save

The tmux version's state is kept at ~/.config/portal/state-tmux.
hooks.json's backup is at ~/.config/portal/state-tmux/hooks.json.tmux.
`

func TestReport(t *testing.T) {
	t.Run("it reports a migration: a summary, the restore, each session and what changed, and where the old state is", func(t *testing.T) {
		got := migrate.Report(migratedOutcome(reportPlan(t)), migrate.Restore{End: migrate.RestoreRan, Restored: 5, Waits: true}, shown)

		if got != wantMigratedReport {
			t.Errorf("report:\n%s\nwant:\n%s", got, wantMigratedReport)
		}
	})

	landed := migrate.Outcome{Course: migrate.AlreadyMigrated, StateDir: reportState, TmuxDir: reportTmux}
	const kept = "\nThe tmux version's state is kept at ~/.config/portal/state-tmux.\n"
	for name, tc := range map[string]struct {
		outcome migrate.Outcome
		restore migrate.Restore
		want    string
	}{
		"nothing to migrate": {
			outcome: migrate.Outcome{Course: migrate.NoState, StateDir: reportState},
			want:    "Nothing to migrate: there's no saved state at ~/.config/portal/state.\n",
		},
		"a state dir this version's already": {
			outcome: migrate.Outcome{Course: migrate.NotTmux, StateDir: reportState},
			want:    "Nothing to migrate: the saved state at ~/.config/portal/state is this version's already.\n",
		},
		"a run cut short, finished": {
			outcome: migrate.Outcome{Course: migrate.Finished, StateDir: reportState, TmuxDir: reportTmux, Backup: reportBackup},
			restore: migrate.Restore{End: migrate.RestoreRan, Restored: 3, Waits: true},
			want: "Finished a migration cut short: the migrated state is in place at ~/.config/portal/state.\n" +
				"Restored 3 sessions. Every saved session is restored.\n" +
				"Each migrated session with a resume hook waits on its resume panel the first time you open it.\n" +
				kept +
				"hooks.json's backup is at ~/.config/portal/state-tmux/hooks.json.tmux.\n",
		},
		"a migration landed, owed no restore, every session restored": {
			outcome: landed,
			want:    "Already migrated.\nEvery saved session is restored.\n" + kept,
		},
		"a migration landed, owed no restore, sessions left that wait on their panel": {
			outcome: landed,
			restore: migrate.Restore{Left: []string{"lantern", "quarry-Gen001"}, Waits: true},
			want: "Already migrated.\n" +
				"Each migrated session with a resume hook waits on its resume panel the first time you open it.\n" +
				"\n" +
				"2 sessions left to restore. x <name> brings one back now; otherwise they come back at the next restore after a reboot:\n" +
				"  lantern\n" +
				"  quarry-Gen001\n" +
				kept,
		},
		"a restore with a session it couldn't make": {
			outcome: landed,
			restore: migrate.Restore{End: migrate.RestoreRan, Restored: 2, Left: []string{"bell"}},
			want: "Already migrated.\n" +
				"Restored 2 sessions.\n" +
				"\n" +
				"1 session left to restore. x <name> brings one back now; otherwise they come back at the next restore after a reboot:\n" +
				"  bell\n" +
				kept,
		},
		"a restore the bootstrap's fatal stopped part way": {
			outcome: landed,
			restore: migrate.Restore{End: migrate.RestoreStopped, Restored: 1, Left: []string{"bell", "tidepool"}},
			want: "Already migrated.\n" +
				"Restored 1 session, then stopped on the error below.\n" +
				"\n" +
				"2 sessions left to restore. Once the error below is fixed, x <name> brings one back; otherwise they come back at the next restore after a reboot:\n" +
				"  bell\n" +
				"  tidepool\n" +
				kept,
		},
		"a bootstrap's fatal before its restore": {
			outcome: landed,
			restore: migrate.Restore{End: migrate.RestoreStopped, Left: []string{"bell"}},
			want: "Already migrated.\n" +
				"Couldn't restore the sessions: the error below says why.\n" +
				"\n" +
				"1 session left to restore. Once the error below is fixed, x <name> brings one back; otherwise they come back at the next restore after a reboot:\n" +
				"  bell\n" +
				kept,
		},
		"a restore that couldn't read the index, nor the read after it": {
			outcome: landed,
			restore: migrate.Restore{End: migrate.RestoreUnread, LeftUnread: errors.New("decode sessions.json: unexpected EOF")},
			want: "Already migrated.\n" +
				"Couldn't restore the sessions: the warning above says why. Portal can't tell which sessions are left to restore: decode sessions.json: unexpected EOF\n" +
				kept,
		},
	} {
		t.Run("it reports "+name, func(t *testing.T) {
			if got := migrate.Report(tc.outcome, tc.restore, shown); got != tc.want {
				t.Errorf("report:\n%s\nwant:\n%s", got, tc.want)
			}
		})
	}

	t.Run("it reports a migration of no session, and no hook, in its summary alone", func(t *testing.T) {
		plan := planOf(t, legacyDir(t, []v1Session{sessionOf("empty")}, nil), nil, naming(t))

		got := migrate.Report(migratedOutcome(plan), migrate.Restore{End: migrate.RestoreRan}, shown)

		want := "The tmux version saved no sessions to migrate. Skipped 1 tmux session.\n" +
			"Restored no sessions. Every saved session is restored.\n" +
			"\n" +
			"Skipped:\n" +
			"  empty: version 1 saved no windows for it\n" +
			"\n" +
			"The tmux version's state is kept at ~/.config/portal/state-tmux.\n" +
			"hooks.json's backup is at ~/.config/portal/state-tmux/hooks.json.tmux.\n"
		if got != want {
			t.Errorf("report:\n%s\nwant:\n%s", got, want)
		}
	})

	t.Run("it tells of no resume panel when no session restored or left waits on one", func(t *testing.T) {
		plan := planSessions(t, session("lantern", pane(0, codeDir+"lantern")))

		got := migrate.Report(migratedOutcome(plan), migrate.Restore{End: migrate.RestoreRan, Restored: 1}, shown)

		if strings.Contains(got, "resume panel") {
			t.Errorf("report tells of the resume panel with no session waiting on one:\n%s", got)
		}
	})

	t.Run("it reads at thirty sessions: one line each, hooks moved in their sessions' order", func(t *testing.T) {
		dir, body := realisticFixture(t)
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: body, SidecarAbsent: true})
		plan := planOf(t, dir, hookKeys(t, store), naming(t, realisticTaken...))

		blocks := strings.Split(migrate.Report(migratedOutcome(plan), migrate.Restore{End: migrate.RestoreRan, Restored: len(plan.Sessions)}, shown), "\n\n")

		sessions := blockOf(t, blocks, "Sessions (name ← tmux name):")
		if len(sessions) != len(plan.Sessions) {
			t.Errorf("%d session lines, want one for each of %d sessions", len(sessions), len(plan.Sessions))
		}
		var order []string
		for _, line := range sessions {
			order = append(order, strings.Fields(line)[0])
		}
		var movedTo []string
		for _, line := range blockOf(t, blocks, "Resume hooks moved:") {
			movedTo = append(movedTo, strings.Fields(line)[2])
		}
		if !slices.IsSortedFunc(movedTo, func(a, b string) int { return slices.Index(order, a) - slices.Index(order, b) }) {
			t.Errorf("hooks moved to %v, want them in the sessions' order %v", movedTo, order)
		}
	})
}

// blockOf is the lines below heading of the block blocks holds under it.
func blockOf(t *testing.T, blocks []string, heading string) []string {
	t.Helper()
	for _, block := range blocks {
		if rest, ok := strings.CutPrefix(block, heading+"\n"); ok {
			return strings.Split(strings.TrimRight(rest, "\n"), "\n")
		}
	}
	t.Fatalf("no block under %q", heading)
	return nil
}
