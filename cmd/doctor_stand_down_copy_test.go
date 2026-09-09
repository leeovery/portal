// The one home for every reason the hook prune stands down under. A reason's
// whole coverage lives in its row here: the level and attrs its log line
// carries, the repair line `portal doctor --fix` prints for a user who asked
// for a repair, the not-evaluable detail the read-only diagnosis reports for
// the same window, and the hooks.json a stand-down must leave exactly as it
// found it. They are pinned together because the two surfaces must name the
// same condition in their own register, because a branch that silently borrows
// another's words is exactly the drift this suite exists to catch, and because
// a wording change must be one edit rather than a hunt through the corpus.
package cmd

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/hooksweep"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/project"
)

// standDownCopyCase is one stand-down reason and everything the two surfaces
// and the log line say about it. skippedLine and notEvaluableLine are both
// whole rendered lines, the repair's prefix and the check's glyph included.
type standDownCopyCase struct {
	name   string
	reason hooksweep.Reason
	// fixture stages an install whose only anomaly is this stand-down and
	// returns the hooks.json path alongside it, so both arms can pin that the
	// stand-down wrote nothing.
	fixture func(t *testing.T) (*DoctorDeps, string)
	// level is where the sweep reports this reason, and attrs is what its line
	// carries beyond the reason itself — every case states one, naming the
	// absence of an error attr where there is none.
	level slog.Level
	attrs func(t *testing.T, rec logtest.Record)
	// sharedPhrase is the const both vocabularies compose this reason's words
	// from — empty for a reason production names no such const for, because its
	// surfaces say different things (the empty pane list).
	sharedPhrase     string
	skippedLine      string
	notEvaluableLine string
	// postRepairNotEvaluable is whether the diagnosis that follows the repair
	// still stands down: the lock is the one reason a repair declines under
	// that a read degrades past, so its post-repair check is evaluable again.
	postRepairNotEvaluable bool
}

func standDownCopyCases() []standDownCopyCase {
	return []standDownCopyCase{
		{
			name:                   "restore window",
			reason:                 hooksweep.ReasonRestoring,
			sharedPhrase:           restoreStandDownPhrase,
			fixture:                restoringStandDownDeps,
			level:                  slog.LevelDebug,
			attrs:                  noStandDownErrorAttr,
			skippedLine:            "Skipped stale hook prune: restore in progress",
			notEvaluableLine:       "  · stale hooks: restore in progress (not evaluable)",
			postRepairNotEvaluable: true,
		},
		{
			name:                   "restore marker unreadable",
			reason:                 hooksweep.ReasonMarkerReadFailed,
			sharedPhrase:           markerReadStandDownPhrase,
			fixture:                markerReadFailedStandDownDeps,
			level:                  slog.LevelDebug,
			attrs:                  standDownErrorAttrExactly("no server running"),
			skippedLine:            "Skipped stale hook prune: could not read the restore marker",
			notEvaluableLine:       "  · stale hooks: could not read the restore marker",
			postRepairNotEvaluable: true,
		},
		{
			name:                   "hooks.json unreadable",
			reason:                 hooksweep.ReasonStoreReadFailed,
			sharedPhrase:           storeReadStandDownPhrase,
			fixture:                storeReadFailedStandDownDeps,
			level:                  slog.LevelWarn,
			attrs:                  standDownErrorAttrCarrying(hooks.ErrStoreRead.Error()),
			skippedLine:            "Skipped stale hook prune: could not read hooks.json",
			notEvaluableLine:       "  · stale hooks: could not read hooks.json",
			postRepairNotEvaluable: true,
		},
		{
			name:                   "pane enumeration failed",
			reason:                 hooksweep.ReasonPaneReadFailed,
			sharedPhrase:           paneReadStandDownPhrase,
			fixture:                paneReadFailedStandDownDeps,
			level:                  slog.LevelWarn,
			attrs:                  standDownErrorAttrExactly("tmux transient"),
			skippedLine:            "Skipped stale hook prune: could not enumerate live panes",
			notEvaluableLine:       "  · stale hooks: could not enumerate live panes",
			postRepairNotEvaluable: true,
		},
		{
			name:                   "empty pane list",
			reason:                 hooksweep.ReasonEmptyPaneRead,
			fixture:                emptyPaneReadStandDownDeps,
			level:                  slog.LevelWarn,
			attrs:                  emptyPaneReadStandDownAttrs,
			skippedLine:            "Skipped stale hook prune: live pane list came back empty",
			notEvaluableLine:       "  · stale hooks: zero live panes with hooks present (not evaluable)",
			postRepairNotEvaluable: true,
		},
		{
			name:             "hooks.json locked",
			reason:           hooksweep.ReasonLockTimeout,
			sharedPhrase:     lockStandDownPhrase,
			fixture:          lockTimeoutStandDownDeps,
			level:            slog.LevelWarn,
			attrs:            standDownErrorAttrCarrying(hooks.ErrLockHeld.Error()),
			skippedLine:      "Skipped stale hook prune: hooks.json is locked",
			notEvaluableLine: "  · stale hooks: hooks.json is locked (not evaluable)",
		},
	}
}

// noStandDownErrorAttr is the attr assertion for a reason no read failure
// produced: the line names the condition and nothing else.
func noStandDownErrorAttr(t *testing.T, rec logtest.Record) {
	t.Helper()
	if rec.HasAttr("error") {
		t.Errorf("stand-down record carries an error attr with no read failure: %+v", rec.Attrs)
	}
}

// standDownErrorAttrExactly and standDownErrorAttrCarrying are the two ways a
// row pins that its line carries the failure that produced it, so the reason is
// diagnosable from the log alone. Which one a row takes is decided by its attr,
// not by taste: a reason whose error text is fixed pins the whole attr, so text
// growing around it is caught; a reason whose error embeds something the
// fixture chose — a temp path — can only be pinned by containment.
func standDownErrorAttrExactly(want string) func(*testing.T, logtest.Record) {
	return func(t *testing.T, rec logtest.Record) {
		t.Helper()
		if got := rec.AttrString(t, "error"); got != want {
			t.Errorf("error attr = %q, want %q", got, want)
		}
	}
}

func standDownErrorAttrCarrying(want string) func(*testing.T, logtest.Record) {
	return func(t *testing.T, rec logtest.Record) {
		t.Helper()
		if got := rec.AttrString(t, "error"); !strings.Contains(got, want) {
			t.Errorf("error attr = %q, want it to carry %q", got, want)
		}
	}
}

// The empty-pane-read line reports how many entries the read left unjudged —
// the whole reason the guard fired — and no error, because the read succeeded.
func emptyPaneReadStandDownAttrs(t *testing.T, rec logtest.Record) {
	t.Helper()
	noStandDownErrorAttr(t, rec)
	if got := rec.IntAttr(t, "entries"); got != 2 {
		t.Errorf("entries = %d, want the two entries the fixture seeds", got)
	}
}

// healthyStandDownDeps stages an install whose only anomaly is the stand-down
// itself — a live runtime, a fresh state dir and a live project record — so an
// exit code that moves can only have moved because of the stand-down.
func healthyStandDownDeps(t *testing.T, lister *stubStaleSweepReader, store *hooks.Store) *DoctorDeps {
	t.Helper()

	dir := t.TempDir()
	seedHealthyStateDir(t, dir)
	projectStore, _ := seedProjectsJSON(t, t.TempDir())
	return staleDeps(dir, lister, store, projectStore)
}

func restoringStandDownDeps(t *testing.T) (*DoctorDeps, string) {
	t.Helper()
	store, path := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(hookstest.LiveSeedA)})
	return healthyStandDownDeps(t, restoringHookLister(), store), path
}

// The server is down, so the marker read fails outright — the state a user
// reaches for doctor in after a reboot.
func markerReadFailedStandDownDeps(t *testing.T) (*DoctorDeps, string) {
	t.Helper()
	store, path := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(hookstest.LiveSeedA)})
	lister := staleHookLister()
	lister.restoringErr = errors.New("no server running")
	return healthyStandDownDeps(t, lister, store), path
}

func storeReadFailedStandDownDeps(t *testing.T) (*DoctorDeps, string) {
	t.Helper()
	store, path := hookstest.StageStore(t, hookstest.Staging{Unreadable: true})
	return healthyStandDownDeps(t, staleHookLister(), store), path
}

func paneReadFailedStandDownDeps(t *testing.T) (*DoctorDeps, string) {
	t.Helper()
	store, path := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(hookstest.LiveSeedA)})
	return healthyStandDownDeps(t, &stubStaleSweepReader{err: errors.New("tmux transient")}, store), path
}

// The guard fires on a completed read of no panes with entries to protect, so
// the fixture seeds entries alongside the empty enumeration — two of them, so
// the count the line reports is not the same number as anything else on it.
func emptyPaneReadStandDownDeps(t *testing.T) (*DoctorDeps, string) {
	t.Helper()
	store, path := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(hookstest.LiveSeedA, hookstest.LiveSeedB)})
	return healthyStandDownDeps(t, &stubStaleSweepReader{rows: tokenRows()}, store), path
}

// The seeded entry is live, so the post-repair diagnosis reads the file (a read
// degrades to unlocked) and finds nothing stale: the lock stands down the
// prune and nothing else.
func lockTimeoutStandDownDeps(t *testing.T) (*DoctorDeps, string) {
	t.Helper()
	hooks.SetLockTimeoutForTest(t, lockBound)
	store, path := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(hookstest.LiveSeedA)})
	hookstest.HoldHooksSidecar(t, path)
	return healthyStandDownDeps(t, &stubStaleSweepReader{rows: tokenRows(hookstest.LiveSeedA)}, store), path
}

// renderStaleHooksLine renders the not-evaluable check line exactly as the
// report does, so the assertion reads the user's line rather than a map value.
func renderStaleHooksLine(reason hooksweep.Reason) string {
	buf := new(bytes.Buffer)
	renderDoctorReport(buf, []checkResult{{
		name:   "stale hooks",
		status: checkNotEvaluable,
		detail: phraseFor(notEvaluableDetails, reason),
	}}, nil)
	for line := range strings.SplitSeq(buf.String(), "\n") {
		if strings.Contains(line, "stale hooks:") {
			return line
		}
	}
	return ""
}

// renderSkippedPruneLine renders the repair line for a reason through the
// vocabulary, so a suite whose subject is not the copy names the line it
// expects without pinning a second copy of the words.
func renderSkippedPruneLine(reason hooksweep.Reason) string {
	return "Skipped stale hook prune: " + phraseFor(skippedPrunePhrases, reason)
}

// hooksPathState renders whatever stands at the hooks.json path — its bytes
// where it is a file, its members where a fixture staged a directory the store
// cannot read — so every reason's untouched check reads the same.
func hooksPathState(t *testing.T, path string) string {
	t.Helper()

	info, err := os.Lstat(path)
	switch {
	case os.IsNotExist(err):
		return "absent"
	case err != nil:
		t.Fatalf("stat %s: %v", path, err)
	}
	if !info.IsDir() {
		return "file:" + string(readFileBytes(t, path))
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatalf("read dir %s: %v", path, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return "dir:" + strings.Join(names, ",")
}

func assertHooksPathUnchanged(t *testing.T, path, before, context string) {
	t.Helper()
	if after := hooksPathState(t, path); after != before {
		t.Errorf("hooks.json %s\nbefore: %s\nafter:  %s", context, before, after)
	}
}

// assertStandDownSweep drives the cycle itself: what it reported to its caller,
// the line it logged, and the file it must have left alone.
func assertStandDownSweep(t *testing.T, tc standDownCopyCase) {
	t.Helper()

	deps, hooksPath := tc.fixture(t)
	before := hooksPathState(t, hooksPath)
	sink := logtest.Install(t)

	outcome, err := hooksweep.Run(deps.HookLister, deps.HookStore)
	if err != nil {
		t.Fatalf("hooksweep.Run: %v", err)
	}
	if outcome.DeclineReason != tc.reason {
		t.Fatalf("DeclineReason = %q, want %q", outcome.DeclineReason, tc.reason)
	}
	if len(outcome.Removed) != 0 {
		t.Errorf("Removed = %v, want none on a declined cycle", outcome.Removed)
	}

	assertHooksPathUnchanged(t, hooksPath, before, "written on a stand-down")
	tc.attrs(t, assertStandDown(t, sink, tc.level, tc.reason))
}

// assertStandDownRepair drives `portal doctor --fix`: the line the user reads,
// the diagnosis that follows it, and the file the repair must not have touched.
func assertStandDownRepair(t *testing.T, tc standDownCopyCase) {
	t.Helper()

	deps, hooksPath := tc.fixture(t)
	before := hooksPathState(t, hooksPath)

	outBuf, _, err := runDoctorWith(t, deps, "--fix")
	if err != nil {
		t.Errorf("doctor --fix err = %v; want nil over a healthy post-repair diagnosis\n%s", err, outBuf.String())
	}
	out := outBuf.String()

	assertSkippedPruneLine(t, out, tc.skippedLine)
	assertHooksPathUnchanged(t, hooksPath, before, "written by the repair on a stand-down")

	// The repair and the diagnosis must tell one story: a prune that stood
	// down cannot be followed by a count of what it deliberately did not judge.
	if tc.postRepairNotEvaluable && !strings.Contains(out, tc.notEvaluableLine) {
		t.Errorf("post-repair report missing %q:\n%s", tc.notEvaluableLine, out)
	}
	if strings.Contains(out, "stale hook entr") {
		t.Errorf("post-repair diagnosis counted what the prune stood down on:\n%s", out)
	}
}

func TestStandDownCopy(t *testing.T) {
	// Every reason the type holds is a reason a cycle declined under, so the
	// table covers the set whole: no member is excluded from it, and a new one
	// arrives here or fails.
	t.Run("it enumerates every stand-down reason with no subtraction list", func(t *testing.T) {
		covered := map[hooksweep.Reason]string{}
		for _, tc := range standDownCopyCases() {
			if prior, ok := covered[tc.reason]; ok {
				t.Errorf("case %q reuses the reason %q already covered by %q; want one row per decline path", tc.name, tc.reason, prior)
			}
			covered[tc.reason] = tc.name
			if !slices.Contains(hooksweep.Reasons, tc.reason) {
				t.Errorf("case %q covers %q, which is no declared reason", tc.name, tc.reason)
			}
		}
		for _, reason := range hooksweep.Reasons {
			if _, ok := covered[reason]; !ok {
				t.Errorf("declared reason %q has no copy case; every reason a cycle declines under is covered here", reason)
			}
		}
	})

	// The uniqueness checks read the *rendered* phrases, not the table's
	// expectations: two reasons that agree on their words are the drift worth
	// catching, and comparing the expectations would only catch an author who
	// copied a row.
	t.Run("it renders a distinct phrase for each of the six reasons on both surfaces", func(t *testing.T) {
		skipped := map[string]hooksweep.Reason{}
		notEvaluable := map[string]hooksweep.Reason{}
		for _, reason := range hooksweep.Reasons {
			renderedSkipped := renderSkippedPruneLine(reason)
			if prior, ok := skipped[renderedSkipped]; ok {
				t.Errorf("reason %q borrows the skipped-prune line already used by %q: %q", reason, prior, renderedSkipped)
			}
			renderedDetail := phraseFor(notEvaluableDetails, reason)
			if prior, ok := notEvaluable[renderedDetail]; ok {
				t.Errorf("reason %q borrows the not-evaluable detail already used by %q: %q", reason, prior, renderedDetail)
			}
			skipped[renderedSkipped] = reason
			notEvaluable[renderedDetail] = reason
		}
	})

	t.Run("it names a phrase for every stand-down reason on both surfaces", func(t *testing.T) {
		for _, tc := range standDownCopyCases() {
			t.Run(tc.name, func(t *testing.T) {
				assertStandDownSweep(t, tc)
				assertStandDownRepair(t, tc)

				if got := renderStaleHooksLine(tc.reason); got != tc.notEvaluableLine {
					t.Errorf("not-evaluable line = %q, want %q", got, tc.notEvaluableLine)
				}
			})
		}
	})

	// A phrase two surfaces share is written once and composed into each, so a
	// re-wording moves both. This reads the rendered entries and pins that the
	// two vocabularies agree on those shared words.
	t.Run("it composes a shared phrase from the const both surfaces name", func(t *testing.T) {
		for _, tc := range standDownCopyCases() {
			if tc.sharedPhrase == "" {
				continue
			}
			if got := phraseFor(skippedPrunePhrases, tc.reason); got != tc.sharedPhrase {
				t.Errorf("skippedPrunePhrases[%q] = %q, want the shared const %q", tc.reason, got, tc.sharedPhrase)
			}
			if got := phraseFor(notEvaluableDetails, tc.reason); !strings.Contains(got, tc.sharedPhrase) {
				t.Errorf("notEvaluableDetails[%q] = %q, want it composed from the shared const %q", tc.reason, got, tc.sharedPhrase)
			}
		}
	})

	t.Run("it renders no raw reason slug on either surface", func(t *testing.T) {
		for _, tc := range standDownCopyCases() {
			if phrase := phraseFor(skippedPrunePhrases, tc.reason); phrase == string(tc.reason) || strings.Contains(phrase, string(tc.reason)) {
				t.Errorf("phraseFor(skippedPrunePhrases, %q) = %q; want a user-facing phrase, not the enum value", tc.reason, phrase)
			}
			if detail := phraseFor(notEvaluableDetails, tc.reason); detail == string(tc.reason) || strings.Contains(detail, string(tc.reason)) {
				t.Errorf("phraseFor(notEvaluableDetails, %q) = %q; want a user-facing phrase, not the enum value", tc.reason, detail)
			}
		}
	})

	// Withdrawn: it was signed off for the empty-live-set guard — a successful
	// read that answered nothing — while reading as a failed read. Neither
	// branch keeps it; each names its own condition.
	t.Run("it renders the withdrawn pane-read phrase on neither surface", func(t *testing.T) {
		const withdrawn = "could not read live panes"
		for _, tc := range standDownCopyCases() {
			if strings.Contains(phraseFor(skippedPrunePhrases, tc.reason), withdrawn) {
				t.Errorf("reason %q still renders the withdrawn phrase %q in the skipped-prune line", tc.reason, withdrawn)
			}
			if strings.Contains(phraseFor(notEvaluableDetails, tc.reason), withdrawn) {
				t.Errorf("reason %q still renders the withdrawn phrase %q in the diagnosis", tc.reason, withdrawn)
			}
		}
	})

	// The copy is a rendering concern and must stay one: a stand-down is not a
	// failed check on either surface, and both exit codes stay the post-repair
	// diagnosis's to drive.
	t.Run("it leaves the exit code to the post-repair diagnosis for every stand-down", func(t *testing.T) {
		for _, tc := range standDownCopyCases() {
			t.Run(tc.name, func(t *testing.T) {
				readDeps, _ := tc.fixture(t)
				readBuf, _, readErr := runDoctorWith(t, readDeps)
				if readErr != nil {
					t.Errorf("doctor err = %v; want nil over a stand-down the diagnosis reports as not evaluable\n%s", readErr, readBuf.String())
				}

				// The diagnosis still owns both exit codes: one genuinely
				// failing check under the same stand-down is non-zero again.
				failFix, _ := tc.fixture(t)
				failFix.SaverPresent = func() (bool, error) { return false, nil }
				fixFailBuf, _, fixFailErr := runDoctorWith(t, failFix, "--fix")
				if !errors.Is(fixFailErr, ErrDoctorUnhealthy) {
					t.Errorf("doctor --fix err = %v; want ErrDoctorUnhealthy with a failing check\n%s", fixFailErr, fixFailBuf.String())
				}

				failRead, _ := tc.fixture(t)
				failRead.SaverPresent = func() (bool, error) { return false, nil }
				readFailBuf, _, readFailErr := runDoctorWith(t, failRead)
				if !errors.Is(readFailErr, ErrDoctorUnhealthy) {
					t.Errorf("doctor err = %v; want ErrDoctorUnhealthy with a failing check\n%s", readFailErr, readFailBuf.String())
				}
			})
		}
	})
}

// rewordNotEvaluableDetail re-words one vocabulary entry for the test's
// duration. Watching a branch follow the re-worded entry tells a branch that
// renders through the vocabulary from one that authors the same words inline:
// an assertion on today's literal cannot separate them.
func rewordNotEvaluableDetail(t *testing.T, reason hooksweep.Reason, detail string) {
	t.Helper()
	prior := notEvaluableDetails[reason]
	notEvaluableDetails[reason] = detail
	t.Cleanup(func() { notEvaluableDetails[reason] = prior })
}

func TestStaleHooksCheckStandDownCopy(t *testing.T) {
	t.Run("it renders the nil-store and failed-load branches from the vocabulary", func(t *testing.T) {
		const reworded = "hooks.json could not be read this run"
		rewordNotEvaluableDetail(t, hooksweep.ReasonStoreReadFailed, reworded)

		unreadable, _ := hookstest.StageStore(t, hookstest.Staging{Unreadable: true})
		cases := []struct {
			name  string
			store *hooks.Store
		}{
			{name: "no store to read", store: nil},
			{name: "store read failed", store: unreadable},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got := checkStaleHooks(staleHookLister(), tc.store)
				if got.status != checkNotEvaluable {
					t.Errorf("status = %v, want not evaluable", got.status)
				}
				if got.detail != reworded {
					t.Errorf("detail = %q, want the re-worded vocabulary entry %q", got.detail, reworded)
				}
			})
		}
	})
}

// unreadableProjectStore stages a projects.json the store cannot read — inside
// a directory with no permissions, so Load fails with a permission error rather
// than reporting the file simply absent.
func unreadableProjectStore(t *testing.T) *project.Store {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "noread")
	if err := os.Mkdir(dir, 0o000); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	return project.NewStore(filepath.Join(dir, "projects.json"))
}

// The projects reaper stands down on a read it could not take, exactly as the
// hooks one does, but takes no reason from the hooks vocabulary, so it has no
// row in the table above. The const's own comment carries why.
func TestStaleProjectsCheckStandDownCopy(t *testing.T) {
	t.Run("it words the read failure as the user reads it", func(t *testing.T) {
		if projectStoreReadStandDownPhrase != "could not read projects.json" {
			t.Errorf("projectStoreReadStandDownPhrase = %q, want the line the report prints", projectStoreReadStandDownPhrase)
		}
	})

	t.Run("it renders the projects read-failure line from its declared const", func(t *testing.T) {
		cases := []struct {
			name  string
			store *project.Store
		}{
			{name: "no store to read", store: nil},
			{name: "store read failed", store: unreadableProjectStore(t)},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got := checkStaleProjects(tc.store)
				if got.status != checkNotEvaluable {
					t.Errorf("status = %v, want not evaluable", got.status)
				}
				if got.detail != projectStoreReadStandDownPhrase {
					t.Errorf("detail = %q, want the declared phrase %q", got.detail, projectStoreReadStandDownPhrase)
				}
			})
		}
	})
}
