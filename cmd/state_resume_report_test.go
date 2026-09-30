package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/themetest"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
)

// refusedClear is the error the production client hands back when tmux answers
// the pending-marker clear with its own stderr, so the chain carries the argv
// and the option name ahead of tmux's words exactly as a real refusal does.
func refusedClear(t *testing.T, cause *tmux.CommandError) error {
	t.Helper()
	client := tmux.NewClient(commandertest.Quiet(commandertest.Fails(cause, "set-option")))
	err := state.UnsetResumePendingMarker(client, tmux.PaneIDTarget("%7"))
	if err == nil {
		t.Fatal("the scripted clear succeeded; the fixture must refuse it")
	}
	return err
}

func tmuxRefusal(t *testing.T) error {
	t.Helper()
	return refusedClear(t, &tmux.CommandError{
		Stderr: "can't find pane: %7\n",
		Err:    &exec.ExitError{ProcessState: &os.ProcessState{}},
		Args:   []string{"set-option", "-pu", "-t", "%7", state.ResumePendingOption},
	})
}

func tmuxUnrunnable(t *testing.T) error {
	t.Helper()
	return refusedClear(t, &tmux.CommandError{
		Err:  &exec.Error{Name: "tmux", Err: exec.ErrNotFound},
		Args: []string{"set-option", "-pu", "-t", "%7", state.ResumePendingOption},
	})
}

func reportCarried(t *testing.T, argv []string) string {
	t.Helper()
	i := slices.Index(argv, flagArg(resumeFlagReport))
	if i < 0 || i+1 >= len(argv) {
		t.Fatalf("redraw argv = %q, want a %s", argv, flagArg(resumeFlagReport))
	}
	return argv[i+1]
}

// discardAgainstStore answers y on the confirmation with the removal routed
// through the production store resolution, every record landing in one sink.
func discardAgainstStore(t *testing.T) (*resumeWaitProbe, *logtest.Sink) {
	t.Helper()
	sink := logtest.Install(t)
	var probe resumeWaitProbe
	payload := confirmationPayload()
	cfg := newResumeWaitConfig(t, &probe, payload, keystrokes(t, "y"))
	cfg.Logger = nil
	cfg.DiscardRegistration = func(hookKey string) (bool, error) {
		return discardResumeRegistration(hookKey, payload.PaneKey)
	}
	if err := runResumeWait(cfg); err != nil {
		t.Fatalf("runResumeWait() error = %v", err)
	}
	return &probe, sink
}

func stageDiscardStore(t *testing.T, staging hookstest.Staging) string {
	t.Helper()
	_, path := hookstest.StageStore(t, staging)
	t.Setenv("PORTAL_HOOKS_FILE", path)
	return path
}

func registeredEntries() map[string]string {
	return map[string]string{samplePayload().HookKey: samplePayload().Command}
}

// assertRefusedOnConfirmation pins what every refused removal leaves: the
// confirmation drawn again with row, the marker untouched, and exactly one
// record at WARN or above — which it returns.
func assertRefusedOnConfirmation(t *testing.T, probe *resumeWaitProbe, sink *logtest.Sink, row string) logtest.Record {
	t.Helper()
	if got := reportCarried(t, probe.execArgs); got != row {
		t.Errorf("report row = %q, want %q", got, row)
	}
	if i := slices.Index(probe.execArgs, flagArg(resumeFlagScreen)); i < 0 || probe.execArgs[i+1] != resumeScreenDiscard {
		t.Errorf("redraw argv = %q, want the confirmation drawn again", probe.execArgs)
	}
	if probe.clearCalls != 0 {
		t.Errorf("ClearMarker called %d times, want 0: the marker stands", probe.clearCalls)
	}
	return sink.Records().AtOrAboveLevel(slog.LevelWarn).Only(t, "the refusal's one record")
}

func assertStoreDiscardWarn(t *testing.T, rec logtest.Record) {
	t.Helper()
	logtest.AssertRecord(t, rec, logtest.RecordWant{
		Level:     slog.LevelWarn,
		Msg:       "discard",
		Component: "hooks",
		Op:        "discard",
		Via:       "panel",
	})
	if got := rec.AttrOrEmpty("hook_key"); got != samplePayload().HookKey {
		t.Errorf("WARN hook_key = %q, want %q", got, samplePayload().HookKey)
	}
}

func TestResumeDiscardReport_StoreRefusals(t *testing.T) {
	t.Run("it names an unreadable hooks.json by the OS's words and logs the whole chain", func(t *testing.T) {
		path := stageDiscardStore(t, hookstest.Staging{Entries: registeredEntries()})
		before := hookstest.HooksFileBytes(t, path)
		denial := themetest.DenyRead(t, path)

		probe, sink := discardAgainstStore(t)

		rec := assertRefusedOnConfirmation(t, probe, sink, "can't read hooks.json: permission denied")
		assertStoreDiscardWarn(t, rec)
		logged := rec.ErrorAttr(t, "error")
		if !errors.Is(logged, syscall.EACCES) || !strings.Contains(logged.Error(), path) {
			t.Errorf("WARN error = %v, want the whole chain naming %s and carrying %v", logged, path, denial)
		}
		if err := os.Chmod(path, 0o600); err != nil {
			t.Fatalf("chmod: %v", err)
		}
		hookstest.AssertHooksFileUnchanged(t, path, before, "changed by a refused read")
	})

	t.Run("it names malformed JSON and logs the parser's error", func(t *testing.T) {
		path := stageDiscardStore(t, hookstest.Staging{Seed: "{not json"})

		probe, sink := discardAgainstStore(t)

		rec := assertRefusedOnConfirmation(t, probe, sink, "can't read hooks.json: malformed JSON")
		assertStoreDiscardWarn(t, rec)
		logged := rec.ErrorAttr(t, "error")
		if !errors.Is(logged, hooks.ErrMalformed) {
			t.Errorf("WARN error = %v, want the malformed-JSON chain", logged)
		}
		if !strings.Contains(logged.Error(), "invalid character") {
			t.Errorf("WARN error = %v, want the parser's own words", logged)
		}
		if got := string(hookstest.HooksFileBytes(t, path)); got != "{not json" {
			t.Errorf("hooks.json = %q, want it untouched", got)
		}
	})

	t.Run("it names a lock another process holds", func(t *testing.T) {
		hooks.SetLockTimeoutForTest(t, 40*time.Millisecond)
		path := stageDiscardStore(t, hookstest.Staging{Entries: registeredEntries()})
		hookstest.HoldHooksSidecar(t, path)

		probe, sink := discardAgainstStore(t)

		rec := assertRefusedOnConfirmation(t, probe, sink, "can't lock hooks.json: another process holds it")
		assertStoreDiscardWarn(t, rec)
		if err := rec.ErrorAttr(t, "error"); !errors.Is(err, hooks.ErrLockHeld) {
			t.Errorf("WARN error = %v, want %v", err, hooks.ErrLockHeld)
		}
	})

	t.Run("it names a lock that cannot be taken by the OS's words", func(t *testing.T) {
		path := stageDiscardStore(t, hookstest.Staging{Entries: registeredEntries()})
		_ = themetest.DenyRead(t, hookstest.SidecarPath(path))

		probe, sink := discardAgainstStore(t)

		rec := assertRefusedOnConfirmation(t, probe, sink, "can't lock hooks.json: permission denied")
		assertStoreDiscardWarn(t, rec)
		if err := rec.ErrorAttr(t, "error"); !errors.Is(err, syscall.EACCES) {
			t.Errorf("WARN error = %v, want the lock's own failure", err)
		}
	})

	t.Run("it names a failed write by the OS's words", func(t *testing.T) {
		path := stageDiscardStore(t, hookstest.Staging{Entries: registeredEntries(), WritesDenied: true})
		before := hookstest.HooksFileBytes(t, path)

		probe, sink := discardAgainstStore(t)

		rec := assertRefusedOnConfirmation(t, probe, sink, "can't write hooks.json: permission denied")
		assertStoreDiscardWarn(t, rec)
		if !rec.HasAttr("error_class") {
			t.Error("WARN carries no error_class, want the store's write-failure record")
		}
		hookstest.AssertHooksFileUnchanged(t, path, before, "changed by a refused write")
	})

	t.Run("it names a store it cannot locate and logs one hydrate WARN naming the pane", func(t *testing.T) {
		t.Setenv("PORTAL_HOOKS_FILE", "")
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("HOME", "")
		_, wantErr := loadHookStore()
		if wantErr == nil {
			t.Fatal("loadHookStore() resolved a store; the fixture must leave none")
		}

		probe, sink := discardAgainstStore(t)

		rec := assertRefusedOnConfirmation(t, probe, sink, "can't locate hooks.json: $HOME is not defined")
		if got := rec.AttrOrEmpty("component"); got != "hydrate" {
			t.Errorf("WARN component = %q, want hydrate", got)
		}
		if got := rec.AttrOrEmpty("hook_key"); got != samplePayload().HookKey {
			t.Errorf("WARN hook_key = %q, want %q", got, samplePayload().HookKey)
		}
		if got := rec.AttrOrEmpty("pane_key"); got != samplePayload().PaneKey {
			t.Errorf("WARN pane_key = %q, want %q", got, samplePayload().PaneKey)
		}
		if got := rec.ErrorAttr(t, "error"); got.Error() != wantErr.Error() {
			t.Errorf("WARN error = %v, want the whole resolution error %v", got, wantErr)
		}
	})
}

func TestResumeDiscardReport_Rows(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"a held lock", fmt.Errorf("%w: /x/hooks.json.lock", hooks.ErrLockHeld), "can't lock hooks.json: another process holds it"},
		{"a write with no space left", fmt.Errorf("%w: failed to write temp file: %w", fileutil.ErrWriteWrite, &os.PathError{Op: "write", Path: "/x/.atomic-1.tmp", Err: syscall.ENOSPC}), "can't write hooks.json: no space left on device"},
		{"an error outside every class", errors.New("something no class names"), "can't carry out that answer"},
	}
	for _, tc := range cases {
		t.Run("it reports "+tc.name+" as "+tc.want, func(t *testing.T) {
			if got := resumeDiscardRefusal(tc.err); got != tc.want {
				t.Errorf("resumeDiscardRefusal(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

func TestResumeClearReport(t *testing.T) {
	answers := []struct {
		name  string
		keys  string
		start resumeChainPayload
	}{
		{"Enter", "\r", samplePayload()},
		{"y after a successful removal", "y", confirmationPayload()},
	}
	refusals := []struct {
		name string
		err  func(t *testing.T) error
		want string
	}{
		{"tmux refuses the clear", tmuxRefusal, "can't unpause this pane: can't find pane: %7"},
		{"tmux cannot be run", tmuxUnrunnable, "can't unpause this pane: tmux could not be run"},
		{"the clear fails outside every class", func(*testing.T) error { return errors.New("marker refused") }, "can't carry out that answer"},
	}
	for _, a := range answers {
		for _, r := range refusals {
			t.Run(a.name+" when "+r.name+" redraws the panel with its reason and one record", func(t *testing.T) {
				var probe resumeWaitProbe
				probe.clearErr = r.err(t)
				if err := runResumeWait(newResumeWaitConfig(t, &probe, a.start, keystrokes(t, a.keys))); err != nil {
					t.Fatalf("runResumeWait() error = %v", err)
				}

				if got := reportCarried(t, probe.execArgs); got != r.want {
					t.Errorf("report row = %q, want %q", got, r.want)
				}
				assertArgvLacks(t, probe.execArgs, resumeFlagScreen)
				if probe.pin.unpinCalls != 0 {
					t.Errorf("unpins = %d, want the pin left for the redraw", probe.pin.unpinCalls)
				}
				rec := probe.sink.Records().AtOrAboveLevel(slog.LevelWarn).Only(t, "the refused clear's record")
				if rec.Msg != "unset resume pending marker failed" {
					t.Errorf("WARN = %q, want the failed clear's record", rec.Msg)
				}
				if got := rec.AttrOrEmpty("hook_key"); got != a.start.HookKey {
					t.Errorf("WARN hook_key = %q, want %q", got, a.start.HookKey)
				}
				if got := rec.AttrOrEmpty("pane_key"); got != a.start.PaneKey {
					t.Errorf("WARN pane_key = %q, want %q", got, a.start.PaneKey)
				}
				if got := rec.ErrorAttr(t, "error"); got.Error() != probe.clearErr.Error() {
					t.Errorf("WARN error = %v, want the whole chain %v", got, probe.clearErr)
				}
			})
		}
	}

	t.Run("its row carries no tmux argv and no option name", func(t *testing.T) {
		row := resumeClearRefusal(tmuxRefusal(t))
		for _, leaked := range []string{"set-option", state.ResumePendingOption, "exit"} {
			if strings.Contains(row, leaked) {
				t.Errorf("row %q carries %q", row, leaked)
			}
		}
	})
}

func TestResumeReportRows_FitTheCard(t *testing.T) {
	type prefixed struct{ act, cause string }
	paint := func(t *testing.T, render func(tui.ResumeScreen) string, row string) string {
		t.Helper()
		return render(tui.ResumeScreen{
			Command:    samplePayload().Command,
			Report:     row,
			Width:      100,
			Height:     30,
			Theme:      themetest.DefaultDark(t),
			Colourless: true,
		})
	}
	renders := []func(tui.ResumeScreen) string{tui.RenderResumePanel, tui.RenderResumeDiscardConfirm}

	whole := []string{
		resumeReportMalformed,
		resumeReportLockHeld,
		resumeReportTmuxAbsent,
		resumeReportFallback,
	}
	for _, p := range []prefixed{
		{resumeReportRead, "permission denied"},
		{resumeReportLock, "permission denied"},
		{resumeReportWrite, "no space left on device"},
		{resumeReportWrite, "permission denied"},
		{resumeReportLocate, "$HOME is not defined"},
		{resumeReportUnpause, "can't find pane: %7"},
	} {
		whole = append(whole, p.act+p.cause)
	}
	for _, row := range whole {
		t.Run(row, func(t *testing.T) {
			for _, render := range renders {
				if painted := paint(t, render, row); !strings.Contains(painted, row) {
					t.Errorf("the card does not carry the whole row %q:\n%s", row, painted)
				}
			}
		})
	}

	// A drop's cause is the operating system's, and can run past the card.
	drop := prefixed{resumeReportDrop, syscall.ENOTTY.Error()}
	t.Run(drop.act+drop.cause, func(t *testing.T) {
		for _, render := range renders {
			painted := paint(t, render, drop.act+drop.cause)
			_, after, found := strings.Cut(painted, drop.act)
			if !found {
				t.Errorf("the card does not carry the whole act %q:\n%s", drop.act, painted)
				continue
			}
			if !cutInsideCause(after, drop.cause) {
				t.Errorf("after the act the card carries %q, want the cause %q whole or cut inside it:\n%s",
					strings.SplitN(after, "\n", 2)[0], drop.cause, painted)
			}
		}
	})
}

// cutInsideCause reports whether rendered, the text following a report row's
// act, opens with the whole cause or with a non-empty proper prefix of it
// closed by the truncation's ellipsis.
func cutInsideCause(rendered, cause string) bool {
	if strings.HasPrefix(rendered, cause) {
		return true
	}
	for kept := len(cause) - 1; kept > 0; kept-- {
		if strings.HasPrefix(rendered, cause[:kept]+"…") {
			return true
		}
	}
	return false
}
