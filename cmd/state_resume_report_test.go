package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/themetest"
	"github.com/leeovery/portal/internal/tui"
)

// flagRefusal is the error a resume-pending flag that can't be lowered
// returns, as the production clear meets it: one in a directory that can't
// be searched.
func flagRefusal(t *testing.T) error {
	t.Helper()
	dir := resumeStateDir(t)
	raiseFlag(t, dir, samplePayload().Session)
	unsearchable(t, filepath.Join(dir, string(state.ResumePending)))
	err := clearResumePending(samplePayload().Session)
	if err == nil {
		t.Fatal("the flag was lowered; the fixture must refuse it")
	}
	return err
}

// unnamedRefusal is the error the clear returns for a session no file can
// carry.
func unnamedRefusal(t *testing.T) error {
	t.Helper()
	resumeStateDir(t)
	err := clearResumePending("")
	if err == nil {
		t.Fatal("an unnamed session's flag was lowered; the fixture must refuse it")
	}
	return err
}

// unresolvedStateRefusal is the error the clear returns when the state
// directory can't be resolved.
func unresolvedStateRefusal(t *testing.T) error {
	t.Helper()
	t.Setenv("PORTAL_STATE_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")
	err := clearResumePending(samplePayload().Session)
	if err == nil {
		t.Fatal("a flag was lowered with no state directory; the fixture must refuse it")
	}
	return err
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
	cfg.DiscardRegistration = discardResumeRegistration
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
	return map[string]string{samplePayload().Session: samplePayload().Command}
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
		t.Errorf("ClearPending called %d times, want 0: the marker stands", probe.clearCalls)
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
	if got := rec.AttrOrEmpty("hook_key"); got != samplePayload().Session {
		t.Errorf("WARN hook_key = %q, want %q", got, samplePayload().Session)
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
		if got := rec.AttrOrEmpty("session"); got != samplePayload().Session {
			t.Errorf("WARN session = %q, want %q", got, samplePayload().Session)
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
		{"the flag's directory can't be searched", flagRefusal, "can't unpause this session: permission denied"},
		{"the session's name can't name a file", unnamedRefusal, "can't unpause this session: " + state.ErrUnsafeName.Error()},
		{"the state directory can't be resolved", unresolvedStateRefusal, "can't unpause this session: $HOME is not defined"},
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
				rec := probe.sink.Records().AtOrAboveLevel(slog.LevelWarn).Only(t, "the refused clear's record")
				if rec.Msg != "clear resume pending flag failed" {
					t.Errorf("WARN = %q, want the failed clear's record", rec.Msg)
				}
				if got := rec.AttrOrEmpty("session"); got != a.start.Session {
					t.Errorf("WARN session = %q, want %q", got, a.start.Session)
				}
				if got := rec.ErrorAttr(t, "error"); got.Error() != probe.clearErr.Error() {
					t.Errorf("WARN error = %v, want the whole chain %v", got, probe.clearErr)
				}
			})
		}
	}

	t.Run("its row carries no path and no flag name", func(t *testing.T) {
		row := resumeClearRefusal(flagRefusal(t))
		for _, leaked := range []string{"/", string(state.ResumePending), "remove"} {
			if strings.Contains(row, leaked) {
				t.Errorf("row %q carries %q", row, leaked)
			}
		}
	})
}

// The rows are built from what backToPicker really returns: runners on a
// registry of the test's own, and the fake backend.
func TestResumeBackReport(t *testing.T) {
	const session = "proj-a1b2"
	notAttached, ending := "the runner is not attached", "the attach is ending"

	t.Run("runners refusing for one reason name it once", func(t *testing.T) {
		r, b := pickerDetachWorld(t, session)
		withBackendDeps(t, BackendDeps{Backend: b})
		registerDetachHandler(t, r, "first1", session, &detachHandler{refuse: errors.New(notAttached)})
		registerDetachHandler(t, r, "second", session, &detachHandler{refuse: errors.New(notAttached)})

		if got, want := resumeBackRefusal(backToPicker(session)), resumeReportBack+notAttached; got != want {
			t.Errorf("row = %q, want %q", got, want)
		}
	})

	t.Run("runners refusing for different reasons name each", func(t *testing.T) {
		r, b := pickerDetachWorld(t, session)
		withBackendDeps(t, BackendDeps{Backend: b})
		registerDetachHandler(t, r, "first1", session, &detachHandler{refuse: errors.New(notAttached)})
		registerDetachHandler(t, r, "second", session, &detachHandler{refuse: errors.New(ending)})

		got := resumeBackRefusal(backToPicker(session))
		reasons, found := strings.CutPrefix(got, resumeReportBack)
		if !found {
			t.Fatalf("row = %q, want it to open %q", got, resumeReportBack)
		}
		if parts := strings.Split(reasons, resumeReasonSeparator); !slices.Contains(parts, notAttached) || !slices.Contains(parts, ending) || len(parts) != 2 {
			t.Errorf("row = %q, want each runner's reason once", got)
		}
	})

	t.Run("a backend that fails to detach names the detach", func(t *testing.T) {
		_, b := pickerDetachWorld(t, session)
		withBackendDeps(t, BackendDeps{Backend: b})
		b.Fail(backendtest.OpDetach, backend.ErrTimeout)

		if got, want := resumeBackRefusal(backToPicker(session)), resumeReportDetach+backend.ErrTimeout.Error(); got != want {
			t.Errorf("row = %q, want %q", got, want)
		}
	})

	t.Run("a state directory that can't be resolved names the cause", func(t *testing.T) {
		t.Setenv("PORTAL_STATE_DIR", "")
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("HOME", "")

		if got, want := resumeBackRefusal(backToPicker(session)), resumeReportBack+"$HOME is not defined"; got != want {
			t.Errorf("row = %q, want %q", got, want)
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
		resumeReportFallback,
	}
	for _, p := range []prefixed{
		{resumeReportRead, "permission denied"},
		{resumeReportLock, "permission denied"},
		{resumeReportWrite, "no space left on device"},
		{resumeReportWrite, "permission denied"},
		{resumeReportLocate, "$HOME is not defined"},
		{resumeReportUnpause, "permission denied"},
		{resumeReportUnpause, "$HOME is not defined"},
		{resumeReportBack, "the runner is not attached"},
		{resumeReportDetach, backend.ErrTimeout.Error()},
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

	// A drop's cause can run past the card.
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
