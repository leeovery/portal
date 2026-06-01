package log

import (
	"io"
	"os"
	"path/filepath"
	"time"
)

// portalLogName is the portal.log basename joined onto the caller-supplied
// stateDir. internal/log accepts stateDir as a plain string and joins this
// itself rather than referencing internal/state.PortalLog — internal/log must
// NOT import internal/state (import-cycle guard).
const portalLogName = "portal.log"

// logFileMode is the permission mode for portal.log: owner read/write only.
const logFileMode = 0o600

// startTime is the package-private logical start of the process, captured by
// Init. Close computes took as time.Since(startTime). It is reset on every Init
// so the most recent Init defines the logical start (idempotent/re-entrant
// contract). Its zero value is harmless to Close (took is large but the
// computation does not panic).
var startTime time.Time

// Init configures the process-wide logger and atomically swaps the configured
// handler into the shared indirection so every For-created logger — including
// those cached at package init, before Init ran — routes through it.
//
// Steps: resolve the level from PORTAL_LOG_LEVEL; capture pid and startTime;
// construct the configured text handler bound to (writer, level, pid, version,
// processRole); swap it in via setHandler.
//
// Init is IDEMPOTENT and re-entrant: a second call re-resolves the level,
// re-opens the writer, re-captures startTime, and re-points the handler. It
// never panics and does not close the previous handler's writer in a way that
// breaks a concurrent Handle — the swap is a single atomic store and the prior
// *os.File is left to the OS (a long-lived process opens portal.log at most a
// handful of times across re-Inits; leaking that handle is preferable to
// closing a file another goroutine may still be writing).
//
// The returned error is ADVISORY: on a writer-open failure Init falls back to a
// stderr text handler (logging must never fail the caller) and returns the open
// error so main can decide. By convention main calls Init first and does not
// abort on a logging failure. By convention only main calls Init in production.
func Init(stateDir, version, processRole string) error {
	level, _, _ := resolveLevel(os.Getenv("PORTAL_LOG_LEVEL"))

	pid := os.Getpid()
	startTime = time.Now()

	writer, openErr := openLogWriter(stateDir)
	setHandler(newTextHandler(writer, level, pid, version, processRole))

	// TODO(phase-2): emit the "process: start" INFO line here, as Init's final
	// action before returning, via log.For("process").Info("start", "cmd",
	// filepath.Base(os.Args[0]), "args", strings.Join(os.Args[1:], " ")). Also
	// emit the "log-level resolved" line. Phase 1 delivers only the wiring; the
	// lifecycle-marker emission bodies are Phase 2.

	return openErr
}

// openLogWriter opens the Phase-1 simple (non-rotating) sink: an append-mode
// portal.log under stateDir. On failure it returns a stderr fallback writer and
// the open error so Init can surface it advisorily while still installing a
// usable handler.
//
// TODO(phase-2): this is the rotating-handler insertion point. Phase 2 replaces
// this plain O_APPEND|O_CREATE|O_WRONLY open with the date-aware rotating
// handler — first-of-day O_CREAT|O_EXCL open plus the portal.log symlink swing
// and retention sweep. For Phase 1 a plain append open keeps Init end-to-end
// functional.
func openLogWriter(stateDir string) (io.Writer, error) {
	path := filepath.Join(stateDir, portalLogName)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, logFileMode)
	if err != nil {
		return os.Stderr, err
	}
	return f, nil
}

// Close computes took from the package-private startTime captured at Init. It
// owns NO control flow — it does NOT call os.Exit. main owns the single
// os.Exit; Close is purely a marker-emitter so it can run on Cobra's
// Execute-error return path (which os.Exit would skip if deferred).
//
// Close is safe to call before any Init: startTime is then the zero value and
// computeTook returns a large but harmless duration without panicking.
//
// TODO(phase-2): emit the "process: exit" INFO line here via
// log.For("process").Info("exit", "code", exitCode, "took", computeTook()).
// Phase 1 lands the signature, the took computation, and the no-control-flow
// guarantee so main (Task 1-7) can call Close; the marker emission body is
// Phase 2.
func Close(exitCode int) {
	_ = exitCode
	_ = computeTook()
}

// computeTook returns the elapsed time since the startTime captured at Init.
// Factored out so the took computation is a single named, testable seam shared
// by Close and the Phase-2 "process: exit" emission.
func computeTook() time.Duration {
	return time.Since(startTime)
}
