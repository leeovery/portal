package cmd

// Tests in this file mutate package-level state (bootstrapDeps, spawnDeps) and MUST NOT use t.Parallel.

import (
	"bytes"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/spawntest"
)

// fakeTerminalDetector is a fake TerminalDetector that returns a fixed
// Identity, letting the spawn command's --detect branch be Executed without
// real tmux, ps, or defaults reads.
type fakeTerminalDetector struct {
	id spawn.Identity
}

func (f fakeTerminalDetector) Detect() spawn.Identity {
	return f.id
}

func TestSpawnCommand(t *testing.T) {
	// nopRunner short-circuits PersistentPreRunE so no real tmux server is
	// dialed. spawn is intentionally NOT in skipTmuxCheck, so this bootstrap
	// injection is load-bearing (TestMain poisons TMUX; a missed injection
	// would fail loudly instead of reaching the developer's real server).
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	t.Run("it prints the friendly name and exact bundle id on --detect for a resolved terminal", func(t *testing.T) {
		spawnDeps = &SpawnDeps{Detector: fakeTerminalDetector{
			id: spawn.Identity{Name: "Ghostty", BundleID: "com.mitchellh.ghostty"},
		}}
		t.Cleanup(func() { spawnDeps = nil })

		resetRootCmd()
		buf := new(bytes.Buffer)
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"spawn", "--detect"})

		err := rootCmd.Execute()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "Ghostty") {
			t.Errorf("output %q missing friendly name %q", out, "Ghostty")
		}
		if !strings.Contains(out, "com.mitchellh.ghostty") {
			t.Errorf("output %q missing bundle id %q", out, "com.mitchellh.ghostty")
		}
	})

	t.Run("it prints the honest no-host-local-terminal line on --detect for a NULL identity", func(t *testing.T) {
		spawnDeps = &SpawnDeps{Detector: fakeTerminalDetector{id: spawn.Identity{}}}
		t.Cleanup(func() { spawnDeps = nil })

		resetRootCmd()
		buf := new(bytes.Buffer)
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"spawn", "--detect"})

		err := rootCmd.Execute()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "no host-local terminal") {
			t.Errorf("output %q missing honest NULL line containing %q", out, "no host-local terminal")
		}
	})

	t.Run("it returns a UsageError when no sessions and no --detect are given", func(t *testing.T) {
		spawnDeps = &SpawnDeps{Detector: fakeTerminalDetector{}}
		t.Cleanup(func() { spawnDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"spawn"})

		err := rootCmd.Execute()

		if err == nil {
			t.Fatal("expected a UsageError, got nil")
		}
		var usageErr *UsageError
		if !errors.As(err, &usageErr) {
			t.Errorf("error %v (%T) does not match *cmd.UsageError", err, err)
		}
	})

	t.Run("it returns a UsageError for an unknown flag", func(t *testing.T) {
		spawnDeps = &SpawnDeps{Detector: fakeTerminalDetector{}}
		t.Cleanup(func() { spawnDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"spawn", "--bogus"})

		err := rootCmd.Execute()

		if err == nil {
			t.Fatal("expected a UsageError, got nil")
		}
		var usageErr *UsageError
		if !errors.As(err, &usageErr) {
			t.Errorf("error %v (%T) does not match *cmd.UsageError", err, err)
		}
	})
}

// Fixed spawn-pipeline composition inputs: an injected ExePath and PATH so each
// recorded OpenWindow argv is a deterministic, exact env-self-sufficient attach
// command with no dependence on the running binary or the developer's PATH.
const (
	spawnPipelineExe  = "/opt/portal/bin/portal"
	spawnPipelinePATH = "/opt/homebrew/bin:/usr/bin:/bin"
)

// ghosttyIdentity is the fixed supported host-terminal identity the pipeline
// tests detect (a real native adapter would resolve for it in production).
func ghosttyIdentity() spawn.Identity {
	return spawn.Identity{Name: "Ghostty", BundleID: "com.mitchellh.ghostty"}
}

// appleTerminalIdentity is a recognised-but-undriven host terminal: it has a
// real friendly name and bundle id (so it is NOT the NULL identity), yet no
// native adapter drives it, so ResolveAdapter classifies it unsupported. The
// N≥2 atomic-no-op gate must name it in the one-line message.
func appleTerminalIdentity() spawn.Identity {
	return spawn.NewIdentity("com.apple.Terminal", "Apple Terminal")
}

// fakeSessionConnector records every self-attach target the pipeline routes
// through it, standing in for the real Attach/Switch connectors so no unit test
// exec-replaces the process or dials tmux. Connect returns err (nil by default).
type fakeSessionConnector struct {
	calls []string
	err   error
}

func (f *fakeSessionConnector) Connect(name string) error {
	f.calls = append(f.calls, name)
	return f.err
}

// spawnPipelineDeps assembles a fully-injected SpawnDeps for the pipeline: the
// fabricated detector/resolver, the recording connector, the fixed
// executable/PATH composition seams, and a capture logger — so the whole
// detect -> resolve -> spawn -> self-attach flow runs with zero real tmux,
// osascript, or process handoff.
func spawnPipelineDeps(id spawn.Identity, resolution spawn.Resolution, adapter spawn.Adapter, conn SessionConnector, logger *slog.Logger) *SpawnDeps {
	return &SpawnDeps{
		Detector:  fakeTerminalDetector{id: id},
		Resolve:   func(spawn.Identity) (spawn.Adapter, spawn.Resolution) { return adapter, resolution },
		Connector: conn,
		ExePath:   func() (string, error) { return spawnPipelineExe, nil },
		Getenv:    func(string) string { return spawnPipelinePATH },
		// Exists defaults to all-present so the pipeline tests model the
		// transparent pre-flight gate; the gone-session tests override it.
		Exists: func(string) bool { return true },
		Logger: logger,
	}
}

// spyDetector is a TerminalDetector that counts Detect calls, letting a test
// prove the pre-flight gate aborts BEFORE detect/resolve (calls stays 0).
type spyDetector struct {
	id    spawn.Identity
	calls int
}

func (d *spyDetector) Detect() spawn.Identity {
	d.calls++
	return d.id
}

// goneExists returns an Exists predicate reporting every name in gone as absent
// (false) and all others present. It models both a session killed between
// picker-load and Enter and — since HasSession folds a probe fault to false — a
// transient tmux probe failure.
func goneExists(gone ...string) func(string) bool {
	set := make(map[string]struct{}, len(gone))
	for _, g := range gone {
		set[g] = struct{}{}
	}
	return func(name string) bool {
		_, missing := set[name]
		return !missing
	}
}

// wantAttachArgv is the exact env-self-sufficient attach argv the pipeline must
// compose for session under the fixed exe/PATH seams.
func wantAttachArgv(session string) []string {
	return []string{
		"/usr/bin/env", "-u", "TMUX", "-u", "TMUX_PANE",
		"PATH=" + spawnPipelinePATH,
		spawnPipelineExe, "attach", session,
	}
}

func TestSpawnPipeline(t *testing.T) {
	// nopRunner short-circuits PersistentPreRunE so no real tmux server is
	// dialed; spawn is intentionally NOT in skipTmuxCheck, so this injection is
	// load-bearing (TestMain poisons TMUX; a missed *Deps injection would fail
	// loudly instead of reaching the developer's real server).
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	t.Run("it spawns N-1 windows in arg order and self-attaches the Nth", func(t *testing.T) {
		adapter := &spawntest.FakeAdapter{}
		conn := &fakeSessionConnector{}
		logger, _ := newCaptureLoggerForComponent(t, "spawn")
		spawnDeps = spawnPipelineDeps(ghosttyIdentity(), spawn.ResolutionNative, adapter, conn, logger)
		t.Cleanup(func() { spawnDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"spawn", "s1", "s2", "s3"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(adapter.Calls) != 2 {
			t.Fatalf("OpenWindow called %d times, want 2 (the N−1 externals)", len(adapter.Calls))
		}
		if got := adapter.Calls[0][len(adapter.Calls[0])-1]; got != "s1" {
			t.Errorf("first spawn session = %q, want %q (arg order)", got, "s1")
		}
		if got := adapter.Calls[1][len(adapter.Calls[1])-1]; got != "s2" {
			t.Errorf("second spawn session = %q, want %q (arg order)", got, "s2")
		}
		if !slices.Equal(conn.calls, []string{"s3"}) {
			t.Errorf("self-attach targets = %#v, want exactly [s3] (the Nth)", conn.calls)
		}
	})

	t.Run("it composes the env-self-sufficient attach command for each spawned window", func(t *testing.T) {
		adapter := &spawntest.FakeAdapter{}
		conn := &fakeSessionConnector{}
		logger, _ := newCaptureLoggerForComponent(t, "spawn")
		spawnDeps = spawnPipelineDeps(ghosttyIdentity(), spawn.ResolutionNative, adapter, conn, logger)
		t.Cleanup(func() { spawnDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"spawn", "alpha", "beta", "trigger"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(adapter.Calls) != 2 {
			t.Fatalf("OpenWindow called %d times, want 2", len(adapter.Calls))
		}
		for i, session := range []string{"alpha", "beta"} {
			want := wantAttachArgv(session)
			if !slices.Equal(adapter.Calls[i], want) {
				t.Errorf("OpenWindow[%d] argv = %#v, want %#v", i, adapter.Calls[i], want)
			}
		}
	})

	t.Run("it self-attaches directly with zero spawns for N=1 regardless of terminal", func(t *testing.T) {
		adapter := &spawntest.FakeAdapter{}
		conn := &fakeSessionConnector{}
		logger, _ := newCaptureLoggerForComponent(t, "spawn")
		// A NULL identity resolving to an unsupported adapter proves N=1 self-
		// attaches directly regardless of the detected terminal.
		spawnDeps = spawnPipelineDeps(spawn.Identity{}, spawn.ResolutionUnsupported, adapter, conn, logger)
		t.Cleanup(func() { spawnDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"spawn", "solo"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(adapter.Calls) != 0 {
			t.Errorf("OpenWindow called %d times, want 0 for N=1", len(adapter.Calls))
		}
		if !slices.Equal(conn.calls, []string{"solo"}) {
			t.Errorf("self-attach targets = %#v, want exactly [solo]", conn.calls)
		}
	})

	t.Run("it skips self-attach and exits 1 when any window fails to open", func(t *testing.T) {
		adapter := &spawntest.FakeAdapter{
			Results: []spawn.Result{spawn.Success("ok"), spawn.SpawnFailed("osascript exited 1: -1743")},
		}
		conn := &fakeSessionConnector{}
		logger, _ := newCaptureLoggerForComponent(t, "spawn")
		spawnDeps = spawnPipelineDeps(ghosttyIdentity(), spawn.ResolutionNative, adapter, conn, logger)
		t.Cleanup(func() { spawnDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"spawn", "s1", "s2", "s3"})

		err := rootCmd.Execute()

		if err == nil {
			t.Fatal("expected a plain error naming the failed session, got nil")
		}
		var usageErr *UsageError
		if errors.As(err, &usageErr) {
			t.Errorf("error is a *UsageError (%v); want a plain error (exit 1, not 2)", err)
		}
		if !strings.Contains(err.Error(), "s2") {
			t.Errorf("error %q does not name the failed session %q", err.Error(), "s2")
		}
		if strings.Contains(err.Error(), "osascript") {
			t.Errorf("error %q leaks the opaque Result.Detail; it must go to the log only", err.Error())
		}
		if len(conn.calls) != 0 {
			t.Errorf("self-attach targets = %#v, want none (self-attach must be skipped on failure)", conn.calls)
		}
	})

	t.Run("it routes self-attach through the inside/outside-tmux connector", func(t *testing.T) {
		// The pipeline routes self-attach through the injected Connector, which
		// in production is buildSessionConnector — branching on tmux.InsideTmux().
		t.Setenv("TMUX", "/private/tmp/tmux-501/default,123,0")
		if got := buildSessionConnector(nil); !isSwitchConnector(got) {
			t.Errorf("inside tmux: buildSessionConnector = %T, want *SwitchConnector", got)
		}
		t.Setenv("TMUX", "")
		if got := buildSessionConnector(nil); !isAttachConnector(got) {
			t.Errorf("outside tmux: buildSessionConnector = %T, want *AttachConnector", got)
		}
	})

	t.Run("it emits a spawn: opened N/N summary without ack or batch attrs", func(t *testing.T) {
		adapter := &spawntest.FakeAdapter{}
		conn := &fakeSessionConnector{}
		logger, sink := newCaptureLoggerForComponent(t, "spawn")
		spawnDeps = spawnPipelineDeps(ghosttyIdentity(), spawn.ResolutionNative, adapter, conn, logger)
		t.Cleanup(func() { spawnDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"spawn", "s1", "s2", "s3"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var summaries []logtest.Record
		for _, rec := range sink.Records() {
			if rec.Level == slog.LevelInfo && strings.HasPrefix(rec.Msg, "opened") {
				summaries = append(summaries, rec)
			}
			if rec.HasAttr("ack") || rec.HasAttr("batch") {
				t.Errorf("record %q carries a Phase-3 attr (ack/batch): keys=%v", rec.Msg, rec.Keys)
			}
		}
		if len(summaries) != 1 {
			t.Fatalf("INFO spawn summaries = %d, want exactly 1; body:\n%s", len(summaries), sink.Body())
		}
		summary := summaries[0]
		if summary.Msg != "opened 3/3" {
			t.Errorf("summary msg = %q, want %q", summary.Msg, "opened 3/3")
		}
		if got := summary.AttrString(t, "resolution"); got != "native" {
			t.Errorf("resolution attr = %q, want %q", got, "native")
		}
		if got := summary.AttrString(t, "terminal"); got != "Ghostty" {
			t.Errorf("terminal attr = %q, want %q", got, "Ghostty")
		}
		if got := summary.AttrString(t, "bundle_id"); got != "com.mitchellh.ghostty" {
			t.Errorf("bundle_id attr = %q, want %q", got, "com.mitchellh.ghostty")
		}
		if got := summary.IntAttr(t, "opened"); got != 3 {
			t.Errorf("opened attr = %d, want 3", got)
		}
		if got := summary.IntAttr(t, "total"); got != 3 {
			t.Errorf("total attr = %d, want 3", got)
		}
	})

	t.Run("it refuses an N>=2 batch on an unsupported terminal atomically with no adapter call", func(t *testing.T) {
		adapter := &spawntest.FakeAdapter{}
		conn := &fakeSessionConnector{}
		logger, _ := newCaptureLoggerForComponent(t, "spawn")
		// The FakeAdapter is wired as the resolved adapter even though resolution
		// is unsupported: the gate must short-circuit BEFORE any adapter call, so
		// zero recorded OpenWindow calls proves the no-op is atomic.
		spawnDeps = spawnPipelineDeps(appleTerminalIdentity(), spawn.ResolutionUnsupported, adapter, conn, logger)
		t.Cleanup(func() { spawnDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"spawn", "s1", "s2"})

		err := rootCmd.Execute()

		if err == nil {
			t.Fatal("expected a plain error for the N>=2 unsupported no-op, got nil")
		}
		if len(adapter.Calls) != 0 {
			t.Errorf("OpenWindow called %d times, want 0 (gate must precede any adapter call)", len(adapter.Calls))
		}
	})

	t.Run("it does not self-attach on an N>=2 unsupported batch and exits 1", func(t *testing.T) {
		adapter := &spawntest.FakeAdapter{}
		conn := &fakeSessionConnector{}
		logger, _ := newCaptureLoggerForComponent(t, "spawn")
		spawnDeps = spawnPipelineDeps(appleTerminalIdentity(), spawn.ResolutionUnsupported, adapter, conn, logger)
		t.Cleanup(func() { spawnDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"spawn", "s1", "s2"})

		err := rootCmd.Execute()

		if err == nil {
			t.Fatal("expected a plain error for the N>=2 unsupported no-op, got nil")
		}
		if len(conn.calls) != 0 {
			t.Errorf("self-attach targets = %#v, want none (no adapter → no self-attach on N>=2)", conn.calls)
		}
		var usageErr *UsageError
		if errors.As(err, &usageErr) {
			t.Errorf("error is a *UsageError (%v); want a plain error (exit 1, not 2)", err)
		}
		if IsSilentExitError(err) {
			t.Errorf("error %v is a silent-exit sentinel; the no-op line must print to stderr", err)
		}
	})

	t.Run("it names the detected terminal (friendly name + bundle id) in the one-line message", func(t *testing.T) {
		adapter := &spawntest.FakeAdapter{}
		conn := &fakeSessionConnector{}
		logger, sink := newCaptureLoggerForComponent(t, "spawn")
		spawnDeps = spawnPipelineDeps(appleTerminalIdentity(), spawn.ResolutionUnsupported, adapter, conn, logger)
		t.Cleanup(func() { spawnDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"spawn", "s1", "s2"})

		err := rootCmd.Execute()

		if err == nil {
			t.Fatal("expected a plain error naming the detected terminal, got nil")
		}
		const want = "spawn: unsupported terminal — Apple Terminal · com.apple.Terminal — nothing opened"
		if err.Error() != want {
			t.Errorf("message = %q, want %q", err.Error(), want)
		}

		// One INFO outcome line carrying ONLY the closed resolution/terminal/
		// bundle_id attrs — no per-window records, no opened/total/ack/batch.
		var outcomes []logtest.Record
		for _, rec := range sink.Records() {
			if rec.Level == slog.LevelInfo {
				outcomes = append(outcomes, rec)
			}
			if rec.HasAttr("opened") || rec.HasAttr("total") || rec.HasAttr("ack") || rec.HasAttr("batch") {
				t.Errorf("record %q carries a per-window/summary attr: keys=%v", rec.Msg, rec.Keys)
			}
		}
		if len(outcomes) != 1 {
			t.Fatalf("INFO outcome lines = %d, want exactly 1; body:\n%s", len(outcomes), sink.Body())
		}
		outcome := outcomes[0]
		if got := outcome.AttrString(t, "resolution"); got != "unsupported" {
			t.Errorf("resolution attr = %q, want %q", got, "unsupported")
		}
		if got := outcome.AttrString(t, "terminal"); got != "Apple Terminal" {
			t.Errorf("terminal attr = %q, want %q", got, "Apple Terminal")
		}
		if got := outcome.AttrString(t, "bundle_id"); got != "com.apple.Terminal" {
			t.Errorf("bundle_id attr = %q, want %q", got, "com.apple.Terminal")
		}
	})

	t.Run("it prints the honest no-host-local-terminal line for a NULL identity N>=2 batch", func(t *testing.T) {
		adapter := &spawntest.FakeAdapter{}
		conn := &fakeSessionConnector{}
		logger, _ := newCaptureLoggerForComponent(t, "spawn")
		// NULL identity (remote/mosh / no host-local client) also resolves
		// unsupported and folds to the same atomic no-op path.
		spawnDeps = spawnPipelineDeps(spawn.Identity{}, spawn.ResolutionUnsupported, adapter, conn, logger)
		t.Cleanup(func() { spawnDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"spawn", "s1", "s2"})

		err := rootCmd.Execute()

		if err == nil {
			t.Fatal("expected the honest no-host-local-terminal error, got nil")
		}
		const want = "spawn: no host-local terminal — nothing opened"
		if err.Error() != want {
			t.Errorf("message = %q, want %q", err.Error(), want)
		}
		if len(adapter.Calls) != 0 {
			t.Errorf("OpenWindow called %d times, want 0 for the NULL no-op", len(adapter.Calls))
		}
		if len(conn.calls) != 0 {
			t.Errorf("self-attach targets = %#v, want none for the NULL no-op", conn.calls)
		}
	})

	t.Run("it still self-attaches for N=1 on an unsupported terminal", func(t *testing.T) {
		adapter := &spawntest.FakeAdapter{}
		conn := &fakeSessionConnector{}
		logger, _ := newCaptureLoggerForComponent(t, "spawn")
		// A recognised-but-undriven terminal proves the N=1 asymmetry: single
		// attach needs no adapter, so the gate is skipped and s1 self-attaches.
		spawnDeps = spawnPipelineDeps(appleTerminalIdentity(), spawn.ResolutionUnsupported, adapter, conn, logger)
		t.Cleanup(func() { spawnDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"spawn", "s1"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(adapter.Calls) != 0 {
			t.Errorf("OpenWindow called %d times, want 0 for N=1", len(adapter.Calls))
		}
		if !slices.Equal(conn.calls, []string{"s1"}) {
			t.Errorf("self-attach targets = %#v, want exactly [s1] (N=1 self-attaches)", conn.calls)
		}
	})
}

func TestSpawnPreflight(t *testing.T) {
	// nopRunner short-circuits PersistentPreRunE so no real tmux server is
	// dialed; spawn is intentionally NOT in skipTmuxCheck, so this injection is
	// load-bearing (TestMain poisons TMUX; a missed *Deps injection would fail
	// loudly instead of reaching the developer's real server).
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	t.Run("it aborts atomically naming the single gone session with no spawn, no self-attach, and never reaching detect", func(t *testing.T) {
		adapter := &spawntest.FakeAdapter{}
		conn := &fakeSessionConnector{}
		detector := &spyDetector{id: ghosttyIdentity()}
		logger, _ := newCaptureLoggerForComponent(t, "spawn")
		deps := spawnPipelineDeps(ghosttyIdentity(), spawn.ResolutionNative, adapter, conn, logger)
		deps.Detector = detector
		deps.Exists = goneExists("s2")
		spawnDeps = deps
		t.Cleanup(func() { spawnDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"spawn", "s1", "s2", "s3"})

		err := rootCmd.Execute()

		if err == nil {
			t.Fatal("expected a plain error naming the gone session, got nil")
		}
		const want = "spawn: 's2' is gone — nothing opened"
		if err.Error() != want {
			t.Errorf("message = %q, want %q", err.Error(), want)
		}
		var usageErr *UsageError
		if errors.As(err, &usageErr) {
			t.Errorf("error is a *UsageError (%v); want a plain error (exit 1, not 2)", err)
		}
		if len(adapter.Calls) != 0 {
			t.Errorf("OpenWindow called %d times, want 0 (pre-flight aborts before any spawn)", len(adapter.Calls))
		}
		if len(conn.calls) != 0 {
			t.Errorf("self-attach targets = %#v, want none (no self-attach on abort)", conn.calls)
		}
		if detector.calls != 0 {
			t.Errorf("Detect called %d times, want 0 (pre-flight precedes detect/resolve)", detector.calls)
		}
	})

	t.Run("it names every gone session in one line when several are missing", func(t *testing.T) {
		adapter := &spawntest.FakeAdapter{}
		conn := &fakeSessionConnector{}
		detector := &spyDetector{id: ghosttyIdentity()}
		logger, _ := newCaptureLoggerForComponent(t, "spawn")
		deps := spawnPipelineDeps(ghosttyIdentity(), spawn.ResolutionNative, adapter, conn, logger)
		deps.Detector = detector
		deps.Exists = goneExists("s2", "s3")
		spawnDeps = deps
		t.Cleanup(func() { spawnDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"spawn", "s1", "s2", "s3"})

		err := rootCmd.Execute()

		if err == nil {
			t.Fatal("expected a plain error naming both gone sessions, got nil")
		}
		const want = "spawn: 's2', 's3' are gone — nothing opened"
		if err.Error() != want {
			t.Errorf("message = %q, want %q", err.Error(), want)
		}
		if len(adapter.Calls) != 0 {
			t.Errorf("OpenWindow called %d times, want 0", len(adapter.Calls))
		}
		if len(conn.calls) != 0 {
			t.Errorf("self-attach targets = %#v, want none", conn.calls)
		}
		if detector.calls != 0 {
			t.Errorf("Detect called %d times, want 0", detector.calls)
		}
	})

	t.Run("it aborts an N=1 batch whose sole session is gone with no self-attach", func(t *testing.T) {
		adapter := &spawntest.FakeAdapter{}
		conn := &fakeSessionConnector{}
		detector := &spyDetector{id: ghosttyIdentity()}
		logger, _ := newCaptureLoggerForComponent(t, "spawn")
		deps := spawnPipelineDeps(ghosttyIdentity(), spawn.ResolutionNative, adapter, conn, logger)
		deps.Detector = detector
		deps.Exists = goneExists("s1")
		spawnDeps = deps
		t.Cleanup(func() { spawnDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"spawn", "s1"})

		err := rootCmd.Execute()

		if err == nil {
			t.Fatal("expected the gone-session error for the N=1 abort, got nil")
		}
		const want = "spawn: 's1' is gone — nothing opened"
		if err.Error() != want {
			t.Errorf("message = %q, want %q", err.Error(), want)
		}
		if len(conn.calls) != 0 {
			t.Errorf("self-attach targets = %#v, want none (pre-flight is not skipped for N=1)", conn.calls)
		}
		if detector.calls != 0 {
			t.Errorf("Detect called %d times, want 0", detector.calls)
		}
	})

	t.Run("it proceeds unchanged when all sessions are present", func(t *testing.T) {
		adapter := &spawntest.FakeAdapter{}
		conn := &fakeSessionConnector{}
		logger, _ := newCaptureLoggerForComponent(t, "spawn")
		deps := spawnPipelineDeps(ghosttyIdentity(), spawn.ResolutionNative, adapter, conn, logger)
		deps.Exists = goneExists() // none gone → transparent gate
		spawnDeps = deps
		t.Cleanup(func() { spawnDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"spawn", "s1", "s2", "s3"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(adapter.Calls) != 2 {
			t.Errorf("OpenWindow called %d times, want 2 (the N−1 externals) — the gate must be transparent", len(adapter.Calls))
		}
		if !slices.Equal(conn.calls, []string{"s3"}) {
			t.Errorf("self-attach targets = %#v, want exactly [s3]", conn.calls)
		}
	})

	t.Run("it aborts conservatively when a session probe fails (treats unprobeable as gone)", func(t *testing.T) {
		adapter := &spawntest.FakeAdapter{}
		conn := &fakeSessionConnector{}
		detector := &spyDetector{id: ghosttyIdentity()}
		logger, _ := newCaptureLoggerForComponent(t, "spawn")
		deps := spawnPipelineDeps(ghosttyIdentity(), spawn.ResolutionNative, adapter, conn, logger)
		deps.Detector = detector
		// HasSession folds a transient tmux probe fault to false; modelling that
		// as Exists→false for s2 must abort conservatively rather than risk a
		// false open.
		deps.Exists = goneExists("s2")
		spawnDeps = deps
		t.Cleanup(func() { spawnDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"spawn", "s1", "s2", "s3"})

		err := rootCmd.Execute()

		if err == nil {
			t.Fatal("expected a conservative abort on an unprobeable session, got nil")
		}
		if len(adapter.Calls) != 0 {
			t.Errorf("OpenWindow called %d times, want 0 (conservative abort on probe fault)", len(adapter.Calls))
		}
		if len(conn.calls) != 0 {
			t.Errorf("self-attach targets = %#v, want none (conservative abort on probe fault)", conn.calls)
		}
		if detector.calls != 0 {
			t.Errorf("Detect called %d times, want 0", detector.calls)
		}
	})

	t.Run("it emits one INFO outcome line naming the gone session and no opened/total summary attrs", func(t *testing.T) {
		adapter := &spawntest.FakeAdapter{}
		conn := &fakeSessionConnector{}
		logger, sink := newCaptureLoggerForComponent(t, "spawn")
		deps := spawnPipelineDeps(ghosttyIdentity(), spawn.ResolutionNative, adapter, conn, logger)
		deps.Exists = goneExists("s2")
		spawnDeps = deps
		t.Cleanup(func() { spawnDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"spawn", "s1", "s2", "s3"})

		if err := rootCmd.Execute(); err == nil {
			t.Fatal("expected the gone-session error, got nil")
		}

		var outcomes []logtest.Record
		for _, rec := range sink.Records() {
			if rec.Level == slog.LevelInfo {
				outcomes = append(outcomes, rec)
			}
			// Nothing was attempted, so no per-window/summary attrs may appear.
			if rec.HasAttr("opened") || rec.HasAttr("total") || rec.HasAttr("ack") || rec.HasAttr("batch") {
				t.Errorf("record %q carries a per-window/summary attr: keys=%v", rec.Msg, rec.Keys)
			}
		}
		if len(outcomes) != 1 {
			t.Fatalf("INFO outcome lines = %d, want exactly 1; body:\n%s", len(outcomes), sink.Body())
		}
		if got := outcomes[0].Msg; !strings.Contains(got, "'s2'") || !strings.Contains(got, "gone") {
			t.Errorf("outcome msg = %q, want it to name 's2' as gone", got)
		}
	})
}

func isSwitchConnector(c SessionConnector) bool {
	_, ok := c.(*SwitchConnector)
	return ok
}

func isAttachConnector(c SessionConnector) bool {
	_, ok := c.(*AttachConnector)
	return ok
}
