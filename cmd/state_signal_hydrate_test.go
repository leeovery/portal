// Tests in this file mutate package-level state (signalHydrateRunFunc) and
// MUST NOT use t.Parallel.
package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// recordingSignaler records each fifoPath runSignalHydrate signals via the
// state.FIFOSignaler seam. Tests use it to assert which paneKey FIFO paths
// were opened and (when needed) inject per-path or global errors. The retry
// ladder + non-blocking open semantics are exhaustively tested at the
// state-package layer (internal/state/signal_hydrate_test.go); cmd-layer
// tests focus on orchestration concerns (which paneKeys get signaled,
// marker-unset is never called, idempotency, soft-fail discipline).
type recordingSignaler struct {
	calls []string
	// errOn returns the configured error on calls whose path equals the
	// key. Useful for "this pane fails, others succeed" scenarios.
	errOn map[string]error
	// err, when non-nil, is returned for every call. Useful for "every
	// signal fails" scenarios (e.g. retry-exhaustion soft-fail).
	err error
}

// SendSignal satisfies state.FIFOSignaler. It records path verbatim and
// returns the configured per-path or global error, otherwise nil.
func (s *recordingSignaler) SendSignal(path string) error {
	s.calls = append(s.calls, path)
	if s.err != nil {
		return s.err
	}
	if e, ok := s.errOn[path]; ok {
		return e
	}
	return nil
}

// Compile-time assertion: the recording fake must satisfy state.FIFOSignaler.
// A drift in the production interface signature surfaces here.
var _ state.FIFOSignaler = (*recordingSignaler)(nil)

// markersOption renders a `show-options -sv` line for a given paneKey set so
// tests can drive ListSkeletonMarkers via recordingCommander.RunFunc.
func markersOption(paneKeys ...string) string {
	if len(paneKeys) == 0 {
		return ""
	}
	out := ""
	for i, k := range paneKeys {
		if i > 0 {
			out += "\n"
		}
		out += "@portal-skeleton-" + k + " 1"
	}
	return out
}

// listPanesOutput renders a `list-panes -s -t <s> -F #{window_index}:#{pane_index}`
// reply for a list of (window, pane) tuples.
func listPanesOutput(panes [][2]int) string {
	out := ""
	for i, p := range panes {
		if i > 0 {
			out += "\n"
		}
		out += fmt.Sprintf("%d:%d", p[0], p[1])
	}
	return out
}

// signalHydrateClient builds a recordingCommander whose RunFunc replies to the
// two tmux calls signal-hydrate makes: show-options -sv and
// list-panes -s -t <session>. Other calls fail the test.
func signalHydrateClient(t *testing.T, markersRaw string, panes [][2]int) (*tmux.Client, *recordingCommander) {
	t.Helper()
	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			if len(args) == 0 {
				return "", nil
			}
			switch args[0] {
			case "show-options":
				return markersRaw, nil
			case "list-panes":
				return listPanesOutput(panes), nil
			}
			t.Fatalf("unexpected tmux call: %v", args)
			return "", nil
		},
	}
	return tmux.NewClient(cmder), cmder
}

func TestSignalHydrate_SignalsEverySkeletonMarkedPane(t *testing.T) {
	dir := t.TempDir()
	session := "foo"
	panes := [][2]int{{0, 0}, {0, 1}}
	keys := []string{
		state.SanitizePaneKey(session, 0, 0),
		state.SanitizePaneKey(session, 0, 1),
	}

	client, _ := signalHydrateClient(t, markersOption(keys...), panes)
	signaler := &recordingSignaler{}

	cfg := signalHydrateConfig{
		Session:  session,
		StateDir: dir,
		Client:   client,
		Signaler: signaler,
	}
	if err := runSignalHydrate(cfg); err != nil {
		t.Fatalf("runSignalHydrate: %v", err)
	}

	// Every marked paneKey must receive exactly one SendSignal call at its
	// canonical FIFO path.
	wantPaths := map[string]struct{}{
		state.FIFOPath(dir, keys[0]): {},
		state.FIFOPath(dir, keys[1]): {},
	}
	if len(signaler.calls) != len(wantPaths) {
		t.Fatalf("SendSignal call count = %d, want %d (calls=%v)", len(signaler.calls), len(wantPaths), signaler.calls)
	}
	for _, path := range signaler.calls {
		if _, ok := wantPaths[path]; !ok {
			t.Errorf("unexpected SendSignal path %q; wanted set %v", path, wantPaths)
		}
	}
}

func TestSignalHydrate_SkipsPanesWithoutSkeletonMarker(t *testing.T) {
	dir := t.TempDir()
	session := "foo"
	panes := [][2]int{{0, 0}, {0, 1}, {1, 0}}
	// Only mark window 0, pane 1.
	markedKey := state.SanitizePaneKey(session, 0, 1)

	client, _ := signalHydrateClient(t, markersOption(markedKey), panes)
	signaler := &recordingSignaler{}

	cfg := signalHydrateConfig{
		Session:  session,
		StateDir: dir,
		Client:   client,
		Signaler: signaler,
	}
	if err := runSignalHydrate(cfg); err != nil {
		t.Fatalf("runSignalHydrate: %v", err)
	}

	wantPath := state.FIFOPath(dir, markedKey)
	if len(signaler.calls) != 1 {
		t.Fatalf("SendSignal call count = %d, want 1 (only marked pane); calls=%v", len(signaler.calls), signaler.calls)
	}
	if signaler.calls[0] != wantPath {
		t.Errorf("SendSignal[0] = %q, want %q", signaler.calls[0], wantPath)
	}
}

func TestSignalHydrate_ZeroMarkersIsNoOp(t *testing.T) {
	dir := t.TempDir()
	session := "foo"
	panes := [][2]int{{0, 0}, {0, 1}}

	// markersRaw is empty — no skeleton markers anywhere on the server.
	client, _ := signalHydrateClient(t, "", panes)
	signaler := &recordingSignaler{}

	cfg := signalHydrateConfig{
		Session:  session,
		StateDir: dir,
		Client:   client,
		Signaler: signaler,
	}
	if err := runSignalHydrate(cfg); err != nil {
		t.Fatalf("runSignalHydrate: %v", err)
	}
	if len(signaler.calls) != 0 {
		t.Errorf("SendSignal called %d times under zero markers, want 0", len(signaler.calls))
	}
}

// TestSignalHydrate_PerFIFOFailureDoesNotUnsetMarker pins the soft-fail
// posture: a SendSignal failure (e.g. ENOENT or retry-exhaustion bubbled up
// through the production state.FIFOSignaler) must NEVER cause runSignalHydrate
// to unset the @portal-skeleton-* marker. Marker ownership belongs to the
// hydrate helper (spec → "The 100ms Settle Sleep") — signal-hydrate must
// remain marker-read-only.
func TestSignalHydrate_PerFIFOFailureDoesNotUnsetMarker(t *testing.T) {
	dir := t.TempDir()
	session := "foo"
	key := state.SanitizePaneKey(session, 0, 0)
	panes := [][2]int{{0, 0}}

	client, cmder := signalHydrateClient(t, markersOption(key), panes)
	signaler := &recordingSignaler{
		err: errors.New("retry exhausted (sentinel)"),
	}

	cfg := signalHydrateConfig{
		Session:  session,
		StateDir: dir,
		Client:   client,
		Signaler: signaler,
	}
	if err := runSignalHydrate(cfg); err != nil {
		t.Fatalf("runSignalHydrate must soft-fail on signal failure, got %v", err)
	}

	// Verify NO `set-option -su <marker>` was issued — signal-hydrate
	// must never touch markers (helper owns marker-unset).
	for _, c := range cmder.Calls {
		if len(c) >= 2 && c[0] == "set-option" && c[1] == "-su" {
			t.Errorf("signal-hydrate must never call set-option -su; got %v", c)
		}
	}
}

func TestSignalHydrate_NeverCallsSetOptionSU(t *testing.T) {
	dir := t.TempDir()
	session := "foo"
	keys := []string{
		state.SanitizePaneKey(session, 0, 0),
		state.SanitizePaneKey(session, 0, 1),
	}
	panes := [][2]int{{0, 0}, {0, 1}}

	client, cmder := signalHydrateClient(t, markersOption(keys...), panes)

	cfg := signalHydrateConfig{
		Session:  session,
		StateDir: dir,
		Client:   client,
		Signaler: &recordingSignaler{},
	}
	if err := runSignalHydrate(cfg); err != nil {
		t.Fatalf("runSignalHydrate: %v", err)
	}

	for _, c := range cmder.Calls {
		if len(c) >= 2 && c[0] == "set-option" && c[1] == "-su" {
			t.Errorf("signal-hydrate issued set-option -su (forbidden): %v", c)
		}
	}
}

func TestSignalHydrate_SoftFailsWhenSessionDoesNotExist(t *testing.T) {
	dir := t.TempDir()
	session := "ghost"

	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			switch args[0] {
			case "show-options":
				return "", nil
			case "list-panes":
				return "", errors.New("can't find session: ghost")
			}
			t.Fatalf("unexpected tmux call: %v", args)
			return "", nil
		},
	}
	client := tmux.NewClient(cmder)
	signaler := &recordingSignaler{}

	cfg := signalHydrateConfig{
		Session:  session,
		StateDir: dir,
		Client:   client,
		Signaler: signaler,
	}
	if err := runSignalHydrate(cfg); err != nil {
		t.Fatalf("runSignalHydrate must soft-fail on missing session, got %v", err)
	}
	if len(signaler.calls) != 0 {
		t.Errorf("SendSignal called %d times when session missing, want 0", len(signaler.calls))
	}
}

func TestSignalHydrate_IsIdempotentAcrossRepeatedInvocations(t *testing.T) {
	dir := t.TempDir()
	session := "foo"
	key := state.SanitizePaneKey(session, 0, 0)
	panes := [][2]int{{0, 0}}

	// First call: marker set. Second call: marker absent.
	var markerSet = true
	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			switch args[0] {
			case "show-options":
				if markerSet {
					return markersOption(key), nil
				}
				return "", nil
			case "list-panes":
				return listPanesOutput(panes), nil
			}
			t.Fatalf("unexpected tmux call: %v", args)
			return "", nil
		},
	}
	client := tmux.NewClient(cmder)

	first := &recordingSignaler{}
	cfg := signalHydrateConfig{
		Session:  session,
		StateDir: dir,
		Client:   client,
		Signaler: first,
	}
	if err := runSignalHydrate(cfg); err != nil {
		t.Fatalf("first invocation: %v", err)
	}
	if len(first.calls) != 1 {
		t.Errorf("first invocation SendSignal calls = %d, want 1", len(first.calls))
	}

	// Second invocation: helper has already cleared its marker.
	markerSet = false
	second := &recordingSignaler{}
	cfg2 := signalHydrateConfig{
		Session:  session,
		StateDir: dir,
		Client:   client,
		Signaler: second,
	}
	if err := runSignalHydrate(cfg2); err != nil {
		t.Fatalf("second invocation: %v", err)
	}
	if len(second.calls) != 0 {
		t.Errorf("second invocation SendSignal calls = %d, want 0 (idempotent)", len(second.calls))
	}
}

// TestSignalHydrate_OpenFIFOForSignalUsesNonBlockingFlags pins the
// production seam's non-blocking flag contract end-to-end through the
// state package. Validates that state.OpenFIFOForSignal — the production
// opener bundled inside state.SendHydrateSignal / state.DefaultFIFOSignaler
// — opens a real FIFO with no reader and returns ENXIO immediately rather
// than blocking. Only O_WRONLY|O_NONBLOCK produces this result on POSIX.
// The retry ladder above this layer is exercised at the state-package
// level; this test pins only the open-flag contract.
func TestSignalHydrate_OpenFIFOForSignalUsesNonBlockingFlags(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("FIFOs are not supported on Windows")
	}
	dir := t.TempDir()
	path := dir + "/no-reader.fifo"
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}

	start := time.Now()
	f, err := state.OpenFIFOForSignal(path)
	elapsed := time.Since(start)

	if f != nil {
		_ = f.Close()
		t.Fatal("state.OpenFIFOForSignal returned non-nil file with no reader; expected ENXIO")
	}
	if !errors.Is(err, syscall.ENXIO) {
		t.Fatalf("state.OpenFIFOForSignal err = %v, want syscall.ENXIO", err)
	}
	// O_NONBLOCK guarantees the call returns immediately rather than blocking
	// for a reader. 100ms is a generous upper bound.
	if elapsed >= 100*time.Millisecond {
		t.Errorf("state.OpenFIFOForSignal blocked for %v; expected ~immediate return (O_NONBLOCK missing?)", elapsed)
	}
}

// TestSignalHydrate_RunEDefersLoggerClose verifies that the cobra RunE for
// `portal state signal-hydrate` defers logger.Close() so the portal.log fd is
// released on RunE return. The production exec path is irrelevant here —
// signal-hydrate never exec's; the deferred close protects against a leaked fd
// in tests and any future caller that invokes RunE without exec'ing.
//
// The seam: signalHydrateRunFunc is replaced with a no-op that captures the
// *state.Logger from cfg. After rootCmd.Execute returns, the test calls
// Close() on the captured logger and asserts the underlying *os.File reports
// os.ErrClosed — proving the deferred Close already ran. A logger with a nil
// internal file (open-failure path) cannot prove closure, so the test sets
// PORTAL_STATE_DIR to a writable temp dir to guarantee the open succeeds.
func TestSignalHydrate_RunEDefersLoggerClose(t *testing.T) {
	t.Setenv("PORTAL_STATE_DIR", t.TempDir())

	var captured *state.Logger
	prev := signalHydrateRunFunc
	signalHydrateRunFunc = func(cfg signalHydrateConfig) error {
		captured = cfg.Logger
		return nil
	}
	t.Cleanup(func() { signalHydrateRunFunc = prev })

	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	resetRootCmd()
	resetStateCmdFlags()
	rootCmd.SetOut(outBuf)
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs([]string{"state", "signal-hydrate", "any-session"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("rootCmd.Execute: %v\nstderr: %s", err, errBuf.String())
	}

	if captured == nil {
		t.Fatal("signalHydrateRunFunc seam was not invoked; captured logger is nil")
	}

	// Calling Close on a logger whose deferred Close already ran returns
	// os.ErrClosed from the underlying *os.File. If the deferred Close had
	// not run, this Close would have been the first and returned nil.
	err := captured.Close()
	if !errors.Is(err, os.ErrClosed) {
		t.Errorf("expected logger.Close() to return os.ErrClosed (deferred Close already ran), got %v", err)
	}
}

// TestStateSignalHydrate_AcceptsLeadingDashSessionViaCobraExecute exercises
// the full cobra/pflag argv-parse path with a leading-dash session name (the
// primary failure mode that motivated the `--` end-of-flags separator).
// Without `--`, pflag treats `-dotfiles-HM9Zhw` as a short-flag cluster and
// the command exits non-zero before runSignalHydrate is reached, so no FIFO
// byte is written and the hydrate helper times out.
func TestStateSignalHydrate_AcceptsLeadingDashSessionViaCobraExecute(t *testing.T) {
	t.Run("with -- separator, leading-dash session name parses, reaches RunE, and signals FIFO", func(t *testing.T) {
		// AC #2: drive cobra Execute() against a leading-dash positional and
		// assert exit 0 + Signaler.SendSignal invoked. We install a
		// signalHydrateRunFunc seam that swaps cfg.Client / cfg.Signaler for
		// in-memory test doubles then calls the real runSignalHydrate so the
		// full enumeration → signal pipeline executes against the captured
		// cobra-parsed session.
		const sessionName = "-dotfiles-HM9Zhw"
		paneKey := state.SanitizePaneKey(sessionName, 0, 0)

		stateDir := t.TempDir()
		t.Setenv("PORTAL_STATE_DIR", stateDir)

		stubClient, _ := signalHydrateClient(t, markersOption(paneKey), [][2]int{{0, 0}})
		stubSignaler := &recordingSignaler{}

		var captured string
		prev := signalHydrateRunFunc
		signalHydrateRunFunc = func(cfg signalHydrateConfig) error {
			captured = cfg.Session
			// Replace external dependencies with test doubles, then invoke
			// the real runSignalHydrate so the SendSignal clause is exercised
			// end-to-end through the cobra Execute() entry point.
			cfg.Client = stubClient
			cfg.Signaler = stubSignaler
			return runSignalHydrate(cfg)
		}
		t.Cleanup(func() { signalHydrateRunFunc = prev })

		outBuf := new(bytes.Buffer)
		errBuf := new(bytes.Buffer)
		resetRootCmd()
		resetStateCmdFlags()
		rootCmd.SetOut(outBuf)
		rootCmd.SetErr(errBuf)
		rootCmd.SetArgs([]string{"state", "signal-hydrate", "--", sessionName})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("expected nil error (exit 0), got %v\nstderr: %s", err, errBuf.String())
		}
		if captured != sessionName {
			t.Errorf("captured session = %q, want %q", captured, sessionName)
		}
		if len(stubSignaler.calls) != 1 {
			t.Fatalf("Signaler.SendSignal calls = %d, want 1; calls=%v", len(stubSignaler.calls), stubSignaler.calls)
		}
		if wantPath := state.FIFOPath(stateDir, paneKey); stubSignaler.calls[0] != wantPath {
			t.Errorf("Signaler.SendSignal[0] = %q, want %q", stubSignaler.calls[0], wantPath)
		}
	})

	t.Run("without -- separator, leading-dash session is misparsed as short-flag cluster", func(t *testing.T) {
		// Negative sub-case: confirms the failure mode the `--` fix addresses.
		// The seam is still installed so an accidental successful parse would
		// be caught by the captured assertion below.
		var captured string
		prev := signalHydrateRunFunc
		signalHydrateRunFunc = func(cfg signalHydrateConfig) error {
			captured = cfg.Session
			return nil
		}
		t.Cleanup(func() { signalHydrateRunFunc = prev })
		t.Setenv("PORTAL_STATE_DIR", t.TempDir())

		outBuf := new(bytes.Buffer)
		errBuf := new(bytes.Buffer)
		resetRootCmd()
		resetStateCmdFlags()
		rootCmd.SetOut(outBuf)
		rootCmd.SetErr(errBuf)
		rootCmd.SetArgs([]string{"state", "signal-hydrate", "-dotfiles-HM9Zhw"})

		err := rootCmd.Execute()
		if err == nil {
			t.Fatalf("expected non-nil error from cobra argv-parse, got nil; captured = %q", captured)
		}
		if !strings.Contains(err.Error(), "unknown shorthand flag") {
			t.Errorf("error %q does not contain `unknown shorthand flag`", err.Error())
		}
		if captured != "" {
			t.Errorf("seam should not have been invoked; captured = %q", captured)
		}
	})
}
