package cmd

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/leeovery/portal/internal/state"
	"github.com/spf13/cobra"
)

// ErrStatusUnhealthy is returned by `portal state status` when the daemon is
// not running, the last save is older than the staleness threshold, or
// recent warnings exist in portal.log. Stderr suppression is provided by
// IsSilentExitError (see cmd/state_commit_now.go) so the stderr-suppression
// contract is compile-time-linked rather than relying on an empty Error()
// string; the rendered status output has already been written to stdout, and
// this sentinel exists solely to drive a non-zero process exit.
var ErrStatusUnhealthy = errors.New("status unhealthy")

// staleSaveThreshold is the cutoff above which the most recent save is
// considered too old for `portal state status` to report a healthy exit.
const staleSaveThreshold = 5 * time.Minute

// stateStatusCmd renders a human-readable diagnostic snapshot of Portal's
// resurrection machinery and exits non-zero when the surface looks unhealthy.
var stateStatusCmd = &cobra.Command{
	Use:           "status",
	Short:         "Show Portal save daemon status and recent diagnostics",
	Args:          cobra.NoArgs,
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		now := time.Now()

		dir, err := state.Dir()
		if err != nil {
			return fmt.Errorf("resolve state dir: %w", err)
		}

		report, err := state.CollectStatus(dir, now)
		if err != nil {
			return fmt.Errorf("collect status: %w", err)
		}

		if err := renderStatus(cmd.OutOrStdout(), report, now); err != nil {
			return fmt.Errorf("render status: %w", err)
		}

		if isUnhealthy(report, now) {
			return ErrStatusUnhealthy
		}
		return nil
	},
}

// renderStatus writes the six-line "Portal state:" report to w. Layout matches
// the spec example in CLI Surface → portal state status. The first write
// error short-circuits the remaining lines and is propagated to the caller.
func renderStatus(w io.Writer, r *state.StatusReport, now time.Time) error {
	pw := &printWriter{w: w}
	pw.printf("Portal state:\n")
	pw.printf("  Save daemon: %s\n", daemonLine(r))
	pw.printf("  Last save: %s\n", lastSaveLine(r, now))
	pw.printf("  Sessions captured: %d\n", r.SessionsCount)
	pw.printf("  Panes captured: %d\n", r.PanesCount)
	pw.printf("  State size: %s on disk\n", formatBytes(r.StateSize))
	pw.printf("  Recent warnings: %s\n", warningsLine(r))
	return pw.err
}

// printWriter is a one-shot stateful wrapper that swallows the second and
// later writes once the first I/O error is observed. The caller checks .err
// at the end. Keeps renderStatus linear and one-line-per-section.
type printWriter struct {
	w   io.Writer
	err error
}

func (p *printWriter) printf(format string, args ...any) {
	if p.err != nil {
		return
	}
	_, p.err = fmt.Fprintf(p.w, format, args...)
}

// daemonLine renders the right-hand side of "Save daemon: …", branching on
// liveness.
func daemonLine(r *state.StatusReport) string {
	if !r.DaemonRunning {
		return "not running"
	}
	return fmt.Sprintf("running (pid %d, version %s)", r.DaemonPID, renderVersion(r.DaemonVersion))
}

// lastSaveLine renders the right-hand side of "Last save: …" — "never" when
// no index file exists yet, otherwise a humanised "X ago".
func lastSaveLine(r *state.StatusReport, now time.Time) string {
	if !r.HasLastSave {
		return "never"
	}
	return formatDuration(now.Sub(r.LastSaveAt))
}

// warningsLine renders the right-hand side of "Recent warnings: …",
// substituting the "(last: none)" suffix when the count is zero.
func warningsLine(r *state.StatusReport) string {
	if r.RecentWarnings == 0 {
		return "0 (last: none)"
	}
	return fmt.Sprintf("%d (last: %s)", r.RecentWarnings, r.LastWarning)
}

// renderVersion substitutes the "unknown" placeholder when the daemon never
// recorded a version marker.
func renderVersion(v string) string {
	if v == "" {
		return "unknown"
	}
	return v
}

// isUnhealthy mirrors the spec's exit-code policy: daemon-down, stale save
// (> staleSaveThreshold), or any recent warnings all count as unhealthy.
// HasLastSave=false is explicitly NOT unhealthy — fresh installs are fine.
func isUnhealthy(r *state.StatusReport, now time.Time) bool {
	if !r.DaemonRunning {
		return true
	}
	if r.HasLastSave && now.Sub(r.LastSaveAt) > staleSaveThreshold {
		return true
	}
	if r.RecentWarnings > 0 {
		return true
	}
	return false
}

// formatDuration renders a coarse "X units ago" string. Sub-2-second
// intervals collapse to "just now" so the report does not flicker rapidly
// for very recent saves.
func formatDuration(d time.Duration) string {
	seconds := int(d.Seconds())
	if seconds < 2 {
		return "just now"
	}
	if seconds < 60 {
		return fmt.Sprintf("%d seconds ago", seconds)
	}
	minutes := seconds / 60
	if minutes < 60 {
		return fmt.Sprintf("%d minutes ago", minutes)
	}
	hours := minutes / 60
	if hours < 24 {
		return fmt.Sprintf("%d hours ago", hours)
	}
	days := hours / 24
	return fmt.Sprintf("%d days ago", days)
}

// formatBytes renders an int64 byte count with one-decimal-place IEC units
// (KB / MB / GB / …). Below 1024 B values render as raw bytes with no
// decimal — "500 B" rather than "0.5 KB".
func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

func init() {
	stateCmd.AddCommand(stateStatusCmd)
}
