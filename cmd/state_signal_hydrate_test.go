// Tests in this file mutate package-level state (signalHydrateRunFunc) and
// MUST NOT use t.Parallel.
package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// fakeSleep records every duration the production code passes to time.Sleep.
type fakeSleep struct {
	Durations []time.Duration
}

func (s *fakeSleep) fn() func(time.Duration) {
	return func(d time.Duration) { s.Durations = append(s.Durations, d) }
}

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

func TestSignalHydrate_WritesOneByteToEachSkeletonMarkedPane(t *testing.T) {
	dir := t.TempDir()
	session := "foo"
	panes := [][2]int{{0, 0}, {0, 1}}
	keys := []string{
		state.SanitizePaneKey(session, 0, 0),
		state.SanitizePaneKey(session, 0, 1),
	}

	client, _ := signalHydrateClient(t, markersOption(keys...), panes)

	// Build a real reader-writer pipe per FIFO so the writer's Write succeeds
	// and the byte is observable on the reader side.
	type pipePair struct {
		r, w *os.File
	}
	pipes := map[string]pipePair{}
	for _, k := range keys {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("os.Pipe: %v", err)
		}
		pipes[state.FIFOPath(dir, k)] = pipePair{r: r, w: w}
	}
	t.Cleanup(func() {
		for _, p := range pipes {
			_ = p.r.Close()
			_ = p.w.Close()
		}
	})

	open := func(path string) (*os.File, error) {
		p, ok := pipes[path]
		if !ok {
			t.Fatalf("unexpected FIFO path: %s", path)
		}
		return p.w, nil
	}

	cfg := signalHydrateConfig{
		Session:  session,
		StateDir: dir,
		Client:   client,
		OpenFIFO: open,
		Sleep:    func(time.Duration) {},
	}
	if err := runSignalHydrate(cfg); err != nil {
		t.Fatalf("runSignalHydrate: %v", err)
	}

	// Verify each reader saw exactly one byte.
	for path, pp := range pipes {
		_ = pp.w.Close()
		buf := make([]byte, 8)
		n, _ := pp.r.Read(buf)
		if n != 1 {
			t.Errorf("pane %s read %d bytes, want 1", path, n)
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

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})

	openCalls := 0
	open := func(path string) (*os.File, error) {
		openCalls++
		want := state.FIFOPath(dir, markedKey)
		if path != want {
			t.Errorf("opened FIFO %s, want %s", path, want)
		}
		return w, nil
	}

	cfg := signalHydrateConfig{
		Session:  session,
		StateDir: dir,
		Client:   client,
		OpenFIFO: open,
		Sleep:    func(time.Duration) {},
	}
	if err := runSignalHydrate(cfg); err != nil {
		t.Fatalf("runSignalHydrate: %v", err)
	}
	if openCalls != 1 {
		t.Errorf("OpenFIFO called %d times, want 1 (only marked pane)", openCalls)
	}
}

func TestSignalHydrate_ZeroMarkersOpensNoFIFO(t *testing.T) {
	dir := t.TempDir()
	session := "foo"
	panes := [][2]int{{0, 0}, {0, 1}}

	// markersRaw is empty — no skeleton markers anywhere on the server.
	client, _ := signalHydrateClient(t, "", panes)

	openCalls := 0
	cfg := signalHydrateConfig{
		Session:  session,
		StateDir: dir,
		Client:   client,
		OpenFIFO: func(_ string) (*os.File, error) {
			openCalls++
			return nil, nil
		},
		Sleep: func(time.Duration) {},
	}
	if err := runSignalHydrate(cfg); err != nil {
		t.Fatalf("runSignalHydrate: %v", err)
	}
	if openCalls != 0 {
		t.Errorf("OpenFIFO called %d times, want 0", openCalls)
	}
}

func TestSignalHydrate_RetriesOnENXIOWithFullDelayLadder(t *testing.T) {
	dir := t.TempDir()
	session := "foo"
	key := state.SanitizePaneKey(session, 0, 0)
	panes := [][2]int{{0, 0}}

	client, _ := signalHydrateClient(t, markersOption(key), panes)

	// Open returns ENXIO twice, then a writable pipe end on the third call.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})

	openCalls := 0
	open := func(path string) (*os.File, error) {
		openCalls++
		switch openCalls {
		case 1, 2:
			return nil, syscall.ENXIO
		default:
			return w, nil
		}
	}

	sleep := &fakeSleep{}
	cfg := signalHydrateConfig{
		Session:  session,
		StateDir: dir,
		Client:   client,
		OpenFIFO: open,
		Sleep:    sleep.fn(),
	}
	if err := runSignalHydrate(cfg); err != nil {
		t.Fatalf("runSignalHydrate: %v", err)
	}

	if openCalls != 3 {
		t.Errorf("OpenFIFO calls = %d, want 3", openCalls)
	}
	want := []time.Duration{
		signalHydrateRetryDelays[0], // 10ms
		signalHydrateRetryDelays[1], // 20ms
	}
	if !reflect.DeepEqual(sleep.Durations, want) {
		t.Errorf("Sleep durations = %v, want %v", sleep.Durations, want)
	}
}

func TestSignalHydrate_RetriesOnEAGAINSameAsENXIO(t *testing.T) {
	dir := t.TempDir()
	session := "foo"
	key := state.SanitizePaneKey(session, 0, 0)
	panes := [][2]int{{0, 0}}

	client, _ := signalHydrateClient(t, markersOption(key), panes)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})

	openCalls := 0
	open := func(_ string) (*os.File, error) {
		openCalls++
		if openCalls == 1 {
			return nil, syscall.EAGAIN
		}
		return w, nil
	}

	sleep := &fakeSleep{}
	cfg := signalHydrateConfig{
		Session:  session,
		StateDir: dir,
		Client:   client,
		OpenFIFO: open,
		Sleep:    sleep.fn(),
	}
	if err := runSignalHydrate(cfg); err != nil {
		t.Fatalf("runSignalHydrate: %v", err)
	}
	if openCalls != 2 {
		t.Errorf("OpenFIFO calls = %d, want 2", openCalls)
	}
	want := []time.Duration{signalHydrateRetryDelays[0]}
	if !reflect.DeepEqual(sleep.Durations, want) {
		t.Errorf("Sleep durations = %v, want %v", sleep.Durations, want)
	}
}

func TestSignalHydrate_DoesNotRetryOnENOENT(t *testing.T) {
	dir := t.TempDir()
	session := "foo"
	key := state.SanitizePaneKey(session, 0, 0)
	panes := [][2]int{{0, 0}}

	client, _ := signalHydrateClient(t, markersOption(key), panes)

	openCalls := 0
	open := func(_ string) (*os.File, error) {
		openCalls++
		return nil, syscall.ENOENT
	}

	sleep := &fakeSleep{}
	cfg := signalHydrateConfig{
		Session:  session,
		StateDir: dir,
		Client:   client,
		OpenFIFO: open,
		Sleep:    sleep.fn(),
	}
	if err := runSignalHydrate(cfg); err != nil {
		t.Fatalf("runSignalHydrate: %v (must be soft-fail on ENOENT)", err)
	}
	if openCalls != 1 {
		t.Errorf("OpenFIFO calls = %d, want 1 (no retry on ENOENT)", openCalls)
	}
	if len(sleep.Durations) != 0 {
		t.Errorf("Sleep called %v times on ENOENT, want 0", len(sleep.Durations))
	}
}

func TestSignalHydrate_RetryExhaustionDoesNotUnsetMarker(t *testing.T) {
	dir := t.TempDir()
	session := "foo"
	key := state.SanitizePaneKey(session, 0, 0)
	panes := [][2]int{{0, 0}}

	client, cmder := signalHydrateClient(t, markersOption(key), panes)

	openCalls := 0
	open := func(_ string) (*os.File, error) {
		openCalls++
		return nil, syscall.ENXIO
	}
	sleep := &fakeSleep{}

	cfg := signalHydrateConfig{
		Session:  session,
		StateDir: dir,
		Client:   client,
		OpenFIFO: open,
		Sleep:    sleep.fn(),
	}
	if err := runSignalHydrate(cfg); err != nil {
		t.Fatalf("runSignalHydrate must soft-fail on retry exhaustion, got %v", err)
	}

	// 7 attempts: initial + 6 retries.
	if openCalls != 7 {
		t.Errorf("OpenFIFO calls = %d, want 7 (initial + 6 retries)", openCalls)
	}
	// 6 sleeps, one before each retry.
	if len(sleep.Durations) != 6 {
		t.Errorf("Sleep called %d times, want 6", len(sleep.Durations))
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

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})

	cfg := signalHydrateConfig{
		Session:  session,
		StateDir: dir,
		Client:   client,
		OpenFIFO: func(_ string) (*os.File, error) { return w, nil },
		Sleep:    func(time.Duration) {},
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

	openCalls := 0
	cfg := signalHydrateConfig{
		Session:  session,
		StateDir: dir,
		Client:   client,
		OpenFIFO: func(_ string) (*os.File, error) {
			openCalls++
			return nil, nil
		},
		Sleep: func(time.Duration) {},
	}
	if err := runSignalHydrate(cfg); err != nil {
		t.Fatalf("runSignalHydrate must soft-fail on missing session, got %v", err)
	}
	if openCalls != 0 {
		t.Errorf("OpenFIFO called %d times when session missing, want 0", openCalls)
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

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})

	firstOpens := 0
	cfg := signalHydrateConfig{
		Session:  session,
		StateDir: dir,
		Client:   client,
		OpenFIFO: func(_ string) (*os.File, error) {
			firstOpens++
			return w, nil
		},
		Sleep: func(time.Duration) {},
	}
	if err := runSignalHydrate(cfg); err != nil {
		t.Fatalf("first invocation: %v", err)
	}
	if firstOpens != 1 {
		t.Errorf("first invocation opens = %d, want 1", firstOpens)
	}

	// Second invocation: helper has already cleared its marker.
	markerSet = false
	secondOpens := 0
	cfg2 := signalHydrateConfig{
		Session:  session,
		StateDir: dir,
		Client:   client,
		OpenFIFO: func(_ string) (*os.File, error) {
			secondOpens++
			return w, nil
		},
		Sleep: func(time.Duration) {},
	}
	if err := runSignalHydrate(cfg2); err != nil {
		t.Fatalf("second invocation: %v", err)
	}
	if secondOpens != 0 {
		t.Errorf("second invocation opens = %d, want 0 (idempotent)", secondOpens)
	}
}

func TestSignalHydrate_OpenFIFOForSignalUsesNonBlockingFlags(t *testing.T) {
	// Validate the production seam by inspecting its observable behavior:
	// open a real FIFO with no reader and verify openFIFOForSignal returns
	// ENXIO immediately rather than blocking. Only O_WRONLY|O_NONBLOCK
	// produces this result on POSIX.
	if runtime.GOOS == "windows" {
		t.Skip("FIFOs are not supported on Windows")
	}
	dir := t.TempDir()
	path := dir + "/no-reader.fifo"
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}

	start := time.Now()
	f, err := openFIFOForSignal(path)
	elapsed := time.Since(start)

	if f != nil {
		_ = f.Close()
		t.Fatal("openFIFOForSignal returned non-nil file with no reader; expected ENXIO")
	}
	if !errors.Is(err, syscall.ENXIO) {
		t.Fatalf("openFIFOForSignal err = %v, want syscall.ENXIO", err)
	}
	// O_NONBLOCK guarantees the call returns immediately rather than blocking
	// for a reader. 100ms is a generous upper bound.
	if elapsed >= 100*time.Millisecond {
		t.Errorf("openFIFOForSignal blocked for %v; expected ~immediate return (O_NONBLOCK missing?)", elapsed)
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
	t.Run("with -- separator, leading-dash session name parses, reaches RunE, and writes FIFO byte", func(t *testing.T) {
		// AC #2: drive cobra Execute() against a leading-dash positional and
		// assert exit 0 + FIFO byte written. We install a signalHydrateRunFunc
		// seam that swaps cfg.Client / cfg.OpenFIFO for in-memory test doubles
		// then calls the real runSignalHydrate so the full enumeration → open
		// → write pipeline executes against the captured cobra-parsed session.
		const sessionName = "-dotfiles-HM9Zhw"
		paneKey := state.SanitizePaneKey(sessionName, 0, 0)

		stateDir := t.TempDir()
		t.Setenv("PORTAL_STATE_DIR", stateDir)

		// Stub FIFO: hand back the writable end of an os.Pipe so the byte
		// runSignalHydrate writes is observable on the read end.
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("os.Pipe: %v", err)
		}
		t.Cleanup(func() {
			_ = r.Close()
			_ = w.Close()
		})

		var openedPath string
		openedCalls := 0
		stubOpen := func(path string) (*os.File, error) {
			openedCalls++
			openedPath = path
			return w, nil
		}

		stubClient, _ := signalHydrateClient(t, markersOption(paneKey), [][2]int{{0, 0}})

		var captured string
		prev := signalHydrateRunFunc
		signalHydrateRunFunc = func(cfg signalHydrateConfig) error {
			captured = cfg.Session
			// Replace external dependencies with test doubles, then invoke
			// the real runSignalHydrate so the FIFO-write clause is exercised
			// end-to-end through the cobra Execute() entry point.
			cfg.Client = stubClient
			cfg.OpenFIFO = stubOpen
			cfg.Sleep = func(time.Duration) {}
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
		if openedCalls != 1 {
			t.Errorf("OpenFIFO calls = %d, want 1", openedCalls)
		}
		if wantPath := state.FIFOPath(stateDir, paneKey); openedPath != wantPath {
			t.Errorf("OpenFIFO path = %q, want %q", openedPath, wantPath)
		}

		// Verify the FIFO seam received exactly the production payload (one
		// byte). Close the writer so Read sees EOF after draining the byte.
		_ = w.Close()
		buf := make([]byte, 8)
		n, _ := r.Read(buf)
		if n != 1 {
			t.Errorf("FIFO read %d bytes, want 1", n)
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

func TestSignalHydrate_CompletesWithin500msCumulativeBudget(t *testing.T) {
	dir := t.TempDir()
	session := "foo"
	key := state.SanitizePaneKey(session, 0, 0)
	panes := [][2]int{{0, 0}}

	client, _ := signalHydrateClient(t, markersOption(key), panes)

	open := func(_ string) (*os.File, error) {
		return nil, syscall.ENXIO
	}
	sleep := &fakeSleep{}

	cfg := signalHydrateConfig{
		Session:  session,
		StateDir: dir,
		Client:   client,
		OpenFIFO: open,
		Sleep:    sleep.fn(),
	}
	if err := runSignalHydrate(cfg); err != nil {
		t.Fatalf("runSignalHydrate: %v", err)
	}

	var total time.Duration
	for _, d := range sleep.Durations {
		total += d
	}
	const budget = 500 * time.Millisecond
	if total > budget {
		t.Errorf("cumulative Sleep budget exceeded: %v > %v (durations: %v)", total, budget, sleep.Durations)
	}
}
