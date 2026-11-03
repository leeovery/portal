package log

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Duplicated rather than taken from internal/state: internal/log must not
// import internal/state.
const portalLogName = "portal.log"

const logFileMode = 0o600

var startTime time.Time

// Init installs the configured handler, so every For-created logger — including
// those cached at package init — routes through it. It is idempotent, and leaves
// the previous writer to the OS rather than closing a file a concurrent Handle
// may still be using. The returned error is advisory: on a writer-open failure
// Init installs a stderr fallback handler and reports it.
func Init(stateDir, version, processRole string) error {
	level, source, raw := resolveLevel(os.Getenv(levelVar))

	pid := os.Getpid()
	startTime = time.Now()

	writer, openErr := openLogWriter(stateDir)
	setHandler(newTextHandler(writer, level, pid, version, processRole))

	emitLifecycleMarkers(level, source, raw)

	return openErr
}

// Must run after the handler is swapped in, and "start" must be the first
// record: its post-write day-roll drains the first-of-day sweeps the probe
// queued, so those breadcrumbs land in portal.log rather than on stderr.
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

func openLogWriter(stateDir string) (io.Writer, error) {
	rotateSize, _ := resolveRotateSize(os.Getenv(rotateSizeVar))
	sink := newRotatingSink(stateDir, rotateSize)
	if err := sink.probe(); err != nil {
		return os.Stderr, err
	}
	return sink, nil
}

// Close does not call os.Exit and owns no control flow, so it can run on an
// error-return path a deferred call would miss. Safe to call before Init, and
// must be skipped on the panic path so exit and panic never both fire.
func Close(exitCode int) {
	For(processComponent).Info("exit", "code", exitCode, "took", computeTook())
}

func computeTook() time.Duration {
	return time.Since(startTime)
}
