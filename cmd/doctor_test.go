package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/state"
)

const doctorZmxPath = "/opt/test/bin/zmx"

func doctorUnsupportedResolve(spawn.Identity) (spawn.Adapter, spawn.Resolution) {
	return nil, spawn.ResolutionUnsupported
}

func zmxFound(string) (string, error) { return doctorZmxPath, nil }

func zmxMissing(string) (string, error) { return "", errors.New("executable file not found in $PATH") }

// protocolAnswers stands in for the driver's protocol read: each named session
// answers with its version or its error, and any other is ErrNoSuchSession.
type protocolAnswers map[string]struct {
	version int
	err     error
}

func (p protocolAnswers) read(_ context.Context, name string) (int, error) {
	a, ok := p[name]
	if !ok {
		return 0, fmt.Errorf("read protocol version of session %q: %w", name, backend.ErrNoSuchSession)
	}
	return a.version, a.err
}

func protocolOne(context.Context, string) (int, error) { return 1, nil }

func seedValidSessionsJSON(t *testing.T, dir string, sessions int) {
	t.Helper()
	entries := make([]sessionindex.Entry, 0, sessions)
	for i := range sessions {
		entries = append(entries, sessionindex.Entry{Name: "s" + strconv.Itoa(i), Dir: "/tmp"})
	}
	idx := entryIndexOf(t, entries...)
	idx.SavedAt = time.Now()
	data, err := sessionindex.Encode(idx)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	writeSessionsJSON(t, dir, data)
}

func writeSessionsJSON(t *testing.T, dir string, data []byte) {
	t.Helper()
	if err := os.WriteFile(sessionindex.Path(dir), data, 0o600); err != nil {
		t.Fatalf("write sessions.json: %v", err)
	}
}

// withHealthyRuntime fills every unset runtime seam with a healthy answer: a
// Fake holding one answering session, zmx on PATH, a protocol 1 daemon, and a
// saver whose daemon, of this Portal's version, records itself in the state
// dir when that dir is there. It also stubs the host-terminal seams so no
// doctor test invokes a real detector, which would read the process tree.
func withHealthyRuntime(t *testing.T, deps *DoctorDeps) *DoctorDeps {
	t.Helper()
	if deps.Version == "" {
		deps.Version = doctorVersion
	}
	if deps.Saver == nil {
		deps.Saver = runningSaver(t, deps.StateDir, deps.Version)
	}
	if deps.DaemonSettle == 0 {
		deps.DaemonSettle = testDaemonSettle
	}
	if deps.ListSessions == nil {
		deps.ListSessions = backendtest.New(backend.Session{Name: "api-x7Kd9a"}).List
	}
	if deps.ZmxDir == "" {
		deps.ZmxDir = t.TempDir()
	}
	if deps.LookPath == nil {
		deps.LookPath = zmxFound
	}
	if deps.ProtocolVersion == nil {
		deps.ProtocolVersion = protocolOne
	}
	if deps.Detector == nil {
		deps.Detector = fakeTerminalDetector{}
	}
	if deps.Resolve == nil {
		deps.Resolve = doctorUnsupportedResolve
	}
	if deps.HookStore == nil {
		deps.HookStore = hooks.NewStore(filepath.Join(t.TempDir(), "hooks.json"))
	}
	return deps
}

func diagnose(t *testing.T, deps *DoctorDeps) []checkResult {
	t.Helper()
	results, err := runDoctorDiagnosis(t.Context(), deps)
	if err != nil {
		t.Fatalf("runDoctorDiagnosis: %v", err)
	}
	return results
}

func findCheck(t *testing.T, results []checkResult, name string) checkResult {
	t.Helper()
	for _, r := range results {
		if r.name == name {
			return r
		}
	}
	t.Fatalf("no check named %q in %+v", name, results)
	return checkResult{}
}

func assertCheck(t *testing.T, got checkResult, status checkStatus, detail string) {
	t.Helper()
	if got.status != status || got.detail != detail {
		t.Errorf("%s = (%v, %q); want (%v, %q)", got.name, got.status, got.detail, status, detail)
	}
}

// unreadableDir makes a directory that stats as one and refuses to be opened.
func unreadableDir(t *testing.T) string {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root bypasses 0o000 directory permissions")
	}
	dir := filepath.Join(t.TempDir(), "noread")
	if err := os.Mkdir(dir, 0o000); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	return dir
}

func TestDoctorAllChecksPassExitsZero(t *testing.T) {
	dir := t.TempDir()
	seedValidSessionsJSON(t, dir, 1)
	projectStore, _ := seedProjectsJSON(t, t.TempDir())
	zmxDir := t.TempDir()

	outBuf, _, err := runDoctorWith(t, withHealthyRuntime(t, &DoctorDeps{StateDir: dir, ZmxDir: zmxDir, ProjectStore: projectStore}))
	if err != nil {
		t.Fatalf("Execute returned %v; want nil when every check passes", err)
	}

	want := "Portal doctor:\n" +
		"  ✓ zmx: " + doctorZmxPath + "\n" +
		"  ✓ zmx dir: " + zmxDir + "\n" +
		"  ✓ sessions: 1 session answering\n" +
		"  ✓ saver: _portal-saver up\n" +
		"  ✓ daemon: running (pid 4242, version " + doctorVersion + ")\n" +
		"  ✓ state dir: " + dir + "\n" +
		"  ✓ state's zmx dir: not recorded yet\n" +
		"  ✓ sessions.json: 1 session indexed\n" +
		"  ✓ stale hooks: no hooks\n" +
		"  ✓ stale projects: no stale projects\n" +
		"    pending resumes: 0 sessions waiting to resume\n" +
		defaultKeyConfigLine + "\n" +
		"    host terminal: unsupported (remote session)\n" +
		"    resume mode: " + installResumeModeOf(nil).String() + "\n" +
		"  11 checks passed\n"
	if got := outBuf.String(); got != want {
		t.Errorf("report =\n%s\nwant\n%s", got, want)
	}
}

func TestDoctorZeroValueCheckResultNotHealthy(t *testing.T) {
	zero := checkResult{}

	if zero.status == checkPass {
		t.Errorf("zero-value checkResult{}.status = %v; want a non-pass sentinel, never checkPass", zero.status)
	}
	if zero.status != checkUnknown {
		t.Errorf("zero-value checkResult{}.status = %v; want checkUnknown (iota 0)", zero.status)
	}
	if checkUnknown != 0 {
		t.Errorf("checkUnknown = %d; want the iota-0 value", checkUnknown)
	}

	if marker := checkMarker(zero.status); marker == "✓" {
		t.Errorf("checkMarker(zero-value) = %q; a zero-value check must not render the pass marker", marker)
	}

	if !doctorUnhealthy([]checkResult{zero}) {
		t.Error("doctorUnhealthy([zero-value]) = false; a forgotten status assignment must not yield a healthy exit")
	}
}

// A fresh install has run no Portal command, so no saver: that is its one
// failure, and every line about what Portal has made passes.
// A fresh install has nothing for a saver to save, so the saver it doesn't
// run yet is stated, and doctor reads it healthy.
func TestDoctorFreshInstallIsHealthy(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	zmxDir := filepath.Join(t.TempDir(), "zmx")
	outBuf, _, err := runDoctorWith(t, withHealthyRuntime(t, &DoctorDeps{StateDir: dir, ZmxDir: zmxDir, ListSessions: backendtest.New().List, Saver: &saverAnswers{}}))
	if err != nil {
		t.Fatalf("Execute err = %v; want a healthy fresh install", err)
	}
	out := outBuf.String()
	for _, line := range []string{
		"✓ zmx dir: not created yet",
		"✓ sessions: no sessions",
		"  saver: _portal-saver not running — nothing to save yet",
		"  daemon: not running (no _portal-saver; nothing to save yet)",
		"✓ state dir: not created yet",
		"✓ state's zmx dir: not recorded yet",
		"✓ sessions.json: no sessions indexed yet",
		"  pending resumes: 0 sessions waiting to resume",
		"9 checks passed",
	} {
		if !strings.Contains(out, line) {
			t.Errorf("report missing %q:\n%s", line, out)
		}
	}
}

func TestDoctorIsReadOnly(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	zmxDir := filepath.Join(t.TempDir(), "zmx")
	fake := backendtest.New(backend.Session{Name: "api"}, backend.Session{Name: "wedged"})
	fake.StopAnswering("wedged")

	if _, _, err := runDoctorWith(t, withHealthyRuntime(t, &DoctorDeps{StateDir: dir, ZmxDir: zmxDir, ListSessions: fake.List})); err != ErrDoctorUnhealthy {
		t.Fatalf("Execute err = %v; want ErrDoctorUnhealthy for the silent session", err)
	}

	for _, made := range []string{dir, zmxDir} {
		if _, err := os.Stat(made); !os.IsNotExist(err) {
			t.Errorf("doctor created %q (stat err = %v); want it left absent", made, err)
		}
	}
	for _, call := range fake.Calls() {
		if call.Op != backendtest.OpList {
			t.Errorf("doctor called %s on the backend; the diagnosis only lists", call.Op)
		}
	}
	if _, ok := fake.Held("wedged"); !ok {
		t.Error("the silent session is gone; doctor must leave it as it found it")
	}
}

func TestDoctorZmxBinaryCheck(t *testing.T) {
	t.Run("it shows the path of the zmx on PATH", func(t *testing.T) {
		got := findCheck(t, diagnose(t, withHealthyRuntime(t, &DoctorDeps{StateDir: t.TempDir()})), "zmx")
		assertCheck(t, got, checkPass, doctorZmxPath)
	})

	t.Run("it fails naming the zmx it needs when zmx is not on PATH", func(t *testing.T) {
		got := findCheck(t, diagnose(t, withHealthyRuntime(t, &DoctorDeps{StateDir: t.TempDir(), LookPath: zmxMissing})), "zmx")
		assertCheck(t, got, checkFail, "zmx not found on PATH; Portal needs a zmx build newer than 0.8.1")
	})

	t.Run("it looks up the zmx program", func(t *testing.T) {
		var looked []string
		lookPath := func(file string) (string, error) {
			looked = append(looked, file)
			return doctorZmxPath, nil
		}
		diagnose(t, withHealthyRuntime(t, &DoctorDeps{StateDir: t.TempDir(), LookPath: lookPath}))
		if !slices.Equal(looked, []string{"zmx"}) {
			t.Errorf("looked up %v; want [zmx]", looked)
		}
	})
}

func TestDoctorZmxDirCheck(t *testing.T) {
	notADir := filepath.Join(t.TempDir(), "zmx")
	if err := os.WriteFile(notADir, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	existing := t.TempDir()

	cases := []struct {
		name   string
		dir    string
		status checkStatus
		detail string
	}{
		{"an existing dir passes with its path", existing, checkPass, existing},
		{"a dir not made yet passes", filepath.Join(t.TempDir(), "zmx"), checkPass, "not created yet"},
		{"an unreadable dir fails", unreadableDir(t), checkFail, "unreadable"},
		{"a file in its place fails", notADir, checkFail, "not a directory"},
		{"a relative dir fails", "zmx", checkFail, `"zmx" is not an absolute path`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := findCheck(t, diagnose(t, withHealthyRuntime(t, &DoctorDeps{StateDir: t.TempDir(), ZmxDir: tc.dir})), "zmx dir")
			assertCheck(t, got, tc.status, tc.detail)
		})
	}
}

// silentSessions is a Fake holding the named sessions, none of them answering.
func silentSessions(names ...string) *backendtest.Fake {
	fake := backendtest.New()
	for _, name := range names {
		fake.Add(backend.Session{Name: name})
		fake.StopAnswering(name)
	}
	return fake
}

func sessionsDeps(t *testing.T, fake *backendtest.Fake, protocolVersion func(context.Context, string) (int, error)) *DoctorDeps {
	t.Helper()
	return withHealthyRuntime(t, &DoctorDeps{StateDir: t.TempDir(), ListSessions: fake.List, ProtocolVersion: protocolVersion})
}

func sessionsCheck(t *testing.T, fake *backendtest.Fake, protocolVersion func(context.Context, string) (int, error)) checkResult {
	t.Helper()
	return findCheck(t, diagnose(t, sessionsDeps(t, fake, protocolVersion)), "sessions")
}

func TestDoctorSessionsCheck(t *testing.T) {
	t.Run("it passes when every session answers", func(t *testing.T) {
		fake := backendtest.New(backend.Session{Name: "api"}, backend.Session{Name: "web"})
		assertCheck(t, sessionsCheck(t, fake, nil), checkPass, "2 sessions answering")
	})

	t.Run("it names a protocol-2 daemon with its version and a silent one as not answering", func(t *testing.T) {
		fake := silentSessions("future", "old-zmx")
		fake.Add(backend.Session{Name: "api"})
		answers := protocolAnswers{
			"future":  {version: 2},
			"old-zmx": {err: fmt.Errorf("no answer in 1s: %w", backend.ErrTimeout)},
		}
		assertCheck(t, sessionsCheck(t, fake, answers.read), checkFail, "2 of 3 not answering: future (speaks protocol 2; Portal speaks 1), old-zmx (no answer)")
	})

	t.Run("it gives a refusing socket the hint that clears it", func(t *testing.T) {
		answers := protocolAnswers{
			"crashed": {err: fmt.Errorf("read protocol version: connection refused: %w", zmx.ErrRefusing)},
			"wedged":  {err: backend.ErrTimeout},
		}
		assertCheck(t, sessionsCheck(t, silentSessions("crashed", "wedged"), answers.read), checkFail,
			"2 of 2 not answering: crashed (refuses connections: a zmx process may still hold it; try again in a moment), wedged (no answer)")
	})

	t.Run("it names Portal's own sessions too", func(t *testing.T) {
		answers := protocolAnswers{"_portal-saver": {err: backend.ErrTimeout}}
		assertCheck(t, sessionsCheck(t, silentSessions("_portal-saver"), answers.read), checkFail, "1 of 1 not answering: _portal-saver (no answer)")
	})

	t.Run("it names a daemon whose header reads protocol 1 as not answering", func(t *testing.T) {
		assertCheck(t, sessionsCheck(t, silentSessions("slow"), protocolOne), checkFail, "1 of 1 not answering: slow (no answer)")
	})

	t.Run("it leaves out a session that ended after the listing", func(t *testing.T) {
		fake := silentSessions("gone")
		fake.Add(backend.Session{Name: "api"})
		assertCheck(t, sessionsCheck(t, fake, protocolAnswers{}.read), checkPass, "1 session answering")
	})

	t.Run("it counts a session that ended beside one still silent out of the total", func(t *testing.T) {
		answers := protocolAnswers{"wedged": {err: backend.ErrTimeout}}
		assertCheck(t, sessionsCheck(t, silentSessions("gone", "wedged"), answers.read), checkFail, "1 of 1 not answering: wedged (no answer)")
	})

	t.Run("it reports no sessions when every listed one has ended", func(t *testing.T) {
		assertCheck(t, sessionsCheck(t, silentSessions("gone"), protocolAnswers{}.read), checkPass, "no sessions")
	})

	t.Run("it reads the protocol of the silent sessions alone", func(t *testing.T) {
		fake := silentSessions("wedged")
		fake.Add(backend.Session{Name: "api"})
		var mu sync.Mutex
		var asked []string
		read := func(_ context.Context, name string) (int, error) {
			mu.Lock()
			defer mu.Unlock()
			asked = append(asked, name)
			return 0, backend.ErrTimeout
		}

		sessionsCheck(t, fake, read)
		if !slices.Equal(asked, []string{"wedged"}) {
			t.Errorf("protocol read for %v; want [wedged]", asked)
		}
	})

	// Each silent daemon costs the probe ceiling, so asking them in turn would
	// cost one ceiling each.
	t.Run("it asks every silent session at once", func(t *testing.T) {
		var arrived sync.WaitGroup
		arrived.Add(2)
		read := func(context.Context, string) (int, error) {
			arrived.Done()
			waited := make(chan struct{})
			go func() { arrived.Wait(); close(waited) }()
			select {
			case <-waited:
				return 0, backend.ErrTimeout
			case <-time.After(5 * time.Second):
				return 7, nil
			}
		}
		assertCheck(t, sessionsCheck(t, silentSessions("a", "b"), read), checkFail, "2 of 2 not answering: a (no answer), b (no answer)")
	})

	// Each probe holds a socket open, so a directory of many silent sessions
	// must not exhaust Portal's file descriptors.
	t.Run("it holds no more than 64 probes in flight", func(t *testing.T) {
		names := make([]string, 100)
		for i := range names {
			names[i] = fmt.Sprintf("s%03d", i)
		}
		var mu sync.Mutex
		inFlight, most := 0, 0
		full := make(chan struct{})
		var fill sync.Once
		read := func(context.Context, string) (int, error) {
			mu.Lock()
			inFlight++
			most = max(most, inFlight)
			if inFlight == maxProtocolProbes {
				fill.Do(func() { close(full) })
			}
			mu.Unlock()
			select {
			case <-full:
			case <-time.After(5 * time.Second):
			}
			mu.Lock()
			inFlight--
			mu.Unlock()
			return 0, backend.ErrTimeout
		}

		got := sessionsCheck(t, silentSessions(names...), read)
		if got.status != checkFail || !strings.HasPrefix(got.detail, "100 of 100 not answering: ") {
			t.Errorf("sessions = (%v, %q); want all 100 named", got.status, got.detail)
		}
		if most != 64 {
			t.Errorf("most probes in flight = %d; want the cap of 64", most)
		}
	})

	t.Run("it is not evaluable when the listing fails", func(t *testing.T) {
		fake := backendtest.New()
		fake.Fail(backendtest.OpList, errors.New("list zmx sessions: permission denied"))

		results := diagnose(t, sessionsDeps(t, fake, nil))
		assertCheck(t, findCheck(t, results, "sessions"), checkNotEvaluable, "could not list sessions: list zmx sessions: permission denied")
		if doctorUnhealthy(results) {
			t.Error("doctorUnhealthy = true; a not-evaluable line must never drive the exit code")
		}
	})
}

func TestDoctorSessionsJSONStates(t *testing.T) {
	cases := []struct {
		name   string
		seed   func(t *testing.T, dir string)
		status checkStatus
		detail string
	}{
		{"a version 2 index reports its sessions", func(t *testing.T, dir string) { seedValidSessionsJSON(t, dir, 3) }, checkPass, "3 sessions indexed"},
		{"an absent file passes", func(*testing.T, string) {}, checkPass, "no sessions indexed yet"},
		{"a corrupt file fails", func(t *testing.T, dir string) { writeSessionsJSON(t, dir, []byte("{not json")) }, checkFail, "corrupt"},
		{"a newer version fails", func(t *testing.T, dir string) { writeSessionsJSON(t, dir, []byte(`{"version": 3}`)) }, checkFail, newerIndexDetail},
		{"a version 1 file fails", func(t *testing.T, dir string) { writeSessionsJSON(t, dir, []byte(`{"version": 1}`)) }, checkFail, legacyIndexDetail},
		{"an unreadable file fails", func(t *testing.T, dir string) {
			writeSessionsJSON(t, dir, []byte(`{"version": 2}`))
			if err := os.Chmod(sessionindex.Path(dir), 0o000); err != nil {
				t.Fatal(err)
			}
		}, checkFail, "unreadable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			tc.seed(t, dir)
			got := findCheck(t, diagnose(t, withHealthyRuntime(t, &DoctorDeps{StateDir: dir})), "sessions.json")
			assertCheck(t, got, tc.status, tc.detail)
		})
	}
}

const (
	legacyIndexDetail = "from the tmux version of Portal; run portal migrate"
	newerIndexDetail  = "version 3, from a newer Portal; this one reads version 2"
)

// Doctor runs past the root's stop on an unusable sessions.json, so it can
// report the index itself.
func TestDoctorReportsAnUnusableIndexThroughTheRoot(t *testing.T) {
	for name, tc := range map[string]struct{ body, detail string }{
		"version 1": {legacyIndexBody, legacyIndexDetail},
		"corrupt":   {`{"version": 2,`, "corrupt"},
		"newer":     {`{"version": 3, "sessions": []}`, newerIndexDetail},
	} {
		t.Run("it reports a "+name+" sessions.json and fails", func(t *testing.T) {
			seedSessionsJSON(t, tc.body)

			outBuf, errBuf, err := runDoctorWith(t, withHealthyRuntime(t, &DoctorDeps{}))
			if err != ErrDoctorUnhealthy {
				t.Fatalf("Execute err = %v; want ErrDoctorUnhealthy from the report, not the root's stop", err)
			}
			if errBuf.Len() != 0 {
				t.Errorf("stderr = %q; want it silent", errBuf.String())
			}
			if want := "\n  ✗ sessions.json: " + tc.detail + "\n"; !strings.Contains(outBuf.String(), want) {
				t.Errorf("report missing %q:\n%s", want, outBuf.String())
			}
		})
	}
}

func TestDoctorStateDirCheck(t *testing.T) {
	notADir := filepath.Join(t.TempDir(), "state")
	if err := os.WriteFile(notADir, []byte("i am a file, not a dir"), 0o600); err != nil {
		t.Fatal(err)
	}
	existing := t.TempDir()
	blockedParent := unreadableDir(t)

	cases := []struct {
		name   string
		dir    string
		status checkStatus
		detail string
	}{
		{"an existing dir passes with its path", existing, checkPass, existing},
		{"a file in its place fails", notADir, checkFail, "not a directory"},
		{"a dir that cannot be stat'ed fails", filepath.Join(blockedParent, "state"), checkFail, "unreadable"},
		{"a dir that cannot be opened fails", blockedParent, checkFail, "unreadable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := findCheck(t, diagnose(t, withHealthyRuntime(t, &DoctorDeps{StateDir: tc.dir})), "state dir")
			assertCheck(t, got, tc.status, tc.detail)
		})
	}

	t.Run("an unresolvable dir fails every state check", func(t *testing.T) {
		unresolved := errors.New("no home")
		assertCheck(t, checkStateDirSane("", unresolved), checkFail, "unresolvable")
		assertCheck(t, checkStateZmxDir("", unresolved, t.TempDir()), checkFail, "unresolvable")
		assertCheck(t, checkSessionsJSON("", unresolved), checkFail, "unresolvable")
	})
}

// decision 76: doctor reads which zmx dir the state dir belongs to, as the
// bootstrap does, and records nothing.
func TestDoctorStateZmxDirCheck(t *testing.T) {
	check := func(t *testing.T, stateDir, zmxDir string) checkResult {
		t.Helper()
		return findCheck(t, diagnose(t, withHealthyRuntime(t, &DoctorDeps{StateDir: stateDir, ZmxDir: zmxDir})), "state's zmx dir")
	}
	recorded := func(t *testing.T, zmxDir string) string {
		t.Helper()
		stateDir := t.TempDir()
		if err := state.RecordZmxDir(stateDir, zmx.CanonicalDir(zmxDir)); err != nil {
			t.Fatal(err)
		}
		return stateDir
	}

	t.Run("a state dir recording none passes, and doctor records none", func(t *testing.T) {
		stateDir := t.TempDir()

		assertCheck(t, check(t, stateDir, t.TempDir()), checkPass, "not recorded yet")
		if _, err := os.Stat(state.ZmxDirRecordPath(stateDir)); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("stat the record = %v, want none: doctor is read-only", err)
		}
	})

	t.Run("a state dir of this zmx dir passes with it", func(t *testing.T) {
		zmxDir := t.TempDir()

		assertCheck(t, check(t, recorded(t, zmxDir), zmxDir), checkPass, zmx.CanonicalDir(zmxDir))
	})

	t.Run("a state dir of another zmx dir fails, naming both and the way out", func(t *testing.T) {
		zmxDir, other := t.TempDir(), t.TempDir()
		real := zmx.CanonicalDir(zmxDir)
		stateDir := recorded(t, zmxDir)

		want := "belongs to the zmx sessions in " + real + " (recorded in " + state.ZmxDirRecordPath(stateDir) + "), but this Portal is using " + other + "; run it with ZMX_DIR=" + real + ", or give it a state of its own with PORTAL_STATE_DIR"
		assertCheck(t, check(t, stateDir, other), checkFail, want)
	})

	t.Run("a record it can't read fails, naming it and the way out", func(t *testing.T) {
		stateDir := t.TempDir()
		if err := os.WriteFile(state.ZmxDirRecordPath(stateDir), []byte(`{"zmx_dir":"zmx"}`), 0o600); err != nil {
			t.Fatal(err)
		}

		want := "can't read " + state.ZmxDirRecordPath(stateDir) + `: "zmx" is not an absolute directory; delete it, and Portal records the directory in use at its next start`
		assertCheck(t, check(t, stateDir, t.TempDir()), checkFail, want)
	})
}

// --fix writes state, so it repairs nothing in a state directory that belongs
// to another zmx directory, or whose record it can't read.
func TestDoctorFixUnderAnotherZmxDir(t *testing.T) {
	staleProject := filepath.Join(t.TempDir(), "gone")
	install := func(t *testing.T) (staleHooksInstall, string) {
		t.Helper()
		in := newStaleHooksInstall(t, hookstest.StaleHookSeed)
		projects, projectsFile := seedProjectsJSON(t, staleProject)
		in.deps.ProjectStore = projects
		return in, projectsFile
	}

	for name, record := range map[string]func(t *testing.T, stateDir string){
		"another zmx dir recorded": func(t *testing.T, stateDir string) {
			if err := state.RecordZmxDir(stateDir, zmx.CanonicalDir(t.TempDir())); err != nil {
				t.Fatal(err)
			}
		},
		"a record it can't read": func(t *testing.T, stateDir string) {
			if err := os.WriteFile(state.ZmxDirRecordPath(stateDir), []byte("{"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run("under "+name+" it repairs nothing and exits non-zero", func(t *testing.T) {
			in, projectsFile := install(t)
			record(t, in.deps.StateDir)
			hooksBefore, projectsBefore := readFileBytes(t, in.hooksPath), readFileBytes(t, projectsFile)

			outBuf, _, err := runDoctorWith(t, in.deps, "--fix")

			if !errors.Is(err, ErrDoctorUnhealthy) {
				t.Errorf("Execute err = %v, want ErrDoctorUnhealthy", err)
			}
			out := outBuf.String()
			if !strings.Contains(out, "  ✗ state's zmx dir: ") || !strings.HasSuffix(out, "\n"+fixRefusedLine+"\n") || strings.Count(out, "Portal doctor:") != 1 {
				t.Errorf("output = %q, want one diagnosis naming the check, then %q", out, fixRefusedLine)
			}
			assertHooksFileUnchanged(t, in.hooksPath, hooksBefore)
			if got := readFileBytes(t, projectsFile); !bytes.Equal(got, projectsBefore) {
				t.Errorf("projects.json = %s, want %s untouched", got, projectsBefore)
			}
		})
	}

	t.Run("with no record it repairs", func(t *testing.T) {
		in, projectsFile := install(t)

		outBuf, _, err := runDoctorWith(t, in.deps, "--fix")

		if err != nil {
			t.Fatalf("Execute err = %v, want nil", err)
		}
		if out := outBuf.String(); strings.Contains(out, fixRefusedLine) || !strings.Contains(out, "Pruned 1 stale hook:") || !strings.Contains(out, "Pruned stale project: proj0") {
			t.Errorf("output = %q, want both repairs made", out)
		}
		if got := readFileBytes(t, projectsFile); bytes.Contains(got, []byte(staleProject)) {
			t.Errorf("projects.json = %s, want %s pruned", got, staleProject)
		}
	})
}

func TestDoctorRegisteredInSkipTmuxCheck(t *testing.T) {
	if !bootstrapExempt["doctor"] {
		t.Error("bootstrapExempt[\"doctor\"] = false; want true (Bootstrap Exemption)")
	}
}

func TestDoctorIsRegisteredCommand(t *testing.T) {
	for _, c := range rootCmd.Commands() {
		if c.Name() == "doctor" {
			return
		}
	}
	t.Error("doctor is not registered on rootCmd")
}

func TestDoctorRejectsArgs(t *testing.T) {
	if _, _, err := runDoctorWith(t, withHealthyRuntime(t, &DoctorDeps{StateDir: t.TempDir()}), "unexpected"); err == nil {
		t.Error("Execute returned nil for `doctor unexpected`; want a NoArgs error")
	}
}

func TestDoctorSilenceFlags(t *testing.T) {
	if !doctorCmd.SilenceErrors {
		t.Error("doctorCmd.SilenceErrors = false; want true")
	}
	if !doctorCmd.SilenceUsage {
		t.Error("doctorCmd.SilenceUsage = false; want true")
	}
}

func TestIsSilentExitErrorRecognisesDoctorUnhealthy(t *testing.T) {
	if !IsSilentExitError(ErrDoctorUnhealthy) {
		t.Error("IsSilentExitError(ErrDoctorUnhealthy) = false; want true")
	}
}

func TestDoctorExitCode(t *testing.T) {
	t.Run("it exits unhealthy, silent on stderr, when a check fails", func(t *testing.T) {
		outBuf, errBuf, err := runDoctorWith(t, withHealthyRuntime(t, &DoctorDeps{StateDir: t.TempDir(), LookPath: zmxMissing}))
		if err != ErrDoctorUnhealthy {
			t.Fatalf("Execute err = %v; want ErrDoctorUnhealthy with zmx missing", err)
		}
		if errBuf.Len() != 0 {
			t.Errorf("stderr = %q; want it silent on an unhealthy exit", errBuf.String())
		}
		if !strings.HasSuffix(outBuf.String(), "\n  10 of 11 checks passed\n") {
			t.Errorf("report does not close with the partial summary:\n%s", outBuf.String())
		}
	})

	t.Run("it exits healthy when the only lines short of a pass are info and not evaluable", func(t *testing.T) {
		fake := backendtest.New()
		fake.Fail(backendtest.OpList, errors.New("transient"))
		results := diagnose(t, withHealthyRuntime(t, &DoctorDeps{StateDir: t.TempDir(), ListSessions: fake.List}))
		for _, name := range []string{"sessions", "stale projects"} {
			if got := findCheck(t, results, name); got.status != checkNotEvaluable {
				t.Fatalf("setup: %s status = %v; want not evaluable", name, got.status)
			}
		}
		if doctorUnhealthy(results) {
			t.Error("doctorUnhealthy = true; info and not-evaluable lines never fail doctor")
		}
	})

	t.Run("it fails for each failing check", func(t *testing.T) {
		silent := func(t *testing.T) *DoctorDeps {
			return &DoctorDeps{StateDir: t.TempDir(), ListSessions: silentSessions("wedged").List}
		}
		corrupt := func(t *testing.T) *DoctorDeps {
			dir := t.TempDir()
			writeSessionsJSON(t, dir, []byte("{"))
			return &DoctorDeps{StateDir: dir}
		}
		staleProject := func(t *testing.T) *DoctorDeps {
			store, _ := seedProjectsJSON(t, filepath.Join(t.TempDir(), "gone"))
			return &DoctorDeps{StateDir: t.TempDir(), ProjectStore: store}
		}
		for name, build := range map[string]func(*testing.T) *DoctorDeps{
			"zmx":            func(t *testing.T) *DoctorDeps { return &DoctorDeps{StateDir: t.TempDir(), LookPath: zmxMissing} },
			"zmx dir":        func(t *testing.T) *DoctorDeps { return &DoctorDeps{StateDir: t.TempDir(), ZmxDir: "relative"} },
			"sessions":       silent,
			"sessions.json":  corrupt,
			"stale projects": staleProject,
		} {
			t.Run(name, func(t *testing.T) {
				results := diagnose(t, withHealthyRuntime(t, build(t)))
				if got := findCheck(t, results, name); got.status != checkFail {
					t.Fatalf("setup: %s status = %v; want checkFail", name, got.status)
				}
				if !doctorUnhealthy(results) {
					t.Errorf("doctorUnhealthy = false; a failing %s check must fail doctor", name)
				}
			})
		}
	})
}

func TestDoctorHostTerminalLine(t *testing.T) {
	hostDeps := func(t *testing.T, id spawn.Identity, resolution spawn.Resolution) *DoctorDeps {
		return withHealthyRuntime(t, &DoctorDeps{
			StateDir: t.TempDir(),
			Detector: fakeTerminalDetector{id: id},
			Resolve: func(spawn.Identity) (spawn.Adapter, spawn.Resolution) {
				return nil, resolution
			},
		})
	}

	t.Run("driven terminal reports supported", func(t *testing.T) {
		deps := hostDeps(t, spawn.Identity{Name: "Ghostty", BundleID: "com.mitchellh.ghostty"}, spawn.ResolutionNative)
		assertCheck(t, findCheck(t, diagnose(t, deps), "host terminal"), checkInfo, "Ghostty (supported)")
	})

	t.Run("null identity reports unsupported remote session regardless of resolve", func(t *testing.T) {
		// Resolve returns Native, which the NULL short-circuit must ignore.
		deps := hostDeps(t, spawn.Identity{}, spawn.ResolutionNative)
		assertCheck(t, findCheck(t, diagnose(t, deps), "host terminal"), checkInfo, "unsupported (remote session)")
	})

	t.Run("an unsure identity reports that it can't tell, not a remote session", func(t *testing.T) {
		deps := hostDeps(t, spawn.Identity{Unsure: true}, spawn.ResolutionNative)
		assertCheck(t, findCheck(t, diagnose(t, deps), "host terminal"), checkInfo, "unsupported (can't tell which terminal Portal is running in)")
	})

	t.Run("recognised but undriven terminal reports unsupported", func(t *testing.T) {
		deps := hostDeps(t, spawn.Identity{Name: "SomeTerm", BundleID: "com.some.term"}, spawn.ResolutionUnsupported)
		assertCheck(t, findCheck(t, diagnose(t, deps), "host terminal"), checkInfo, "SomeTerm (unsupported)")
	})
}

func TestDoctorHostTerminalNeverDrivesExit(t *testing.T) {
	t.Run("unsupported host with a healthy runtime stays healthy", func(t *testing.T) {
		store, _ := seedProjectsJSON(t)
		results := diagnose(t, withHealthyRuntime(t, &DoctorDeps{StateDir: t.TempDir(), ProjectStore: store}))
		if got := findCheck(t, results, "host terminal"); got.detail != "unsupported (remote session)" {
			t.Fatalf("setup: host terminal detail = %q; want the unsupported line", got.detail)
		}
		if doctorUnhealthy(results) {
			t.Error("doctorUnhealthy = true; an unsupported host must never drive the exit code")
		}
	})

	t.Run("supported host does not rescue a real check failure", func(t *testing.T) {
		deps := withHealthyRuntime(t, &DoctorDeps{
			StateDir: t.TempDir(),
			LookPath: zmxMissing,
			Detector: fakeTerminalDetector{id: spawn.Identity{Name: "Ghostty", BundleID: "com.mitchellh.ghostty"}},
			Resolve: func(spawn.Identity) (spawn.Adapter, spawn.Resolution) {
				return nil, spawn.ResolutionNative
			},
		})
		results := diagnose(t, deps)
		if got := findCheck(t, results, "host terminal"); got.detail != "Ghostty (supported)" {
			t.Fatalf("setup: host terminal detail = %q; want the supported line", got.detail)
		}
		if !doctorUnhealthy(results) {
			t.Error("doctorUnhealthy = false; a real check failure must stay unhealthy even with a supported host")
		}
	})
}

func TestDoctorCheckOrder(t *testing.T) {
	results := diagnose(t, withHealthyRuntime(t, &DoctorDeps{StateDir: t.TempDir()}))
	want := []string{"zmx", "zmx dir", "sessions", "saver", "daemon", "state dir", "state's zmx dir", "sessions.json", "stale hooks", "stale projects", "pending resumes", "key config", "host terminal", "resume mode"}
	var got []string
	for _, r := range results {
		got = append(got, r.name)
	}
	if !slices.Equal(got, want) {
		t.Errorf("checks = %v; want %v", got, want)
	}
}

func seedProjectsJSON(t *testing.T, paths ...string) (*project.Store, string) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "projects.json")
	ps := []project.Project{}
	for i, p := range paths {
		ps = append(ps, project.Project{Path: p, Name: "proj" + strconv.Itoa(i)})
	}
	payload := struct {
		Projects []project.Project `json:"projects"`
	}{Projects: ps}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("marshal projects.json: %v", err)
	}
	if err := os.WriteFile(file, data, 0o600); err != nil {
		t.Fatalf("write projects.json: %v", err)
	}
	return project.NewStore(file), file
}

// staleDeps is a healthy install whose project store is the one given.
func staleDeps(t *testing.T, projectStore *project.Store) *DoctorDeps {
	t.Helper()
	dir := t.TempDir()
	seedValidSessionsJSON(t, dir, 1)
	return withHealthyRuntime(t, &DoctorDeps{StateDir: dir, ProjectStore: projectStore})
}

// runDoctorWith executes `portal doctor` with the given args against caller-wired
// seams, returning its stdout and stderr.
func runDoctorWith(t *testing.T, deps *DoctorDeps, args ...string) (*bytes.Buffer, *bytes.Buffer, error) {
	t.Helper()
	// terminals.json is read eagerly when the deps resolve; isolate it so
	// Execute never touches the developer's real config file.
	isolateTerminalsFile(t)
	withDoctorDeps(t, *deps)

	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	resetRootCmd()
	rootCmd.SetOut(outBuf)
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs(append([]string{"doctor"}, args...))
	err := rootCmd.Execute()
	return outBuf, errBuf, err
}

func TestDoctorStaleProjectsCheck(t *testing.T) {
	t.Run("gone dir fails, live dir retained", func(t *testing.T) {
		projectStore, _ := seedProjectsJSON(t, t.TempDir(), filepath.Join(t.TempDir(), "does-not-exist"))
		assertCheck(t, findCheck(t, diagnose(t, staleDeps(t, projectStore)), "stale projects"), checkFail, "1 stale project")
	})

	t.Run("multiple stale projects use the plural count copy", func(t *testing.T) {
		projectStore, _ := seedProjectsJSON(t, filepath.Join(t.TempDir(), "gone-a"), filepath.Join(t.TempDir(), "gone-b"))
		assertCheck(t, findCheck(t, diagnose(t, staleDeps(t, projectStore)), "stale projects"), checkFail, "2 stale projects")
	})

	t.Run("all live passes", func(t *testing.T) {
		projectStore, _ := seedProjectsJSON(t, t.TempDir())
		assertCheck(t, findCheck(t, diagnose(t, staleDeps(t, projectStore)), "stale projects"), checkPass, "no stale projects")
	})

	t.Run("count equals the shared predicate", func(t *testing.T) {
		projectStore, _ := seedProjectsJSON(t, t.TempDir(), filepath.Join(t.TempDir(), "gone-a"), filepath.Join(t.TempDir(), "gone-b"))
		stale, err := projectStore.StaleEntries()
		if err != nil || len(stale) != 2 {
			t.Fatalf("StaleEntries = (%d, %v); want 2 (fixture sanity)", len(stale), err)
		}
		got := findCheck(t, diagnose(t, staleDeps(t, projectStore)), "stale projects")
		assertCheck(t, got, checkFail, pluralCount(len(stale), "stale project", "stale projects"))
	})

	t.Run("it reads projects.json without changing it", func(t *testing.T) {
		projectStore, projectsPath := seedProjectsJSON(t, t.TempDir(), filepath.Join(t.TempDir(), "gone"))
		before := readFileBytes(t, projectsPath)
		diagnose(t, staleDeps(t, projectStore))
		if after := readFileBytes(t, projectsPath); !bytes.Equal(before, after) {
			t.Errorf("projects.json mutated by diagnosis\nbefore: %s\nafter:  %s", before, after)
		}
	})
}

func TestDoctorFix(t *testing.T) {
	t.Run("it prunes stale projects, then re-diagnoses clean", func(t *testing.T) {
		liveDir := t.TempDir()
		goneDir := filepath.Join(t.TempDir(), "gone")
		projectStore, projectsPath := seedProjectsJSON(t, liveDir, goneDir)

		outBuf, _, err := runDoctorWith(t, staleDeps(t, projectStore), "--fix")
		if err != nil {
			t.Fatalf("Execute err = %v; want nil (healthy post-repair)", err)
		}

		after := string(readFileBytes(t, projectsPath))
		if strings.Contains(after, goneDir) || !strings.Contains(after, liveDir) {
			t.Errorf("projects.json after the prune = %s; want %s gone and %s kept", after, goneDir, liveDir)
		}
		out := outBuf.String()
		if !strings.Contains(out, "\n  10 of 11 checks passed\nPruned stale project: proj1 ("+goneDir+")\nPortal doctor:\n") {
			t.Errorf("want the failing report, the prune, then the re-diagnosis:\n%s", out)
		}
		if !strings.HasSuffix(out, "✓ stale projects: no stale projects\n    pending resumes: 0 sessions waiting to resume\n"+defaultKeyConfigLine+"\n    host terminal: unsupported (remote session)\n    resume mode: "+installResumeModeOf(nil).String()+"\n  11 checks passed\n") {
			t.Errorf("post-repair report not clean:\n%s", out)
		}
	})

	t.Run("it exits on the re-diagnosis, which a repair cannot heal", func(t *testing.T) {
		deps := staleDeps(t, nil)
		deps.LookPath = zmxMissing
		outBuf, _, err := runDoctorWith(t, deps, "--fix")
		if err != ErrDoctorUnhealthy {
			t.Fatalf("Execute err = %v; want ErrDoctorUnhealthy with zmx still missing", err)
		}
		if n := strings.Count(outBuf.String(), "✗ zmx: "); n != 2 {
			t.Errorf("zmx failure lines = %d; want one per report:\n%s", n, outBuf.String())
		}
	})

	t.Run("it sweeps old logs without moving the exit code", func(t *testing.T) {
		projectStore, _ := seedProjectsJSON(t)
		deps := staleDeps(t, projectStore)
		// A rotated log dated well before today: only a sweep running against
		// deps.StateDir would delete it.
		staleLog := filepath.Join(deps.StateDir, "portal.log.2000-01-01")
		if err := os.WriteFile(staleLog, []byte("old\n"), 0o600); err != nil {
			t.Fatalf("seed stale rotated log: %v", err)
		}

		outBuf, _, err := runDoctorWith(t, deps, "--fix")
		if err != nil {
			t.Fatalf("Execute err = %v; want nil — a log sweep must never drive the exit code", err)
		}
		if _, statErr := os.Stat(staleLog); !os.IsNotExist(statErr) {
			t.Errorf("stale rotated log not swept (stat err = %v)", statErr)
		}
		if n := strings.Count(outBuf.String(), "\n  11 checks passed\n"); n != 2 {
			t.Errorf("all-passed summary count = %d; want 2:\n%s", n, outBuf.String())
		}
	})
}

// Doctor's own host-terminal detection runs through the driver bound on the
// zmx dir doctor judges, and only reads it: inside a session the dir doesn't
// hold, it asks for the session's leader, finds none, and leaves the dir as
// it was.
func TestDoctorDetectsThroughTheZmxDirItJudges(t *testing.T) {
	for _, args := range [][]string{nil, {"--fix"}} {
		t.Run(strings.Join(append([]string{"doctor"}, args...), " "), func(t *testing.T) {
			sink := logtest.Install(t)
			t.Setenv("ZMX_SESSION", "dev")
			deps := healthyDoctorDeps(t)
			deps.Detector, deps.Resolve = nil, nil
			t.Setenv("ZMX_DIR", deps.ZmxDir)

			_, _, _ = runDoctorWith(t, deps, args...)

			// --fix diagnoses twice, detecting each time.
			recs := sink.Records().Matching("spawn", "detection transient failure")
			if len(recs) == 0 {
				t.Fatal("doctor did not detect the host terminal")
			}
			for _, rec := range recs {
				if detail := rec.AttrString(t, "detail"); !strings.Contains(detail, `read the leader of session "dev"`) {
					t.Errorf("detection detail = %q, want the leader read of dev", detail)
				}
			}
			if entries, err := os.ReadDir(deps.ZmxDir); err != nil || len(entries) != 0 {
				t.Errorf("zmx dir holds %v, %v after doctor; want it untouched", entries, err)
			}
		})
	}
}

// A session of another zmx directory is none of the one doctor judges, so
// doctor detects as from outside a session, with no WARN for what is the
// environment's state.
func TestDoctorDetectsFromOutsideInASessionOfAnotherZmxDir(t *testing.T) {
	sink := logtest.Install(t)
	t.Setenv("ZMX_SESSION", "dev")
	deps := healthyDoctorDeps(t)
	deps.Detector, deps.Resolve = nil, nil

	_, _, _ = runDoctorWith(t, deps)

	if warns := sink.Records().Matching("spawn", "detection transient failure"); len(warns) != 0 {
		t.Errorf("doctor warned %+v for a session of another zmx dir", warns)
	}
	var routes []string
	for _, rec := range sink.Records() {
		if rec.HasAttr("detail") && strings.HasPrefix(rec.Msg, "detection") {
			routes = append(routes, rec.AttrString(t, "detail"))
		}
	}
	if len(routes) != 1 || !strings.HasPrefix(routes[0], "outside-session") {
		t.Errorf("detection details = %q, want one, from outside a session", routes)
	}
}
