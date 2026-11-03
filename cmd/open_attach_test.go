package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/attachkeys"
	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/runner"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/spf13/cobra"
)

// pickerScript stands in for the picker an attach's end asks for: it records
// the flash each opens on and answers with picks in turn, quitting once they
// run out.
type pickerScript struct {
	picks   []string
	flashes []string
	err     error
	// onOpen runs as each picker opens.
	onOpen func()
}

func stagePicker(t *testing.T, picks ...string) *pickerScript {
	t.Helper()
	p := &pickerScript{picks: picks}
	withFuncSeam(t, &reopenPickerFunc, func(_ *cobra.Command, _ backend.Backend, flash string) (string, error) {
		p.flashes = append(p.flashes, flash)
		if p.onOpen != nil {
			p.onOpen()
		}
		if p.err != nil || len(p.picks) == 0 {
			return "", p.err
		}
		pick := p.picks[0]
		p.picks = p.picks[1:]
		return pick, nil
	})
	return p
}

func outcome(action attachkeys.Action) runResult {
	return runResult{out: runner.Outcome{Action: action}}
}

func ranSessions(rec *attachRecorder) []string {
	names := make([]string, 0, len(rec.runs))
	for _, cfg := range rec.runs {
		names = append(names, cfg.Session)
	}
	return names
}

func TestAttach_AnEndThatExitsToTheShell(t *testing.T) {
	boom := errors.New("pty: boom")
	for _, tc := range []struct {
		name     string
		result   runResult
		wantCode int
		wantErr  error
	}{
		{name: "detach to the shell exits 0", result: outcome(attachkeys.DetachToShell)},
		{name: "a session that ends with status 0 exits 0", result: runResult{}},
		{name: "a session that ends with a status exits with it", result: runResult{out: runner.Outcome{ExitCode: 3}}, wantCode: 3},
		{name: "a signal exits as a shell reports one", result: runResult{out: runner.Outcome{ExitCode: 129}}, wantCode: 129},
		{name: "an error from the runner is returned", result: runResult{err: boom}, wantErr: boom},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, rec := stageAttach(t, "api-x7Kd9a")
			rec.answer(tc.result)
			picker := stagePicker(t)

			err := openSession(contextCommand(), "api-x7Kd9a")

			switch {
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("openSession = %v, want %v", err, tc.wantErr)
				}
			case tc.wantCode != 0:
				exit, ok := errors.AsType[*ExitCodeError](err)
				if !ok || exit.Code != tc.wantCode {
					t.Fatalf("openSession = %v, want exit code %d", err, tc.wantCode)
				}
				if !IsSilentExitError(err) {
					t.Error("the exit code error is not silent; the shell would see Portal's words")
				}
			case err != nil:
				t.Errorf("openSession = %v, want nil", err)
			}
			if len(picker.flashes) != 0 {
				t.Errorf("the picker opened %d times, want never", len(picker.flashes))
			}
			rec.onlyRun(t)
		})
	}
}

func TestAttach_PickerLoop(t *testing.T) {
	t.Run("a detach opens the picker, and its pick attaches", func(t *testing.T) {
		_, rec := stageAttach(t, "api-x7Kd9a", "blog-c3d4")
		rec.answer(outcome(attachkeys.Detach), outcome(attachkeys.DetachToShell))
		picker := stagePicker(t, "blog-c3d4")

		if err := openSession(contextCommand(), "api-x7Kd9a"); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		if want := []string{"api-x7Kd9a", "blog-c3d4"}; !slices.Equal(ranSessions(rec), want) {
			t.Errorf("attached %q, want %q", ranSessions(rec), want)
		}
		if want := []string{""}; !slices.Equal(picker.flashes, want) {
			t.Errorf("picker flashes = %q, want one picker with no flash", picker.flashes)
		}
	})

	t.Run("quitting the picker exits 0", func(t *testing.T) {
		_, rec := stageAttach(t, "api-x7Kd9a")
		rec.answer(outcome(attachkeys.Detach))
		picker := stagePicker(t)

		if err := openSession(contextCommand(), "api-x7Kd9a"); err != nil {
			t.Errorf("openSession = %v, want nil", err)
		}
		if len(picker.flashes) != 1 || len(rec.runs) != 1 {
			t.Errorf("pickers %d, attaches %d; want one of each", len(picker.flashes), len(rec.runs))
		}
	})

	t.Run("the loop goes on until the picker is quit", func(t *testing.T) {
		_, rec := stageAttach(t, "api-x7Kd9a", "blog-c3d4")
		rec.answer(outcome(attachkeys.Detach), outcome(attachkeys.Kill), outcome(attachkeys.Detach))
		picker := stagePicker(t, "blog-c3d4", "api-x7Kd9a")

		if err := openSession(contextCommand(), "api-x7Kd9a"); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		if want := []string{"api-x7Kd9a", "blog-c3d4", "api-x7Kd9a"}; !slices.Equal(ranSessions(rec), want) {
			t.Errorf("attached %q, want %q", ranSessions(rec), want)
		}
		if len(picker.flashes) != 3 {
			t.Errorf("pickers = %d, want 3", len(picker.flashes))
		}
	})

	t.Run("a kill opens the picker with no flash", func(t *testing.T) {
		_, rec := stageAttach(t, "api-x7Kd9a")
		rec.answer(outcome(attachkeys.Kill))
		picker := stagePicker(t)

		if err := openSession(contextCommand(), "api-x7Kd9a"); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		if want := []string{""}; !slices.Equal(picker.flashes, want) {
			t.Errorf("picker flashes = %q, want one picker with no flash", picker.flashes)
		}
	})

	t.Run("a failed kill flashes in the picker", func(t *testing.T) {
		_, rec := stageAttach(t, "api-x7Kd9a")
		const why = "session api-x7Kd9a is not answering; the kill may still land"
		rec.answer(runResult{out: runner.Outcome{Action: attachkeys.Kill, Err: errors.New(why)}})
		picker := stagePicker(t)

		if err := openSession(contextCommand(), "api-x7Kd9a"); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		if want := []string{why}; !slices.Equal(picker.flashes, want) {
			t.Errorf("picker flashes = %q, want %q", picker.flashes, want)
		}
	})

	t.Run("a picker that fails returns its error", func(t *testing.T) {
		_, rec := stageAttach(t, "api-x7Kd9a")
		rec.answer(outcome(attachkeys.Detach))
		picker := stagePicker(t)
		picker.err = errors.New("list sessions: boom")

		if err := openSession(contextCommand(), "api-x7Kd9a"); !errors.Is(err, picker.err) {
			t.Errorf("openSession = %v, want the picker's %v", err, picker.err)
		}
	})

	t.Run("a pick inside a session execs its switch and ends the loop", func(t *testing.T) {
		fake, rec := stageAttach(t, "api-x7Kd9a", "blog-c3d4")
		rec.answer(outcome(attachkeys.Detach))
		picker := stagePicker(t, "blog-c3d4")
		picker.onOpen = func() { fake.SetCurrent("api-x7Kd9a") }

		if err := openSession(contextCommand(), "api-x7Kd9a"); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		if len(rec.runs) != 1 || len(rec.execs) != 1 || rec.execs[0].argv[1] != "blog-c3d4" {
			t.Errorf("runs %+v, execs %+v; want the first attach run and the pick exec'd", ranSessions(rec), rec.execs)
		}
	})
}

func TestAttach_KillKeyKillsAsPortalKillDoes(t *testing.T) {
	t.Run("the session goes, its index entry with it", func(t *testing.T) {
		fake, rec := stageAttach(t, "api-x7Kd9a")
		stateDir := seedSessionsJSON(t, `{"version":2,"sessions":[{"name":"api-x7Kd9a","label":"work"}]}`)
		stagePicker(t)

		if err := openSession(contextCommand(), "api-x7Kd9a"); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		if err := rec.onlyRun(t).Kill(context.Background(), "api-x7Kd9a"); err != nil {
			t.Fatalf("Kill: %v", err)
		}
		if alive, _ := fake.Exists(context.Background(), "api-x7Kd9a"); alive {
			t.Error("the session outlived the kill")
		}
		idx, err := sessionindex.Read(stateDir)
		if err != nil {
			t.Fatalf("read index: %v", err)
		}
		if _, ok := idx.Lookup("api-x7Kd9a"); ok {
			t.Errorf("index = %+v, want the killed session's entry gone", idx.Sessions)
		}
	})

	t.Run("a refusal is worded as portal kill words it", func(t *testing.T) {
		fake, rec := stageAttach(t, "api-x7Kd9a")
		isolateStateDir(t)
		fake.Fail(backendtest.OpKill, backend.ErrTimeout)
		stagePicker(t)

		if err := openSession(contextCommand(), "api-x7Kd9a"); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		err := rec.onlyRun(t).Kill(context.Background(), "api-x7Kd9a")
		if want := "session api-x7Kd9a is not answering; the kill may still land"; err == nil || err.Error() != want {
			t.Errorf("Kill = %v, want %q", err, want)
		}
	})
}

func TestAttach_KeyConfig(t *testing.T) {
	const altX, altD = "\x1bx", "\x1bd"

	scans := func(t *testing.T, cfg runner.Config, read string) (attachkeys.Action, bool) {
		t.Helper()
		_, action, found := cfg.Scanner.Scan([]byte(read))
		return action, found
	}

	t.Run("the attach watches for the keys prefs.json binds", func(t *testing.T) {
		setPrefsFile(t, `{"keys":{"detach":"alt+x"}}`)
		_, rec := stageAttach(t, "api-x7Kd9a")

		if err := openSession(contextCommand(), "api-x7Kd9a"); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		cfg := rec.onlyRun(t)
		if action, found := scans(t, cfg, altX); !found || action != attachkeys.Detach {
			t.Errorf("alt+x scans as %v, %v; want detach", action, found)
		}
		if _, found := scans(t, cfg, altD); found {
			t.Error("alt+d is still bound after prefs.json moved detach to alt+x")
		}
	})

	t.Run("they are read at each attach", func(t *testing.T) {
		path := setPrefsFile(t, `{"keys":{"detach":"alt+x"}}`)
		_, rec := stageAttach(t, "api-x7Kd9a")
		rec.answer(outcome(attachkeys.Detach))
		picker := stagePicker(t, "api-x7Kd9a")
		picker.onOpen = func() {
			if err := os.WriteFile(path, []byte(`{}`), 0o644); err != nil {
				t.Fatal(err)
			}
		}

		if err := openSession(contextCommand(), "api-x7Kd9a"); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		if len(rec.runs) != 2 {
			t.Fatalf("attaches = %d, want 2", len(rec.runs))
		}
		if action, found := scans(t, rec.runs[1], altD); !found || action != attachkeys.Detach {
			t.Errorf("the second attach scans alt+d as %v, %v; want detach, its default once prefs.json binds nothing", action, found)
		}
	})

	t.Run("each warning is logged at WARN at each attach", func(t *testing.T) {
		sink := logtest.Install(t)
		const keys = `{"detach":"alt+D","kil":"alt+k"}`
		setPrefsFile(t, `{"keys":`+keys+`}`)
		_, rec := stageAttach(t, "api-x7Kd9a")
		rec.answer(outcome(attachkeys.Detach))
		stagePicker(t, "api-x7Kd9a")

		if err := openSession(contextCommand(), "api-x7Kd9a"); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		_, warnings := attachkeys.Parse(json.RawMessage(keys))
		if len(warnings) != 2 {
			t.Fatalf("Parse gave %d warnings, want 2; the case is unarmed", len(warnings))
		}
		records := sink.Records().Matching("attach", "key config")
		if want := 2 * len(warnings); len(records) != want {
			t.Fatalf("key config records = %d, want %d: every warning at each of two attaches", len(records), want)
		}
		for i, r := range records {
			if r.Level != slog.LevelWarn {
				t.Errorf("record %d level = %v, want WARN", i, r.Level)
			}
			if got, want := r.AttrString(t, "detail"), warnings[i%len(warnings)].String(); got != want {
				t.Errorf("record %d detail = %q, want %q", i, got, want)
			}
			if got := r.AttrString(t, "session"); got != "api-x7Kd9a" {
				t.Errorf("record %d session = %q, want api-x7Kd9a", i, got)
			}
		}
		if action, found := scans(t, rec.runs[0], altD); !found || action != attachkeys.Detach {
			t.Errorf("alt+d scans as %v, %v; want detach, the default a bad binding falls back to", action, found)
		}
	})

	t.Run("unreadable prefs keep the defaults, with a WARN", func(t *testing.T) {
		sink := logtest.Install(t)
		setPrefsFile(t, `{"keys":`)
		_, rec := stageAttach(t, "api-x7Kd9a")

		if err := openSession(contextCommand(), "api-x7Kd9a"); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		r := sink.Records().Matching("attach", "key config").Only(t, "key config record")
		if r.Level != slog.LevelWarn {
			t.Errorf("level = %v, want WARN", r.Level)
		}
		if r.AttrString(t, "error") == "" {
			t.Error("the record names no error")
		}
		for read, want := range map[string]attachkeys.Action{altD: attachkeys.Detach, "\x1bD": attachkeys.DetachToShell, "\x1bq": attachkeys.Kill} {
			if action, found := scans(t, rec.onlyRun(t), read); !found || action != want {
				t.Errorf("%q scans as %v, %v; want %v", read, action, found, want)
			}
		}
	})

	t.Run("keys that all parse log nothing", func(t *testing.T) {
		sink := logtest.Install(t)
		setPrefsFile(t, `{"keys":{"kill":["alt+q","ctrl+o"]}}`)
		stageAttach(t, "api-x7Kd9a")

		if err := openSession(contextCommand(), "api-x7Kd9a"); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		if got := sink.Records().Matching("attach", "key config"); len(got) != 0 {
			t.Errorf("key config records = %+v, want none", got)
		}
	})
}

func TestBuildTUIModel_OpensOnTheLandingsFlash(t *testing.T) {
	const why = "session api-x7Kd9a is not answering; the kill may still land"
	m := landPicker(t, buildTUIModel(defaultTestTUIConfig(), pickerLanding{flash: why}, nil))

	if view := ansi.Strip(m.View().Content); !strings.Contains(view, why) {
		t.Errorf("the picker shows no %q:\n%s", why, view)
	}
	if m.SessionListFilterValue() != "" || m.CommandPending() {
		t.Errorf("filter %q, command pending %v; want a plain picker", m.SessionListFilterValue(), m.CommandPending())
	}
}
