package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/sessionindex"
)

const keptSessionsCheck = "kept sessions"

// keptDoctorDeps is a healthy doctor over a state directory saving entries,
// the listing holding live.
func keptDoctorDeps(t *testing.T, live []string, entries ...sessionindex.Entry) *DoctorDeps {
	t.Helper()
	dir := t.TempDir()
	saveIndex(t, dir, entries...)
	sessions := make([]backend.Session, 0, len(live))
	for _, name := range live {
		sessions = append(sessions, backend.Session{Name: name})
	}
	return withHealthyRuntime(t, &DoctorDeps{StateDir: dir, ListSessions: backendtest.New(sessions...).List, ValidateName: backendtest.New().ValidateName})
}

func hasCheck(results []checkResult, name string) bool {
	for _, r := range results {
		if r.name == name {
			return true
		}
	}
	return false
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

	t.Run("it is absent when nothing is kept", func(t *testing.T) {
		deps := keptDoctorDeps(t, []string{"api"}, sessionindex.Entry{Name: "api", CreatedAt: savedBeforeBoot})

		if results := diagnose(t, deps); hasCheck(results, keptSessionsCheck) {
			t.Errorf("results = %+v, want no %s line", results, keptSessionsCheck)
		}
	})

	for name, change := range map[string]func(t *testing.T, deps *DoctorDeps){
		"the sessions can't be listed": func(_ *testing.T, deps *DoctorDeps) {
			deps.ListSessions = func(context.Context) ([]backend.Session, error) { return nil, errors.New("list: refused") }
		},
		"sessions.json can't be read":  func(t *testing.T, deps *DoctorDeps) { writeSessionsJSON(t, deps.StateDir, []byte("{")) },
		"the name rule can't be bound": func(_ *testing.T, deps *DoctorDeps) { deps.ValidateName = nil },
	} {
		t.Run("it is absent when "+name, func(t *testing.T) {
			deps := keptDoctorDeps(t, nil, sessionindex.Entry{Name: "work", CreatedAt: savedBeforeBoot})
			change(t, deps)

			if results := diagnose(t, deps); hasCheck(results, keptSessionsCheck) {
				t.Errorf("results = %+v, want no %s line", results, keptSessionsCheck)
			}
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
