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

// countLogLines matches on the "<LEVEL> <msg>" prefix so an attr value
// containing the phrase cannot false-match.
func countLogLines(body, level, msg string) int {
	prefix := level + " " + msg
	n := 0
	for line := range strings.SplitSeq(body, "\n") {
		if line == prefix || strings.HasPrefix(line, prefix+" ") {
			n++
		}
	}
	return n
}

// execLookedUpRegistration drives the exec the way an unwaiting session's
// hand-off does: the registration looked up, and no session parked.
func execLookedUpRegistration(cfg hydrateConfig) {
	cfg.Logger = hydrateLoggerOrDefault(cfg.Logger)
	execShellOrHookAndExit(cfg, lookupOnResumeOrLog(cfg), nil)
}

// storeHolding stages a hooks.json holding body.
func storeHolding(t *testing.T, body map[string]map[string]string) *hooks.Store {
	t.Helper()
	store, _ := hookstest.StageStore(t, hookstest.Staging{Dir: t.TempDir(), SidecarAbsent: true, Body: body})
	return store
}

func TestHydrateExecLog_ALookupThatFindsNothingRecordsAMissAndExecsTheLoginShell(t *testing.T) {
	for name, store := range map[string]func(t *testing.T) *hooks.Store{
		"no hook store":           func(*testing.T) *hooks.Store { return nil },
		"an unregistered session": emptyHookStore,
		"a registration elsewhere": func(t *testing.T) *hooks.Store {
			return storeHolding(t, map[string]map[string]string{"proj-other": {"on-resume": "echo hi"}})
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("SHELL", "/bin/zsh")
			logger, sink := newCaptureLoggerForComponent(t, "hydrate")
			exec := &stubExecShell{}

			execLookedUpRegistration(hydrateConfig{Session: hydrateSession, Logger: logger, HookStore: store(t), ExecShell: exec.fn()})

			if exec.target != "/bin/zsh" {
				t.Errorf("ExecShell target = %q, want /bin/zsh", exec.target)
			}
			body := sink.Body()
			dbg := execLogLine(t, body, "DEBUG", "hook lookup")
			for _, want := range []string{"hook_key=" + hydrateSession, "result=miss"} {
				if !strings.Contains(dbg, want) {
					t.Errorf("DEBUG line %q missing %s", dbg, want)
				}
			}
			if strings.Contains(dbg, "error=") {
				t.Errorf("a miss must not carry an error attr: %q", dbg)
			}
			info := execLogLine(t, body, "INFO", "exec")
			for _, want := range []string{"target=/bin/zsh", "args=-zsh", "hook_present=false"} {
				if !strings.Contains(info, want) {
					t.Errorf("exec INFO %q missing %s", info, want)
				}
			}
		})
	}
}

func TestHydrateExecLog_LookupError_ErrorResultWithAttrWarnRetainedLoginShell(t *testing.T) {
	store, _ := hookstest.StageStore(t, hookstest.Staging{Dir: t.TempDir(), SidecarAbsent: true, Unreadable: true})
	t.Setenv("SHELL", "/bin/zsh")
	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	exec := &stubExecShell{}

	execLookedUpRegistration(hydrateConfig{Session: hydrateSession, Logger: logger, HookStore: store, ExecShell: exec.fn()})

	if exec.target != "/bin/zsh" {
		t.Errorf("ExecShell target = %q, want /bin/zsh (lookup error → login shell)", exec.target)
	}
	body := sink.Body()
	dbg := execLogLine(t, body, "DEBUG", "hook lookup")
	for _, want := range []string{"result=error", "error=", "hook_key=" + hydrateSession} {
		if !strings.Contains(dbg, want) {
			t.Errorf("DEBUG line %q missing %s", dbg, want)
		}
	}
	if n := strings.Count(body, "lookup on-resume hook failed"); n != 1 {
		t.Errorf("want exactly one lookup WARN, got %d: %q", n, body)
	}
	if info := execLogLine(t, body, "INFO", "exec"); !strings.Contains(info, "hook_present=false") {
		t.Errorf("exec INFO missing hook_present=false: %q", info)
	}
}

func TestHydrateExecLog_RegisteredHook_HitThenHookChainExec(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")
	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	exec := &stubExecShell{}
	store := storeHolding(t, map[string]map[string]string{hydrateSession: {"on-resume": "echo hi"}})

	execLookedUpRegistration(hydrateConfig{Session: hydrateSession, Logger: logger, HookStore: store, ExecShell: exec.fn()})

	if exec.target != "/bin/sh" {
		t.Errorf("ExecShell target = %q, want /bin/sh (hook chain)", exec.target)
	}
	body := sink.Body()
	if dbg := execLogLine(t, body, "DEBUG", "hook lookup"); !strings.Contains(dbg, "result=hit") {
		t.Errorf("a registered hook must map to result=hit: %q", dbg)
	}
	info := execLogLine(t, body, "INFO", "exec")
	for _, want := range []string{"target=/bin/sh", `args=sh -c 'trap : TERM; echo hi; exec '\''/bin/zsh'\'' -l'`, "hook_present=true"} {
		if !strings.Contains(info, want) {
			t.Errorf("exec INFO %q missing %s", info, want)
		}
	}
	if strings.Contains(info, "path=") {
		t.Errorf("exec INFO must use the target attr, never the reserved path attr: %q", info)
	}
}

func TestHydrateExecLog_HitQuotesTheChainAsOneArgumentIncludingEmbeddedQuotes(t *testing.T) {
	rawCmd := "echo 'it works' && echo \"quoted\""
	t.Setenv("SHELL", "/bin/zsh")
	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	store := storeHolding(t, map[string]map[string]string{hydrateSession: {"on-resume": rawCmd}})

	execLookedUpRegistration(hydrateConfig{Session: hydrateSession, Logger: logger, HookStore: store, ExecShell: (&stubExecShell{}).fn()})

	info := execLogLine(t, sink.Body(), "INFO", "exec")
	if want := `args=sh -c 'trap : TERM; echo '\''it works'\'' && echo "quoted"; exec '\''/bin/zsh'\'' -l'`; !strings.Contains(info, want) {
		t.Errorf("exec INFO args not quoted as the argv exec'd; want substring %q in %q", want, info)
	}
}

func TestHydrateExecLog_ExecInfoEmittedImmediatelyBeforeExecShell(t *testing.T) {
	for name, store := range map[string]func(t *testing.T) *hooks.Store{
		"login shell, no store": func(*testing.T) *hooks.Store { return nil },
		"login shell, a miss":   emptyHookStore,
		"hook chain, a hit": func(t *testing.T) *hooks.Store {
			return storeHolding(t, map[string]map[string]string{hydrateSession: {"on-resume": "echo hi"}})
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("SHELL", "/bin/zsh")
			logger, sink := newCaptureLoggerForComponent(t, "hydrate")
			var bodyAtExec string

			execLookedUpRegistration(hydrateConfig{Session: hydrateSession, Logger: logger, HookStore: store(t), ExecShell: func(string, []string) {
				bodyAtExec = sink.Body()
			}})

			execLogLine(t, bodyAtExec, "INFO", "exec")
		})
	}
}
