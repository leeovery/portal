package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/statetest"
	"github.com/leeovery/portal/internal/tmux"
)

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

// signalHydrateClient replies to the tmux calls signal-hydrate makes, finding no
// pane waiting on its resume panel; anything else fails the test.
func signalHydrateClient(t *testing.T, markersRaw string, panes [][2]int) (*tmux.Client, *commandertest.Scripted) {
	t.Helper()
	cmder := commandertest.New(t,
		noWaitingPanes(),
		commandertest.Returns(markersRaw, "show-options"),
		commandertest.Returns(listPanesOutput(panes), "list-panes"),
	)
	return tmux.NewClient(cmder), cmder
}

// isWaitingPanesRead matches the repaint's list-panes, told apart from
// hydration's by the resume-pending marker its format reads.
func isWaitingPanesRead(args []string) bool {
	return commandertest.ArgvPrefix("list-panes")(args) &&
		slices.ContainsFunc(args, func(a string) bool { return strings.Contains(a, state.ResumePendingOption) })
}

// noWaitingPanes scripts the repaint's read to find no pane waiting. It goes
// ahead of any list-panes entry hydration's read is scripted by.
func noWaitingPanes() commandertest.Entry {
	return commandertest.When(isWaitingPanesRead, "", nil)
}

// repaintRecorder stands in for the process-group signal, so no test signals a
// process it did not spawn.
type repaintRecorder struct {
	pids []int
	errs map[int]error
}

func (r *repaintRecorder) repaint(pid int) error {
	r.pids = append(r.pids, pid)
	return r.errs[pid]
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

	// signal-hydrate must never touch markers: the hydrate helper owns the unset.
	for _, c := range cmder.Calls() {
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

	for _, c := range cmder.Calls() {
		if len(c) >= 2 && c[0] == "set-option" && c[1] == "-su" {
			t.Errorf("signal-hydrate issued set-option -su (forbidden): %v", c)
		}
	}
}

func TestSignalHydrate_SoftFailsWhenSessionDoesNotExist(t *testing.T) {
	dir := t.TempDir()
	session := "ghost"

	cmder := commandertest.New(t,
		commandertest.Returns("", "show-options"),
		commandertest.Fails(errors.New("can't find session: ghost"), "list-panes"),
	)
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

	var markerSet = true
	// The marker entry's predicate is evaluated per call, so clearing markerSet
	// between invocations retires it in favour of the empty read below it.
	cmder := commandertest.New(t,
		commandertest.When(func(args []string) bool {
			return markerSet && commandertest.ArgvPrefix("show-options")(args)
		}, markersOption(key), nil),
		commandertest.Returns("", "show-options"),
		noWaitingPanes(),
		commandertest.Returns(listPanesOutput(panes), "list-panes"),
	)
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

// cfg.Logger is left nil deliberately so the production default binding is what
// emits; injecting a pre-bound logger would false-pass this.
func TestSignalHydrate_WARNsRenderUnderSignalComponent(t *testing.T) {
	cases := []struct {
		name    string
		script  []commandertest.Entry
		signal  state.FIFOSignaler
		repaint *repaintRecorder
		wantMsg string
	}{
		{
			name: "list skeleton markers failed",
			script: []commandertest.Entry{
				commandertest.Fails(errors.New("show-options boom"), "show-options"),
				noWaitingPanes(),
			},
			signal:  &statetest.RecordingFIFOSignaler{},
			wantMsg: "list skeleton markers failed",
		},
		{
			name: "list panes for session failed",
			script: []commandertest.Entry{
				commandertest.Returns(markersOption(state.SanitizePaneKey("foo", 0, 0)), "show-options"),
				noWaitingPanes(),
				commandertest.Fails(errors.New("can't find session: foo"), "list-panes"),
			},
			signal:  &statetest.RecordingFIFOSignaler{},
			wantMsg: "list panes for session failed",
		},
		{
			name: "write fifo failed",
			script: []commandertest.Entry{
				commandertest.Returns(markersOption(state.SanitizePaneKey("foo", 0, 0)), "show-options"),
				noWaitingPanes(),
				commandertest.Returns(listPanesOutput([][2]int{{0, 0}}), "list-panes"),
			},
			signal:  &statetest.RecordingFIFOSignaler{Err: errors.New("retry exhausted (sentinel)")},
			wantMsg: "write fifo failed",
		},
		{
			name: "list waiting panes failed",
			script: []commandertest.Entry{
				commandertest.When(isWaitingPanesRead, "", errors.New("can't find session: foo")),
				commandertest.Returns("", "show-options"),
				commandertest.Returns(listPanesOutput([][2]int{{0, 0}}), "list-panes"),
			},
			signal:  &statetest.RecordingFIFOSignaler{},
			wantMsg: "list waiting panes failed",
		},
		{
			name: "repaint waiting pane failed",
			script: []commandertest.Entry{
				commandertest.When(isWaitingPanesRead, "1|4906|%4", nil),
				commandertest.Returns("", "show-options"),
				commandertest.Returns(listPanesOutput([][2]int{{0, 0}}), "list-panes"),
			},
			signal:  &statetest.RecordingFIFOSignaler{},
			repaint: &repaintRecorder{errs: map[int]error{4906: errors.New("operation not permitted")}},
			wantMsg: "repaint waiting pane failed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sink := logtest.Install(t)

			repaint := tc.repaint
			if repaint == nil {
				repaint = &repaintRecorder{}
			}
			cfg := signalHydrateConfig{
				Session:  "foo",
				StateDir: t.TempDir(),
				Client:   tmux.NewClient(commandertest.New(t, tc.script...)),
				Signaler: tc.signal,
				Repaint:  repaint.repaint,
			}
			if err := runSignalHydrate(cfg); err != nil {
				t.Fatalf("runSignalHydrate: %v", err)
			}
			assertSignalComponentWARN(t, sink, tc.wantMsg)
		})
	}
}

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

// Without the `--` end-of-flags separator, pflag reads a leading-dash session
// name as a short-flag cluster and the command exits before runSignalHydrate is
// reached, so no FIFO byte is ever written.
func TestStateSignalHydrate_AcceptsLeadingDashSessionViaCobraExecute(t *testing.T) {
	t.Run("with -- separator, leading-dash session name parses, reaches RunE, and signals FIFO", func(t *testing.T) {
		const sessionName = "-dotfiles-HM9Zhw"
		paneKey := state.SanitizePaneKey(sessionName, 0, 0)

		stateDir := t.TempDir()
		t.Setenv("PORTAL_STATE_DIR", stateDir)

		stubClient, _ := signalHydrateClient(t, markersOption(paneKey), [][2]int{{0, 0}})
		stubSignaler := &statetest.RecordingFIFOSignaler{}

		var captured string
		withFuncSeam(t, &signalHydrateRunFunc, func(cfg signalHydrateConfig) error {
			captured = cfg.Session
			cfg.Client = stubClient
			cfg.Signaler = stubSignaler
			cfg.Repaint = (&repaintRecorder{}).repaint
			return runSignalHydrate(cfg)
		})

		outBuf := new(bytes.Buffer)
		errBuf := new(bytes.Buffer)
		resetRootCmd()
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
		// The seam stays installed so an accidental successful parse surfaces here.
		var captured string
		withFuncSeam(t, &signalHydrateRunFunc, func(cfg signalHydrateConfig) error {
			captured = cfg.Session
			return nil
		})
		t.Setenv("PORTAL_STATE_DIR", t.TempDir())

		outBuf := new(bytes.Buffer)
		errBuf := new(bytes.Buffer)
		resetRootCmd()
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

func TestSignalHydrate_RepaintsEveryWaitingPaneOfTheSession(t *testing.T) {
	cmder := commandertest.New(t,
		commandertest.When(isWaitingPanesRead, "1|4906|%4\n|5000|%5\n1|6100|%6", nil),
		commandertest.Returns("", "show-options"),
		commandertest.Returns(listPanesOutput([][2]int{{0, 0}, {0, 1}, {1, 0}}), "list-panes"),
	)
	repaint := &repaintRecorder{}

	cfg := signalHydrateConfig{
		Session:  "foo",
		StateDir: t.TempDir(),
		Client:   tmux.NewClient(cmder),
		Signaler: &statetest.RecordingFIFOSignaler{},
		Repaint:  repaint.repaint,
	}
	if err := runSignalHydrate(cfg); err != nil {
		t.Fatalf("runSignalHydrate: %v", err)
	}

	if want := []int{4906, 6100}; !slices.Equal(repaint.pids, want) {
		t.Errorf("repainted pids = %v, want %v: every waiting pane, and no pane that is not", repaint.pids, want)
	}
	reads := cmder.CallsMatching("list-panes", state.ResumePendingOption)
	if len(reads) != 1 {
		t.Fatalf("waiting-pane reads = %d, want 1", len(reads))
	}
	if !slices.Contains(reads[0].Argv, string(tmux.CoordTargetExact("foo"))) {
		t.Errorf("waiting-pane read = %q, want it scoped to the attached session by its exact target", reads[0].Argv)
	}
}

func TestSignalHydrate_RepaintsWhenHydrationCannotRead(t *testing.T) {
	cmder := commandertest.New(t,
		commandertest.When(isWaitingPanesRead, "1|4906|%4", nil),
		commandertest.Fails(errors.New("show-options boom"), "show-options"),
	)
	repaint := &repaintRecorder{}

	cfg := signalHydrateConfig{
		Session:  "foo",
		StateDir: t.TempDir(),
		Client:   tmux.NewClient(cmder),
		Signaler: &statetest.RecordingFIFOSignaler{},
		Repaint:  repaint.repaint,
	}
	if err := runSignalHydrate(cfg); err != nil {
		t.Fatalf("runSignalHydrate: %v", err)
	}
	if want := []int{4906}; !slices.Equal(repaint.pids, want) {
		t.Errorf("repainted pids = %v, want %v: a failed skeleton read must not cost the repaint", repaint.pids, want)
	}
}

func TestSignalHydrate_KeepsRepaintingPastAFailedPane(t *testing.T) {
	cmder := commandertest.New(t,
		commandertest.When(isWaitingPanesRead, "1|4906|%4\n1|6100|%6", nil),
		commandertest.Returns("", "show-options"),
		commandertest.Returns(listPanesOutput([][2]int{{0, 0}, {0, 1}}), "list-panes"),
	)
	repaint := &repaintRecorder{errs: map[int]error{4906: errors.New("operation not permitted")}}

	cfg := signalHydrateConfig{
		Session:  "foo",
		StateDir: t.TempDir(),
		Client:   tmux.NewClient(cmder),
		Signaler: &statetest.RecordingFIFOSignaler{},
		Repaint:  repaint.repaint,
	}
	if err := runSignalHydrate(cfg); err != nil {
		t.Fatalf("runSignalHydrate must soft-fail on a refused repaint, got %v", err)
	}
	if want := []int{4906, 6100}; !slices.Equal(repaint.pids, want) {
		t.Errorf("repainted pids = %v, want %v", repaint.pids, want)
	}
}

func TestRepaintPaneGroup_RefusesAGroupItMustNeverSignal(t *testing.T) {
	// kill(-1) signals every process the caller may signal, and kill(0) the
	// caller's own group: neither is a pane.
	for _, pid := range []int{-5, 0, 1} {
		if err := repaintPaneGroup(pid); err == nil {
			t.Errorf("repaintPaneGroup(%d) error = nil, want a refusal", pid)
		}
	}
}

func TestRepaintPaneGroup_SignalsTheGroupWithSIGWINCH(t *testing.T) {
	// The test owns this group: a child it starts as a group leader, with
	// SIGWINCH caught by a shell that reports it and exits.
	dir := t.TempDir()
	child := spawnWinchReporter(t, dir)

	if err := repaintPaneGroup(child.Process.Pid); err != nil {
		t.Fatalf("repaintPaneGroup: %v", err)
	}
	awaitWinchReport(t, child, dir)
}

// spawnWinchReporter starts a shell as the leader of its own process group,
// standing in for a parked pane: it records a SIGWINCH to a file and exits. It
// returns once the trap is installed, since a SIGWINCH before that is ignored.
func spawnWinchReporter(t *testing.T, dir string) *exec.Cmd {
	t.Helper()
	script := `trap 'echo winch > "$1/got"; exit 0' WINCH; : > "$1/ready"; while :; do sleep 0.02; done`
	child := exec.Command("/bin/sh", "-c", script, "sh", dir)
	child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := child.Start(); err != nil {
		t.Fatalf("start the stand-in pane process: %v", err)
	}
	t.Cleanup(func() {
		_ = child.Process.Signal(syscall.SIGKILL)
		_ = child.Wait()
	})
	if !harnesstest.PollUntil(t, 5*time.Second, 10*time.Millisecond, func() bool {
		_, err := os.Stat(filepath.Join(dir, "ready"))
		return err == nil
	}) {
		t.Fatal("the stand-in pane process never installed its SIGWINCH trap")
	}
	return child
}

func awaitWinchReport(t *testing.T, child *exec.Cmd, dir string) {
	t.Helper()
	if !harnesstest.PollUntil(t, 5*time.Second, 10*time.Millisecond, func() bool {
		_, err := os.Stat(filepath.Join(dir, "got"))
		return err == nil
	}) {
		t.Fatal("the stand-in pane process's group never received SIGWINCH")
	}
	_ = child.Wait()
}
