// Tests in this file mutate package-level state (signalHydrateRunFunc) and
// MUST NOT use t.Parallel.
package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/statetest"
	"github.com/leeovery/portal/internal/tmux"
)

// markersOption renders a `show-options -sv` line for a given paneKey set so
// tests can drive ListSkeletonMarkers via recordingCommander.RunFunc.
func markersOption(paneKeys ...string) string {
	if len(paneKeys) == 0 {
		return ""
	}
	var out strings.Builder
	for i, k := range paneKeys {
		if i > 0 {
			out.WriteString("\n")
		}
		out.WriteString("@portal-skeleton-" + k + " 1")
	}
	return out.String()
}

// listPanesOutput renders a `list-panes -s -t <s> -F #{window_index}:#{pane_index}`
// reply for a list of (window, pane) tuples.
func listPanesOutput(panes [][2]int) string {
	var out strings.Builder
	for i, p := range panes {
		if i > 0 {
			out.WriteString("\n")
		}
		fmt.Fprintf(&out, "%d:%d", p[0], p[1])
	}
	return out.String()
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
	signaler := &statetest.RecordingFIFOSignaler{}

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
	if len(signaler.Calls) != len(wantPaths) {
		t.Fatalf("SendSignal call count = %d, want %d (calls=%v)", len(signaler.Calls), len(wantPaths), signaler.Calls)
	}
	for _, path := range signaler.Calls {
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
	signaler := &statetest.RecordingFIFOSignaler{}

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
	if len(signaler.Calls) != 1 {
		t.Fatalf("SendSignal call count = %d, want 1 (only marked pane); calls=%v", len(signaler.Calls), signaler.Calls)
	}
	if signaler.Calls[0] != wantPath {
		t.Errorf("SendSignal[0] = %q, want %q", signaler.Calls[0], wantPath)
	}
}

func TestSignalHydrate_ZeroMarkersIsNoOp(t *testing.T) {
	dir := t.TempDir()
	session := "foo"
	panes := [][2]int{{0, 0}, {0, 1}}

	// markersRaw is empty — no skeleton markers anywhere on the server.
	client, _ := signalHydrateClient(t, "", panes)
	signaler := &statetest.RecordingFIFOSignaler{}

	cfg := signalHydrateConfig{
		Session:  session,
		StateDir: dir,
		Client:   client,
		Signaler: signaler,
	}
	if err := runSignalHydrate(cfg); err != nil {
		t.Fatalf("runSignalHydrate: %v", err)
	}
	if len(signaler.Calls) != 0 {
		t.Errorf("SendSignal called %d times under zero markers, want 0", len(signaler.Calls))
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
	signaler := &statetest.RecordingFIFOSignaler{
		Err: errors.New("retry exhausted (sentinel)"),
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
		Signaler: &statetest.RecordingFIFOSignaler{},
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
	signaler := &statetest.RecordingFIFOSignaler{}

	cfg := signalHydrateConfig{
		Session:  session,
		StateDir: dir,
		Client:   client,
		Signaler: signaler,
	}
	if err := runSignalHydrate(cfg); err != nil {
		t.Fatalf("runSignalHydrate must soft-fail on missing session, got %v", err)
	}
	if len(signaler.Calls) != 0 {
		t.Errorf("SendSignal called %d times when session missing, want 0", len(signaler.Calls))
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

	first := &statetest.RecordingFIFOSignaler{}
	cfg := signalHydrateConfig{
		Session:  session,
		StateDir: dir,
		Client:   client,
		Signaler: first,
	}
	if err := runSignalHydrate(cfg); err != nil {
		t.Fatalf("first invocation: %v", err)
	}
	if len(first.Calls) != 1 {
		t.Errorf("first invocation SendSignal calls = %d, want 1", len(first.Calls))
	}

	// Second invocation: helper has already cleared its marker.
	markerSet = false
	second := &statetest.RecordingFIFOSignaler{}
	cfg2 := signalHydrateConfig{
		Session:  session,
		StateDir: dir,
		Client:   client,
		Signaler: second,
	}
	if err := runSignalHydrate(cfg2); err != nil {
		t.Fatalf("second invocation: %v", err)
	}
	if len(second.Calls) != 0 {
		t.Errorf("second invocation SendSignal calls = %d, want 0 (idempotent)", len(second.Calls))
	}
}

// TestSignalHydrate_WARNsRenderUnderSignalComponent pins the cycle-3 signal-vs-
// hydrate attribution decision (task 9-2, option a): runSignalHydrate's three
// enumeration/per-FIFO WARNs render under component=signal — matching the
// structural sibling EagerSignalHydrate — so `grep "signal:"` reconstructs the
// hook-driven FIFO-signaling path. They must NOT render under component=hydrate
// (the command's process_role stays hydrate; the subsystem component is signal —
// the two attrs are orthogonal).
//
// cfg.Logger is left nil deliberately so the test exercises the PRODUCTION
// default (signalLoggerOrDefault → the package-level signalLogger bound via
// log.For("signal")), not a test-injected component binding. log.SetTestHandler
// re-points the shared handler indirection at a logtest.Sink so the cached
// signalLogger's records are captured in-process — this is what would have
// false-passed had we injected a pre-bound logger, so it is load-bearing.
func TestSignalHydrate_WARNsRenderUnderSignalComponent(t *testing.T) {
	cases := []struct {
		name    string
		runFunc func(args ...string) (string, error)
		signal  state.FIFOSignaler
		wantMsg string
	}{
		{
			name: "list skeleton markers failed",
			runFunc: func(args ...string) (string, error) {
				if len(args) > 0 && args[0] == "show-options" {
					return "", errors.New("show-options boom")
				}
				return "", fmt.Errorf("unexpected tmux call: %v", args)
			},
			signal:  &statetest.RecordingFIFOSignaler{},
			wantMsg: "list skeleton markers failed",
		},
		{
			name: "list panes for session failed",
			runFunc: func(args ...string) (string, error) {
				switch args[0] {
				case "show-options":
					return markersOption(state.SanitizePaneKey("foo", 0, 0)), nil
				case "list-panes":
					return "", errors.New("can't find session: foo")
				}
				return "", fmt.Errorf("unexpected tmux call: %v", args)
			},
			signal:  &statetest.RecordingFIFOSignaler{},
			wantMsg: "list panes for session failed",
		},
		{
			name: "write fifo failed",
			runFunc: func(args ...string) (string, error) {
				switch args[0] {
				case "show-options":
					return markersOption(state.SanitizePaneKey("foo", 0, 0)), nil
				case "list-panes":
					return listPanesOutput([][2]int{{0, 0}}), nil
				}
				return "", fmt.Errorf("unexpected tmux call: %v", args)
			},
			signal:  &statetest.RecordingFIFOSignaler{Err: errors.New("retry exhausted (sentinel)")},
			wantMsg: "write fifo failed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sink := &logtest.Sink{}
			log.SetTestHandler(t, sink)

			cfg := signalHydrateConfig{
				Session:  "foo",
				StateDir: t.TempDir(),
				Client:   tmux.NewClient(&recordingCommander{RunFunc: tc.runFunc}),
				// Logger intentionally nil — exercise the production default.
				Signaler: tc.signal,
			}
			if err := runSignalHydrate(cfg); err != nil {
				t.Fatalf("runSignalHydrate: %v", err)
			}
			assertSignalComponentWARN(t, sink, tc.wantMsg)
		})
	}
}

// assertSignalComponentWARN asserts the sink captured a WARN line carrying the
// given message under component=signal and NOT under component=hydrate.
func assertSignalComponentWARN(t *testing.T, sink *logtest.Sink, msg string) {
	t.Helper()
	body := sink.Body()
	if !strings.Contains(body, msg) {
		t.Fatalf("expected a WARN %q line; body=%q", msg, body)
	}
	matched := 0
	for _, line := range sink.Lines() {
		if !strings.Contains(line, msg) {
			continue
		}
		matched++
		if !strings.HasPrefix(line, "WARN ") {
			t.Errorf("%q line is not WARN-level: %q", msg, line)
		}
		if !strings.Contains(line, "component=signal") {
			t.Errorf("%q must render under component=signal; got %q", msg, line)
		}
		if strings.Contains(line, "component=hydrate") {
			t.Errorf("%q must NOT render under component=hydrate; got %q", msg, line)
		}
	}
	if matched == 0 {
		t.Fatalf("no captured line contained %q; body=%q", msg, body)
	}
}

// NOTE: The former TestSignalHydrate_RunEDefersLoggerClose was removed in the
// observability migration. The signal-hydrate RunE no longer opens or closes a
// per-process file-backed logger — logging is owned by internal/log's handler
// (configured once via main -> log.Init), so there is no per-helper fd to
// defer-close. The behaviour it asserted no longer exists.

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
		stubSignaler := &statetest.RecordingFIFOSignaler{}

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
		if len(stubSignaler.Calls) != 1 {
			t.Fatalf("Signaler.SendSignal calls = %d, want 1; calls=%v", len(stubSignaler.Calls), stubSignaler.Calls)
		}
		if wantPath := state.FIFOPath(stateDir, paneKey); stubSignaler.Calls[0] != wantPath {
			t.Errorf("Signaler.SendSignal[0] = %q, want %q", stubSignaler.Calls[0], wantPath)
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
