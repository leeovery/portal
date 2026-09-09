package cmd

import (
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
)

// execLogLine returns the single captured line with the given message, matched
// on the "<LEVEL> <msg>" prefix so an attr value carrying the same phrase cannot
// false-match.
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
	cfg := hydrateCfg(t, hydrateCfgOpts{
		HookKey:   "nil:0.0",
		OpenFIFO:  unexpectedOpenFIFO(t),
		Logger:    logger,
		HookStore: nil,
		ExecShell: exec.fn(),
	})
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
	// An unreadable hooks.json makes the lookup read fail.
	store, _ := hookstest.StageStore(t, hookstest.Staging{Dir: dir, SidecarAbsent: true, Unreadable: true})

	t.Setenv("SHELL", "/bin/zsh")
	logger, sink := newCaptureLoggerForComponent(t, "hydrate")

	exec := &stubExecShell{}
	cfg := hydrateCfg(t, hydrateCfgOpts{
		HookKey:   "err:0.0",
		OpenFIFO:  unexpectedOpenFIFO(t),
		Logger:    logger,
		HookStore: store,
		ExecShell: exec.fn(),
	})
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
	store, _ := hookstest.StageStore(t, hookstest.Staging{Dir: dir, SidecarAbsent: true, Body: map[string]map[string]string{}})

	t.Setenv("SHELL", "/bin/zsh")
	logger, sink := newCaptureLoggerForComponent(t, "hydrate")

	exec := &stubExecShell{}
	cfg := hydrateCfg(t, hydrateCfgOpts{
		HookKey:   "miss:0.0",
		OpenFIFO:  unexpectedOpenFIFO(t),
		Logger:    logger,
		HookStore: store,
		ExecShell: exec.fn(),
	})
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
	store, _ := hookstest.StageStore(t, hookstest.Staging{Dir: dir, SidecarAbsent: true, Body: map[string]map[string]string{
		"hit:0.0": {"on-resume": "echo hi"},
	}})

	t.Setenv("SHELL", "/bin/zsh")
	logger, sink := newCaptureLoggerForComponent(t, "hydrate")

	exec := &stubExecShell{}
	cfg := hydrateCfg(t, hydrateCfgOpts{
		HookKey:   "hit:0.0",
		OpenFIFO:  unexpectedOpenFIFO(t),
		Logger:    logger,
		HookStore: store,
		ExecShell: exec.fn(),
	})
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
	store, _ := hookstest.StageStore(t, hookstest.Staging{Dir: dir, SidecarAbsent: true, Body: map[string]map[string]string{
		"q:0.0": {"on-resume": rawCmd},
	}})

	t.Setenv("SHELL", "/bin/zsh")
	logger, sink := newCaptureLoggerForComponent(t, "hydrate")

	exec := &stubExecShell{}
	cfg := hydrateCfg(t, hydrateCfgOpts{
		HookKey:   "q:0.0",
		OpenFIFO:  unexpectedOpenFIFO(t),
		Logger:    logger,
		HookStore: store,
		ExecShell: exec.fn(),
	})
	execShellOrHookAndExit(cfg)

	body := sink.Body()
	info := execLogLine(t, body, "INFO", "exec")
	wantArgs := "args=sh -c " + rawCmd + "; exec /bin/zsh"
	if !strings.Contains(info, wantArgs) {
		t.Errorf("exec INFO args not rendered verbatim; want substring %q in %q", wantArgs, info)
	}
}

func TestHydrateExecLog_ExecInfoUsesTargetAttrNotPath(t *testing.T) {
	dir := t.TempDir()
	store, _ := hookstest.StageStore(t, hookstest.Staging{Dir: dir, SidecarAbsent: true, Body: map[string]map[string]string{
		"hit:0.0": {"on-resume": "echo hi"},
	}})

	t.Setenv("SHELL", "/bin/zsh")
	logger, sink := newCaptureLoggerForComponent(t, "hydrate")

	exec := &stubExecShell{}
	cfg := hydrateCfg(t, hydrateCfgOpts{
		HookKey:   "hit:0.0",
		OpenFIFO:  unexpectedOpenFIFO(t),
		Logger:    logger,
		HookStore: store,
		ExecShell: exec.fn(),
	})
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
				store, _ := hookstest.StageStore(t, hookstest.Staging{Dir: dir, SidecarAbsent: true, Body: map[string]map[string]string{}})
				return store
			},
			hookKey: "m:0.0",
		},
		{
			name: "hook-chain-hit",
			hookStore: func(t *testing.T, dir string) *hooks.Store {
				store, _ := hookstest.StageStore(t, hookstest.Staging{Dir: dir, SidecarAbsent: true, Body: map[string]map[string]string{
					"h:0.0": {"on-resume": "echo hi"},
				}})
				return store
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
			cfg := hydrateCfg(t, hydrateCfgOpts{
				HookKey:   tc.hookKey,
				OpenFIFO:  unexpectedOpenFIFO(t),
				Logger:    logger,
				HookStore: tc.hookStore(t, dir),
				ExecShell: func(_ string, _ []string) {
					// Snapshot at the instant of handoff: the exec INFO must
					// already be present.
					bodyAtExec = sink.Body()
				},
			})
			execShellOrHookAndExit(cfg)

			execLogLine(t, bodyAtExec, "INFO", "exec")
		})
	}
}
