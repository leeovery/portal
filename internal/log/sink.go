package log

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// dateLayout is the calendar-day key format. Rotation keys on the date STRING
// (not elapsed duration) so DST 23/25-hour days and timezone changes are handled
// by construction: a repeated date appends, a forward jump opens a new file.
const dateLayout = "2006-01-02"

// nowFunc is the injectable clock seam. Production uses time.Now; date-change
// tests advance it deterministically. It is a package var rather than a sink
// field so tests can drive every sink in the package through one swap and the
// production path pays no per-sink indirection.
var nowFunc = time.Now

// openSegmentFunc is the test-only seam over os.OpenFile used by the size-cap
// overflow open. Forcing a genuine EEXIST on a specific segment N is otherwise
// non-deterministic (it requires a concurrent writer to win the race between our
// glob-based discovery and our O_EXCL open). Tests swap it to inject os.ErrExist
// on chosen N values and restore via t.Cleanup; production always uses
// os.OpenFile. It mirrors the existing symlinkFunc / chmodFunc seam convention.
var openSegmentFunc = os.OpenFile

// rotatingSink is the date-aware, inode-identity-checked log writer that owns the
// per-Handle fd-management critical section. It is the io.Writer the textHandler
// renders into: the handler builds one line, then a single sink.Write performs
// the fd-selection (reuse / reopen) and one unbuffered write(2) under the sink
// mutex, so concurrent Handle calls serialise the reopen + write.
//
// The writer is deliberately UNBUFFERED — every record is its own write(2) to the
// *os.File (no bufio). This sink SURFACES open/write errors to the handler via
// Write's error; the best-effort policy (swallow + single stderr fallback, never
// propagate to the slog caller) lives in textHandler.Handle (Task 2-7), which
// consumes that error. Keeping the sink honest lets probe() detect a
// configuration failure at Init time while the per-Handle path stays best-effort.
type rotatingSink struct {
	// stateDir is the directory the day files and the portal.log symlink live in.
	// Stored as a plain string; internal/log never imports internal/state.
	stateDir string

	mu sync.Mutex // guards the fd-management + write critical section.

	// file is the currently-open day file, or nil before the first Write.
	file *os.File
	// date is the calendar key the open file was opened for (the <date> in
	// portal.log.<date>). Empty before the first open.
	date string
	// dev / ino are the file's identity captured via fstat at open time, compared
	// against the live symlink target on every Write to detect a mid-day swap.
	dev uint64
	ino uint64

	// rotateSize is the size-cap safety valve in bytes, resolved ONCE at
	// construction (production: resolveRotateSize(os.Getenv("PORTAL_LOG_ROTATE_SIZE"))
	// passed in via init.go). It is NEVER re-read per Write. When the open file's
	// current size plus the next record's length would reach this cap, Write rolls
	// to a fresh same-day portal.log.<today>.<N> segment (see rotateSameDay).
	rotateSize int64

	// dayRoll is the day-roll sweep seam, fired ONLY when the calendar date
	// advances (NOT on a same-day inode-mismatch reopen), AFTER the new day's
	// file is open and the symlink is swung (so the sweeps observe today's file
	// as already opened). The date advance is RECORDED under mu (pendingDayRoll)
	// by reopen and the callback runs from Write only after the mutex is
	// released: the sweeps log through rotateLogger, whose records re-enter this
	// sink's Write, so firing them under mu self-deadlocks — the 2026-07-06
	// incident that froze the live daemon at its first midnight with a
	// retention-deletion candidate. today is the calendar key the roll opened,
	// captured at reopen time. Production wiring (newRotatingSink) composes the
	// day-roll sweeps here; tests override it to count fires or inject their own
	// sweep body.
	dayRoll func(today string)

	// pendingDayRoll is the calendar key of a date advance whose day-roll sweeps
	// have not yet fired, or "" when none is pending. Set under mu by reopen;
	// drained by fireDayRoll outside mu. probe deliberately leaves it queued —
	// probe runs before Init installs the configured handler, so firing there
	// would route the sweep breadcrumbs to the pre-Init stderr default and lose
	// them from portal.log; the first Write through the configured handler
	// (process: start) fires it instead.
	pendingDayRoll string
}

// newRotatingSink constructs a sink rooted at stateDir with rotateSize as the
// resolved size-cap (bytes). No file is opened until the first Write so a process
// that never logs touches no disk. The cap is resolved ONCE by the caller
// (init.go via resolveRotateSize) and stored on the sink — Write never re-reads
// the env.
//
// The dayRoll seam is wired to the day-roll sweep chain: on a real calendar-day
// roll it seals all past-day files (Task 2-5, Invariant 1) AND runs the
// single-winner retention sweep (Task 2-8) that bounds rotated history and emits
// per-deletion breadcrumbs. The closure receives today (captured at reopen time)
// and runs outside the sink mutex, after the record that triggered the roll is
// written, so both sweeps observe today's file as already opened — the deletion
// INFO lines and the retention WARN land in today's file through the normal
// logging path, never in the file being aged out. seal-then-retention ordering
// is arbitrary (both key off today); the seam stays composable.
func newRotatingSink(stateDir string, rotateSize int64) *rotatingSink {
	s := &rotatingSink{stateDir: stateDir, rotateSize: rotateSize}
	s.dayRoll = func(today string) {
		sealPastDayFiles(s.stateDir, today)
		runRetentionSweep(s.stateDir, today, true)
	}
	return s
}

// Write runs the per-Handle fd-selection step then performs one unbuffered
// write(2) of p to the now-current day file, all under the sink mutex so a
// concurrent Write cannot observe a half-swapped fd. A date advance detected
// inside the critical section is only RECORDED there; the day-roll sweeps fire
// via fireDayRoll AFTER the mutex is released, because their own log records
// re-enter this Write (see dayRoll).
func (s *rotatingSink) Write(p []byte) (int, error) {
	n, err := s.lockedWrite(p)
	s.fireDayRoll()
	return n, err
}

// lockedWrite is Write's critical section: fd selection (reuse / reopen), the
// size-cap check, and the single unbuffered write(2), under s.mu.
func (s *rotatingSink) lockedWrite(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureCurrent(); err != nil {
		return 0, err
	}

	// Size-cap safety valve (rotation rule step 3): the fd is now current. If the
	// open file's size plus this record would reach the resolved cap, roll to a
	// fresh same-day portal.log.<today>.<N> segment BEFORE writing so a runaway
	// cannot fill the disk. Unlike the day roll, the prior segment is NOT sealed —
	// a peer may still hold an open O_APPEND fd on it (see rotateSameDay).
	if err := s.rotateIfOverCap(p); err != nil {
		return 0, err
	}

	// Unbuffered writer is a locked constraint (spec § Defensive invariants —
	// Flush): the serialized record is written directly to the *os.File O_APPEND
	// fd with NO bufio wrapper, so the bytes are already in the kernel by the time
	// the originating Info(...) returns. os.Exit / syscall.Exec do not discard
	// kernel buffers, so a marker survives for a later reader without any
	// Sync()/flush. Do NOT introduce a bufio.Writer here.
	return s.file.Write(p)
}

// fireDayRoll drains a pending day roll and runs the sweep callback OUTSIDE the
// sink mutex. Draining under mu guarantees exactly one firing per recorded date
// advance even under concurrent Writes (whichever caller swaps the key out runs
// the sweeps); running the callback after unlock is the deadlock fix — the
// sweeps' rotateLogger records re-enter Write, which must be able to take mu
// (the 2026-07-06 daemon midnight freeze). A re-entrant record's own
// fireDayRoll finds pendingDayRoll empty and no-ops, so the recursion
// terminates after one level.
func (s *rotatingSink) fireDayRoll() {
	s.mu.Lock()
	today := s.pendingDayRoll
	s.pendingDayRoll = ""
	s.mu.Unlock()

	if today == "" || s.dayRoll == nil {
		return
	}
	s.dayRoll(today)
}

// rotateIfOverCap performs the size-cap check (rotation rule step 3) against the
// open fd: if current_size + len(p) >= s.rotateSize it rotates to the next free
// same-day segment via rotateSameDay. It must be called with s.mu held and after
// ensureCurrent has made s.file current. A size that cannot be stat'd is treated
// as "do not rotate" — the next Write retries the check; a transient stat error
// must not corrupt the write path.
func (s *rotatingSink) rotateIfOverCap(p []byte) error {
	info, err := s.file.Stat()
	if err != nil {
		return nil // Cannot determine current size: skip the cap check this Write.
	}
	if info.Size()+int64(len(p)) < s.rotateSize {
		return nil // Below cap (the steady-state path): no rotation.
	}
	return s.rotateSameDay(nowFunc().Format(dateLayout))
}

// rotateSameDay rolls the open fd onto a fresh same-day overflow segment
// portal.log.<today>.<N> (rotation rule step 3). It discovers the next free N as
// (max existing .N for today) + 1, opens it O_CREAT|O_EXCL (retrying N+1 on
// EEXIST so a racing writer or a stale gap is absorbed), swings the portal.log
// symlink to it, and swaps s.file/date/dev/ino onto the new segment (closing the
// prior fd in THIS process). It must be called with s.mu held.
//
// The previous segment is DELIBERATELY NOT chmod'd: it is a same-day file, a peer
// process may still hold an open O_APPEND fd on it (chmod does not evict an
// already-open writer on Unix), and it remains part of today's active write
// surface. Same-day segments are sealed only on the day roll (sealPastDayFiles,
// Task 2-5), which chmods all of yesterday's segments at once. A peer that did
// not observe this rotation simply keeps appending to the prior segment — today's
// writes split across two readable same-day files with the symlink pointing at
// the newest. That is acceptable: the size cap is a disk-fill valve, not a
// correctness boundary.
func (s *rotatingSink) rotateSameDay(today string) error {
	f, n, err := s.claimNextSegment(today)
	if err != nil {
		return err
	}

	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	dev, ino, _ := devIno(info)

	// Swing the symlink to the new segment (bare relative basename — same dir).
	// Best-effort, mirroring reopen: a swing failure leaves the prior symlink in
	// place and writes continue to the freshly-opened fd. The next Write's inode
	// check then forces a benign retry.
	_ = swingSymlink(s.stateDir, filepath.Base(daySegmentFile(s.stateDir, today, n)))

	if s.file != nil {
		_ = s.file.Close() // Close THIS process's prior-segment fd; do NOT chmod it.
	}
	s.file = f
	s.date = today
	s.dev = dev
	s.ino = ino
	return nil
}

// claimNextSegment opens the next free same-day overflow segment for today via
// O_CREAT|O_EXCL, starting at nextSegmentN and retrying N+1 on EEXIST until a
// free N is won (another writer beat us to this N, or a stale gap left a claimed
// N below the discovered max). It returns the open file and the claimed N.
func (s *rotatingSink) claimNextSegment(today string) (*os.File, int, error) {
	for n := s.nextSegmentN(today); ; n++ {
		f, err := openSegmentFunc(daySegmentFile(s.stateDir, today, n), os.O_CREATE|os.O_EXCL|os.O_APPEND|os.O_WRONLY, logFileMode)
		if errors.Is(err, os.ErrExist) {
			continue // This N is taken; try N+1.
		}
		if err != nil {
			return nil, 0, err
		}
		return f, n, nil
	}
}

// nextSegmentN returns the next free same-day overflow segment number for today:
// (max existing portal.log.<today>.<N>) + 1, or 1 when no .N segments exist. A
// gap (.1 and .3 present) yields max+1 (.4), not the gap (.2) — monotonic past
// the highest existing N so a claimed-then-vanished segment is never reused. A
// Glob error (only on a malformed pattern, which this never is) yields 1.
func (s *rotatingSink) nextSegmentN(today string) int {
	matches, err := filepath.Glob(filepath.Join(s.stateDir, portalLogName+"."+today+".*"))
	if err != nil {
		return 1
	}

	max := 0
	prefix := portalLogName + "." + today + "."
	for _, path := range matches {
		rest, found := strings.CutPrefix(filepath.Base(path), prefix)
		if !found {
			continue
		}
		n, err := strconv.Atoi(rest)
		if err != nil || n <= 0 {
			continue // Not a numeric .N segment (e.g. a future non-log sibling).
		}
		if n > max {
			max = n
		}
	}
	return max + 1
}

// ensureCurrent guarantees s.file points at today's live day file, reopening when
// the date advanced or the open fd's inode no longer matches the symlink target.
// It must be called with s.mu held.
func (s *rotatingSink) ensureCurrent() error {
	today := nowFunc().Format(dateLayout)

	if s.file != nil {
		dateChanged := s.date != today
		if !dateChanged && s.inodeMatchesSymlink() {
			// Reuse: same day AND fd inode still matches the live symlink target.
			return nil
		}
		if !dateChanged {
			// Same-day inode mismatch / ENOENT: today's file was unlinked or
			// replaced out from under us (the 2026-05-28 unknown-zeroing
			// scenario). Reopen onto the live target — but do NOT run the
			// day-roll sweeps; the date did not change.
			return s.reopen(today, false)
		}
		// Date change: take the new-day path and signal the day-roll sweeps.
		return s.reopen(today, true)
	}

	// First Write ever: this IS the first-of-day Handle for this fresh
	// (short-lived OR just-started) process, so it fires the day-roll sweeps too
	// (spec § Retention "first Handle of each calendar date" — for a fresh process
	// that is its process: start line, not a within-process date advance). Firing
	// on every fresh startup is safe: the retention sweep's single-winner gate
	// makes losers no-op (portal.log.swept.<today> EEXIST), and sealPastDayFiles
	// is idempotent (skips date==today and already-0400 files, emits no INFO — only
	// a WARN on chmod failure). The per-process seal-of-past-day overhead is
	// acceptable.
	return s.reopen(today, true)
}

// inodeMatchesSymlink reports whether the open fd's identity (fstat Dev+Ino)
// still matches the file the portal.log symlink resolves to (stat FOLLOWS the
// symlink). A missing target (ENOENT) or any stat error is treated as a
// mismatch so the caller reopens onto the live file.
func (s *rotatingSink) inodeMatchesSymlink() bool {
	fdInfo, err := s.file.Stat()
	if err != nil {
		return false
	}
	fdDev, fdIno, ok := devIno(fdInfo)
	if !ok {
		return false
	}

	linkInfo, err := os.Stat(symlinkPath(s.stateDir)) // follows the symlink.
	if err != nil {
		return false // ENOENT (target gone) or other error => mismatch => reopen.
	}
	linkDev, linkIno, ok := devIno(linkInfo)
	if !ok {
		return false
	}
	return fdDev == linkDev && fdIno == linkIno
}

// reopen swaps s.file onto the live today file, closing the prior fd. When
// dateChanged is true it fires the day-roll sweep seam after the new fd is in
// place (so sweeps observe today's file as already opened). The reopen follows
// the symlink-establishment seam ordering: migration-guard (Task 2-4) -> open ->
// symlink-swing (Task 2-3).
func (s *rotatingSink) reopen(today string, dateChanged bool) error {
	// First-run migration guard (Task 2-4): BEFORE swinging the symlink, clear a
	// pre-migration regular-file portal.log (and any portal.log.old) so the swing
	// below can claim the portal.log name as a symlink. Best-effort — the returned
	// error is swallowed (mirrors the swingSymlink swallow), so a guard failure
	// never aborts the reopen. The very next swing makes portal.log a symlink, so
	// this guard no-ops on every subsequent reopen.
	_ = migrationGuard(s.stateDir)

	f, dev, ino, err := openDayFile(s.stateDir, today)
	if err != nil {
		return err
	}

	// Atomic pid-scoped symlink swing: re-point ${stateDir}/portal.log at the
	// freshly-opened day file via swingSymlink (os.Symlink to a pid-scoped temp +
	// atomic os.Rename, with prior-crash temp reclamation). The swing is
	// BEST-EFFORT: a failure leaves the prior symlink in place and writes continue
	// to the freshly-opened fd, so a swing error must NOT fail the reopen. The next
	// Write's inode-identity check then forces a benign retry. The LOCKED
	// behaviour (Task 2-7) is "writes continue to the open fd"; a WARN under
	// log-rotate on the swing failure is acceptable but secondary and is NOT added
	// here — the error stays swallowed-and-continue.
	target := portalLogName + "." + today // relative bare filename — same dir.
	_ = swingSymlink(s.stateDir, target)

	if s.file != nil {
		_ = s.file.Close()
	}
	s.file = f
	s.date = today
	s.dev = dev
	s.ino = ino

	if dateChanged {
		// SEAM (Tasks 2-5 / 2-8 — day-roll sweeps): the chmod past-day sweep and
		// retention sweep are NOT run here — reopen holds s.mu, and the sweeps
		// log through this same sink, so a re-entrant Write would self-deadlock
		// (the 2026-07-06 daemon midnight freeze). Record the roll; Write fires
		// it via fireDayRoll once the mutex is released. probe leaves it queued
		// for the first post-Init Write (see pendingDayRoll).
		s.pendingDayRoll = today
	}
	return nil
}

// openDayFile opens ${stateDir}/portal.log.<today> via the first-of-day path:
// O_CREAT|O_EXCL|O_APPEND|O_WRONLY mode 0600. On EEXIST (lost the cross-process
// create race, or the file already exists from a same-day reopen) it retries with
// O_APPEND|O_WRONLY. It returns the open file plus its fstat Dev+Ino identity.
func openDayFile(stateDir, today string) (*os.File, uint64, uint64, error) {
	// The state dir may not exist yet on first run (Init precedes bootstrap's
	// EnsureDir); create it so process: start lands in portal.log, not stderr
	// (mirrors the legacy logger's MkdirAll). Best-effort + swallowed like the
	// swingSymlink / migrationGuard seams — the O_CREATE|O_EXCL open below still
	// surfaces a genuine error (e.g. ENOTDIR when a path component is a file).
	_ = os.MkdirAll(stateDir, 0o700)

	path := dayFile(stateDir, today)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_APPEND|os.O_WRONLY, logFileMode)
	if errors.Is(err, os.ErrExist) {
		f, err = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, logFileMode)
	}
	if err != nil {
		return nil, 0, 0, err
	}

	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, 0, 0, err
	}
	dev, ino, _ := devIno(info)
	return f, dev, ino, nil
}

// probe eagerly opens today's file so a configuration failure (unwritable
// stateDir) surfaces synchronously at Init rather than on the first record. The
// probe-opened fd is retained for reuse by the next Write. It holds the sink
// mutex for the open, mirroring lockedWrite's critical section. The
// first-of-day roll queued by the probe's reopen deliberately stays pending:
// probe runs BEFORE Init installs the configured handler, so firing here would
// route the sweep breadcrumbs to the pre-Init stderr default and lose them from
// portal.log. The first record through the configured handler (process: start)
// fires the queued roll instead.
func (s *rotatingSink) probe() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ensureCurrent()
}

// close releases the open fd. It is the sink's teardown counterpart, used by
// tests and any future explicit shutdown. It is safe to call when no file is
// open.
func (s *rotatingSink) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return nil
	}
	err := s.file.Close()
	s.file = nil
	return err
}

// devIno extracts a file's (Dev, Ino) identity from a FileInfo, normalised to
// uint64 for portable comparison across darwin/linux (where the syscall.Stat_t
// field types differ). The ok return is false when the underlying Sys() is not a
// *syscall.Stat_t — on supported platforms this never happens; a false ok forces
// the caller to treat identity as unknown (reopen), which is the safe default.
func devIno(info os.FileInfo) (dev, ino uint64, ok bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}
	return uint64(st.Dev), uint64(st.Ino), true
}
