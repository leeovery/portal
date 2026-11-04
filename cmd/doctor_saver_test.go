package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/devversion"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/saverhost"
	"github.com/leeovery/portal/internal/state"
	"golang.org/x/sys/unix"
)

const (
	// doctorVersion is the version the healthy runtime's Portal and daemon
	// share.
	doctorVersion = "1.2.3"
	// doctorSaverPID is the pid the healthy runtime's saver session runs. It
	// may be a live process on the machine running the tests, so nothing
	// reads it or signals it.
	doctorSaverPID = 4242
)

// saverAnswers stands in for the saver doctor reads: whether it is there and
// the pid it runs, or err for both reads, and proof for every pid it is asked
// to prove, each of which it records.
type saverAnswers struct {
	present bool
	pid     int
	err     error
	proof   error
	proved  []int
}

func (s *saverAnswers) Present(context.Context) (bool, error) { return s.present, s.err }

func (s *saverAnswers) SaverPID(context.Context) (int, bool, error) {
	return s.pid, s.present, s.err
}

func (s *saverAnswers) Prove(pid int) error {
	s.proved = append(s.proved, pid)
	return s.proof
}

// runningSaver is a saver at doctorSaverPID whose daemon has recorded itself
// at version in dir, when dir is a directory it can write to.
func runningSaver(t *testing.T, dir, version string) *saverAnswers {
	t.Helper()
	if info, err := os.Stat(dir); err == nil && info.IsDir() && unix.Access(dir, unix.W_OK) == nil {
		recordDaemon(t, dir, doctorSaverPID, version)
	}
	return &saverAnswers{present: true, pid: doctorSaverPID}
}

func recordDaemon(t *testing.T, dir string, pid int, version string) {
	t.Helper()
	if err := state.WritePIDFile(dir, pid); err != nil {
		t.Fatalf("write daemon.pid: %v", err)
	}
	if err := state.WriteVersionFile(dir, version, nil); err != nil {
		t.Fatalf("write daemon.version: %v", err)
	}
}

func saverCheck(t *testing.T, s doctorSaver) checkResult {
	t.Helper()
	return findCheck(t, diagnose(t, withHealthyRuntime(t, &DoctorDeps{StateDir: t.TempDir(), Saver: s})), "saver")
}

func TestDoctorSaverCheck(t *testing.T) {
	t.Run("it passes a saver session that is there", func(t *testing.T) {
		assertCheck(t, saverCheck(t, &saverAnswers{present: true, pid: doctorSaverPID}), checkPass, "_portal-saver up")
	})

	t.Run("it fails when there is no saver session", func(t *testing.T) {
		assertCheck(t, saverCheck(t, &saverAnswers{}), checkFail, "_portal-saver not running — run portal open to start it")
	})

	t.Run("it is not evaluable when the backend can't tell", func(t *testing.T) {
		got := saverCheck(t, &saverAnswers{err: fmt.Errorf("session %q: %w", saver.SessionName, backend.ErrTimeout)})
		assertCheck(t, got, checkNotEvaluable, `could not read _portal-saver: session "_portal-saver": backend did not answer in time`)
	})

	t.Run("it is not evaluable with no saver to read", func(t *testing.T) {
		assertCheck(t, checkSaver(t.Context(), nil), checkNotEvaluable, "could not read _portal-saver")
	})
}

// daemonInstall is a state dir and a saver running doctorSaverPID.
type daemonInstall struct {
	dir   string
	saver *saverAnswers
}

func newDaemonInstall(t *testing.T) daemonInstall {
	t.Helper()
	return daemonInstall{dir: t.TempDir(), saver: &saverAnswers{present: true, pid: doctorSaverPID}}
}

func (d daemonInstall) check(t *testing.T, version string) checkResult {
	t.Helper()
	return checkDaemon(t.Context(), d.saver, d.dir, nil, version)
}

func TestDoctorDaemonCheck(t *testing.T) {
	t.Run("it passes the saver's process recorded as the daemon, at this version", func(t *testing.T) {
		d := newDaemonInstall(t)
		recordDaemon(t, d.dir, doctorSaverPID, doctorVersion)

		assertCheck(t, d.check(t, doctorVersion), checkPass, "running (pid 4242, version 1.2.3)")
		if want := []int{doctorSaverPID}; !slices.Equal(d.saver.proved, want) {
			t.Errorf("proved %v, want %v", d.saver.proved, want)
		}
	})

	t.Run("it passes on the saver's word when the proof can't be read", func(t *testing.T) {
		d := newDaemonInstall(t)
		recordDaemon(t, d.dir, doctorSaverPID, doctorVersion)
		d.saver.proof = fmt.Errorf("%w: ps: not supported", saverhost.ErrUnreadable)

		assertCheck(t, d.check(t, doctorVersion), checkPass, "running (pid 4242, version 1.2.3)")
	})

	for _, tc := range []struct {
		name   string
		stage  func(t *testing.T, d daemonInstall)
		detail string
	}{
		{"there is no saver session", func(t *testing.T, d daemonInstall) {
			recordDaemon(t, d.dir, doctorSaverPID, doctorVersion)
			d.saver.present = false
		}, "not running (no _portal-saver)"},
		{"no daemon recorded itself", func(*testing.T, daemonInstall) {}, "not running (no daemon.pid; _portal-saver runs pid 4242)"},
		{"daemon.pid names a process the saver doesn't run", func(t *testing.T, d daemonInstall) {
			recordDaemon(t, d.dir, 5151, doctorVersion)
		}, "daemon.pid names pid 5151, but _portal-saver runs pid 4242"},
		{"the daemon has exited", func(t *testing.T, d daemonInstall) {
			recordDaemon(t, d.dir, doctorSaverPID, doctorVersion)
			d.saver.proof = fmt.Errorf("pid 4242: %w", saverhost.ErrGone)
		}, "not running (pid 4242 has exited)"},
		{"the saver runs something else", func(t *testing.T, d daemonInstall) {
			recordDaemon(t, d.dir, doctorSaverPID, doctorVersion)
			d.saver.proof = fmt.Errorf("pid 4242 is no portal state daemon: %w", saverhost.ErrNotSaverDaemon)
		}, "pid 4242, which _portal-saver runs, isn't Portal's save daemon"},
	} {
		t.Run("it fails when "+tc.name, func(t *testing.T) {
			d := newDaemonInstall(t)
			tc.stage(t, d)
			assertCheck(t, d.check(t, doctorVersion), checkFail, tc.detail)
		})
	}

	t.Run("it never proves a pid the saver doesn't run", func(t *testing.T) {
		d := newDaemonInstall(t)
		recordDaemon(t, d.dir, 5151, doctorVersion)

		d.check(t, doctorVersion)

		if len(d.saver.proved) != 0 {
			t.Errorf("proved %v; daemon.pid named no process of the saver's", d.saver.proved)
		}
	})

	for _, tc := range []struct {
		name, recorded, version, detail string
	}{
		{"another version", "1.2.2", doctorVersion, "running (pid 4242, version 1.2.2); this Portal is 1.2.3"},
		{"no version", "", doctorVersion, "running (pid 4242, version unknown); this Portal is 1.2.3"},
		{"a version, when this Portal's build is unidentified", devversion.Unidentified, devversion.Unidentified, "running (pid 4242, version dev); this Portal's build is unidentified"},
	} {
		t.Run("it reports, never fails, a daemon recording "+tc.name, func(t *testing.T) {
			d := newDaemonInstall(t)
			recordDaemon(t, d.dir, doctorSaverPID, tc.recorded)
			if tc.recorded == "" {
				if err := os.Remove(state.DaemonVersion(d.dir)); err != nil {
					t.Fatal(err)
				}
			}
			assertCheck(t, d.check(t, tc.version), checkInfo, tc.detail)
		})
	}

	t.Run("it is not evaluable when it can't read what it judges", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			check  func(t *testing.T) checkResult
			detail string
		}{
			{"no saver to read", func(t *testing.T) checkResult {
				return checkDaemon(t.Context(), nil, t.TempDir(), nil, doctorVersion)
			}, "could not read _portal-saver"},
			{"a saver the backend can't tell", func(t *testing.T) checkResult {
				return checkDaemon(t.Context(), &saverAnswers{err: backend.ErrTimeout}, t.TempDir(), nil, doctorVersion)
			}, "could not read _portal-saver: backend did not answer in time"},
			{"no state dir", func(t *testing.T) checkResult {
				return checkDaemon(t.Context(), &saverAnswers{present: true, pid: doctorSaverPID}, "", errors.New("no home"), doctorVersion)
			}, stateDirStandDownPhrase},
			{"a daemon.pid that doesn't parse", func(t *testing.T) checkResult {
				d := newDaemonInstall(t)
				if err := os.WriteFile(state.DaemonPID(d.dir), []byte("not a pid\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				return d.check(t, doctorVersion)
			}, "could not read daemon.pid"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				assertCheck(t, tc.check(t), checkNotEvaluable, tc.detail)
			})
		}
	})
}

func TestDoctorPendingResumesCheck(t *testing.T) {
	listed := []backend.Session{{Name: "alpha-a1b2"}, {Name: "bravo-c3d4"}, {Name: "charlie-e5f6"}}
	raise := func(t *testing.T, dir string, names ...string) {
		t.Helper()
		for _, name := range names {
			if err := state.ResumePending.Set(dir, name); err != nil {
				t.Fatal(err)
			}
		}
	}

	for _, tc := range []struct {
		name   string
		raised []string
		detail string
	}{
		{"none", nil, "0 sessions waiting to resume"},
		{"one", []string{"bravo-c3d4"}, "1 session waiting to resume"},
		{"each live session flagged, and none that has ended", []string{"alpha-a1b2", "charlie-e5f6", "ended-g7h8"}, "2 sessions waiting to resume"},
	} {
		t.Run("it counts "+tc.name, func(t *testing.T) {
			dir := t.TempDir()
			raise(t, dir, tc.raised...)
			assertCheck(t, checkPendingResumes(dir, nil, listed, nil), checkInfo, tc.detail)
		})
	}

	t.Run("it counts none in a state dir not made yet", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "state")
		assertCheck(t, checkPendingResumes(dir, nil, listed, nil), checkInfo, "0 sessions waiting to resume")
		if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("the count made the state dir (stat err = %v)", err)
		}
	})

	t.Run("it is not evaluable when it can't read what it counts", func(t *testing.T) {
		unreadable := func(t *testing.T) string {
			if os.Geteuid() == 0 {
				t.Skip("root reads through a directory's permissions")
			}
			dir := t.TempDir()
			raise(t, dir, "alpha-a1b2")
			flags := filepath.Join(dir, string(state.ResumePending))
			if err := os.Chmod(flags, 0o000); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(flags, 0o700) })
			return dir
		}
		for _, tc := range []struct {
			name   string
			check  func(t *testing.T) checkResult
			detail string
		}{
			{"no state dir", func(*testing.T) checkResult {
				return checkPendingResumes("", errors.New("no home"), listed, nil)
			}, stateDirStandDownPhrase},
			{"no listing", func(t *testing.T) checkResult {
				return checkPendingResumes(t.TempDir(), nil, nil, errors.New("read zmx dir: permission denied"))
			}, listStandDownPhrase},
			{"flags it can't read", func(t *testing.T) checkResult {
				return checkPendingResumes(unreadable(t), nil, listed, nil)
			}, "could not read the resume-pending flags"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				assertCheck(t, tc.check(t), checkNotEvaluable, tc.detail)
			})
		}
	})
}

// The fixture is a whole install: sessions.json, a daemon recorded as the
// saver's process, a session waiting on its panel, a hook. Doctor reads it
// all through the production saver host, its process table read included,
// and its fingerprint is the same after. Every way cmd has to a process
// panics under TestMain, so a doctor that signalled would not return.
func TestDoctorLeavesTheInstallAsItFoundIt(t *testing.T) {
	dir := t.TempDir()
	seedValidSessionsJSON(t, dir, 1)
	recordDaemon(t, dir, os.Getpid(), doctorVersion)
	if err := state.ResumePending.Set(dir, "s0"); err != nil {
		t.Fatal(err)
	}
	store, _ := hookstest.StageStore(t, hookstest.Staging{Dir: dir, Seed: hookstest.StaleHookSeed, SidecarAbsent: true})
	fake := backendtest.New(backend.Session{Name: "s0"}, backend.Session{Name: saver.SessionName, PID: os.Getpid()})
	fake.SetReachEnv([]string{"ZMX_DIR=" + t.TempDir()})
	host, err := newReadOnlySaverHost(fake, dir)
	if err != nil {
		t.Fatalf("newReadOnlySaverHost: %v", err)
	}
	before := snapshotDir(t, dir)

	outBuf, _, _ := runDoctorWith(t, withHealthyRuntime(t, &DoctorDeps{StateDir: dir, ListSessions: fake.List, Saver: host, HookStore: store}))

	assertDirUnchanged(t, dir, before)
	for _, line := range []string{
		"✓ saver: _portal-saver up",
		fmt.Sprintf("✗ daemon: pid %d, which _portal-saver runs, isn't Portal's save daemon", os.Getpid()),
		"  pending resumes: 1 session waiting to resume",
	} {
		if !strings.Contains(outBuf.String(), line) {
			t.Errorf("report missing %q:\n%s", line, outBuf.String())
		}
	}
	for _, call := range fake.Calls() {
		if !slices.Contains([]backendtest.Op{backendtest.OpList, backendtest.OpExists, backendtest.OpSession}, call.Op) {
			t.Errorf("doctor called %s on the backend; it only reads", call.Op)
		}
	}
}
