package log

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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
	level, source, raw := resolveLevel(os.Getenv("PORTAL_LOG_LEVEL"))

	pid := os.Getpid()
	startTime = time.Now()

	writer, openErr := openLogWriter(stateDir)
	setHandler(newTextHandler(writer, level, pid, version, processRole))

	emitLifecycleMarkers(level, source, raw)

	return openErr
}

// emitLifecycleMarkers writes the per-process lifecycle markers as Init's final
// pre-return action, AFTER the configured handler is swapped in so they route to
// the real day file (spec § Defensive invariants — process: start, and §
// Log-level propagation verification). Emission order is fixed:
//
//  1. process: start — the FIRST record to the handler. Via the rotating sink's
//     first-of-day open this is what triggers portal.log creation + the gated
//     retention sweep; it must precede any other portal logging.
//  2. process: log-level resolved — immediately after start, declaring the
//     resolved level and how it was resolved.
//
// Both lines are component=process with messages in the closed lifecycle set, so
// the handler bypasses the level filter and they appear even at WARN/ERROR. The
// baseline attrs (pid/version/process_role) are auto-injected per-record by the
// handler — the call sites here deliberately do NOT pass them.
//
// When the level resolved via fallback (an invalid PORTAL_LOG_LEVEL), an
// additional bootstrap-component WARN is emitted (spec § Log-level discipline —
// Default and invalid-value handling). It does NOT bypass the level filter, but a
// fallback always resolves to info, so the configured handler is at INFO and the
// WARN (slog WARN >= INFO) is visible.
//
// Init is idempotent: each call re-emits these markers — the most recent Init
// defines the logical start. In production main calls Init exactly once.
func emitLifecycleMarkers(level slog.Level, source, raw string) {
	process := For(processComponent)
	process.Info("start",
		"cmd", filepath.Base(os.Args[0]),
		"args", strings.Join(os.Args[1:], " "),
	)
	process.Info("log-level resolved",
		"resolved", levelString(level),
		"source", source,
		"raw", raw,
	)
	if source == sourceFallback {
		For(bootstrapComponent).Warn("invalid PORTAL_LOG_LEVEL",
			"raw", raw,
			"resolved", "info",
		)
	}
}

// openLogWriter constructs the date-aware rotating sink for stateDir and probes
// it with one eager fd-open so a configuration failure (unwritable stateDir) is
// surfaced to Init synchronously rather than silently on the first record. On
// probe failure it returns a stderr fallback writer and the open error so Init
// can surface it advisorily while still installing a usable handler.
//
// The sink itself opens lazily (no file is touched until the first Write); the
// eager probe here exists only to preserve the Phase-1 stderr-fallback-on-open-
// failure contract. The probe-opened fd is retained by the sink for reuse, so the
// probe is not wasted work — the next Write reuses it.
//
// The rotation machinery (first-of-day O_CREAT|O_EXCL open, inode-identity reopen,
// minimal portal.log symlink establishment) lives in rotatingSink. Sibling tasks
// add the atomic pid-scoped symlink swing (2-3), the migration guard (2-4), the
// chmod past-day sweep (2-5), the size-cap valve (2-6), best-effort write-failure
// handling (2-7), and the retention sweep (2-8) behind the seams marked in sink.go.
func openLogWriter(stateDir string) (io.Writer, error) {
	rotateSize, _ := resolveRotateSize(os.Getenv("PORTAL_LOG_ROTATE_SIZE"))
	sink := newRotatingSink(stateDir, rotateSize)
	if err := sink.probe(); err != nil {
		return os.Stderr, err
	}
	return sink, nil
}

// Close emits exactly one "process: exit" INFO marker — the terminal half of
// the per-process lifecycle pairing (spec § Defensive invariants — process: exit
// and the main exit shape). The line carries the passed exitCode verbatim (0
// clean, 1 error, 2 usage/panic) and took computed from the package-private
// startTime captured at Init. The baseline attrs (pid/version/process_role) are
// auto-injected per-record by the handler — Close does NOT pass them.
//
// Close owns NO control flow — it does NOT call os.Exit. main owns the single
// os.Exit; Close is purely a marker-emitter so it can run on Cobra's
// Execute-error return path (which os.Exit would skip if deferred). The "exit"
// message is in the closed process-lifecycle set, so the handler bypasses the
// level filter and the line appears even at PORTAL_LOG_LEVEL=warn/error.
//
// Exactly one exit line fires per Close call (no double-emit). On the panic path
// main skips Close (Task 2-13) so exit and panic never both fire.
//
// Close is safe to call before any Init: startTime is then the zero value, so
// computeTook returns a large-but-bounded (finite, non-negative) duration and
// the line still renders — it routes to the pre-Init default stderr-text handler
// held behind the swap indirection (always a valid handler, so no nil-deref) and
// never panics.
func Close(exitCode int) {
	For(processComponent).Info("exit", "code", exitCode, "took", computeTook())
}

// computeTook returns the elapsed time since the startTime captured at Init.
// Factored out as a single named, testable seam so the took attr on the
// "process: exit" line emitted by Close is computed in exactly one place.
func computeTook() time.Duration {
	return time.Since(startTime)
}
