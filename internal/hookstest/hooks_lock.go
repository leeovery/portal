package hookstest

import (
	"log/slog"
	"os"
	"sync"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/logtest"
)

// SidecarPath returns the path of the lock file a mutation takes beside the
// hooks.json at hooksPath. The suffix is the store's own, so a fixture naming
// the sidecar goes through here rather than restating it.
func SidecarPath(hooksPath string) string {
	return hooksPath + ".lock"
}

// CreateHooksSidecar pre-creates the lock file a mutation opens, so a fixture
// that denies writes to the directory still fails where it means to, and so a
// read under the fixture takes its shared lock rather than degrading on a
// missing sidecar the fixture never meant to model.
func CreateHooksSidecar(t *testing.T, hooksPath string) {
	t.Helper()
	if err := os.WriteFile(SidecarPath(hooksPath), nil, 0o600); err != nil {
		t.Fatalf("hookstest.CreateHooksSidecar: create sidecar lock: %v", err)
	}
}

// HoldHooksSidecar takes the sidecar exclusively from an independent open file
// description, modelling a writer in another process: every read taken while it
// is held must degrade rather than fail, and every mutation must time out at the
// bound and write nothing. The returned release lets a caller free the lock
// mid-test and retry the operation that could not take it; it is idempotent and
// also runs on cleanup.
func HoldHooksSidecar(t *testing.T, hooksPath string) func() {
	t.Helper()
	f := openSidecar(t, hooksPath)
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatalf("hookstest.HoldHooksSidecar: flock sidecar: %v", err)
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
			_ = f.Close()
		})
	}
	t.Cleanup(release)
	return release
}

// HoldHooksSidecarShared takes the sidecar LOCK_SH from an independent open file
// description, modelling another reader. An exclusive acquire blocks against it
// to the bound; a shared one is granted immediately — which is what makes it the
// discriminator between the two read modes.
func HoldHooksSidecarShared(t *testing.T, hooksPath string) {
	t.Helper()
	f := openSidecar(t, hooksPath)
	if err := unix.Flock(int(f.Fd()), unix.LOCK_SH|unix.LOCK_NB); err != nil {
		t.Fatalf("hookstest.HoldHooksSidecarShared: flock sidecar shared: %v", err)
	}
	t.Cleanup(func() {
		_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
		_ = f.Close()
	})
}

// AssertSidecarFree proves the operation under test released its hold: an
// exclusive non-blocking acquire from a fresh open file description succeeds
// only when no other fd holds the sidecar. It creates the sidecar if it is
// absent, so it can never stand in for an assertion that none exists.
func AssertSidecarFree(t *testing.T, hooksPath string) {
	t.Helper()
	f := openSidecar(t, hooksPath)
	defer func() { _ = f.Close() }()
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Errorf("hookstest.AssertSidecarFree: sidecar is still held: %v", err)
		return
	}
	_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
}

// AssertSidecarHeld proves the operation under test holds the sidecar
// exclusively: a shared non-blocking acquire from a fresh open file
// description fails only while another fd holds it exclusively.
func AssertSidecarHeld(t harnesstest.TestingT, hooksPath string) {
	t.Helper()
	f := openSidecar(t, hooksPath)
	defer func() { _ = f.Close() }()
	if err := unix.Flock(int(f.Fd()), unix.LOCK_SH|unix.LOCK_NB); err == nil {
		_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
		t.Errorf("hookstest.AssertSidecarHeld: the sidecar is not held exclusively")
	}
}

func openSidecar(t harnesstest.TestingT, hooksPath string) *os.File {
	t.Helper()
	f, err := os.OpenFile(SidecarPath(hooksPath), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatalf("open sidecar: %v", err)
	}
	return f
}

// UnlockedRecords returns the degradation breadcrumbs the sink captured, so a
// test can assert a read did not degrade as well as that it did.
func UnlockedRecords(t *testing.T, sink *logtest.Sink) logtest.Records {
	t.Helper()
	return sink.Records().WithMessage("load-unlocked")
}

// AssertLockWarn pins the whole of the single line a mutation that could not
// take the sidecar leaves: exactly one record at or above WARN, carrying the
// operation's own op, the key it could not write, the caller and the lock
// error — and neither error_class nor value, the two attrs a write phase adds,
// whose absence is what says no write was ever attempted.
func AssertLockWarn(t harnesstest.TestingT, sink *logtest.Sink, op, key, via string) {
	t.Helper()
	rec := sink.Records().AtOrAboveLevel(slog.LevelWarn).Only(t, "record at or above WARN")
	logtest.AssertRecord(t, rec, logtest.RecordWant{
		Level:     slog.LevelWarn,
		Msg:       op,
		Component: "hooks",
		Op:        op,
		Via:       via,
	})
	if got := rec.AttrString(t, "hook_key"); got != key {
		t.Errorf("hook_key = %q, want %q", got, key)
	}
	if rec.HasAttr("error_class") {
		t.Errorf("lock WARN carries error_class — no write phase ran: %+v", rec.Attrs)
	}
	if rec.HasAttr("value") {
		t.Errorf("lock WARN carries value — the file was never opened: %+v", rec.Attrs)
	}
	if err := rec.ErrorAttr(t, "error"); err == nil || err.Error() == "" {
		t.Errorf("error attr is empty — the lock failure must be carried: %+v", rec.Attrs)
	}
}

// AssertDegradedRead pins the whole shape of a degraded read's single
// breadcrumb: exactly one record, DEBUG, op=load-unlocked, via naming the
// caller, and the lock error carried.
func AssertDegradedRead(t *testing.T, sink *logtest.Sink, wantVia string) {
	t.Helper()
	r := UnlockedRecords(t, sink).Only(t, "load-unlocked record")
	if r.Level != slog.LevelDebug {
		t.Errorf("level = %v, want DEBUG", r.Level)
	}
	if op := r.AttrString(t, "op"); op != "load-unlocked" {
		t.Errorf("op = %q, want load-unlocked", op)
	}
	if via := r.AttrString(t, "via"); via != wantVia {
		t.Errorf("via = %q, want %q", via, wantVia)
	}
	if errAttr := r.AttrString(t, "error"); errAttr == "" {
		t.Error("error attr is empty — the lock failure must be carried")
	}
}
