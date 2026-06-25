// Tests in this file mutate package-level cobra command state and MUST NOT use t.Parallel.
//
// Coverage for the Phase 6 hydrate-helper forensic trail (Task
// portal-observability-layer-6-1): the hook-lookup DEBUG breadcrumb and the
// terminal "hydrate: exec" INFO emitted by execShellOrHookAndExit /
// execShellAndExit, instrumenting everything up to the syscall.Exec handoff.
package cmd

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/tmux"
)

// execLogLine returns the single captured line whose message is the given
// terse phrase (rendered by logtest.Sink as "<LEVEL> <msg> key=value..."),
// matched on the "<LEVEL> <msg>" prefix so an attr value that happens to
// contain the phrase cannot false-match. Fails the test if zero or more than
// one line matches.
func execLogLine(t *testing.T, body, level, msg string) string {
	t.Helper()
	prefix := level + " " + msg
	var matches []string
	for line := range strings.SplitSeq(body, "\n") {
		if line == prefix || strings.HasPrefix(line, prefix+" ") {
			matches = append(matches, line)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("want exactly one %q line, got %d: %q", prefix, len(matches), body)
	}
	return matches[0]
}

func TestHydrateExecLog_NilHookStore_MissThenBareShellExec(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")
	logger, sink := newCaptureLoggerForComponent(t, "hydrate")

	exec := &stubExecShell{}
	cfg := hydrateConfig{
		HookKey:   "nil:0.0",
		Stdout:    io.Discard,
		Client:    tmux.NewClient(&recordingCommander{}),
		Logger:    logger,
		HookStore: nil,
		ExecShell: exec.fn(),
	}
	execShellOrHookAndExit(cfg)

	if exec.target != "/bin/zsh" {
		t.Errorf("ExecShell target = %q, want /bin/zsh (nil store → bare shell)", exec.target)
	}

	body := sink.Body()
	dbg := execLogLine(t, body, "DEBUG", "hook lookup")
	if !strings.Contains(dbg, "hook_key=nil:0.0") {
		t.Errorf("DEBUG line missing hook_key=nil:0.0: %q", dbg)
	}
	if !strings.Contains(dbg, "result=miss") {
		t.Errorf("nil store must map to result=miss: %q", dbg)
	}
	if strings.Contains(dbg, "error=") {
		t.Errorf("nil-store miss must NOT carry an error attr: %q", dbg)
	}

	info := execLogLine(t, body, "INFO", "exec")
	if !strings.Contains(info, "target=/bin/zsh") {
		t.Errorf("exec INFO missing target=/bin/zsh: %q", info)
	}
	if !strings.Contains(info, "args=/bin/zsh") {
		t.Errorf("exec INFO missing args=/bin/zsh: %q", info)
	}
	if !strings.Contains(info, "hook_present=false") {
		t.Errorf("exec INFO missing hook_present=false: %q", info)
	}
}

func TestHydrateExecLog_LookupError_ErrorResultWithAttrWarnRetainedBareShell(t *testing.T) {
	dir := t.TempDir()
	// hooks.json is a directory, not a file → LookupOnResume read fails.
	hooksDir := dir + "/hooks.json"
	if err := os.Mkdir(hooksDir, 0o700); err != nil {
		t.Fatalf("mkdir hooks.json: %v", err)
	}
	store := hooks.NewStore(hooksDir)

	t.Setenv("SHELL", "/bin/zsh")
	logger, sink := newCaptureLoggerForComponent(t, "hydrate")

	exec := &stubExecShell{}
	cfg := hydrateConfig{
		HookKey:   "err:0.0",
		Stdout:    io.Discard,
		Client:    tmux.NewClient(&recordingCommander{}),
		Logger:    logger,
		HookStore: store,
		ExecShell: exec.fn(),
	}
	execShellOrHookAndExit(cfg)

	if exec.target != "/bin/zsh" {
		t.Errorf("ExecShell target = %q, want /bin/zsh (lookup error → bare shell)", exec.target)
	}

	body := sink.Body()
	dbg := execLogLine(t, body, "DEBUG", "hook lookup")
	if !strings.Contains(dbg, "result=error") {
		t.Errorf("lookup error must map to result=error: %q", dbg)
	}
	if !strings.Contains(dbg, "error=") {
		t.Errorf("result=error must carry the error attr: %q", dbg)
	}
	if !strings.Contains(dbg, "hook_key=err:0.0") {
		t.Errorf("DEBUG line missing hook_key=err:0.0: %q", dbg)
	}

	// Existing WARN retained.
	if n := strings.Count(body, "lookup on-resume hook failed"); n != 1 {
		t.Errorf("want exactly one existing WARN line, got %d: %q", n, body)
	}

	info := execLogLine(t, body, "INFO", "exec")
	if !strings.Contains(info, "target=/bin/zsh") {
		t.Errorf("exec INFO missing target=/bin/zsh: %q", info)
	}
	if !strings.Contains(info, "hook_present=false") {
		t.Errorf("exec INFO missing hook_present=false: %q", info)
	}
}

func TestHydrateExecLog_UnregisteredPaneKey_MissThenBareShellExec(t *testing.T) {
	dir := t.TempDir()
	store := seedHookStore(t, dir, map[string]map[string]string{}) // empty

	t.Setenv("SHELL", "/bin/zsh")
	logger, sink := newCaptureLoggerForComponent(t, "hydrate")

	exec := &stubExecShell{}
	cfg := hydrateConfig{
		HookKey:   "miss:0.0",
		Stdout:    io.Discard,
		Client:    tmux.NewClient(&recordingCommander{}),
		Logger:    logger,
		HookStore: store,
		ExecShell: exec.fn(),
	}
	execShellOrHookAndExit(cfg)

	if exec.target != "/bin/zsh" {
		t.Errorf("ExecShell target = %q, want /bin/zsh (no hook → bare shell)", exec.target)
	}

	body := sink.Body()
	dbg := execLogLine(t, body, "DEBUG", "hook lookup")
	if !strings.Contains(dbg, "result=miss") {
		t.Errorf("unregistered pane key must map to result=miss: %q", dbg)
	}
	if strings.Contains(dbg, "error=") {
		t.Errorf("miss must NOT carry an error attr: %q", dbg)
	}

	info := execLogLine(t, body, "INFO", "exec")
	if !strings.Contains(info, "hook_present=false") {
		t.Errorf("exec INFO missing hook_present=false: %q", info)
	}
}

func TestHydrateExecLog_RegisteredHook_HitThenHookChainExec(t *testing.T) {
	dir := t.TempDir()
	store := seedHookStore(t, dir, map[string]map[string]string{
		"hit:0.0": {"on-resume": "echo hi"},
	})

	t.Setenv("SHELL", "/bin/zsh")
	logger, sink := newCaptureLoggerForComponent(t, "hydrate")

	exec := &stubExecShell{}
	cfg := hydrateConfig{
		HookKey:   "hit:0.0",
		Stdout:    io.Discard,
		Client:    tmux.NewClient(&recordingCommander{}),
		Logger:    logger,
		HookStore: store,
		ExecShell: exec.fn(),
	}
	execShellOrHookAndExit(cfg)

	if exec.target != "/bin/sh" {
		t.Errorf("ExecShell target = %q, want /bin/sh (hook chain)", exec.target)
	}

	body := sink.Body()
	dbg := execLogLine(t, body, "DEBUG", "hook lookup")
	if !strings.Contains(dbg, "result=hit") {
		t.Errorf("registered hook must map to result=hit: %q", dbg)
	}

	info := execLogLine(t, body, "INFO", "exec")
	if !strings.Contains(info, "target=/bin/sh") {
		t.Errorf("exec INFO missing target=/bin/sh: %q", info)
	}
	// args is the space-joined argv: "sh -c echo hi; exec /bin/zsh".
	if !strings.Contains(info, "args=sh -c echo hi; exec /bin/zsh") {
		t.Errorf("exec INFO missing joined hook-chain args: %q", info)
	}
	if !strings.Contains(info, "hook_present=true") {
		t.Errorf("exec INFO missing hook_present=true: %q", info)
	}
}

func TestHydrateExecLog_HitRendersArgsVerbatimIncludingEmbeddedQuotes(t *testing.T) {
	dir := t.TempDir()
	rawCmd := "echo 'it works' && echo \"quoted\""
	store := seedHookStore(t, dir, map[string]map[string]string{
		"q:0.0": {"on-resume": rawCmd},
	})

	t.Setenv("SHELL", "/bin/zsh")
	logger, sink := newCaptureLoggerForComponent(t, "hydrate")

	exec := &stubExecShell{}
	cfg := hydrateConfig{
		HookKey:   "q:0.0",
		Stdout:    io.Discard,
		Client:    tmux.NewClient(&recordingCommander{}),
		Logger:    logger,
		HookStore: store,
		ExecShell: exec.fn(),
	}
	execShellOrHookAndExit(cfg)

	body := sink.Body()
	info := execLogLine(t, body, "INFO", "exec")
	// args = "sh -c " + "<rawCmd>; exec /bin/zsh", embedded quotes verbatim.
	wantArgs := "args=sh -c " + rawCmd + "; exec /bin/zsh"
	if !strings.Contains(info, wantArgs) {
		t.Errorf("exec INFO args not rendered verbatim; want substring %q in %q", wantArgs, info)
	}
}

func TestHydrateExecLog_ExecInfoUsesTargetAttrNotPath(t *testing.T) {
	dir := t.TempDir()
	store := seedHookStore(t, dir, map[string]map[string]string{
		"hit:0.0": {"on-resume": "echo hi"},
	})

	t.Setenv("SHELL", "/bin/zsh")
	logger, sink := newCaptureLoggerForComponent(t, "hydrate")

	exec := &stubExecShell{}
	cfg := hydrateConfig{
		HookKey:   "hit:0.0",
		Stdout:    io.Discard,
		Client:    tmux.NewClient(&recordingCommander{}),
		Logger:    logger,
		HookStore: store,
		ExecShell: exec.fn(),
	}
	execShellOrHookAndExit(cfg)

	info := execLogLine(t, sink.Body(), "INFO", "exec")
	if !strings.Contains(info, "target=") {
		t.Errorf("exec INFO must use the target attr: %q", info)
	}
	if strings.Contains(info, "path=") {
		t.Errorf("exec INFO must NOT use the reserved path attr: %q", info)
	}
}

func TestHydrateExecLog_ExecInfoEmittedImmediatelyBeforeExecShell(t *testing.T) {
	cases := []struct {
		name      string
		hookStore func(t *testing.T, dir string) *hooks.Store
		hookKey   string
	}{
		{
			name:      "bare-shell-nil-store",
			hookStore: func(_ *testing.T, _ string) *hooks.Store { return nil },
			hookKey:   "b:0.0",
		},
		{
			name: "bare-shell-miss",
			hookStore: func(t *testing.T, dir string) *hooks.Store {
				return seedHookStore(t, dir, map[string]map[string]string{})
			},
			hookKey: "m:0.0",
		},
		{
			name: "hook-chain-hit",
			hookStore: func(t *testing.T, dir string) *hooks.Store {
				return seedHookStore(t, dir, map[string]map[string]string{
					"h:0.0": {"on-resume": "echo hi"},
				})
			},
			hookKey: "h:0.0",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SHELL", "/bin/zsh")
			logger, sink := newCaptureLoggerForComponent(t, "hydrate")
			dir := t.TempDir()

			var bodyAtExec string
			cfg := hydrateConfig{
				HookKey:   tc.hookKey,
				Stdout:    io.Discard,
				Client:    tmux.NewClient(&recordingCommander{}),
				Logger:    logger,
				HookStore: tc.hookStore(t, dir),
				ExecShell: func(_ string, _ []string) {
					// Snapshot the captured log at the instant of handoff: the
					// exec INFO must already be present (it is the immediately-
					// preceding statement).
					bodyAtExec = sink.Body()
				},
			}
			execShellOrHookAndExit(cfg)

			execLogLine(t, bodyAtExec, "INFO", "exec")
		})
	}
}
