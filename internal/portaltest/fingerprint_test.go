// White-box tests for the fingerprint-diff backstop.
//
// These tests intentionally live in `package portaltest` (not
// `_test`) so they can drive the unexported reportStateDirDelta
// surface (alongside the exported SnapshotStateDir helper, which
// is also exercised by out-of-package integration tests). The
// diff logic is exercised against
// a controlled t.TempDir() root — never the developer's real
// state directory — so a bug in the backstop cannot itself corrupt
// the host install.
//
// The errorReporter seam (a func type compatible with t.Errorf)
// lets tests record violations without polluting the host
// *testing.T with intentional failures. A real-world cleanup hands
// t.Errorf to reportStateDirDelta; meta-tests hand it a recorder.

package portaltest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// recorder collects all formatted reports produced by
// reportStateDirDelta. Each Errorf-style call becomes one entry —
// the count and content together pin the contract.
type recorder struct {
	msgs []string
}

func (r *recorder) report(format string, args ...any) {
	r.msgs = append(r.msgs, fmt.Sprintf(format, args...))
}

// writeFile is a tiny test helper that fails fast on errors so
// individual cases stay readable.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// hasDelta returns true if msgs contains an entry citing path and
// deltaType. Tests prefer this over substring searches because the
// canonical message format embeds both fields verbatim.
func hasDelta(msgs []string, path, deltaType string) bool {
	want := "portaltest backstop: developer state dir mutated at " + path + ": " + deltaType
	for _, m := range msgs {
		if m == want {
			return true
		}
	}
	return false
}

// containsAny is a debug helper used by failure messages to
// summarise what the recorder actually saw.
func containsAny(msgs []string, fragment string) bool {
	for _, m := range msgs {
		if strings.Contains(m, fragment) {
			return true
		}
	}
	return false
}

// --- SnapshotStateDir contract tests -----------------------------

func TestSnapshotStateDir_NonexistentRoot_ReturnsEmptyMap(t *testing.T) {
	root := filepath.Join(t.TempDir(), "absent")

	snap, err := SnapshotStateDir(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snap) != 0 {
		t.Errorf("expected empty map, got %d entries", len(snap))
	}
}

func TestSnapshotStateDir_RecordsRegularFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "sessions.json"), "alpha")

	snap, err := SnapshotStateDir(root)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	fp, ok := snap["sessions.json"]
	if !ok {
		t.Fatalf("expected sessions.json entry, got keys: %v", keys(snap))
	}
	if fp.Size != 5 {
		t.Errorf("size = %d, want 5", fp.Size)
	}
	if !fp.Hashed {
		t.Errorf("expected hashed=true for small regular file")
	}
}

func TestSnapshotStateDir_RecordsSymlinkViaLstat(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	writeFile(t, target, "x")
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	snap, err := SnapshotStateDir(root)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	fp, ok := snap["link"]
	if !ok {
		t.Fatalf("expected link entry; keys: %v", keys(snap))
	}
	if !fp.IsSymlink {
		t.Errorf("link not recorded as symlink")
	}
	if fp.SymlinkTarget != target {
		t.Errorf("symlinkTarget = %q, want %q", fp.SymlinkTarget, target)
	}
	// hashed must remain false for symlinks — symlink hashing would
	// follow the target via os.ReadFile, defeating lstat semantics.
	if fp.Hashed {
		t.Errorf("symlink should not be hashed")
	}
}

func TestSnapshotStateDir_LargeFile_SkipsHash(t *testing.T) {
	root := t.TempDir()
	big := make([]byte, hashSizeCap+1)
	for i := range big {
		big[i] = 'a'
	}
	path := filepath.Join(root, "big.bin")
	if err := os.WriteFile(path, big, 0o600); err != nil {
		t.Fatalf("write big: %v", err)
	}
	snap, err := SnapshotStateDir(root)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	fp := snap["big.bin"]
	if fp.Hashed {
		t.Errorf("expected hashed=false for file >hashSizeCap")
	}
	if fp.Size != int64(hashSizeCap+1) {
		t.Errorf("size = %d, want %d", fp.Size, hashSizeCap+1)
	}
}

// TestSnapshotStateDir_DetectsModifiedBinFile is the spec-mandated
// meta-test guarding the snapshot-diff implementation: write a .bin
// file, snapshot, mutate the file's content with size preserved and
// mtime force-reset to the pre-snapshot value, snapshot again, and
// assert the two Fingerprint maps differ on the .bin path. Without
// this guard a regression where SnapshotStateDir silently returned
// identical maps for divergent content would slip past the
// kill-barrier-escalation no-final-flush integration test (which
// compares two snapshots taken across SIGKILL and would silently
// green-pass if the helper itself were a no-op).
//
// The test exercises the content-hash channel in isolation by
// pinning size and mtime to the pre-snapshot values; the hash field
// is the sole remaining delta signal and must catch it.
func TestSnapshotStateDir_DetectsModifiedBinFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "scrollback", "pane__0.0.bin")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir scrollback: %v", err)
	}
	if err := os.WriteFile(path, []byte("alpha"), 0o600); err != nil {
		t.Fatalf("write .bin: %v", err)
	}

	pre, err := SnapshotStateDir(root)
	if err != nil {
		t.Fatalf("pre snapshot: %v", err)
	}
	prePath := filepath.Join("scrollback", "pane__0.0.bin")
	preFP, ok := pre[prePath]
	if !ok {
		t.Fatalf("expected pre to contain %s; keys=%v", prePath, keys(pre))
	}

	// Mutate content with size preserved. Then pin mtime back to the
	// pre value so the test isolates the hash channel: any equality
	// the snapshot still reports would have to come from a no-op
	// content read.
	if err := os.WriteFile(path, []byte("betaX"), 0o600); err != nil {
		t.Fatalf("rewrite .bin: %v", err)
	}
	resetTimes(t, path, preFP.MtimeNanos)

	post, err := SnapshotStateDir(root)
	if err != nil {
		t.Fatalf("post snapshot: %v", err)
	}
	postFP, ok := post[prePath]
	if !ok {
		t.Fatalf("expected post to contain %s; keys=%v", prePath, keys(post))
	}

	if preFP.Sha256 == postFP.Sha256 {
		t.Fatalf("SnapshotStateDir returned identical Sha256 across content mutation\n"+
			"  pre.Size=%d post.Size=%d\n"+
			"  pre.Hashed=%v post.Hashed=%v\n"+
			"  pre.Sha256=%x\n"+
			"  post.Sha256=%x",
			preFP.Size, postFP.Size,
			preFP.Hashed, postFP.Hashed,
			preFP.Sha256, postFP.Sha256)
	}
	if !preFP.Hashed || !postFP.Hashed {
		t.Errorf("expected both fingerprints Hashed=true (file ≤ 1 MiB); pre=%v post=%v",
			preFP.Hashed, postFP.Hashed)
	}
}

// --- reportStateDirDelta contract tests --------------------------

func TestReportStateDirDelta_NoChange_PassesCleanup(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "sessions.json"), "alpha")

	pre, err := SnapshotStateDir(root)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	rec := &recorder{}
	reportStateDirDelta(rec.report, root, pre)

	if len(rec.msgs) != 0 {
		t.Errorf("expected no deltas, got: %v", rec.msgs)
	}
}

func TestReportStateDirDelta_FileCreated_FailsCleanup(t *testing.T) {
	root := t.TempDir()
	pre, _ := SnapshotStateDir(root)

	// Mutate AFTER snapshot: simulate a stray test that bypassed
	// the env override and wrote into the dev state dir.
	writeFile(t, filepath.Join(root, "leaked.json"), "leak")

	rec := &recorder{}
	reportStateDirDelta(rec.report, root, pre)

	if !hasDelta(rec.msgs, "leaked.json", "created") {
		t.Errorf("expected 'created' delta for leaked.json; got: %v", rec.msgs)
	}
}

func TestReportStateDirDelta_FileDeleted_FailsCleanup(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sessions.json")
	writeFile(t, path, "alpha")
	pre, _ := SnapshotStateDir(root)

	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}

	rec := &recorder{}
	reportStateDirDelta(rec.report, root, pre)
	if !hasDelta(rec.msgs, "sessions.json", "deleted") {
		t.Errorf("expected 'deleted' delta; got: %v", rec.msgs)
	}
}

func TestReportStateDirDelta_SizeChanged_FailsCleanup(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sessions.json")
	writeFile(t, path, "alpha")
	pre, _ := SnapshotStateDir(root)

	// Rewrite with a larger payload; mtime will also bump but the
	// size delta must be reported independently.
	writeFile(t, path, "alpha-extended")

	rec := &recorder{}
	reportStateDirDelta(rec.report, root, pre)

	if !hasDelta(rec.msgs, "sessions.json", "size-changed") {
		t.Errorf("expected 'size-changed' delta; got: %v", rec.msgs)
	}
}

func TestReportStateDirDelta_ContentChanged_FailsCleanup(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sessions.json")
	writeFile(t, path, "alpha")
	pre, _ := SnapshotStateDir(root)

	// Same size, different content — only the hash catches this.
	// We must also clamp mtime/ctime back to the pre-value so the
	// only delta is content; otherwise mtime-changed dominates.
	writeFile(t, path, "betaX")
	preMtime, preCtime := lookupMtimes(t, root, "sessions.json", pre)
	resetTimes(t, path, preMtime)
	// Re-snapshot pre so its mtime/ctime reflect the reset baseline
	// (the test cares about the content channel in isolation).
	pre2, _ := SnapshotStateDir(root)
	// Now mutate content again with size + mtime preserved.
	if err := os.WriteFile(path, []byte("gamma"), 0o600); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	resetTimes(t, path, preMtime)
	_ = preCtime // ctime cannot be force-set portably; size+mtime
	// pinning is sufficient to surface the hash channel.

	rec := &recorder{}
	reportStateDirDelta(rec.report, root, pre2)

	if !hasDelta(rec.msgs, "sessions.json", "content-changed") {
		t.Errorf("expected 'content-changed' delta; got: %v", rec.msgs)
	}
}

func TestReportStateDirDelta_MtimeBumped_FailsCleanup(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sessions.json")
	writeFile(t, path, "alpha")
	pre, _ := SnapshotStateDir(root)

	// Bump mtime without changing size/content.
	future := time.Now().Add(2 * time.Hour)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	rec := &recorder{}
	reportStateDirDelta(rec.report, root, pre)
	if !hasDelta(rec.msgs, "sessions.json", "mtime-changed") {
		t.Errorf("expected 'mtime-changed' delta; got: %v", rec.msgs)
	}
}

func TestReportStateDirDelta_BecameSymlink_FailsCleanup(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sessions.json")
	writeFile(t, path, "alpha")
	pre, _ := SnapshotStateDir(root)

	// Replace the regular file with a symlink pointing elsewhere.
	target := filepath.Join(root, "other.txt")
	writeFile(t, target, "x")
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	rec := &recorder{}
	reportStateDirDelta(rec.report, root, pre)
	if !hasDelta(rec.msgs, "sessions.json", "became-symlink") {
		t.Errorf("expected 'became-symlink' delta; got: %v", rec.msgs)
	}
}

func TestReportStateDirDelta_SymlinkTargetChanged_FailsCleanup(t *testing.T) {
	root := t.TempDir()
	targetA := filepath.Join(root, "a.txt")
	targetB := filepath.Join(root, "b.txt")
	writeFile(t, targetA, "a")
	writeFile(t, targetB, "b")
	link := filepath.Join(root, "link")
	if err := os.Symlink(targetA, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	pre, _ := SnapshotStateDir(root)

	if err := os.Remove(link); err != nil {
		t.Fatalf("remove link: %v", err)
	}
	if err := os.Symlink(targetB, link); err != nil {
		t.Fatalf("re-symlink: %v", err)
	}

	rec := &recorder{}
	reportStateDirDelta(rec.report, root, pre)
	if !hasDelta(rec.msgs, "link", "symlink-target-changed") {
		t.Errorf("expected 'symlink-target-changed' delta; got: %v", rec.msgs)
	}
}

func TestReportStateDirDelta_LargeFile_DetectsSizeWithoutHash(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "big.bin")
	big := make([]byte, hashSizeCap+1)
	if err := os.WriteFile(path, big, 0o600); err != nil {
		t.Fatalf("write big: %v", err)
	}
	pre, _ := SnapshotStateDir(root)
	if pre["big.bin"].Hashed {
		t.Fatalf("large file should not be hashed in pre-snapshot")
	}

	// Grow the file by one byte — size channel catches it without
	// any hash to consult.
	bigger := make([]byte, hashSizeCap+2)
	if err := os.WriteFile(path, bigger, 0o600); err != nil {
		t.Fatalf("rewrite big: %v", err)
	}

	rec := &recorder{}
	reportStateDirDelta(rec.report, root, pre)
	if !hasDelta(rec.msgs, "big.bin", "size-changed") {
		t.Errorf("expected 'size-changed' delta on large file; got: %v", rec.msgs)
	}
}

func TestReportStateDirDelta_ReportsAllDeltas_NotJustFirst(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "keep.json"), "k")
	writeFile(t, filepath.Join(root, "doomed.json"), "d")
	pre, _ := SnapshotStateDir(root)

	// Three independent deltas at three different paths.
	if err := os.Remove(filepath.Join(root, "doomed.json")); err != nil {
		t.Fatalf("remove doomed: %v", err)
	}
	writeFile(t, filepath.Join(root, "fresh.json"), "f")
	writeFile(t, filepath.Join(root, "keep.json"), "k-extended")

	rec := &recorder{}
	reportStateDirDelta(rec.report, root, pre)

	if !hasDelta(rec.msgs, "doomed.json", "deleted") {
		t.Errorf("missing 'deleted' for doomed.json; got: %v", rec.msgs)
	}
	if !hasDelta(rec.msgs, "fresh.json", "created") {
		t.Errorf("missing 'created' for fresh.json; got: %v", rec.msgs)
	}
	if !containsAny(rec.msgs, "keep.json") {
		t.Errorf("missing some delta for keep.json; got: %v", rec.msgs)
	}
	if len(rec.msgs) < 3 {
		t.Errorf("expected >=3 deltas reported, got %d: %v", len(rec.msgs), rec.msgs)
	}
}

func TestReportStateDirDelta_WalksOnlyRoot_NotSiblings(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "state")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	// Sibling file outside root — must be invisible to snapshot.
	writeFile(t, filepath.Join(parent, "projects.json"), "p")

	pre, _ := SnapshotStateDir(root)

	// Mutate sibling AFTER snapshot. The backstop must not notice.
	writeFile(t, filepath.Join(parent, "projects.json"), "p-changed-and-bigger")

	rec := &recorder{}
	reportStateDirDelta(rec.report, root, pre)

	if len(rec.msgs) != 0 {
		t.Errorf("expected no deltas (siblings out of scope); got: %v", rec.msgs)
	}
}

func TestReportStateDirDelta_NonexistentRoot_EmptyPreSnapshot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "absent")

	pre, err := SnapshotStateDir(root)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(pre) != 0 {
		t.Fatalf("expected empty pre-snapshot; got %d entries", len(pre))
	}

	// Create the root post-snapshot with a file inside; this must
	// surface as a "created" delta.
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	writeFile(t, filepath.Join(root, "late.json"), "l")

	rec := &recorder{}
	reportStateDirDelta(rec.report, root, pre)

	if !hasDelta(rec.msgs, "late.json", "created") {
		t.Errorf("expected 'created' for late.json; got: %v", rec.msgs)
	}
}

// --- resolveDevStateDir contract tests ---------------------------

func TestResolveDevStateDir_UsesXDGConfigHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg-fake")
	t.Setenv("HOME", "/tmp/home-fake")

	got := resolveDevStateDir()
	want := filepath.Join("/tmp/xdg-fake", "portal", "state")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveDevStateDir_FallsBackToHomeConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/tmp/home-fake")

	got := resolveDevStateDir()
	want := filepath.Join("/tmp/home-fake", ".config", "portal", "state")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// --- installBackstopCleanup wiring meta-test ---------------------

// fakeBackstopT implements backstopT for meta-testing. It captures
// the cleanup func registered by installBackstopCleanup and the
// Errorf calls that fire when the cleanup runs. The host *testing.T
// stays clean — its assertions only inspect the captured state.
type fakeBackstopT struct {
	cleanups []func()
	errorfs  []string
}

func (f *fakeBackstopT) Cleanup(fn func()) {
	f.cleanups = append(f.cleanups, fn)
}

func (f *fakeBackstopT) Errorf(format string, args ...any) {
	f.errorfs = append(f.errorfs, fmt.Sprintf(format, args...))
}

// runCleanups simulates the *testing.T post-test hook, executing
// every registered cleanup in LIFO order (mirrors real testing.T).
func (f *fakeBackstopT) runCleanups() {
	for i := len(f.cleanups) - 1; i >= 0; i-- {
		f.cleanups[i]()
	}
}

// TestBackstopCleanupFiresOnExternalMutation is the spec-mandated
// meta-test for the fingerprint backstop. It wires
// installBackstopCleanup through a fake backstopT, simulates a
// stray test writing to the dev state dir AFTER snapshot, runs the
// cleanup, and asserts t.Errorf was called citing the leaked path
// and "created" delta type.
func TestBackstopCleanupFiresOnExternalMutation(t *testing.T) {
	devStateDir := t.TempDir() // controlled stand-in for ~/.config/portal/state
	pre, err := SnapshotStateDir(devStateDir)
	if err != nil {
		t.Fatalf("pre-snapshot: %v", err)
	}

	fake := &fakeBackstopT{}
	installBackstopCleanup(fake, devStateDir, pre)

	// Simulate the failure mode: a test that bypassed the env
	// override and wrote directly to the dev state dir.
	leakPath := filepath.Join(devStateDir, "leaked.json")
	if err := os.WriteFile(leakPath, []byte("leak"), 0o600); err != nil {
		t.Fatalf("write leak: %v", err)
	}

	fake.runCleanups()

	if !hasDelta(fake.errorfs, "leaked.json", "created") {
		t.Errorf("expected backstop to Errorf about leaked.json:created; got: %v", fake.errorfs)
	}
}

// TestBackstopCleanupSilentOnClean asserts the wiring stays
// silent when the dev state dir is unchanged — i.e., the cleanup
// only escalates real deltas, never false positives.
func TestBackstopCleanupSilentOnClean(t *testing.T) {
	devStateDir := t.TempDir()
	pre, _ := SnapshotStateDir(devStateDir)

	fake := &fakeBackstopT{}
	installBackstopCleanup(fake, devStateDir, pre)

	fake.runCleanups()

	if len(fake.errorfs) != 0 {
		t.Errorf("expected zero Errorf calls on clean exit; got: %v", fake.errorfs)
	}
}

// --- helpers -----------------------------------------------------

func keys(m map[string]Fingerprint) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// lookupMtimes returns the pre-recorded mtime/ctime for rel.
func lookupMtimes(t *testing.T, root, rel string, snap map[string]Fingerprint) (mtime int64, ctime int64) {
	t.Helper()
	fp, ok := snap[rel]
	if !ok {
		t.Fatalf("snap missing %s", rel)
	}
	return fp.MtimeNanos, fp.CtimeNanos
}

// resetTimes sets atime+mtime on path to the supplied nano value.
// Used by the content-change test to pin mtime so the hash
// channel is the sole reported delta.
func resetTimes(t *testing.T, path string, nanos int64) {
	t.Helper()
	ts := time.Unix(0, nanos)
	if err := os.Chtimes(path, ts, ts); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
}
