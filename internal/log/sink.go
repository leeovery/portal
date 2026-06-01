package log

import (
	"errors"
	"os"
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

// rotatingSink is the date-aware, inode-identity-checked log writer that owns the
// per-Handle fd-management critical section. It is the io.Writer the textHandler
// renders into: the handler builds one line, then a single sink.Write performs
// the fd-selection (reuse / reopen) and one unbuffered write(2) under the sink
// mutex, so concurrent Handle calls serialise the reopen + write.
//
// The writer is deliberately UNBUFFERED — every record is its own write(2) to the
// *os.File (no bufio). Best-effort failure handling (stderr fallback) is Task
// 2-7; this sink surfaces open/write errors to the handler via Write's error.
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

	// dayRoll is the day-roll sweep seam, fired ONLY when the calendar date
	// advances (NOT on a same-day inode-mismatch reopen). Tasks 2-5 (chmod
	// past-day sweep) and 2-8 (retention sweep) wire their bodies behind this
	// callback. nil in this task's production wiring — the seam is gated on
	// dateChanged so the sweeps run only on a real day roll.
	dayRoll func()
}

// newRotatingSink constructs a sink rooted at stateDir. No file is opened until
// the first Write so a process that never logs touches no disk.
func newRotatingSink(stateDir string) *rotatingSink {
	return &rotatingSink{stateDir: stateDir}
}

// Write runs the per-Handle fd-selection step then performs one unbuffered
// write(2) of p to the now-current day file. The whole sequence holds the sink
// mutex so a concurrent Write cannot observe a half-swapped fd.
func (s *rotatingSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureCurrent(); err != nil {
		return 0, err
	}

	// SEAM (Task 2-6 — size-cap safety valve): the fd is now current. Before the
	// write, Task 2-6 checks current_size + len(p) >= PORTAL_LOG_ROTATE_SIZE and,
	// if exceeded, rotates to portal.log.<today>.<N> + swings the symlink. Left
	// unimplemented here: this task owns only date/inode-driven reopen.

	return s.file.Write(p)
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

	// First Write ever: open today's file (no sweeps — there is no prior day to
	// roll over).
	return s.reopen(today, false)
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
	// Write's inode-identity check then forces a benign retry. Task 2-7 upgrades
	// this swallowed error into a WARN-and-continue log line.
	target := portalLogName + "." + today // relative bare filename — same dir.
	_ = swingSymlink(s.stateDir, target)

	if s.file != nil {
		_ = s.file.Close()
	}
	s.file = f
	s.date = today
	s.dev = dev
	s.ino = ino

	if dateChanged && s.dayRoll != nil {
		// SEAM (Tasks 2-5 / 2-8 — day-roll sweeps): chmod past-day sweep and
		// retention sweep run here, gated on the date having changed.
		s.dayRoll()
	}
	return nil
}

// openDayFile opens ${stateDir}/portal.log.<today> via the first-of-day path:
// O_CREAT|O_EXCL|O_APPEND|O_WRONLY mode 0600. On EEXIST (lost the cross-process
// create race, or the file already exists from a same-day reopen) it retries with
// O_APPEND|O_WRONLY. It returns the open file plus its fstat Dev+Ino identity.
func openDayFile(stateDir, today string) (*os.File, uint64, uint64, error) {
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
// mutex for the open, mirroring Write's critical section.
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
