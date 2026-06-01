package cmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/leeovery/portal/internal/log"
)

// initTestLogToStateDir wires the production internal/log handler so log
// records emitted via the package-level component loggers (daemonLogger,
// hydrateLogger, …) land in <dir>/portal.log with the pid/version/process_role
// baseline attrs injected — exactly as main -> log.Init does in the real
// binary. Used by tests that drive a command body directly (without going
// through main) yet assert on portal.log contents. processRole defaults to
// "daemon".
func initTestLogToStateDir(t *testing.T, dir, version string) {
	t.Helper()
	initTestLogToStateDirAs(t, dir, version, "daemon")
}

// initTestLogToStateDirAs is initTestLogToStateDir with an explicit
// process_role baseline. It ensures dir exists (log.Init's Phase-1 writer does
// not create parent directories) and brackets log.Init with a SetTestHandler
// snapshot-and-restore so the process-wide handler swap does not leak into
// sibling tests (t.Cleanup runs LIFO, so the pre-test handler is restored
// after log.Init's file handler).
func initTestLogToStateDirAs(t *testing.T, dir, version, processRole string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	log.SetTestHandler(t, slog.New(slog.NewTextHandler(io.Discard, nil)).Handler())
	if err := log.Init(dir, version, processRole); err != nil {
		t.Fatalf("log.Init: %v", err)
	}
}

// cmdCaptureSink is a slog.Handler used by the cmd package's in-process
// logging tests. It records every record and renders a text body in the
// shape "<LEVEL> component=<c> <msg> key=value..." so the existing substring
// assertions (level label, component, message phrase, attrs) keep working
// after the observability migration retyped every cmd-layer logging seam to
// *slog.Logger.
type cmdCaptureSink struct {
	mu    sync.Mutex
	lines []string
	// shared points at the lines-owning sink so handlers derived via
	// WithAttrs/WithGroup (notably the .With("component", ...) binding) record
	// into the same buffer; nil on the root sink.
	shared *cmdCaptureSink
	// bound holds attrs accumulated via WithAttrs so the component (bound at
	// the logger, not at each call site) is rendered on every record.
	bound []slog.Attr
}

func (s *cmdCaptureSink) owner() *cmdCaptureSink {
	if s.shared != nil {
		return s.shared
	}
	return s
}

func (s *cmdCaptureSink) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (s *cmdCaptureSink) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Attr, 0, len(s.bound)+len(attrs))
	next = append(next, s.bound...)
	next = append(next, attrs...)
	return &cmdCaptureSink{shared: s.owner(), bound: next}
}

func (s *cmdCaptureSink) WithGroup(_ string) slog.Handler {
	return &cmdCaptureSink{shared: s.owner(), bound: s.bound}
}

func (s *cmdCaptureSink) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(r.Level.String())
	b.WriteString(" ")
	b.WriteString(r.Message)
	for _, a := range s.bound {
		fmt.Fprintf(&b, " %s=%v", a.Key, a.Value.Any())
	}
	r.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(&b, " %s=%v", a.Key, a.Value.Any())
		return true
	})
	owner := s.owner()
	owner.mu.Lock()
	owner.lines = append(owner.lines, b.String())
	owner.mu.Unlock()
	return nil
}

func (s *cmdCaptureSink) body() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Join(s.lines, "\n")
}

// newCaptureLoggerForComponent returns a capturing *slog.Logger bound to the
// given component plus its sink, so a cmd test can inject the logger into a
// *Deps / config struct and assert on the rendered body.
func newCaptureLoggerForComponent(t *testing.T, component string) (*slog.Logger, *cmdCaptureSink) {
	t.Helper()
	sink := &cmdCaptureSink{}
	return slog.New(sink).With("component", component), sink
}
