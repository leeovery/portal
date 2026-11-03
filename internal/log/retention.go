package log

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

const sweptSentinelMode os.FileMode = 0o600

var removeFunc = os.Remove

func runRetentionSweep(stateDir, today string, gated bool) {
	runRetentionSweepWithDays(stateDir, today, gated, nil)
}

// Best-effort: a partial sweep is made good by the next day's winner — retention
// is a disk-space bound, not a correctness boundary.
func runRetentionSweepWithDays(stateDir, today string, gated bool, forcedDays *int) {
	if gated && !claimSweepGate(stateDir, today) {
		return
	}

	retentionDays := resolveSweepRetentionDays(forcedDays)

	cutoff, ok := retentionCutoff(today, retentionDays)
	if !ok {
		return
	}

	deletePastCutoff(stateDir, cutoff, retentionDays)
	pruneStaleSentinels(stateDir, today, gated)
}

func resolveSweepRetentionDays(forcedDays *int) int {
	if forcedDays != nil {
		return *forcedDays
	}

	retentionDays, source, raw := resolveRetentionDays(os.Getenv(retentionDaysVar))
	if source == sourceFallback {
		rotateLogger.Warn("invalid PORTAL_LOG_RETENTION_DAYS", "raw", raw, "retention", retentionDays)
	}
	return retentionDays
}

// SweepLogsForClean is the user-invoked sweep: ungated, cutoff today, so every
// prior-day rotated file and swept sentinel goes. It always returns nil —
// per-file failures warn and the sweep continues.
func SweepLogsForClean(stateDir string) error {
	today := nowFunc().Format(dateLayout)
	cleanCutoffDays := 0
	runRetentionSweepWithDays(stateDir, today, false, &cleanCutoffDays)
	return nil
}

// claimSweepGate reports whether this process won today's sweep. Any open error,
// not just EEXIST, loses: an ungatable sweep is skipped, not run un-deduped.
func claimSweepGate(stateDir, today string) bool {
	f, err := os.OpenFile(sweptSentinelFile(stateDir, today), os.O_CREATE|os.O_EXCL|os.O_WRONLY, sweptSentinelMode)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

func retentionCutoff(today string, retentionDays int) (cutoff time.Time, ok bool) {
	todayTime, err := time.Parse(dateLayout, today)
	if err != nil {
		return time.Time{}, false
	}
	return todayTime.AddDate(0, 0, -retentionDays), true
}

func deletePastCutoff(stateDir string, cutoff time.Time, retentionDays int) {
	matches, err := filepath.Glob(filepath.Join(stateDir, portalLogName+".*"))
	if err != nil {
		return
	}

	for _, path := range matches {
		date, ok := pastDayLogDate(filepath.Base(path))
		if !ok {
			continue
		}
		fileDate, err := time.Parse(dateLayout, date)
		if err != nil || !fileDate.Before(cutoff) {
			continue
		}

		// Breadcrumb before the unlink, so it survives a kill between the two.
		rotateLogger.Info("deleted", "path", path, "retention", retentionDays)
		if err := removeFunc(path); err != nil {
			rotateLogger.Warn("delete failed", "error", err, "path", path)
		}
	}
}

func pruneStaleSentinels(stateDir, today string, gated bool) {
	matches, err := filepath.Glob(filepath.Join(stateDir, sweptPrefix+"*"))
	if err != nil {
		return
	}

	for _, path := range matches {
		date, ok := sweptSentinelDate(filepath.Base(path))
		if !ok {
			continue
		}
		if gated && date == today {
			continue
		}
		if err := removeFunc(path); err != nil {
			rotateLogger.Warn("sentinel prune failed", "error", err, "path", path)
		}
	}
}

// sweptSentinelDate returns the date slot verbatim, with no strict YYYY-MM-DD
// parse, so a malformed or legacy sentinel is still prunable.
func sweptSentinelDate(base string) (date string, ok bool) {
	rest, found := strings.CutPrefix(base, sweptPrefix)
	if !found || rest == "" {
		return "", false
	}
	return rest, true
}
