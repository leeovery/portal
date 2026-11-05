package cmd

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

const keptSessionsCheck = "kept sessions"

// keptDoctorDeps is a healthy doctor over a state directory saving entries,
// the backend holding live.
func keptDoctorDeps(t *testing.T, live []string, entries ...sessionindex.Entry) *DoctorDeps {
	t.Helper()
	dir := t.TempDir()
	saveIndex(t, dir, entries...)
	sessions := make([]backend.Session, 0, len(live))
	for _, name := range live {
		sessions = append(sessions, backend.Session{Name: name})
	}
	b := backendtest.New(sessions...)
	return withHealthyRuntime(t, &DoctorDeps{StateDir: dir, ListSessions: b.List, Names: b})
}

func hasCheck(results []checkResult, name string) bool {
	for _, r := range results {
		if r.name == name {
			return true
		}
	}
	return false
}

// countedPresence counts the asks whether the saver runs.
type countedPresence struct {
	*saverAnswers
	asked int
}

func (c *countedPresence) Present(ctx context.Context) (bool, error) {
	c.asked++
	return c.saverAnswers.Present(ctx)
}

func assertNoKeptLine(t *testing.T, deps *DoctorDeps) {
	t.Helper()
	if results := diagnose(t, deps); hasCheck(results, keptSessionsCheck) {
		t.Errorf("results = %+v, want no %s line", results, keptSessionsCheck)
	}
}

// decision 78: doctor names the saved sessions x <name> can bring back.
func TestDoctorKeptSessions(t *testing.T) {
	t.Run("it names each kept session, in the index's order, saying what they are and how to bring one back", func(t *testing.T) {
		deps := keptDoctorDeps(t, []string{"api"},
			sessionindex.Entry{Name: "notes", CreatedAt: savedBeforeBoot},
			sessionindex.Entry{Name: "api", CreatedAt: savedBeforeBoot},
			sessionindex.Entry{Name: "ended", CreatedAt: testBoot.Add(time.Hour)},
			sessionindex.Entry{Name: "work"},
		)

		got := findCheck(t, diagnose(t, deps), keptSessionsCheck)

		assertCheck(t, got, checkInfo, "notes, work — saved but not running, back at the next restore; x <name> brings one back now")
	})

	// zmx finds a session by statting its socket, so on a filesystem that
	// folds case the live Notes holds notes's name.
	t.Run("it leaves out a session another spelling of whose name is live", func(t *testing.T) {
		deps := keptDoctorDeps(t, []string{"Notes"},
			sessionindex.Entry{Name: "notes", CreatedAt: savedBeforeBoot},
			sessionindex.Entry{Name: "work", CreatedAt: savedBeforeBoot},
		)

		got := findCheck(t, diagnose(t, deps), keptSessionsCheck)

		assertCheck(t, got, checkInfo, "work — saved but not running, back at the next restore; x <name> brings one back now")
	})

	// decision 58: with no saver running, the next bootstrap marks every saved
	// session not live owed.
	t.Run("with no saver running it names every saved session not live, Portal's own aside", func(t *testing.T) {
		deps := keptDoctorDeps(t, []string{"api"},
			sessionindex.Entry{Name: "notes", CreatedAt: savedBeforeBoot},
			sessionindex.Entry{Name: "api", CreatedAt: testBoot.Add(time.Hour)},
			sessionindex.Entry{Name: "ended", CreatedAt: testBoot.Add(time.Hour)},
			sessionindex.Entry{Name: "_portal-saver", CreatedAt: testBoot.Add(time.Hour)},
		)
		deps.Saver = &saverAnswers{present: false}

		got := findCheck(t, diagnose(t, deps), keptSessionsCheck)

		assertCheck(t, got, checkInfo, "notes, ended — saved but not running, back at the next restore; x <name> brings one back now")
	})

	t.Run("it reads the saver check's own answer, asking the saver nothing more", func(t *testing.T) {
		deps := keptDoctorDeps(t, nil, sessionindex.Entry{Name: "ended", CreatedAt: testBoot.Add(time.Hour)})
		saver := &countedPresence{saverAnswers: &saverAnswers{present: false}}
		deps.Saver = saver

		got := findCheck(t, diagnose(t, deps), keptSessionsCheck)

		assertCheck(t, got, checkInfo, "ended — saved but not running, back at the next restore; x <name> brings one back now")
		if saver.asked != 1 {
			t.Errorf("asked whether the saver runs %d times, want once", saver.asked)
		}
	})

	t.Run("a saver it can't read counts as running, naming only what is kept either way", func(t *testing.T) {
		deps := keptDoctorDeps(t, nil,
			sessionindex.Entry{Name: "notes", CreatedAt: savedBeforeBoot},
			sessionindex.Entry{Name: "ended", CreatedAt: testBoot.Add(time.Hour)},
		)
		deps.Saver = &saverAnswers{err: errors.New("zmx: refused")}

		got := findCheck(t, diagnose(t, deps), keptSessionsCheck)

		assertCheck(t, got, checkInfo, "notes — saved but not running, back at the next restore; x <name> brings one back now")
	})

	t.Run("it judges the boot without the claim rule that logs a boot time it can't read", func(t *testing.T) {
		deps := keptDoctorDeps(t, nil, sessionindex.Entry{Name: "notes", CreatedAt: savedBeforeBoot})
		withFuncSeam(t, &claimRule, func(dir string, names func(string) error) sessionindex.Rule {
			t.Error("doctor built claimRule, which warns of a boot time it can't read, want silentClaimRule")
			return testClaimRule(dir, names)
		})

		findCheck(t, diagnose(t, deps), keptSessionsCheck)
	})

	t.Run("it is absent when nothing is kept", func(t *testing.T) {
		assertNoKeptLine(t, keptDoctorDeps(t, []string{"api"}, sessionindex.Entry{Name: "api", CreatedAt: savedBeforeBoot}))
	})

	for name, change := range map[string]func(t *testing.T, deps *DoctorDeps){
		"the sessions can't be listed": func(_ *testing.T, deps *DoctorDeps) {
			deps.ListSessions = func(context.Context) ([]backend.Session, error) { return nil, errors.New("list: refused") }
		},
		"sessions.json can't be read": func(t *testing.T, deps *DoctorDeps) { writeSessionsJSON(t, deps.StateDir, []byte("{")) },
		"the backend can't be bound":  func(_ *testing.T, deps *DoctorDeps) { deps.Names = nil },
		"the state is another zmx dir's, so the sessions were listed elsewhere": func(t *testing.T, deps *DoctorDeps) {
			if err := state.RecordZmxDir(deps.StateDir, zmx.CanonicalDir(t.TempDir())); err != nil {
				t.Fatal(err)
			}
		},
		"the state's zmx-dir record can't be read": func(t *testing.T, deps *DoctorDeps) {
			if err := os.WriteFile(state.ZmxDirRecordPath(deps.StateDir), []byte("{"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run("it is absent when "+name, func(t *testing.T) {
			deps := keptDoctorDeps(t, nil, sessionindex.Entry{Name: "work", CreatedAt: savedBeforeBoot})
			change(t, deps)

			assertNoKeptLine(t, deps)
		})
	}

	t.Run("it never fails the exit code, and reads sessions.json without changing it", func(t *testing.T) {
		deps := keptDoctorDeps(t, nil, sessionindex.Entry{Name: "work", CreatedAt: savedBeforeBoot})
		before := readFileBytes(t, sessionindex.Path(deps.StateDir))

		out, _, err := runDoctorWith(t, deps)

		if err != nil || !strings.Contains(out.String(), "\n    "+keptSessionsCheck+": work — ") {
			t.Errorf("doctor = %v, report:\n%s\nwant a clean exit naming work", err, out)
		}
		if after := readFileBytes(t, sessionindex.Path(deps.StateDir)); string(after) != string(before) {
			t.Errorf("sessions.json changed by diagnosis\nbefore: %s\nafter:  %s", before, after)
		}
	})
}
