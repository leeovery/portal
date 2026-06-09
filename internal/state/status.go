package state

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/leeovery/portal/internal/log"
)

// recentWarningWindow is the look-back interval scanRecentWarnings applies to
// portal.log entries. Entries with timestamps strictly before now-window are
// skipped. Matches the spec's "recent = last hour" rule.
const recentWarningWindow = time.Hour

// StatusReport is the data-only result of CollectStatus. Each field maps to a
// section of `portal state status` output; the formatting layer is separate.
type StatusReport struct {
	// DaemonRunning is true when daemon.pid points at a live, signalable
	// process. False when the PID file is missing, unparseable, or names a
	// dead process.
	DaemonRunning bool

	// DaemonPID is the parsed contents of daemon.pid, or 0 when the file is
	// missing or unparseable.
	DaemonPID int

	// DaemonVersion is the trimmed contents of daemon.version, or "" when
	// the file is missing.
	DaemonVersion string

	// LastSaveAt is the saved_at field of sessions.json. Zero when the
	// index is missing or unreadable.
	LastSaveAt time.Time

	// HasLastSave reports whether sessions.json was successfully read.
	HasLastSave bool

	// SessionsCount is len(idx.Sessions). Zero when the index is missing.
	SessionsCount int

	// PanesCount is the total number of panes across all sessions and
	// windows in the index. Zero when the index is missing.
	PanesCount int

	// StateSize is the on-disk size of the state directory: the sum of
	// regular-file sizes under the state dir tree (including subdirectories
	// such as scrollback/). Zero when the directory is missing.
	StateSize int64

	// RecentWarnings is the number of WARN/ERROR entries in portal.log
	// whose timestamp falls within the last recentWarningWindow relative
	// to the now passed to CollectStatus.
	RecentWarnings int

	// LastWarning is the most recent qualifying entry rendered as
	// `<LEVEL> <component>: <msg>` — timestamp prefix and trailing
	// attrs/baselines omitted. Empty when there are no qualifying entries.
	LastWarning string
}

// CollectStatus gathers diagnostic data about the daemon, last save, and
// recent log activity for `portal state status` to render. It is a pure
// data-collection layer: no I/O beyond reading state files, no formatting,
// no logging.
//
// Behaviour:
//   - Missing daemon.pid → DaemonPID=0, DaemonRunning=false.
//   - Missing daemon.version → DaemonVersion="".
//   - Missing or unparseable sessions.json → HasLastSave=false, counts=0.
//   - Missing state directory → StateSize=0.
//   - Missing portal.log → RecentWarnings=0, LastWarning="".
//
// The caller-supplied now anchors the recent-warnings window. Genuine I/O
// errors that aren't "missing file" cases are not propagated — status is a
// best-effort diagnostic and partial data is more useful than no data. The
// error return is reserved for future use (currently always nil).
func CollectStatus(dir string, now time.Time) (*StatusReport, error) {
	rep := &StatusReport{}

	collectDaemonState(rep, dir)
	collectIndexState(rep, dir)
	rep.StateSize = computeStateSize(dir)
	scanRecentWarnings(rep, PortalLog(dir), now.Add(-recentWarningWindow))

	return rep, nil
}

// collectDaemonState fills DaemonPID, DaemonRunning, and DaemonVersion from
// daemon.pid and daemon.version. Any read or parse error is swallowed —
// status is a diagnostic surface and missing/unreadable daemon files are
// the normal "no daemon yet" condition rather than report-blocking errors.
func collectDaemonState(rep *StatusReport, dir string) {
	pid, err := ReadPIDFile(dir)
	if err == nil {
		rep.DaemonPID = pid
		rep.DaemonRunning = IsProcessAlive(pid)
	}

	version, err := ReadVersionFile(dir)
	if err == nil {
		rep.DaemonVersion = version
	}
}

// collectIndexState fills LastSaveAt, HasLastSave, SessionsCount, and
// PanesCount from sessions.json. Skip-with-error from ReadIndex (corrupt
// JSON, unsupported version, permission error) leaves the counts at zero
// and HasLastSave at false — same observable shape as a missing file.
func collectIndexState(rep *StatusReport, dir string) {
	idx, skip, err := ReadIndex(dir)
	if skip || err != nil {
		return
	}
	rep.HasLastSave = true
	rep.LastSaveAt = idx.SavedAt
	rep.SessionsCount = len(idx.Sessions)
	rep.PanesCount = countPanes(idx)
}

// countPanes returns the total number of panes across every window in every
// session of idx. The zero-pane case (empty session or window) contributes 0,
// so the count is exact regardless of canonicalisation state.
func countPanes(idx Index) int {
	total := 0
	for _, s := range idx.Sessions {
		for _, w := range s.Windows {
			total += len(w.Panes)
		}
	}
	return total
}

// computeStateSize walks dir and returns the sum of regular-file sizes under
// it, including files in subdirectories such as scrollback/. A missing
// directory yields 0 (not an error). Walk errors and per-entry stat errors
// are silently skipped — the report falls back to the partial sum we have
// rather than failing the whole status surface.
func computeStateSize(dir string) int64 {
	var total int64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// Per-entry errors (e.g. a removed file, permission denied on
			// a single subdir) are tolerated: skip the entry, keep walking
			// so the sum reflects everything else.
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}
		total += info.Size()
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		// Any non-ENOENT walk-level error (the root itself is unreadable,
		// for example) collapses to size=0 to keep the status surface
		// best-effort.
		return 0
	}
	if errors.Is(err, fs.ErrNotExist) {
		return 0
	}
	return total
}

// scanRecentWarnings reads logPath line by line, parsing each line once via
// log.ParseLogLine (the single inverse of the writer's text format), and
// updates rep.RecentWarnings and rep.LastWarning for every WARN/ERROR entry
// whose timestamp is at or after cutoff. Missing log file: zero counts (not an
// error). Unparseable lines and lines whose level is not WARN/ERROR or whose
// timestamp predates cutoff are silently skipped (swallow-and-skip). LastWarning
// is last-wins, positional top-to-bottom: because the writer only ever appends
// in chronological order, the last qualifying line in the file is the most
// recent one.
func scanRecentWarnings(rep *StatusReport, logPath string, cutoff time.Time) {
	f, err := os.Open(logPath)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		parsed, ok := log.ParseLogLine(scanner.Text())
		if !ok {
			continue
		}
		if parsed.Level != "WARN" && parsed.Level != "ERROR" {
			continue
		}
		if parsed.Time.Before(cutoff) {
			continue
		}
		rep.RecentWarnings++
		rep.LastWarning = composeLastWarning(parsed)
	}
}

// composeLastWarning renders a qualifying log line as the LastWarning summary:
// "<LEVEL> <component>: <msg>". The component segment is omitted when empty
// (yielding "<LEVEL>: <msg>" with no stray space before the colon) and the
// message segment is omitted when empty (yielding "<LEVEL> <component>:" with no
// trailing space) — assembling the level, optional " <component>", ":", and
// optional " <msg>" pieces so no stray space appears in any combination.
func composeLastWarning(parsed log.LogLine) string {
	summary := parsed.Level
	if parsed.Component != "" {
		summary += " " + parsed.Component
	}
	summary += ":"
	if parsed.Message != "" {
		summary += " " + parsed.Message
	}
	return summary
}
