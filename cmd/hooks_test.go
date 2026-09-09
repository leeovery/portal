package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

func TestHooksListCommand(t *testing.T) {
	t.Run("outputs hooks in tab-separated format", func(t *testing.T) {
		hooksFileInTempDir(t, map[string]map[string]string{
			hookstest.SubjectSeedA: {"on-resume": "claude --resume abc123"},
		})

		// Stub bootstrap so the real orchestrator never runs against the test's
		// tmux server.
		withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

		withHooksDeps(t, HooksDeps{PaneLister: &recordingPaneHookLister{rows: []tmux.PaneHookRow{
			{Token: hookstest.SubjectSeedA, Location: "my-project-abc123:0.0"},
		}}})

		got := runHookList(t)
		want := hookstest.SubjectSeedA + "\ton-resume\tclaude --resume abc123\tmy-project-abc123:0.0\n"
		if got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
	})

	t.Run("produces empty output when no hooks registered", func(t *testing.T) {
		hooksFileInTempDir(t, map[string]map[string]string{})

		withHooksDeps(t, HooksDeps{PaneLister: &loudPaneHookLister{t: t}})

		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"hooks", "list"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := buf.String()
		if got != "" {
			t.Errorf("output = %q, want empty string", got)
		}
	})

	t.Run("produces empty output when hooks file does not exist", func(t *testing.T) {
		hooksFileInTempDir(t, nil)

		withHooksDeps(t, HooksDeps{PaneLister: &loudPaneHookLister{t: t}})

		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"hooks", "list"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := buf.String()
		if got != "" {
			t.Errorf("output = %q, want empty string", got)
		}
	})

	t.Run("it keeps the sort by key then event", func(t *testing.T) {
		hooksFileInTempDir(t, map[string]map[string]string{
			hookstest.SubjectSeedC: {"on-resume": "claude --resume def456"},
			hookstest.SubjectSeedB: {"on-resume": "claude --resume abc123"},
			hookstest.SubjectSeedA: {"on-resume": "npm start"},
		})

		// Stub bootstrap so the real orchestrator never runs against the test's
		// tmux server.
		withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

		withHooksDeps(t, HooksDeps{PaneLister: &recordingPaneHookLister{rows: []tmux.PaneHookRow{
			{Token: hookstest.SubjectSeedC, Location: "proj-abc:1.0"},
			{Token: hookstest.SubjectSeedB, Location: "proj-abc:0.0"},
			{Token: hookstest.SubjectSeedA, Location: "other-proj:0.0"},
		}}})

		got := runHookList(t)
		want := hookstest.SubjectSeedA + "\ton-resume\tnpm start\tother-proj:0.0\n" +
			hookstest.SubjectSeedB + "\ton-resume\tclaude --resume abc123\tproj-abc:0.0\n" +
			hookstest.SubjectSeedC + "\ton-resume\tclaude --resume def456\tproj-abc:1.0\n"
		if got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
	})

	t.Run("accepts no arguments", func(t *testing.T) {
		hooksFileInTempDir(t, nil)

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "list", "extraarg"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected error for extra argument, got nil")
		}
	})
}

func TestHooksListLocationColumn(t *testing.T) {
	t.Run("it appends the resolved location as a fourth column", func(t *testing.T) {
		hooksFileInTempDir(t, map[string]map[string]string{
			hookstest.SubjectSeedA: {"on-resume": "claude --resume abc123"},
		})

		withHooksDeps(t, HooksDeps{PaneLister: &recordingPaneHookLister{rows: []tmux.PaneHookRow{
			{Token: hookstest.SubjectSeedA, Location: "my-project-abc123:0.0"},
		}}})

		got := runHookList(t)
		want := hookstest.SubjectSeedA + "\ton-resume\tclaude --resume abc123\tmy-project-abc123:0.0\n"
		if got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
	})

	t.Run("it does not let an unstamped pane's row lend its location", func(t *testing.T) {
		hooksFileInTempDir(t, map[string]map[string]string{
			"": {"on-resume": "npm start"},
		})

		withHooksDeps(t, HooksDeps{PaneLister: &recordingPaneHookLister{rows: []tmux.PaneHookRow{
			{Token: "", Location: "unstamped-sess:0.0"},
		}}})

		got := runHookList(t)
		want := "\ton-resume\tnpm start\t\n"
		if got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
	})

	t.Run("it renders one line for a token carried by two rows", func(t *testing.T) {
		hooksFileInTempDir(t, map[string]map[string]string{
			hookstest.SubjectSeedA: {"on-resume": "npm start"},
		})

		withHooksDeps(t, HooksDeps{PaneLister: &recordingPaneHookLister{rows: []tmux.PaneHookRow{
			{Token: hookstest.SubjectSeedA, Location: "first-sess:0.0"},
			{Token: hookstest.SubjectSeedA, Location: "second-sess:1.2"},
		}}})

		got := runHookList(t)
		want := hookstest.SubjectSeedA + "\ton-resume\tnpm start\tfirst-sess:0.0\n"
		if got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
	})

	t.Run("it renders every fourth field empty when the enumeration fails", func(t *testing.T) {
		hooksFileInTempDir(t, map[string]map[string]string{
			hookstest.SubjectSeedA: {"on-resume": "claude --resume abc123"},
			hookstest.SubjectSeedB: {"on-resume": "npm start"},
		})

		// Rows alongside the error: a failed read must be judged by the error, not
		// by whether the lister also handed back rows.
		withHooksDeps(t, HooksDeps{PaneLister: &recordingPaneHookLister{
			rows: []tmux.PaneHookRow{{Token: hookstest.SubjectSeedA, Location: "my-project:0.0"}},
			err:  errors.New("no server running"),
		}})

		got := runHookList(t)
		want := hookstest.SubjectSeedA + "\ton-resume\tclaude --resume abc123\t\n" + hookstest.SubjectSeedB + "\ton-resume\tnpm start\t\n"
		if got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
	})

	t.Run("it takes no enumeration read when there are no entries", func(t *testing.T) {
		hooksFileInTempDir(t, map[string]map[string]string{})

		withHooksDeps(t, HooksDeps{PaneLister: &loudPaneHookLister{t: t}})

		got := runHookList(t)
		if got != "" {
			t.Errorf("output = %q, want empty string", got)
		}
	})

	t.Run("it renders an empty fourth field for a token no row carries", func(t *testing.T) {
		hooksFileInTempDir(t, map[string]map[string]string{
			hookstest.SubjectSeedC: {"on-resume": "npm start"},
		})

		withHooksDeps(t, HooksDeps{PaneLister: &recordingPaneHookLister{rows: []tmux.PaneHookRow{
			{Token: hookstest.SubjectSeedA, Location: "my-project:0.0"},
			{Token: hookstest.SubjectSeedB, Location: "my-project:0.1"},
		}}})

		got := runHookList(t)
		want := hookstest.SubjectSeedC + "\ton-resume\tnpm start\t\n"
		if got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
	})

	t.Run("it renders an empty fourth field for an old-format key", func(t *testing.T) {
		hooksFileInTempDir(t, map[string]map[string]string{
			hookstest.SubjectSeedA:     {"on-resume": "claude --resume abc123"},
			hookstest.UnjudgeableSeedA: {"on-resume": "npm start"},
		})

		withHooksDeps(t, HooksDeps{PaneLister: &recordingPaneHookLister{rows: []tmux.PaneHookRow{
			{Token: hookstest.SubjectSeedA, Location: "sess:0.0"},
		}}})

		got := runHookList(t)
		want := hookstest.SubjectSeedA + "\ton-resume\tclaude --resume abc123\tsess:0.0\n" +
			hookstest.UnjudgeableSeedA + "\ton-resume\tnpm start\t\n"
		if got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
	})

	t.Run("it renders a location whose session name carries a pipe verbatim", func(t *testing.T) {
		hooksFileInTempDir(t, map[string]map[string]string{
			hookstest.SubjectSeedA: {"on-resume": "npm start"},
		})

		withHooksDeps(t, HooksDeps{PaneLister: &recordingPaneHookLister{rows: []tmux.PaneHookRow{
			{Token: hookstest.SubjectSeedA, Location: "a|b:0.0"},
		}}})

		got := runHookList(t)
		want := hookstest.SubjectSeedA + "\ton-resume\tnpm start\ta|b:0.0\n"
		if got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
	})

	t.Run("it takes exactly one enumeration read for many entries", func(t *testing.T) {
		hooksFileInTempDir(t, map[string]map[string]string{
			hookstest.SubjectSeedA: {"on-resume": "one"},
			hookstest.SubjectSeedB: {"on-resume": "two"},
			hookstest.SubjectSeedC: {"on-resume": "three"},
			hookstest.SubjectSeedD: {"on-resume": "four"},
		})

		lister := &recordingPaneHookLister{rows: []tmux.PaneHookRow{
			{Token: hookstest.SubjectSeedA, Location: "my-project:0.0"},
		}}
		withHooksDeps(t, HooksDeps{PaneLister: lister})

		runHookList(t)

		if lister.calls != 1 {
			t.Errorf("enumeration reads = %d, want 1", lister.calls)
		}
	})
}

// loudPaneHookLister fails the test the moment it is read from: nothing to
// resolve must mean no tmux read at all.
type loudPaneHookLister struct{ t *testing.T }

func (l *loudPaneHookLister) ListAllPaneHookKeys() ([]tmux.PaneHookRow, error) {
	l.t.Helper()
	l.t.Error("enumeration read taken with no entries to resolve")
	return nil, nil
}

func TestHooksSetCommand(t *testing.T) {
	t.Run("sets hook for current pane", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, nil)
		t.Setenv("TMUX_PANE", "%3")

		resolver := &mockKeyResolver{key: hookstest.SubjectSeedA}
		withHooksDeps(t, HooksDeps{KeyResolver: resolver})

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "set", "--on-resume", "claude --resume abc123"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data := readHooksJSON(t, hooksFile)
		if data[hookstest.SubjectSeedA]["on-resume"] != "claude --resume abc123" {
			t.Errorf("hook command = %q, want %q", data[hookstest.SubjectSeedA]["on-resume"], "claude --resume abc123")
		}
	})

	t.Run("reads pane ID from TMUX_PANE environment variable", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, nil)
		t.Setenv("TMUX_PANE", "%99")

		resolver := &mockKeyResolver{key: hookstest.SubjectSeedB}
		withHooksDeps(t, HooksDeps{KeyResolver: resolver})

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "set", "--on-resume", "some-cmd"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data := readHooksJSON(t, hooksFile)
		if _, ok := data[hookstest.SubjectSeedB]; !ok {
			t.Error("expected a hook entry for the resolved hook key, not found")
		}
		if _, ok := data["%99"]; ok {
			t.Error("raw pane ID %99 should not be used as key")
		}
	})

	t.Run("returns error when TMUX_PANE is not set", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, nil)
		t.Setenv("TMUX_PANE", "")

		resolver := &mockKeyResolver{key: hookstest.SubjectSeedA}
		withHooksDeps(t, HooksDeps{KeyResolver: resolver})

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetErr(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "set", "--on-resume", "some-cmd"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "must be run from inside a tmux pane") {
			t.Errorf("error = %q, want it to contain %q", err.Error(), "must be run from inside a tmux pane")
		}

		if _, statErr := os.Stat(hooksFile); statErr == nil {
			t.Error("hooks file should not have been created")
		}
	})

	t.Run("returns error when on-resume flag is not provided", func(t *testing.T) {
		hooksFileInTempDir(t, nil)
		t.Setenv("TMUX_PANE", "%3")

		resolver := &mockKeyResolver{key: hookstest.SubjectSeedA}
		withHooksDeps(t, HooksDeps{KeyResolver: resolver})

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetErr(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "set"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected error for missing --on-resume flag, got nil")
		}
		if !strings.Contains(err.Error(), "on-resume") {
			t.Errorf("error = %q, want it to mention %q", err.Error(), "on-resume")
		}
	})

	t.Run("overwrites existing hook for same pane idempotently", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, nil)
		t.Setenv("TMUX_PANE", "%3")

		resolver := &mockKeyResolver{key: hookstest.SubjectSeedA}
		withHooksDeps(t, HooksDeps{KeyResolver: resolver})

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "set", "--on-resume", "old-cmd"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("first set: unexpected error: %v", err)
		}

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "set", "--on-resume", "new-cmd"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("second set: unexpected error: %v", err)
		}

		data := readHooksJSON(t, hooksFile)
		if data[hookstest.SubjectSeedA]["on-resume"] != "new-cmd" {
			t.Errorf("hook command = %q, want %q", data[hookstest.SubjectSeedA]["on-resume"], "new-cmd")
		}
	})

	t.Run("writes correct JSON structure to hooks file", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, nil)
		t.Setenv("TMUX_PANE", "%3")

		resolver := &mockKeyResolver{key: hookstest.SubjectSeedA}
		withHooksDeps(t, HooksDeps{KeyResolver: resolver})

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "set", "--on-resume", "claude --resume abc123"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data := readHooksJSON(t, hooksFile)

		if len(data) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(data))
		}

		events, ok := data[hookstest.SubjectSeedA]
		if !ok {
			t.Fatalf("expected an entry for hook key %s", hookstest.SubjectSeedA)
		}
		if len(events) != 1 {
			t.Fatalf("expected 1 event for %s, got %d", hookstest.SubjectSeedA, len(events))
		}
		if events["on-resume"] != "claude --resume abc123" {
			t.Errorf("on-resume = %q, want %q", events["on-resume"], "claude --resume abc123")
		}
	})

	t.Run("it aborts hook set when the hook-key read fails", func(t *testing.T) {
		const stderr = "no such pane: %999"

		_, hooksFile := hooksFileInTempDir(t, nil)
		t.Setenv("TMUX_PANE", "%999")

		resolver := &mockKeyResolver{err: &tmux.CommandError{Stderr: stderr}}
		withHooksDeps(t, HooksDeps{KeyResolver: resolver, PaneStamper: &recordingPaneStamper{}})

		_, err := runHookSet(t, "some-cmd")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if err.Error() != stderr {
			t.Errorf("error = %q, want tmux's own words %q unaltered", err.Error(), stderr)
		}
		if _, ok := errors.AsType[*tmux.CommandError](err); !ok {
			t.Errorf("errors.AsType[*tmux.CommandError](err) = false; want a recoverable *tmux.CommandError; err = %v", err)
		}

		if _, statErr := os.Stat(hooksFile); statErr == nil {
			t.Error("hooks file should not have been created")
		}
	})

	t.Run("it stores the hook under the resolved hook key", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, nil)
		t.Setenv("TMUX_PANE", "%3")

		resolver := &mockKeyResolver{key: hookstest.SubjectSeedA}
		withHooksDeps(t, HooksDeps{KeyResolver: resolver})

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "set", "--on-resume", "claude --resume abc123"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data := readHooksJSON(t, hooksFile)
		if data[hookstest.SubjectSeedA]["on-resume"] != "claude --resume abc123" {
			t.Errorf("hook command = %q, want %q", data[hookstest.SubjectSeedA]["on-resume"], "claude --resume abc123")
		}
	})
}

func TestHooksRmCommand(t *testing.T) {
	t.Run("returns error when TMUX_PANE is not set", func(t *testing.T) {
		hooksFileInTempDir(t, nil)
		t.Setenv("TMUX_PANE", "")

		resolver := &mockKeyResolver{key: hookstest.SubjectSeedA}
		withHooksDeps(t, HooksDeps{KeyResolver: resolver})

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetErr(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "rm", "--on-resume"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "must be run from inside a tmux pane") {
			t.Errorf("error = %q, want it to contain %q", err.Error(), "must be run from inside a tmux pane")
		}
	})

	t.Run("returns error when on-resume flag is not provided", func(t *testing.T) {
		hooksFileInTempDir(t, nil)
		t.Setenv("TMUX_PANE", "%3")

		resolver := &mockKeyResolver{key: hookstest.SubjectSeedA}
		withHooksDeps(t, HooksDeps{KeyResolver: resolver})

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetErr(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "rm"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected error for missing --on-resume flag, got nil")
		}
		if !strings.Contains(err.Error(), "on-resume") {
			t.Errorf("error = %q, want it to mention %q", err.Error(), "on-resume")
		}
	})

	t.Run("cleans up pane key when last event removed", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, map[string]map[string]string{
			hookstest.SubjectSeedA: {"on-resume": "some-cmd"},
		})
		t.Setenv("TMUX_PANE", "%5")

		resolver := &mockKeyResolver{key: hookstest.SubjectSeedA}
		withHooksDeps(t, HooksDeps{KeyResolver: resolver})

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "rm", "--on-resume"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data := readHooksJSON(t, hooksFile)
		if _, ok := data[hookstest.SubjectSeedA]; ok {
			t.Error("expected the hook key to be removed when the last event was deleted")
		}
		if len(data) != 0 {
			t.Errorf("expected empty hooks file, got %d entries", len(data))
		}
	})

	t.Run("it aborts hooks rm when the hook-key read fails and leaves the entry intact", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, map[string]map[string]string{
			hookstest.SubjectSeedA: {"on-resume": "some-cmd"},
		})
		t.Setenv("TMUX_PANE", "%3")

		resolver := &mockKeyResolver{err: fmt.Errorf("tmux not responding")}
		withHooksDeps(t, HooksDeps{KeyResolver: resolver})

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetErr(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "rm", "--on-resume"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if err.Error() != "tmux not responding" {
			t.Errorf("error = %q, want the read's own words %q unaltered", err.Error(), "tmux not responding")
		}

		data := readHooksJSON(t, hooksFile)
		if _, ok := data[hookstest.SubjectSeedA]; !ok {
			t.Error("hook should not have been removed on resolver failure")
		}
	})

	t.Run("--pane-key flag removes specified key without requiring TMUX_PANE", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, map[string]map[string]string{
			hookstest.SubjectSeedA:     {"on-resume": "claude --resume xyz"},
			hookstest.UnjudgeableSeedA: {"on-resume": "npm start"},
		})
		t.Setenv("TMUX_PANE", "")

		resolver, stamper := paneKeyPathSeams()
		withHooksDeps(t, HooksDeps{KeyResolver: resolver, PaneStamper: stamper})

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetErr(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "rm", "--pane-key", hookstest.SubjectSeedA, "--on-resume"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data := readHooksJSON(t, hooksFile)
		if _, ok := data[hookstest.SubjectSeedA]; ok {
			t.Error("expected the named entry to be removed via --pane-key")
		}
		if data[hookstest.UnjudgeableSeedA]["on-resume"] != "npm start" {
			t.Errorf("the untouched entry's on-resume = %q, want %q", data[hookstest.UnjudgeableSeedA]["on-resume"], "npm start")
		}
		assertNoPaneTmuxCalls(t, resolver, stamper)
	})

	t.Run("--pane-key unset falls back to resolveCurrentPaneKey", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, map[string]map[string]string{
			hookstest.SubjectSeedD: {"on-resume": "some-cmd"},
		})
		t.Setenv("TMUX_PANE", "%7")

		resolver := &mockKeyResolver{key: hookstest.SubjectSeedD}
		withHooksDeps(t, HooksDeps{KeyResolver: resolver})

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "rm", "--on-resume"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data := readHooksJSON(t, hooksFile)
		if _, ok := data[hookstest.SubjectSeedD]; ok {
			t.Error("expected the resolved entry to be removed via fallback")
		}
	})

	t.Run("--pane-key help describes a verbatim hook key", func(t *testing.T) {
		flag := hooksRmCmd.Flags().Lookup("pane-key")
		if flag == nil {
			t.Fatal("hook rm has no --pane-key flag")
		}

		want := "Hook key of the entry to remove, taken verbatim — any key, including an old-format one (defaults to the current pane's token)"
		if flag.Usage != want {
			t.Errorf("--pane-key usage = %q, want %q", flag.Usage, want)
		}
	})
}

func TestHookCommandRename(t *testing.T) {
	t.Run("exposes hook as the canonical command name", func(t *testing.T) {
		if hookCmd.Name() != "hook" {
			t.Errorf("hookCmd.Name() = %q, want %q", hookCmd.Name(), "hook")
		}
	})

	t.Run("declares hooks as an alias of hook", func(t *testing.T) {
		found := false
		for _, a := range hookCmd.Aliases {
			if a == "hooks" {
				found = true
			}
		}
		if !found {
			t.Errorf("hookCmd.Aliases = %v, want to contain %q", hookCmd.Aliases, "hooks")
		}
	})

	t.Run("resolves the hooks alias to the hook subcommands", func(t *testing.T) {
		for _, sub := range []string{"list", "set", "rm"} {
			c, _, err := rootCmd.Find([]string{"hooks", sub})
			if err != nil {
				t.Fatalf("Find([hooks %s]) error: %v", sub, err)
			}
			if c.Name() != sub {
				t.Errorf("Find([hooks %s]) resolved to %q, want %q", sub, c.Name(), sub)
			}
			if c.Parent() == nil || c.Parent().Name() != "hook" {
				t.Errorf("Find([hooks %s]) parent = %v, want parent named hook", sub, c.Parent())
			}
		}
	})

	t.Run("keeps hooks working as a silent cobra alias", func(t *testing.T) {
		hooksFileInTempDir(t, map[string]map[string]string{})

		withHooksDeps(t, HooksDeps{PaneLister: &loudPaneHookLister{t: t}})

		outBuf := new(bytes.Buffer)
		errBuf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(outBuf)
		rootCmd.SetErr(errBuf)
		rootCmd.SetArgs([]string{"hooks", "list"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		combined := strings.ToLower(outBuf.String() + errBuf.String())
		if strings.Contains(combined, "deprecat") {
			t.Errorf("hooks alias produced deprecation text: out=%q err=%q", outBuf.String(), errBuf.String())
		}
		if strings.Contains(combined, "warning") {
			t.Errorf("hooks alias produced warning text: out=%q err=%q", outBuf.String(), errBuf.String())
		}
	})

	t.Run("machine-generated hooks set still persists via the alias", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, nil)
		t.Setenv("TMUX_PANE", "%3")

		resolver := &mockKeyResolver{key: hookstest.SubjectSeedA}
		withHooksDeps(t, HooksDeps{KeyResolver: resolver})

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "set", "--on-resume", "claude --resume abc123"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data := readHooksJSON(t, hooksFile)
		if data[hookstest.SubjectSeedA]["on-resume"] != "claude --resume abc123" {
			t.Errorf("hook command = %q, want %q", data[hookstest.SubjectSeedA]["on-resume"], "claude --resume abc123")
		}
	})
}

// runHookSetForKey drives `hook set` with the pane resolver stubbed to answer key,
// so the command writes under it without a stamp.
func runHookSetForKey(t *testing.T, key, command string) error {
	t.Helper()

	t.Setenv("TMUX_PANE", "%3")
	withHooksDeps(t, HooksDeps{KeyResolver: &mockKeyResolver{key: key}})

	_, err := runHookSet(t, command)
	return err
}

func saveRequestedExists(t *testing.T, stateDir string) bool {
	t.Helper()
	_, err := os.Stat(state.SaveRequested(stateDir))
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("stat save.requested: %v", err)
	}
	return err == nil
}

// assertTouchWarn asserts the sink holds exactly one record at WARN or above and
// that it is the dirty-flag touch failure for wantKey.
func assertTouchWarn(t *testing.T, sink *logtest.Sink, wantKey string) {
	t.Helper()

	rec := sink.Records().AtOrAboveLevel(slog.LevelWarn).Only(t, "record at or above WARN")
	assertHooksRecord(t, rec, hooksRecordWant{
		level: slog.LevelWarn,
		msg:   "touch-save-requested",
		op:    "touch-save-requested",
		via:   "cli",
	})
	if got := rec.AttrString(t, "hook_key"); got != wantKey {
		t.Errorf("hook_key = %q, want %q", got, wantKey)
	}
	if got := rec.AttrString(t, "error"); got == "" {
		t.Error("error attr is empty, want the failure it carries")
	}
}

func TestHooksSetTouchesSaveRequested(t *testing.T) {
	t.Run("it touches save.requested after a successful registration", func(t *testing.T) {
		hooksFileInTempDir(t, nil)
		stateDir := t.TempDir()
		t.Setenv("PORTAL_STATE_DIR", stateDir)

		if err := runHookSetForKey(t, hookstest.SubjectSeedA, "claude --resume abc"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !saveRequestedExists(t, stateDir) {
			t.Error("save.requested was not created in the state directory")
		}
	})

	t.Run("it exits 0 when the state directory cannot be resolved", func(t *testing.T) {
		dir, hooksFile := hooksFileInTempDir(t, nil)

		blocker := filepath.Join(dir, "not-a-dir")
		if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
			t.Fatalf("write blocker file: %v", err)
		}
		t.Setenv("PORTAL_STATE_DIR", filepath.Join(blocker, "state"))

		sink := logtest.Install(t)

		if err := runHookSetForKey(t, hookstest.SubjectSeedA, "claude --resume abc"); err != nil {
			t.Fatalf("hook set failed on an unresolvable state directory: %v", err)
		}

		data := readHooksJSON(t, hooksFile)
		if data[hookstest.SubjectSeedA]["on-resume"] != "claude --resume abc" {
			t.Errorf("hook command = %q, want %q", data[hookstest.SubjectSeedA]["on-resume"], "claude --resume abc")
		}
		assertTouchWarn(t, sink, hookstest.SubjectSeedA)
	})

	t.Run("it exits 0 when the touch itself fails", func(t *testing.T) {
		dir, hooksFile := hooksFileInTempDir(t, nil)

		stateDir := filepath.Join(dir, "state")
		if err := os.MkdirAll(filepath.Join(stateDir, "scrollback"), 0o700); err != nil {
			t.Fatalf("mkdir state dir: %v", err)
		}
		if err := os.Chmod(stateDir, 0o500); err != nil {
			t.Fatalf("chmod state dir: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(stateDir, 0o700) })
		t.Setenv("PORTAL_STATE_DIR", stateDir)

		sink := logtest.Install(t)

		if err := runHookSetForKey(t, hookstest.SubjectSeedA, "claude --resume abc"); err != nil {
			t.Fatalf("hook set failed on a failing touch: %v", err)
		}

		data := readHooksJSON(t, hooksFile)
		if data[hookstest.SubjectSeedA]["on-resume"] != "claude --resume abc" {
			t.Errorf("hook command = %q, want %q", data[hookstest.SubjectSeedA]["on-resume"], "claude --resume abc")
		}
		assertTouchWarn(t, sink, hookstest.SubjectSeedA)
	})

	t.Run("it emits no set WARN when only the dirty-flag touch fails", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, nil)
		t.Setenv("PORTAL_STATE_DIR", filepath.Join(hooksFile, "state"))

		sink := logtest.Install(t)

		if err := runHookSetForKey(t, hookstest.SubjectSeedA, "claude --resume abc"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for _, r := range sink.Records().AtOrAboveLevel(slog.LevelWarn) {
			if r.HasAttr("op") && r.AttrString(t, "op") == "set" {
				t.Errorf("a set WARN was emitted for a registration that succeeded: %+v", r)
			}
		}
	})

	t.Run("it emits exactly one warn per failing hook set", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, nil)
		t.Setenv("PORTAL_STATE_DIR", filepath.Join(hooksFile, "state"))

		sink := logtest.Install(t)

		if err := runHookSetForKey(t, hookstest.SubjectSeedA, "claude --resume abc"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if warns := sink.Records().AtOrAboveLevel(slog.LevelWarn); len(warns) != 1 {
			t.Errorf("WARN record count = %d, want 1: %+v", len(warns), warns)
		}
	})

	t.Run("it does not touch when the write fails", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, nil)
		// A directory at the hooks.json path makes the store's own read fail, so
		// the command aborts before it could touch anything.
		if err := os.MkdirAll(hooksFile, 0o700); err != nil {
			t.Fatalf("mkdir bogus hooks path: %v", err)
		}
		stateDir := t.TempDir()
		t.Setenv("PORTAL_STATE_DIR", stateDir)

		sink := logtest.Install(t)

		if err := runHookSetForKey(t, hookstest.SubjectSeedA, "claude --resume abc"); err == nil {
			t.Fatal("expected an error from a failed write, got nil")
		}

		if saveRequestedExists(t, stateDir) {
			t.Error("save.requested was touched despite the write failing")
		}
		for _, r := range sink.Records() {
			if r.Msg == "touch-save-requested" {
				t.Errorf("touch emission on a failed write: %+v", r)
			}
		}
	})

	t.Run("it touches on a no-op re-registration", func(t *testing.T) {
		hooksFileInTempDir(t, nil)
		stateDir := t.TempDir()
		t.Setenv("PORTAL_STATE_DIR", stateDir)

		if err := runHookSetForKey(t, hookstest.SubjectSeedA, "same-cmd"); err != nil {
			t.Fatalf("first set: unexpected error: %v", err)
		}
		if err := os.Remove(state.SaveRequested(stateDir)); err != nil {
			t.Fatalf("remove save.requested: %v", err)
		}

		if err := runHookSetForKey(t, hookstest.SubjectSeedA, "same-cmd"); err != nil {
			t.Fatalf("second set: unexpected error: %v", err)
		}

		if !saveRequestedExists(t, stateDir) {
			t.Error("a no-op re-registration did not touch save.requested")
		}
	})

	t.Run("it does not touch from hook rm", func(t *testing.T) {
		hooksFileInTempDir(t, map[string]map[string]string{
			hookstest.SubjectSeedA: {"on-resume": "claude --resume abc"},
		})
		stateDir := t.TempDir()
		t.Setenv("PORTAL_STATE_DIR", stateDir)
		t.Setenv("TMUX_PANE", "%3")

		withHooksDeps(t, HooksDeps{KeyResolver: &mockKeyResolver{key: hookstest.SubjectSeedA}})

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetErr(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hook", "rm", "--on-resume"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if saveRequestedExists(t, stateDir) {
			t.Error("hook rm touched save.requested")
		}
	})
}
