package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/color"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/session"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
	"github.com/spf13/cobra"
)

type testAliasLookup struct {
	aliases map[string]string
}

func (t *testAliasLookup) Get(name string) (string, bool) {
	path, ok := t.aliases[name]
	return path, ok
}

func (t *testAliasLookup) Keys() []string {
	keys := make([]string, 0, len(t.aliases))
	for name := range t.aliases {
		keys = append(keys, name)
	}
	slices.Sort(keys)
	return keys
}

type testZoxideQuerier struct {
	result string
	err    error
}

func (t *testZoxideQuerier) Query(terms string) (string, error) {
	return t.result, t.err
}

type testDirValidator struct {
	existing map[string]bool
}

func (t *testDirValidator) Exists(path string) bool {
	return t.existing[path]
}

type testSessionLister struct {
	names []string
	err   error
}

func (t *testSessionLister) ListSessionNames() ([]string, error) {
	return t.names, t.err
}

func TestOpenCommand_PathArgument_NonExistentPath(t *testing.T) {
	// The session-domain pre-check reads the tmux client from context before
	// path resolution runs.
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}, Client: tmux.NewClient(stubTmuxCommander())})

	resetRootCmd()
	buf := new(bytes.Buffer)
	rootCmd.SetErr(buf)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"open", "/nonexistent/path/that/does/not/exist"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for non-existent path, got nil")
	}

	want := "Directory not found: /nonexistent/path/that/does/not/exist"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestOpenCommand_PathArgument_FileNotDirectory(t *testing.T) {
	// The session-domain pre-check reads the tmux client from context before
	// path resolution runs.
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}, Client: tmux.NewClient(stubTmuxCommander())})

	dir := t.TempDir()
	filePath := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(filePath, []byte("content"), 0o644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	resetRootCmd()
	buf := new(bytes.Buffer)
	rootCmd.SetErr(buf)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"open", filePath})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for file path, got nil")
	}

	want := "not a directory: " + filePath
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestOpenCommand_PathArgument_SkipsTUI(t *testing.T) {
	if !resolver.IsPathArgument(".") {
		t.Error("expected IsPathArgument(\".\") to return true")
	}
	if !resolver.IsPathArgument("./subdir") {
		t.Error("expected IsPathArgument(\"./subdir\") to return true")
	}
	if !resolver.IsPathArgument("~/Code") {
		t.Error("expected IsPathArgument(\"~/Code\") to return true")
	}
	if resolver.IsPathArgument("myproject") {
		t.Error("expected IsPathArgument(\"myproject\") to return false")
	}
}

func TestOpenCommand_QueryResolution_AliasNotFound(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{"myapp": "/nonexistent/alias/path"}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	})

	resetRootCmd()
	buf := new(bytes.Buffer)
	rootCmd.SetErr(buf)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"open", "myapp"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for non-existent alias path, got nil")
	}

	want := "Directory not found: /nonexistent/alias/path"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestOpenCommand_QueryResolution_ZoxideNotFound(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{result: "/gone/zoxide/dir"},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	})

	resetRootCmd()
	buf := new(bytes.Buffer)
	rootCmd.SetErr(buf)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"open", "myquery"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for non-existent zoxide path, got nil")
	}

	want := "Directory not found: /gone/zoxide/dir"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestOpenCommand_SessionNameHit_RoutesToSessionConnector(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{names: []string{"api-x7Kd9a"}},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	})

	var connectedTo string
	withFuncSeam(t, &openSessionFunc, func(_ *cobra.Command, name string) error {
		connectedTo = name
		return nil
	})

	pathCalled := false
	withFuncSeam(t, &openPathFunc, func(_ *cobra.Command, _ string, _ []string) error {
		pathCalled = true
		return nil
	})

	tuiCalled := false
	withFuncSeam(t, &openTUIFunc, func(_ *cobra.Command, _ pickerLanding, _ []string, _ bool) error {
		tuiCalled = true
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "api-x7Kd9a"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if connectedTo != "api-x7Kd9a" {
		t.Errorf("openSessionFunc called with %q, want %q", connectedTo, "api-x7Kd9a")
	}
	if pathCalled {
		t.Error("openPathFunc must not be called for a session-name hit")
	}
	if tuiCalled {
		t.Error("openTUIFunc must not be called for a session-name hit")
	}
}

func TestOpenCommand_SessionPin_ExactHit_RoutesToConnector(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{names: []string{"api-x7Kd9a"}},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	})

	var connectedTo string
	withFuncSeam(t, &openSessionFunc, func(_ *cobra.Command, name string) error {
		connectedTo = name
		return nil
	})

	pathCalled := false
	withFuncSeam(t, &openPathFunc, func(_ *cobra.Command, _ string, _ []string) error {
		pathCalled = true
		return nil
	})

	tuiCalled := false
	withFuncSeam(t, &openTUIFunc, func(_ *cobra.Command, _ pickerLanding, _ []string, _ bool) error {
		tuiCalled = true
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-s", "api-x7Kd9a"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if connectedTo != "api-x7Kd9a" {
		t.Errorf("openSessionFunc called with %q, want %q", connectedTo, "api-x7Kd9a")
	}
	if pathCalled {
		t.Error("openPathFunc must not be called for a -s pin (never mints)")
	}
	if tuiCalled {
		t.Error("openTUIFunc must not be called for a -s pin (never opens the picker)")
	}
}

func TestOpenCommand_BareSessionAttach_WithCommand_UsageError(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{names: []string{"dev"}},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	})

	sessionCalled := false
	withFuncSeam(t, &openSessionFunc, func(_ *cobra.Command, _ string) error {
		sessionCalled = true
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "dev", "-e", "claude"})
	err := rootCmd.Execute()

	if err == nil {
		t.Fatal("expected usage error, got nil")
	}
	want := "a command (-e/--) can only run in a newly-created session, not an existing one"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	if _, ok := errors.AsType[*UsageError](err); !ok {
		t.Errorf("expected *UsageError (exit 2), got %T", err)
	}
	if sessionCalled {
		t.Error("openSessionFunc must not be called: no attach may happen when a command targets an existing session")
	}
}

func TestOpenCommand_BareSessionAttach_WithDashDashCommand_UsageError(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{names: []string{"dev"}},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	})

	sessionCalled := false
	withFuncSeam(t, &openSessionFunc, func(_ *cobra.Command, _ string) error {
		sessionCalled = true
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "dev", "--", "claude"})
	err := rootCmd.Execute()

	if err == nil {
		t.Fatal("expected usage error, got nil")
	}
	want := "a command (-e/--) can only run in a newly-created session, not an existing one"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	if _, ok := errors.AsType[*UsageError](err); !ok {
		t.Errorf("expected *UsageError (exit 2), got %T", err)
	}
	if sessionCalled {
		t.Error("openSessionFunc must not be called: no attach may happen when a command targets an existing session")
	}
}

func TestOpenCommand_SessionPin_WithCommand_UsageError(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{names: []string{"dev"}},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	})

	sessionCalled := false
	withFuncSeam(t, &openSessionFunc, func(_ *cobra.Command, _ string) error {
		sessionCalled = true
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-s", "dev", "-e", "claude"})
	err := rootCmd.Execute()

	if err == nil {
		t.Fatal("expected usage error, got nil")
	}
	want := "a command (-e/--) can only run in a newly-created session, not an existing one"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	if _, ok := errors.AsType[*UsageError](err); !ok {
		t.Errorf("expected *UsageError (exit 2), got %T", err)
	}
	if sessionCalled {
		t.Error("openSessionFunc must not be called: no attach may happen when a command targets a -s pinned session")
	}
}

func TestOpenCommand_SessionPin_Glob_HardFailsNoFirstMatch(t *testing.T) {
	// Under `go test` the multi-target gate is inert (the test binary's argv
	// holds no "open" token), so a glob-bearing -s reaches ResolveSessionPin
	// directly — where it must hard-fail loudly rather than attach the first
	// match. Glob fan-out is the burst's job alone.
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{names: []string{"api-1", "api-2"}},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	})

	attached := false
	withFuncSeam(t, &openSessionFunc, func(_ *cobra.Command, _ string) error {
		attached = true
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-s", "api-*"})
	err := rootCmd.Execute()

	if err == nil {
		t.Fatal("expected hard-fail error for a glob-bearing -s value at the single-pin, got nil")
	}
	if want := "No session found: api-*"; err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	if attached {
		t.Error("openSessionFunc must not be called — a multi-match glob must not collapse to the first match")
	}
}

func TestOpenCommand_SessionPin_Miss_HardFailsNoPicker(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{names: []string{"web-abc"}},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	})

	tuiCalled := false
	withFuncSeam(t, &openTUIFunc, func(_ *cobra.Command, _ pickerLanding, _ []string, _ bool) error {
		tuiCalled = true
		return nil
	})

	pathCalled := false
	withFuncSeam(t, &openPathFunc, func(_ *cobra.Command, _ string, _ []string) error {
		pathCalled = true
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-s", "api"})
	err := rootCmd.Execute()

	if err == nil {
		t.Fatal("expected hard-fail error for a -s miss, got nil")
	}
	want := "No session found: api"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	if tuiCalled {
		t.Error("openTUIFunc must not be called on a -s miss")
	}
	if pathCalled {
		t.Error("openPathFunc must not be called on a -s miss")
	}
	if _, ok := errors.AsType[*UsageError](err); ok {
		t.Error("-s miss error must be a plain error, not a *UsageError")
	}
}

func TestOpenCommand_SessionPin_EmitsNoResolveLine(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{names: []string{"dev"}},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	})

	withFuncSeam(t, &openSessionFunc, func(_ *cobra.Command, _ string) error { return nil })

	sink := logtest.Install(t)

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-s", "dev"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if recs := sink.Records().Matching("resolve", "resolved"); len(recs) != 0 {
		t.Fatalf("expected no resolve records for a -s pin, got %d", len(recs))
	}
}

func TestOpenCommand_PathPin_Mints_NoPicker(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	})

	dir := t.TempDir()

	var mintedPath string
	withFuncSeam(t, &openPathFunc, func(_ *cobra.Command, path string, _ []string) error {
		mintedPath = path
		return nil
	})

	sessionCalled := false
	withFuncSeam(t, &openSessionFunc, func(_ *cobra.Command, _ string) error {
		sessionCalled = true
		return nil
	})

	tuiCalled := false
	withFuncSeam(t, &openTUIFunc, func(_ *cobra.Command, _ pickerLanding, _ []string, _ bool) error {
		tuiCalled = true
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-p", dir})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mintedPath != dir {
		t.Errorf("openPathFunc minted %q, want %q", mintedPath, dir)
	}
	if sessionCalled {
		t.Error("openSessionFunc must not be called for a -p pin (never attaches)")
	}
	if tuiCalled {
		t.Error("openTUIFunc must not be called for a -p pin (never opens the picker)")
	}
}

func TestOpenCommand_PathPin_GlobNamedDir_Mints(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	})

	tmp := t.TempDir()
	globDir := filepath.Join(tmp, "foo[1]")
	if err := os.Mkdir(globDir, 0o755); err != nil {
		t.Fatalf("failed to create glob-named dir: %v", err)
	}

	var mintedPath string
	withFuncSeam(t, &openPathFunc, func(_ *cobra.Command, path string, _ []string) error {
		mintedPath = path
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-p", globDir})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mintedPath != globDir {
		t.Errorf("openPathFunc minted %q, want %q", mintedPath, globDir)
	}
}

func TestOpenCommand_PathPin_Miss_HardFailsNoPicker(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	})

	missDir := filepath.Join(t.TempDir(), "does-not-exist")

	tuiCalled := false
	withFuncSeam(t, &openTUIFunc, func(_ *cobra.Command, _ pickerLanding, _ []string, _ bool) error {
		tuiCalled = true
		return nil
	})

	pathCalled := false
	withFuncSeam(t, &openPathFunc, func(_ *cobra.Command, _ string, _ []string) error {
		pathCalled = true
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-p", missDir})
	err := rootCmd.Execute()

	if err == nil {
		t.Fatal("expected hard-fail error for a -p miss, got nil")
	}
	want := "Directory not found: " + missDir
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	if tuiCalled {
		t.Error("openTUIFunc must not be called on a -p miss")
	}
	if pathCalled {
		t.Error("openPathFunc must not be called on a -p miss (never mints)")
	}
	if _, ok := errors.AsType[*UsageError](err); ok {
		t.Error("-p miss error must be a plain error, not a *UsageError")
	}
}

func TestOpenCommand_PathPin_ThreadsCommandIntoMint(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	})

	dir := t.TempDir()

	var gotPath string
	var gotCommand []string
	withFuncSeam(t, &openPathFunc, func(_ *cobra.Command, path string, command []string) error {
		gotPath = path
		gotCommand = command
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-p", dir, "-e", "claude"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPath != dir {
		t.Errorf("minted path = %q, want %q", gotPath, dir)
	}
	wantCmd := []string{"claude"}
	if !slices.Equal(gotCommand, wantCmd) {
		t.Errorf("threaded command = %v, want %v", gotCommand, wantCmd)
	}
}

func TestOpenCommand_PathPin_EmitsNoResolveLine(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	})

	dir := t.TempDir()

	withFuncSeam(t, &openPathFunc, func(_ *cobra.Command, _ string, _ []string) error { return nil })

	sink := logtest.Install(t)

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-p", dir})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if recs := sink.Records().Matching("resolve", "resolved"); len(recs) != 0 {
		t.Fatalf("expected no resolve records for a -p pin, got %d", len(recs))
	}
}

func TestOpenCommand_AliasPin_Mints_NoPicker(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	dir := t.TempDir()

	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{names: []string{"myapp"}},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{"myapp": dir}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{dir: true}},
	})

	var mintedPath string
	withFuncSeam(t, &openPathFunc, func(_ *cobra.Command, path string, _ []string) error {
		mintedPath = path
		return nil
	})

	sessionCalled := false
	withFuncSeam(t, &openSessionFunc, func(_ *cobra.Command, _ string) error {
		sessionCalled = true
		return nil
	})

	tuiCalled := false
	withFuncSeam(t, &openTUIFunc, func(_ *cobra.Command, _ pickerLanding, _ []string, _ bool) error {
		tuiCalled = true
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-a", "myapp"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mintedPath != dir {
		t.Errorf("openPathFunc minted %q, want %q", mintedPath, dir)
	}
	if sessionCalled {
		t.Error("openSessionFunc must not be called for a -a pin (bypasses the shadowing session, never attaches)")
	}
	if tuiCalled {
		t.Error("openTUIFunc must not be called for a -a pin (never opens the picker)")
	}
}

func TestOpenCommand_AliasPin_UnknownKey_HardFailsNoPicker(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{"known": "/code/known"}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	})

	tuiCalled := false
	withFuncSeam(t, &openTUIFunc, func(_ *cobra.Command, _ pickerLanding, _ []string, _ bool) error {
		tuiCalled = true
		return nil
	})

	pathCalled := false
	withFuncSeam(t, &openPathFunc, func(_ *cobra.Command, _ string, _ []string) error {
		pathCalled = true
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-a", "nope"})
	err := rootCmd.Execute()

	if err == nil {
		t.Fatal("expected hard-fail error for a -a unknown key, got nil")
	}
	want := "No alias found: nope"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	if tuiCalled {
		t.Error("openTUIFunc must not be called on a -a unknown-key miss")
	}
	if pathCalled {
		t.Error("openPathFunc must not be called on a -a unknown-key miss")
	}
	if _, ok := errors.AsType[*UsageError](err); ok {
		t.Error("-a unknown-key error must be a plain error, not a *UsageError")
	}
}

func TestOpenCommand_AliasPin_ThreadsCommandIntoMint(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	dir := t.TempDir()

	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{"myapp": dir}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{dir: true}},
	})

	var gotPath string
	var gotCommand []string
	withFuncSeam(t, &openPathFunc, func(_ *cobra.Command, path string, command []string) error {
		gotPath = path
		gotCommand = command
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-a", "myapp", "-e", "claude"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPath != dir {
		t.Errorf("minted path = %q, want %q", gotPath, dir)
	}
	wantCmd := []string{"claude"}
	if !slices.Equal(gotCommand, wantCmd) {
		t.Errorf("threaded command = %v, want %v", gotCommand, wantCmd)
	}
}

func TestOpenCommand_AliasPin_EmitsNoResolveLine(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	dir := t.TempDir()

	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{"myapp": dir}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{dir: true}},
	})

	withFuncSeam(t, &openPathFunc, func(_ *cobra.Command, _ string, _ []string) error { return nil })

	sink := logtest.Install(t)

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-a", "myapp"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if recs := sink.Records().Matching("resolve", "resolved"); len(recs) != 0 {
		t.Fatalf("expected no resolve records for a -a pin, got %d", len(recs))
	}
}

func TestOpenCommand_ZoxidePin_Mints_NoPicker(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	dir := t.TempDir()

	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{result: dir},
		DirValidator:  &testDirValidator{existing: map[string]bool{dir: true}},
	})

	var mintedPath string
	withFuncSeam(t, &openPathFunc, func(_ *cobra.Command, path string, _ []string) error {
		mintedPath = path
		return nil
	})

	sessionCalled := false
	withFuncSeam(t, &openSessionFunc, func(_ *cobra.Command, _ string) error {
		sessionCalled = true
		return nil
	})

	tuiCalled := false
	withFuncSeam(t, &openTUIFunc, func(_ *cobra.Command, _ pickerLanding, _ []string, _ bool) error {
		tuiCalled = true
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-z", "proj"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mintedPath != dir {
		t.Errorf("openPathFunc minted %q, want %q", mintedPath, dir)
	}
	if sessionCalled {
		t.Error("openSessionFunc must not be called for a -z pin (never attaches)")
	}
	if tuiCalled {
		t.Error("openTUIFunc must not be called for a -z pin (never opens the picker)")
	}
}

func TestOpenCommand_ZoxidePin_NotInstalled_ErrorsNoPicker(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrZoxideNotInstalled},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	})

	tuiCalled := false
	withFuncSeam(t, &openTUIFunc, func(_ *cobra.Command, _ pickerLanding, _ []string, _ bool) error {
		tuiCalled = true
		return nil
	})

	pathCalled := false
	withFuncSeam(t, &openPathFunc, func(_ *cobra.Command, _ string, _ []string) error {
		pathCalled = true
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-z", "proj"})
	err := rootCmd.Execute()

	if err == nil {
		t.Fatal("expected an error when zoxide is not installed, got nil")
	}
	if !errors.Is(err, resolver.ErrZoxideNotInstalled) {
		t.Fatalf("expected ErrZoxideNotInstalled, got %v", err)
	}
	if tuiCalled {
		t.Error("openTUIFunc must not be called when zoxide is not installed")
	}
	if pathCalled {
		t.Error("openPathFunc must not be called when zoxide is not installed")
	}
}

func TestOpenCommand_ZoxidePin_NoMatch_HardFailsNoPicker(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	})

	tuiCalled := false
	withFuncSeam(t, &openTUIFunc, func(_ *cobra.Command, _ pickerLanding, _ []string, _ bool) error {
		tuiCalled = true
		return nil
	})

	pathCalled := false
	withFuncSeam(t, &openPathFunc, func(_ *cobra.Command, _ string, _ []string) error {
		pathCalled = true
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-z", "nope"})
	err := rootCmd.Execute()

	if err == nil {
		t.Fatal("expected hard-fail error for a -z no-match, got nil")
	}
	want := "No zoxide match for: nope"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	if tuiCalled {
		t.Error("openTUIFunc must not be called on a -z no-match")
	}
	if pathCalled {
		t.Error("openPathFunc must not be called on a -z no-match")
	}
	if _, ok := errors.AsType[*UsageError](err); ok {
		t.Error("-z no-match error must be a plain error, not a *UsageError")
	}
}

func TestOpenCommand_ZoxidePin_ThreadsCommandIntoMint(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	dir := t.TempDir()

	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{result: dir},
		DirValidator:  &testDirValidator{existing: map[string]bool{dir: true}},
	})

	var gotPath string
	var gotCommand []string
	withFuncSeam(t, &openPathFunc, func(_ *cobra.Command, path string, command []string) error {
		gotPath = path
		gotCommand = command
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-z", "proj", "-e", "claude"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPath != dir {
		t.Errorf("minted path = %q, want %q", gotPath, dir)
	}
	wantCmd := []string{"claude"}
	if !slices.Equal(gotCommand, wantCmd) {
		t.Errorf("threaded command = %v, want %v", gotCommand, wantCmd)
	}
}

func TestOpenCommand_ZoxidePin_EmitsNoResolveLine(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	dir := t.TempDir()

	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{result: dir},
		DirValidator:  &testDirValidator{existing: map[string]bool{dir: true}},
	})

	withFuncSeam(t, &openPathFunc, func(_ *cobra.Command, _ string, _ []string) error { return nil })

	sink := logtest.Install(t)

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-z", "proj"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if recs := sink.Records().Matching("resolve", "resolved"); len(recs) != 0 {
		t.Fatalf("expected no resolve records for a -z pin, got %d", len(recs))
	}
}

func TestOpenSession_DelegatesToBuildSessionConnector(t *testing.T) {
	t.Setenv("TMUX", "/tmp/fake-socket,1,0")

	cmder := commandertest.Quiet()
	client := tmux.NewClient(cmder)
	cmd := cmdWithClient(client)

	if err := openSession(cmd, "api-x7Kd9a"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantCall := []string{"switch-client", "-t", "=api-x7Kd9a:"}
	found := false
	for _, c := range cmder.Calls() {
		if slices.Equal(c, wantCall) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected switch-client call %v, got calls %v", wantCall, cmder.Calls())
	}
}

type mockSwitchClient struct {
	switchedTo string
	err        error
}

func (m *mockSwitchClient) SwitchClient(name string) error {
	m.switchedTo = name
	return m.err
}

func TestSwitchConnector(t *testing.T) {
	t.Run("calls SwitchClient with session name", func(t *testing.T) {
		mock := &mockSwitchClient{}
		connector := &SwitchConnector{client: mock}

		err := connector.Connect("my-session")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mock.switchedTo != "my-session" {
			t.Errorf("SwitchClient called with %q, want %q", mock.switchedTo, "my-session")
		}
	})

	t.Run("returns error when SwitchClient fails", func(t *testing.T) {
		mock := &mockSwitchClient{err: fmt.Errorf("session not found")}
		connector := &SwitchConnector{client: mock}

		err := connector.Connect("nonexistent")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

type mockSessionCreator struct {
	createdDir     string
	createdCommand []string
	sessionName    string
	err            error
}

func (m *mockSessionCreator) CreateFromDir(dir string, command []string) (string, error) {
	m.createdDir = dir
	m.createdCommand = command
	return m.sessionName, m.err
}

type mockQuickStarter struct {
	ranPath    string
	ranCommand []string
	result     *session.QuickStartResult
	err        error
}

func (m *mockQuickStarter) Run(path string, command []string) (*session.QuickStartResult, error) {
	m.ranPath = path
	m.ranCommand = command
	return m.result, m.err
}

type mockExecer struct {
	calledPath string
	calledArgs []string
	calledEnv  []string
	err        error
}

func (m *mockExecer) Exec(argv0 string, argv []string, envv []string) error {
	m.calledPath = argv0
	m.calledArgs = argv
	m.calledEnv = envv
	return m.err
}

func TestPathOpener(t *testing.T) {
	t.Run("inside tmux creates session detached then switches", func(t *testing.T) {
		creator := &mockSessionCreator{sessionName: "myproject-abc123"}
		switcher := &mockSwitchClient{}
		qs := &mockQuickStarter{}
		execer := &mockExecer{}

		opener := &PathOpener{
			insideTmux: true,
			creator:    creator,
			switcher:   switcher,
			qs:         qs,
			execer:     execer,
		}

		err := opener.Open("/home/user/project", nil)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if creator.createdDir != "/home/user/project" {
			t.Errorf("CreateFromDir called with %q, want %q", creator.createdDir, "/home/user/project")
		}

		if switcher.switchedTo != "myproject-abc123" {
			t.Errorf("SwitchClient called with %q, want %q", switcher.switchedTo, "myproject-abc123")
		}

		if execer.calledPath != "" {
			t.Errorf("exec was called with %q, expected no exec inside tmux", execer.calledPath)
		}
	})

	t.Run("outside tmux creates session with exec handoff", func(t *testing.T) {
		creator := &mockSessionCreator{}
		switcher := &mockSwitchClient{}
		qs := &mockQuickStarter{
			result: &session.QuickStartResult{
				SessionName: "myproject-abc123",
				Dir:         "/home/user/project",
				ExecArgs:    []string{"tmux", "new-session", "-A", "-s", "myproject-abc123", "-c", "/home/user/project"},
			},
		}
		execer := &mockExecer{}

		opener := &PathOpener{
			insideTmux: false,
			creator:    creator,
			switcher:   switcher,
			qs:         qs,
			execer:     execer,
			tmuxPath:   "/usr/bin/tmux",
		}

		err := opener.Open("/home/user/project", nil)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if qs.ranPath != "/home/user/project" {
			t.Errorf("QuickStart.Run called with %q, want %q", qs.ranPath, "/home/user/project")
		}

		if execer.calledPath != "/usr/bin/tmux" {
			t.Errorf("exec path = %q, want %q", execer.calledPath, "/usr/bin/tmux")
		}
		wantArgs := []string{"tmux", "new-session", "-A", "-s", "myproject-abc123", "-c", "/home/user/project"}
		if len(execer.calledArgs) != len(wantArgs) {
			t.Fatalf("exec args = %v, want %v", execer.calledArgs, wantArgs)
		}
		for i, arg := range execer.calledArgs {
			if arg != wantArgs[i] {
				t.Errorf("exec args[%d] = %q, want %q", i, arg, wantArgs[i])
			}
		}

		if creator.createdDir != "" {
			t.Errorf("CreateFromDir should not be called outside tmux, but was called with %q", creator.createdDir)
		}
	})

	t.Run("inside tmux switch-client called with correct session name", func(t *testing.T) {
		creator := &mockSessionCreator{sessionName: "portal-z9y8x7"}
		switcher := &mockSwitchClient{}

		opener := &PathOpener{
			insideTmux: true,
			creator:    creator,
			switcher:   switcher,
			qs:         &mockQuickStarter{},
			execer:     &mockExecer{},
		}

		err := opener.Open("/some/dir", nil)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if switcher.switchedTo != "portal-z9y8x7" {
			t.Errorf("SwitchClient called with %q, want %q", switcher.switchedTo, "portal-z9y8x7")
		}
	})

	t.Run("inside tmux returns error when session creation fails", func(t *testing.T) {
		creator := &mockSessionCreator{err: fmt.Errorf("tmux error")}
		switcher := &mockSwitchClient{}

		opener := &PathOpener{
			insideTmux: true,
			creator:    creator,
			switcher:   switcher,
			qs:         &mockQuickStarter{},
			execer:     &mockExecer{},
		}

		err := opener.Open("/some/dir", nil)

		if err == nil {
			t.Fatal("expected error, got nil")
		}

		if switcher.switchedTo != "" {
			t.Errorf("SwitchClient should not be called when creation fails, but was called with %q", switcher.switchedTo)
		}
	})

	t.Run("inside tmux returns error when switch-client fails", func(t *testing.T) {
		creator := &mockSessionCreator{sessionName: "myproject-abc123"}
		switcher := &mockSwitchClient{err: fmt.Errorf("switch failed")}

		opener := &PathOpener{
			insideTmux: true,
			creator:    creator,
			switcher:   switcher,
			qs:         &mockQuickStarter{},
			execer:     &mockExecer{},
		}

		err := opener.Open("/some/dir", nil)

		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("inside tmux passes command to session creator", func(t *testing.T) {
		creator := &mockSessionCreator{sessionName: "myproject-abc123"}
		switcher := &mockSwitchClient{}

		opener := &PathOpener{
			insideTmux: true,
			creator:    creator,
			switcher:   switcher,
			qs:         &mockQuickStarter{},
			execer:     &mockExecer{},
		}

		command := []string{"claude", "--resume"}
		err := opener.Open("/home/user/project", command)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(creator.createdCommand) != len(command) {
			t.Fatalf("command = %v, want %v", creator.createdCommand, command)
		}
		for i, arg := range creator.createdCommand {
			if arg != command[i] {
				t.Errorf("command[%d] = %q, want %q", i, arg, command[i])
			}
		}
	})

	t.Run("outside tmux passes command to quickstart", func(t *testing.T) {
		qs := &mockQuickStarter{
			result: &session.QuickStartResult{
				SessionName: "myproject-abc123",
				Dir:         "/home/user/project",
				ExecArgs:    []string{"tmux", "new-session", "-A", "-s", "myproject-abc123", "-c", "/home/user/project", "/bin/zsh -ic 'claude --resume; exec /bin/zsh'"},
			},
		}
		execer := &mockExecer{}

		opener := &PathOpener{
			insideTmux: false,
			creator:    &mockSessionCreator{},
			switcher:   &mockSwitchClient{},
			qs:         qs,
			execer:     execer,
			tmuxPath:   "/usr/bin/tmux",
		}

		command := []string{"claude", "--resume"}
		err := opener.Open("/home/user/project", command)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(qs.ranCommand) != len(command) {
			t.Fatalf("command = %v, want %v", qs.ranCommand, command)
		}
		for i, arg := range qs.ranCommand {
			if arg != command[i] {
				t.Errorf("command[%d] = %q, want %q", i, arg, command[i])
			}
		}
	})

	t.Run("outside tmux returns error when quickstart fails", func(t *testing.T) {
		qs := &mockQuickStarter{err: fmt.Errorf("git error")}

		opener := &PathOpener{
			insideTmux: false,
			creator:    &mockSessionCreator{},
			switcher:   &mockSwitchClient{},
			qs:         qs,
			execer:     &mockExecer{},
			tmuxPath:   "/usr/bin/tmux",
		}

		err := opener.Open("/some/dir", nil)

		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

}

func newTestOpenCmd() (*cobra.Command, *cobra.Command) {
	child := &cobra.Command{
		Use:  "open",
		Args: cobra.ArbitraryArgs,
	}
	child.Flags().StringP("exec", "e", "", "command to execute in the new session")

	root := &cobra.Command{Use: "portal", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(child)

	return root, child
}

func TestParseCommandArgs(t *testing.T) {
	tests := []struct {
		name         string
		args         []string // args to set on the root command (e.g. ["open", "-e", "claude"])
		wantCmd      []string
		wantDest     string
		wantErr      string
		wantUsageErr bool
	}{
		{
			name:     "no flags produces nil command",
			args:     []string{"open"},
			wantCmd:  nil,
			wantDest: "",
		},
		{
			name:     "destination only produces nil command",
			args:     []string{"open", "myproject"},
			wantCmd:  nil,
			wantDest: "myproject",
		},
		{
			name:     "parses -e flag into command slice",
			args:     []string{"open", "-e", "claude"},
			wantCmd:  []string{"claude"},
			wantDest: "",
		},
		{
			name:     "parses --exec flag into command slice",
			args:     []string{"open", "--exec", "claude"},
			wantCmd:  []string{"claude"},
			wantDest: "",
		},
		{
			name:     "destination parsed correctly with -e flag",
			args:     []string{"open", "-e", "claude", "myproject"},
			wantCmd:  []string{"claude"},
			wantDest: "myproject",
		},
		{
			name:     "parses -- args into command slice",
			args:     []string{"open", "--", "claude", "--resume"},
			wantCmd:  []string{"claude", "--resume"},
			wantDest: "",
		},
		{
			name:     "destination parsed correctly with -- syntax",
			args:     []string{"open", "myproject", "--", "claude", "--resume", "--model", "opus"},
			wantCmd:  []string{"claude", "--resume", "--model", "opus"},
			wantDest: "myproject",
		},
		{
			name:         "-e with empty string produces exit code 2",
			args:         []string{"open", "-e", ""},
			wantErr:      "-e/--exec value must not be empty",
			wantUsageErr: true,
		},
		{
			name:         "-- with no arguments produces exit code 2",
			args:         []string{"open", "--"},
			wantErr:      "no command specified after --",
			wantUsageErr: true,
		},
		{
			name:         "both -e and -- produces exit code 2",
			args:         []string{"open", "-e", "vim", "--", "claude", "--resume"},
			wantErr:      "cannot use both -e/--exec and -- to specify a command",
			wantUsageErr: true,
		},
		{
			name:         "-- with destination but no command args produces exit code 2",
			args:         []string{"open", "myproject", "--"},
			wantErr:      "no command specified after --",
			wantUsageErr: true,
		},
		{
			name:         "both -e and -- with multiple targets produces exit code 2",
			args:         []string{"open", "api", "web", "-e", "vim", "--", "claude"},
			wantErr:      "cannot use both -e/--exec and -- to specify a command",
			wantUsageErr: true,
		},
		{
			name:         "-e empty with multiple targets produces exit code 2",
			args:         []string{"open", "api", "web", "-e", ""},
			wantErr:      "-e/--exec value must not be empty",
			wantUsageErr: true,
		},
		{
			name:         "-- with multiple targets but no command args produces exit code 2",
			args:         []string{"open", "api", "web", "--"},
			wantErr:      "no command specified after --",
			wantUsageErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, child := newTestOpenCmd()

			var gotCmd []string
			var gotDest string
			var gotErr error

			child.RunE = func(cmd *cobra.Command, args []string) error {
				c, d, err := parseCommandArgs(cmd, args)
				gotCmd = c
				gotDest = d
				gotErr = err
				return err
			}

			root.SetArgs(tt.args)
			err := root.Execute()

			if tt.wantErr != "" {
				if gotErr == nil && err == nil {
					t.Fatalf("expected error %q, got nil", tt.wantErr)
				}
				errMsg := ""
				if gotErr != nil {
					errMsg = gotErr.Error()
				} else {
					errMsg = err.Error()
				}
				if errMsg != tt.wantErr {
					t.Errorf("error = %q, want %q", errMsg, tt.wantErr)
				}
				if tt.wantUsageErr {
					checkErr := gotErr
					if checkErr == nil {
						checkErr = err
					}
					if _, ok := errors.AsType[*UsageError](checkErr); !ok {
						t.Errorf("expected UsageError for exit code 2, got %T", checkErr)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotErr != nil {
				t.Fatalf("unexpected parse error: %v", gotErr)
			}

			if tt.wantCmd == nil {
				if gotCmd != nil {
					t.Errorf("command = %v, want nil", gotCmd)
				}
			} else {
				if len(gotCmd) != len(tt.wantCmd) {
					t.Fatalf("command = %v, want %v", gotCmd, tt.wantCmd)
				}
				for i, arg := range gotCmd {
					if arg != tt.wantCmd[i] {
						t.Errorf("command[%d] = %q, want %q", i, arg, tt.wantCmd[i])
					}
				}
			}

			if gotDest != tt.wantDest {
				t.Errorf("destination = %q, want %q", gotDest, tt.wantDest)
			}
		})
	}
}

type stubProjectStore struct {
	projects []project.Project
}

func (s *stubProjectStore) List() ([]project.Project, error) { return s.projects, nil }
func (s *stubProjectStore) CleanStale() ([]project.Project, error) {
	return s.projects, nil
}
func (s *stubProjectStore) Remove(_, _ string) error { return nil }

type stubSessionKiller struct{}

func (s *stubSessionKiller) KillSession(_ string) error { return nil }

type stubSessionRenamer struct{}

func (s *stubSessionRenamer) RenameSession(_, _ string) error { return nil }

type stubTUISessionCreator struct{}

func (s *stubTUISessionCreator) CreateFromDir(_ string, _ []string) (string, error) {
	return "stub-session", nil
}

type stubProjectEditor struct{}

func (s *stubProjectEditor) Rename(_, _, _ string) error { return nil }

func (s *stubProjectEditor) AddTag(_, _ string) error { return nil }

func (s *stubProjectEditor) RemoveTag(_, _ string) error { return nil }

type stubAliasEditor struct {
	aliases map[string]string
}

func (s *stubAliasEditor) Load() (map[string]string, error) {
	result := make(map[string]string)
	maps.Copy(result, s.aliases)
	return result, nil
}
func (s *stubAliasEditor) SetAndSave(name, path, _ string) error {
	if s.aliases == nil {
		s.aliases = make(map[string]string)
	}
	s.aliases[name] = path
	return nil
}
func (s *stubAliasEditor) DeleteAndSave(name, _ string) (bool, error) {
	_, ok := s.aliases[name]
	if !ok {
		return false, nil
	}
	delete(s.aliases, name)
	return true, nil
}

// stubTmuxCommander answers the session enumeration with one stub row and
// stays quiet for every other tmux call.
func stubTmuxCommander() *commandertest.Scripted {
	return commandertest.Quiet(commandertest.Returns("stub|1|0|", "list-sessions"))
}

func defaultTestTUIConfig() tuiConfig {
	return tuiConfig{
		lister:         &mockSessionLister{},
		killer:         &stubSessionKiller{},
		renamer:        &stubSessionRenamer{},
		projectStore:   &stubProjectStore{},
		sessionCreator: &stubTUISessionCreator{},
		cwd:            "/home/user",
	}
}

func TestBuildTUIModel(t *testing.T) {
	t.Run("no command and no filter creates default model", func(t *testing.T) {
		cfg := defaultTestTUIConfig()

		m := buildTUIModel(cfg, pickerLanding{}, nil)

		if m.Selected() != "" {
			t.Errorf("Selected() = %q, want empty", m.Selected())
		}
		if m.InitialFilter() != "" {
			t.Errorf("InitialFilter() = %q, want empty", m.InitialFilter())
		}
		if m.CommandPending() {
			t.Error("CommandPending() = true, want false")
		}
		if m.InsideTmux() {
			t.Error("InsideTmux() = true, want false")
		}
		if m.ActivePage() != tui.PageSessions {
			t.Errorf("ActivePage() = %d, want PageSessions (0)", m.ActivePage())
		}
	})

	t.Run("command creates model in command-pending mode", func(t *testing.T) {
		cfg := defaultTestTUIConfig()

		m := buildTUIModel(cfg, pickerLanding{}, []string{"claude"})

		if !m.CommandPending() {
			t.Error("CommandPending() = false, want true")
		}
		if m.ActivePage() != tui.PageProjects {
			t.Errorf("ActivePage() = %d, want PageProjects (1)", m.ActivePage())
		}
		wantCmd := []string{"claude"}
		gotCmd := m.Command()
		if len(gotCmd) != len(wantCmd) {
			t.Fatalf("Command() = %v, want %v", gotCmd, wantCmd)
		}
		for i, arg := range gotCmd {
			if arg != wantCmd[i] {
				t.Errorf("Command()[%d] = %q, want %q", i, arg, wantCmd[i])
			}
		}
	})

	t.Run("filter creates model with initial filter", func(t *testing.T) {
		cfg := defaultTestTUIConfig()

		m := buildTUIModel(cfg, pickerLanding{filter: "myapp"}, nil)

		if m.InitialFilter() != "myapp" {
			t.Errorf("InitialFilter() = %q, want %q", m.InitialFilter(), "myapp")
		}
		if m.CommandPending() {
			t.Error("CommandPending() = true, want false")
		}
	})

	t.Run("command and filter combines both", func(t *testing.T) {
		cfg := defaultTestTUIConfig()

		m := buildTUIModel(cfg, pickerLanding{filter: "myapp"}, []string{"claude"})

		if m.InitialFilter() != "myapp" {
			t.Errorf("InitialFilter() = %q, want %q", m.InitialFilter(), "myapp")
		}
		if !m.CommandPending() {
			t.Error("CommandPending() = false, want true")
		}
		if m.ActivePage() != tui.PageProjects {
			t.Errorf("ActivePage() = %d, want PageProjects (1)", m.ActivePage())
		}
	})

	t.Run("inside tmux detection passes session name to model", func(t *testing.T) {
		cfg := defaultTestTUIConfig()
		cfg.insideTmux = true
		cfg.currentSession = "my-session"

		m := buildTUIModel(cfg, pickerLanding{}, nil)

		if !m.InsideTmux() {
			t.Error("InsideTmux() = false, want true")
		}
		if m.CurrentSession() != "my-session" {
			t.Errorf("CurrentSession() = %q, want %q", m.CurrentSession(), "my-session")
		}
		if m.SessionListTitle() != "Sessions (current: my-session)" {
			t.Errorf("SessionListTitle() = %q, want %q", m.SessionListTitle(), "Sessions (current: my-session)")
		}
	})

	t.Run("cwd wired correctly", func(t *testing.T) {
		cfg := defaultTestTUIConfig()
		cfg.cwd = "/home/user/projects"

		m := buildTUIModel(cfg, pickerLanding{}, nil)

		if m.CWD() != "/home/user/projects" {
			t.Errorf("CWD() = %q, want %q", m.CWD(), "/home/user/projects")
		}
	})

	t.Run("project and alias editors wired enables edit modal", func(t *testing.T) {
		projects := []project.Project{
			{Path: "/code/portal", Name: "portal"},
		}
		cfg := defaultTestTUIConfig()
		cfg.projectStore = &stubProjectStore{projects: projects}
		cfg.projectEditor = &stubProjectEditor{}
		cfg.aliasEditor = &stubAliasEditor{aliases: map[string]string{}}

		m := buildTUIModel(cfg, pickerLanding{}, nil)

		var model tea.Model = m
		// Build arms the first-paint gate, so View renders a blank frame until OSC
		// 11 detection resolves. Deliver the reply as the live program does, or the
		// asserted content is never painted.
		model, _ = model.Update(tea.BackgroundColorMsg{Color: color.RGBA{R: 0x0b, G: 0x0c, B: 0x14, A: 0xff}})
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		model, _ = model.Update(tui.ProjectsLoadedMsg{Projects: projects})

		model, _ = model.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})

		view := model.View().Content
		if !strings.Contains(view, "Edit Project") {
			t.Errorf("expected edit modal to open when editors are wired, got view:\n%s", view)
		}
	})
}

func TestBuildTUIModel_ServerStarted(t *testing.T) {
	t.Run("serverStarted true starts on loading page", func(t *testing.T) {
		cfg := defaultTestTUIConfig()
		cfg.serverStarted = true

		m := buildTUIModel(cfg, pickerLanding{}, nil)

		if m.ActivePage() != tui.PageLoading {
			t.Errorf("ActivePage() = %d, want PageLoading (%d)", m.ActivePage(), tui.PageLoading)
		}
		if !m.ServerStarted() {
			t.Error("ServerStarted() = false, want true")
		}
	})

	t.Run("serverStarted false starts on sessions page", func(t *testing.T) {
		cfg := defaultTestTUIConfig()
		cfg.serverStarted = false

		m := buildTUIModel(cfg, pickerLanding{}, nil)

		if m.ActivePage() != tui.PageSessions {
			t.Errorf("ActivePage() = %d, want PageSessions (%d)", m.ActivePage(), tui.PageSessions)
		}
		if m.ServerStarted() {
			t.Error("ServerStarted() = true, want false")
		}
	})

	t.Run("default serverStarted starts on sessions page", func(t *testing.T) {
		cfg := defaultTestTUIConfig()

		m := buildTUIModel(cfg, pickerLanding{}, nil)

		if m.ActivePage() != tui.PageSessions {
			t.Errorf("ActivePage() = %d, want PageSessions (%d)", m.ActivePage(), tui.PageSessions)
		}
	})

	t.Run("serverStarted true preserves other options", func(t *testing.T) {
		cfg := defaultTestTUIConfig()
		cfg.insideTmux = true
		cfg.currentSession = "dev"
		cfg.serverStarted = true

		m := buildTUIModel(cfg, pickerLanding{}, nil)

		if !m.ServerStarted() {
			t.Error("ServerStarted() = false, want true")
		}
		if !m.InsideTmux() {
			t.Error("InsideTmux() = false, want true")
		}
		if m.CurrentSession() != "dev" {
			t.Errorf("CurrentSession() = %q, want %q", m.CurrentSession(), "dev")
		}
	})
}

func TestProcessTUIResult(t *testing.T) {
	t.Run("clean exit without selection returns nil", func(t *testing.T) {
		m := tui.New(&mockSessionLister{})
		connector := &mockSessionConnector{}

		err := processTUIResult(m, connector)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if connector.connectedTo != "" {
			t.Errorf("connector should not be called on clean exit, but was called with %q", connector.connectedTo)
		}
	})

	t.Run("selected session name forwarded to connector", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "dev", Windows: 3},
		}
		m := tui.NewModelWithSessions(sessions)
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		m = updated.(tui.Model)
		updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		m = updated.(tui.Model)

		connector := &mockSessionConnector{}

		err := processTUIResult(m, connector)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if connector.connectedTo != "dev" {
			t.Errorf("connector called with %q, want %q", connector.connectedTo, "dev")
		}
	})
}

func TestOpenCommand_TotalMiss_HardFails(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	})

	tuiCalled := false
	withFuncSeam(t, &openTUIFunc, func(_ *cobra.Command, _ pickerLanding, _ []string, _ bool) error {
		tuiCalled = true
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "blog"})
	err := rootCmd.Execute()

	if err == nil {
		t.Fatal("expected hard-fail error for total miss, got nil")
	}
	want := "nothing resolved for 'blog' — try -f blog"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	if tuiCalled {
		t.Error("openTUIFunc must not be called on a total miss")
	}
	if _, ok := errors.AsType[*UsageError](err); ok {
		t.Error("miss error must be a plain error, not a *UsageError")
	}
}

func TestOpenCommand_ResolveLog_SessionHit(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{names: []string{"dev"}},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	})

	withFuncSeam(t, &openSessionFunc, func(_ *cobra.Command, _ string) error { return nil })

	sink := logtest.Install(t)

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "dev"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r := sink.Records().Matching("resolve", "resolved").Only(t, "resolve resolved record")
	if r.Level != slog.LevelInfo {
		t.Errorf("resolve record level = %v, want INFO", r.Level)
	}
	assertResolveAttr(t, r, "target", "dev")
	assertResolveAttr(t, r, "domain", "session")
	assertResolveAttr(t, r, "resolved_path", "dev")
}

func TestOpenCommand_ResolveLog_ZoxideMint(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{result: "/Users/lee/Code/blog"},
		DirValidator:  &testDirValidator{existing: map[string]bool{"/Users/lee/Code/blog": true}},
	})

	withFuncSeam(t, &openPathFunc, func(_ *cobra.Command, _ string, _ []string) error { return nil })

	sink := logtest.Install(t)

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "blog"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r := sink.Records().Matching("resolve", "resolved").Only(t, "resolve resolved record")
	if r.Level != slog.LevelInfo {
		t.Errorf("resolve record level = %v, want INFO", r.Level)
	}
	assertResolveAttr(t, r, "target", "blog")
	assertResolveAttr(t, r, "domain", "zoxide")
	assertResolveAttr(t, r, "resolved_path", "/Users/lee/Code/blog")
}

func TestOpenCommand_ResolveLog_Miss(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	})

	sink := logtest.Install(t)

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "blog"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected hard-fail error for total miss, got nil")
	}
	want := "nothing resolved for 'blog' — try -f blog"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}

	r := sink.Records().Matching("resolve", "resolved").Only(t, "resolve resolved record")
	if r.Level != slog.LevelInfo {
		t.Errorf("resolve record level = %v, want INFO", r.Level)
	}
	assertResolveAttr(t, r, "target", "blog")
	assertResolveAttr(t, r, "domain", "miss")
	assertResolveAttr(t, r, "resolved_path", "")
}

func TestOpenCommand_ResolveLog_GlobEmitsNoLine(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{names: []string{"dev-1", "dev-2"}},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	})

	withFuncSeam(t, &runOpenBurstFunc, func(_ *cobra.Command, _ []spawn.Surface, _ []string) error { return nil })

	withFuncSeam(t, &openRawArgs, func() []string { return []string{"portal", "open", "dev*"} })

	sink := logtest.Install(t)

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "dev*"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if recs := sink.Records().Matching("resolve", "resolved"); len(recs) != 0 {
		t.Fatalf("expected no resolve records for a glob target, got %d", len(recs))
	}
}

func TestEmitResolveDecision_Helper(t *testing.T) {
	t.Run("non-glob target emits exactly one resolve line", func(t *testing.T) {
		sink := logtest.Install(t)

		emitResolveDecision("dev", &resolver.SessionResult{Name: "dev", Domain: "session"})

		rec := sink.Records().Matching("resolve", "resolved").Only(t, "resolve resolved record")
		if rec.Level != slog.LevelInfo {
			t.Errorf("resolve record level = %v, want INFO", rec.Level)
		}
		assertResolveAttr(t, rec, "target", "dev")
		assertResolveAttr(t, rec, "domain", "session")
		assertResolveAttr(t, rec, "resolved_path", "dev")
	})

	t.Run("glob target emits no line (gate lives in the helper)", func(t *testing.T) {
		sink := logtest.Install(t)

		emitResolveDecision("dev*", &resolver.SessionResult{Name: "dev-1", Domain: "glob"})

		if recs := sink.Records().Matching("resolve", "resolved"); len(recs) != 0 {
			t.Fatalf("expected no resolve records for a glob target, got %d", len(recs))
		}
	})
}

func TestLogExecHandoff_Helper(t *testing.T) {
	t.Run("strips argv[0] and joins the rest under target=tmux", func(t *testing.T) {
		sink := logtest.Install(t)

		logExecHandoff([]string{"tmux", "attach-session", "-t", "=foo:"})

		rec := sink.Records().Matching("process", "exec").Only(t, "process exec record")
		if rec.Level != slog.LevelInfo {
			t.Errorf("exec marker level = %v, want INFO", rec.Level)
		}
		if target := rec.AttrString(t, "target"); target != "tmux" {
			t.Errorf("target attr = %q, want %q", target, "tmux")
		}
		if gotArgs := rec.AttrString(t, "args"); gotArgs != "attach-session -t =foo:" {
			t.Errorf("args attr = %q, want %q", gotArgs, "attach-session -t =foo:")
		}
	})

	t.Run("defensive: empty argv does not panic and logs empty args", func(t *testing.T) {
		sink := logtest.Install(t)

		logExecHandoff(nil)

		rec := sink.Records().Matching("process", "exec").Only(t, "process exec record")
		if gotArgs := rec.AttrString(t, "args"); gotArgs != "" {
			t.Errorf("args attr = %q, want empty", gotArgs)
		}
	})
}

func TestOpenCommand_BareProjectName_MintsNeverAttaches(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{names: []string{"api-x7Kd9a"}},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{"api": "/Users/lee/Code/api"}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{"/Users/lee/Code/api": true}},
	})

	var mintedPath string
	withFuncSeam(t, &openPathFunc, func(_ *cobra.Command, path string, _ []string) error {
		mintedPath = path
		return nil
	})

	sessionCalled := false
	withFuncSeam(t, &openSessionFunc, func(_ *cobra.Command, _ string) error {
		sessionCalled = true
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "api"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mintedPath != "/Users/lee/Code/api" {
		t.Errorf("openPathFunc minted %q, want %q", mintedPath, "/Users/lee/Code/api")
	}
	if sessionCalled {
		t.Error("openSessionFunc must not be called for a bare project name (no reattach)")
	}
}

func TestOpenCommand_CommandThreadsIntoMintedTarget(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{"api": "/Users/lee/Code/api"}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{"/Users/lee/Code/api": true}},
	})

	var gotPath string
	var gotCommand []string
	withFuncSeam(t, &openPathFunc, func(_ *cobra.Command, path string, command []string) error {
		gotPath = path
		gotCommand = command
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "api", "--", "vim", "."})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPath != "/Users/lee/Code/api" {
		t.Errorf("minted path = %q, want %q", gotPath, "/Users/lee/Code/api")
	}
	wantCmd := []string{"vim", "."}
	if !slices.Equal(gotCommand, wantCmd) {
		t.Errorf("threaded command = %v, want %v", gotCommand, wantCmd)
	}
}

func TestOpenCommand_DirectTUI_PassesServerStarted(t *testing.T) {
	runner := &recordingRunner{started: true}
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner})

	var capturedServerStarted bool
	withFuncSeam(t, &openTUIFunc, func(_ *cobra.Command, landing pickerLanding, command []string, serverStarted bool) error {
		capturedServerStarted = serverStarted
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open"})
	err := rootCmd.Execute()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !capturedServerStarted {
		t.Error("direct TUI path passed serverStarted=false; expected true when server was just started")
	}
}

type recordingFilterLister struct {
	names  []string
	called bool
}

func (r *recordingFilterLister) ListSessionNames() ([]string, error) {
	r.called = true
	return r.names, nil
}

func TestOpenCommand_Filter_OpensPickerPrefilteredAndSkipsResolution(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	lister := &recordingFilterLister{}
	withOpenDeps(t, OpenDeps{
		SessionLister: lister,
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	})

	var gotLanding pickerLanding
	var gotCommand []string
	tuiCalled := false
	withFuncSeam(t, &openTUIFunc, func(_ *cobra.Command, landing pickerLanding, command []string, _ bool) error {
		tuiCalled = true
		gotLanding = landing
		gotCommand = command
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-f", "blog"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !tuiCalled {
		t.Fatal("openTUIFunc must be called for -f")
	}
	if got, want := shapeOfLanding(gotLanding), (landingShape{filter: "blog"}); got != want {
		t.Errorf("landing = %+v, want %+v", got, want)
	}
	if gotCommand != nil {
		t.Errorf("command = %v, want nil", gotCommand)
	}
	if lister.called {
		t.Error("query resolver must not be consulted for a -f invocation (resolution skipped)")
	}
}

func TestOpenCommand_Filter_WithPositionalTarget_UsageError(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	lister := &recordingFilterLister{}
	withOpenDeps(t, OpenDeps{
		SessionLister: lister,
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	})

	tuiCalled := false
	withFuncSeam(t, &openTUIFunc, func(_ *cobra.Command, _ pickerLanding, _ []string, _ bool) error {
		tuiCalled = true
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-f", "blog", "api"})
	err := rootCmd.Execute()

	if err == nil {
		t.Fatal("expected usage error, got nil")
	}
	want := "cannot use -f/--filter with a target or a domain pin (-s/-p/-z/-a)"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	if _, ok := errors.AsType[*UsageError](err); !ok {
		t.Errorf("expected *UsageError (exit 2), got %T", err)
	}
	if tuiCalled {
		t.Error("openTUIFunc must not be called when -f conflicts with a positional target")
	}
	if lister.called {
		t.Error("query resolver must not be consulted on a -f/target conflict")
	}
}

func TestOpenCommand_Filter_WithPin_UsageError(t *testing.T) {
	cases := []struct {
		name string
		flag string
		val  string
	}{
		{"session pin", "-s", "api"},
		{"path pin", "-p", "~/Code/api"},
		{"zoxide pin", "-z", "api"},
		{"alias pin", "-a", "api"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

			lister := &recordingFilterLister{}
			withOpenDeps(t, OpenDeps{
				SessionLister: lister,
				AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
				Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
				DirValidator:  &testDirValidator{existing: map[string]bool{}},
			})

			tuiCalled := false
			withFuncSeam(t, &openTUIFunc, func(_ *cobra.Command, _ pickerLanding, _ []string, _ bool) error {
				tuiCalled = true
				return nil
			})

			sessionCalled := false
			withFuncSeam(t, &openSessionFunc, func(_ *cobra.Command, _ string) error {
				sessionCalled = true
				return nil
			})

			pathCalled := false
			withFuncSeam(t, &openPathFunc, func(_ *cobra.Command, _ string, _ []string) error {
				pathCalled = true
				return nil
			})

			resetRootCmd()
			rootCmd.SetArgs([]string{"open", "-f", "blog", tc.flag, tc.val})
			err := rootCmd.Execute()

			if err == nil {
				t.Fatal("expected usage error, got nil")
			}
			want := "cannot use -f/--filter with a target or a domain pin (-s/-p/-z/-a)"
			if err.Error() != want {
				t.Errorf("error = %q, want %q", err.Error(), want)
			}
			if _, ok := errors.AsType[*UsageError](err); !ok {
				t.Errorf("expected *UsageError (exit 2), got %T", err)
			}
			if tuiCalled {
				t.Error("openTUIFunc must not be called when -f conflicts with a pin")
			}
			if sessionCalled || pathCalled {
				t.Error("no resolution outcome (attach/mint) may fire on a -f/pin conflict")
			}
			if lister.called {
				t.Error("query resolver must not be consulted on a -f/pin conflict")
			}
		})
	}
}

func TestOpenCommand_Filter_WithMultiplePins_UsageError(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	lister := &recordingFilterLister{}
	withOpenDeps(t, OpenDeps{
		SessionLister: lister,
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	})

	tuiCalled := false
	withFuncSeam(t, &openTUIFunc, func(_ *cobra.Command, _ pickerLanding, _ []string, _ bool) error {
		tuiCalled = true
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-f", "blog", "-s", "api", "-p", "~/Code/new"})
	err := rootCmd.Execute()

	if err == nil {
		t.Fatal("expected usage error, got nil")
	}
	want := "cannot use -f/--filter with a target or a domain pin (-s/-p/-z/-a)"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	if _, ok := errors.AsType[*UsageError](err); !ok {
		t.Errorf("expected *UsageError (exit 2), got %T", err)
	}
	if tuiCalled {
		t.Error("openTUIFunc must not be called when -f conflicts with pins")
	}
	if lister.called {
		t.Error("query resolver must not be consulted on a -f/pins conflict")
	}
}

func TestOpenCommand_Filter_EmptyValue_UsageError(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	tuiCalled := false
	withFuncSeam(t, &openTUIFunc, func(_ *cobra.Command, _ pickerLanding, _ []string, _ bool) error {
		tuiCalled = true
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-f", ""})
	err := rootCmd.Execute()

	if err == nil {
		t.Fatal("expected usage error, got nil")
	}
	want := "-f/--filter value must not be empty"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	if _, ok := errors.AsType[*UsageError](err); !ok {
		t.Errorf("expected *UsageError (exit 2), got %T", err)
	}
	if tuiCalled {
		t.Error("openTUIFunc must not be called for an empty -f value")
	}
}

func TestOpenCommand_NoArgs_NoFilter_LaunchesPicker(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	var gotLanding pickerLanding
	tuiCalled := false
	withFuncSeam(t, &openTUIFunc, func(_ *cobra.Command, landing pickerLanding, _ []string, _ bool) error {
		tuiCalled = true
		gotLanding = landing
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !tuiCalled {
		t.Fatal("openTUIFunc must be called for no-arg open")
	}
	if got := shapeOfLanding(gotLanding); got != (landingShape{}) {
		t.Errorf("landing = %+v, want the zero landing", got)
	}
}

func TestOpenCommand_CommandNoTarget_ExecFlag_OpensProjectsPicker(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	lister := &recordingFilterLister{}
	withOpenDeps(t, OpenDeps{
		SessionLister: lister,
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	})

	var gotLanding pickerLanding
	var gotCommand []string
	tuiCalled := false
	withFuncSeam(t, &openTUIFunc, func(_ *cobra.Command, landing pickerLanding, command []string, _ bool) error {
		tuiCalled = true
		gotLanding = landing
		gotCommand = command
		return nil
	})

	sessionCalled := false
	withFuncSeam(t, &openSessionFunc, func(_ *cobra.Command, _ string) error {
		sessionCalled = true
		return nil
	})

	pathCalled := false
	withFuncSeam(t, &openPathFunc, func(_ *cobra.Command, _ string, _ []string) error {
		pathCalled = true
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-e", "claude"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("open -e <cmd> with no target must NOT be a usage error, got: %v", err)
	}

	if !tuiCalled {
		t.Fatal("openTUIFunc must be called for a command with no target (Projects-mode picker)")
	}
	if got := shapeOfLanding(gotLanding); got != (landingShape{}) {
		t.Errorf("landing = %+v, want the zero landing", got)
	}
	wantCmd := []string{"claude"}
	if !slices.Equal(gotCommand, wantCmd) {
		t.Errorf("command = %v, want %v (threaded into Projects mode)", gotCommand, wantCmd)
	}
	if sessionCalled || pathCalled {
		t.Error("no resolution outcome (attach/mint) may fire on the no-target command path — the command-on-attach guard must not run")
	}
	if lister.called {
		t.Error("query resolver must not be consulted on the no-target command path (resolution skipped)")
	}
}

func TestOpenCommand_CommandNoTarget_DashDash_OpensProjectsPicker(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	lister := &recordingFilterLister{}
	withOpenDeps(t, OpenDeps{
		SessionLister: lister,
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	})

	var gotLanding pickerLanding
	var gotCommand []string
	tuiCalled := false
	withFuncSeam(t, &openTUIFunc, func(_ *cobra.Command, landing pickerLanding, command []string, _ bool) error {
		tuiCalled = true
		gotLanding = landing
		gotCommand = command
		return nil
	})

	sessionCalled := false
	withFuncSeam(t, &openSessionFunc, func(_ *cobra.Command, _ string) error {
		sessionCalled = true
		return nil
	})

	pathCalled := false
	withFuncSeam(t, &openPathFunc, func(_ *cobra.Command, _ string, _ []string) error {
		pathCalled = true
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "--", "claude"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("open -- <cmd> with no target must NOT be a usage error, got: %v", err)
	}

	if !tuiCalled {
		t.Fatal("openTUIFunc must be called for a -- command with no target (Projects-mode picker)")
	}
	if got := shapeOfLanding(gotLanding); got != (landingShape{}) {
		t.Errorf("landing = %+v, want the zero landing", got)
	}
	wantCmd := []string{"claude"}
	if !slices.Equal(gotCommand, wantCmd) {
		t.Errorf("command = %v, want %v (threaded into Projects mode)", gotCommand, wantCmd)
	}
	if sessionCalled || pathCalled {
		t.Error("no resolution outcome (attach/mint) may fire on the no-target command path — the command-on-attach guard must not run")
	}
	if lister.called {
		t.Error("query resolver must not be consulted on the no-target command path (resolution skipped)")
	}
}

func TestOpenCommand_Filter_ThreadsCommandToPicker(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	var gotLanding pickerLanding
	var gotCommand []string
	withFuncSeam(t, &openTUIFunc, func(_ *cobra.Command, landing pickerLanding, command []string, _ bool) error {
		gotLanding = landing
		gotCommand = command
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-f", "web", "-e", "claude"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, want := shapeOfLanding(gotLanding), (landingShape{filter: "web"}); got != want {
		t.Errorf("landing = %+v, want %+v", got, want)
	}
	wantCmd := []string{"claude"}
	if !slices.Equal(gotCommand, wantCmd) {
		t.Errorf("command = %v, want %v", gotCommand, wantCmd)
	}
}

func TestOpenCommand_Filter_ThreadsDashDashCommandToPicker(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	var gotLanding pickerLanding
	var gotCommand []string
	withFuncSeam(t, &openTUIFunc, func(_ *cobra.Command, landing pickerLanding, command []string, _ bool) error {
		gotLanding = landing
		gotCommand = command
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-f", "web", "--", "claude"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, want := shapeOfLanding(gotLanding), (landingShape{filter: "web"}); got != want {
		t.Errorf("landing = %+v, want %+v", got, want)
	}
	wantCmd := []string{"claude"}
	if !slices.Equal(gotCommand, wantCmd) {
		t.Errorf("command = %v, want %v", gotCommand, wantCmd)
	}
}

func TestBuildSessionConnector(t *testing.T) {
	t.Run("returns SwitchConnector when inside tmux", func(t *testing.T) {
		t.Setenv("TMUX", "/tmp/tmux-501/default,12345,0")

		client := tmux.NewClient(&tmux.RealCommander{})
		connector := buildSessionConnector(client)

		if _, ok := connector.(*SwitchConnector); !ok {
			t.Errorf("expected *SwitchConnector, got %T", connector)
		}
	})

	t.Run("returns AttachConnector when outside tmux", func(t *testing.T) {
		t.Setenv("TMUX", "")

		client := tmux.NewClient(&tmux.RealCommander{})
		connector := buildSessionConnector(client)

		if _, ok := connector.(*AttachConnector); !ok {
			t.Errorf("expected *AttachConnector, got %T", connector)
		}
	})
}

// Captures the argv without the real syscall.Exec, which would replace the
// test process.
type recordingExecer struct {
	argv0 string
	argv  []string
}

func (r *recordingExecer) Exec(argv0 string, argv []string, _ []string) error {
	r.argv0 = argv0
	r.argv = argv
	return nil
}

func TestAttachConnectorConnectArgv(t *testing.T) {
	t.Run("it attaches through CoordTargetExact", func(t *testing.T) {
		rec := &recordingExecer{}
		ac := &AttachConnector{
			execer:   rec,
			tmuxPath: "/usr/bin/tmux",
		}

		if err := ac.Connect("foo"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if rec.argv0 != "/usr/bin/tmux" {
			t.Errorf("argv0 = %q, want %q", rec.argv0, "/usr/bin/tmux")
		}
		want := []string{"tmux", "attach-session", "-t", "=foo:"}
		if !slices.Equal(rec.argv, want) {
			t.Errorf("argv = %v, want %v", rec.argv, want)
		}
	})

	// The argv is exec'd rather than run through the client, so it spends its
	// target as a plain string — the pinning must still come from the typed
	// constructor rather than from a second spelling of the same prefix.
	t.Run("it pins a hand-composed exec-chain target through the typed constructor", func(t *testing.T) {
		rec := &recordingExecer{}
		ac := &AttachConnector{
			execer:   rec,
			tmuxPath: "/usr/bin/tmux",
		}

		if err := ac.Connect("foo"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{"tmux", "attach-session", "-t", string(tmux.CoordTargetExact("foo"))}
		if !slices.Equal(rec.argv, want) {
			t.Errorf("argv = %v, want %v", rec.argv, want)
		}
	})
}

func assertResolveAttr(t *testing.T, rec logtest.Record, key, want string) {
	t.Helper()
	if got := rec.AttrString(t, key); got != want {
		t.Errorf("resolve record %q = %q, want %q", key, got, want)
	}
}

// Reads the captured records at Exec time, so the exec marker's emission can be
// placed before the handoff.
type orderingExecer struct {
	sink           *logtest.Sink
	argv0          string
	argv           []string
	execMarkerSeen bool
}

func (e *orderingExecer) Exec(argv0 string, argv []string, _ []string) error {
	e.argv0 = argv0
	e.argv = argv
	if len(e.sink.Records().Matching("process", "exec")) > 0 {
		e.execMarkerSeen = true
	}
	return nil
}

func TestAttachConnector_EmitsExecMarkerBeforeExec(t *testing.T) {
	t.Run("emits process: exec target=tmux args before exec", func(t *testing.T) {
		sink := logtest.Install(t)

		ex := &orderingExecer{sink: sink}
		ac := &AttachConnector{execer: ex, tmuxPath: "/usr/bin/tmux"}

		if err := ac.Connect("foo"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		r := sink.Records().Matching("process", "exec").Only(t, "process exec record")

		if r.Level != slog.LevelInfo {
			t.Errorf("exec marker level = %v, want INFO", r.Level)
		}
		if target := r.AttrString(t, "target"); target != "tmux" {
			t.Errorf("target attr = %q, want %q", target, "tmux")
		}
		gotArgs := r.AttrString(t, "args")
		wantArgs := "attach-session -t =foo:"
		if gotArgs != wantArgs {
			t.Errorf("args attr = %q, want %q", gotArgs, wantArgs)
		}
	})

	t.Run("marker emitted before the exec call (ordering)", func(t *testing.T) {
		sink := logtest.Install(t)

		ex := &orderingExecer{sink: sink}
		ac := &AttachConnector{execer: ex, tmuxPath: "/usr/bin/tmux"}

		if err := ac.Connect("foo"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !ex.execMarkerSeen {
			t.Fatal("process: exec marker was not present in the records captured at Exec invocation time — it must be emitted BEFORE syscall.Exec")
		}
	})
}

func TestPathOpener_EmitsExecMarkerBeforeExec_OutsideTmux(t *testing.T) {
	t.Run("emits process: exec target=tmux args=joined ExecArgs before exec", func(t *testing.T) {
		sink := logtest.Install(t)

		ex := &orderingExecer{sink: sink}
		opener := &PathOpener{
			insideTmux: false,
			creator:    &mockSessionCreator{},
			switcher:   &mockSwitchClient{},
			qs: &mockQuickStarter{
				result: &session.QuickStartResult{
					SessionName: "myproject-abc123",
					Dir:         "/home/user/project",
					ExecArgs:    []string{"tmux", "new-session", "-A", "-s", "myproject-abc123", "-c", "/home/user/project"},
				},
			},
			execer:   ex,
			tmuxPath: "/usr/bin/tmux",
		}

		if err := opener.Open("/home/user/project", nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		r := sink.Records().Matching("process", "exec").Only(t, "process exec record")

		if r.Level != slog.LevelInfo {
			t.Errorf("exec marker level = %v, want INFO", r.Level)
		}
		if target := r.AttrString(t, "target"); target != "tmux" {
			t.Errorf("target attr = %q, want %q", target, "tmux")
		}
		gotArgs := r.AttrString(t, "args")
		wantArgs := "new-session -A -s myproject-abc123 -c /home/user/project"
		if gotArgs != wantArgs {
			t.Errorf("args attr = %q, want %q", gotArgs, wantArgs)
		}
	})

	t.Run("marker emitted before the exec call (ordering)", func(t *testing.T) {
		sink := logtest.Install(t)

		ex := &orderingExecer{sink: sink}
		opener := &PathOpener{
			insideTmux: false,
			creator:    &mockSessionCreator{},
			switcher:   &mockSwitchClient{},
			qs: &mockQuickStarter{
				result: &session.QuickStartResult{
					ExecArgs: []string{"tmux", "new-session", "-A", "-s", "myproject-abc123", "-c", "/home/user/project"},
				},
			},
			execer:   ex,
			tmuxPath: "/usr/bin/tmux",
		}

		if err := opener.Open("/home/user/project", nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !ex.execMarkerSeen {
			t.Fatal("process: exec marker was not present in the records captured at Exec invocation time — it must be emitted BEFORE syscall.Exec")
		}
	})
}

func TestExecMarker_ArgsLoggedVerbatim(t *testing.T) {
	// Privacy posture: args land in portal.log verbatim (single-user threat
	// model), so a multi-word command tail must survive unredacted.
	sink := logtest.Install(t)

	ex := &orderingExecer{sink: sink}
	shellCmd := "/bin/zsh -ic 'claude --resume; exec /bin/zsh'"
	opener := &PathOpener{
		insideTmux: false,
		creator:    &mockSessionCreator{},
		switcher:   &mockSwitchClient{},
		qs: &mockQuickStarter{
			result: &session.QuickStartResult{
				ExecArgs: []string{"tmux", "new-session", "-A", "-s", "myproject-abc123", "-c", "/home/user/project", shellCmd},
			},
		},
		execer:   ex,
		tmuxPath: "/usr/bin/tmux",
	}

	if err := opener.Open("/home/user/project", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rec := sink.Records().Matching("process", "exec").Only(t, "process exec record")
	gotArgs := rec.AttrString(t, "args")
	wantArgs := "new-session -A -s myproject-abc123 -c /home/user/project " + shellCmd
	if gotArgs != wantArgs {
		t.Errorf("args attr = %q, want %q (verbatim)", gotArgs, wantArgs)
	}
}

func TestSwitchConnector_EmitsNoExecMarker(t *testing.T) {
	sink := logtest.Install(t)

	mock := &mockSwitchClient{}
	connector := &SwitchConnector{client: mock}

	if err := connector.Connect("my-session"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if recs := sink.Records().Matching("process", "exec"); len(recs) != 0 {
		t.Errorf("SwitchConnector must emit no process: exec marker, got %d", len(recs))
	}
}

func TestPathOpener_InsideTmux_EmitsNoExecMarker(t *testing.T) {
	sink := logtest.Install(t)

	ex := &orderingExecer{sink: sink}
	opener := &PathOpener{
		insideTmux: true,
		creator:    &mockSessionCreator{sessionName: "myproject-abc123"},
		switcher:   &mockSwitchClient{},
		qs:         &mockQuickStarter{},
		execer:     ex,
	}

	if err := opener.Open("/home/user/project", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if recs := sink.Records().Matching("process", "exec"); len(recs) != 0 {
		t.Errorf("PathOpener inside-tmux must emit no process: exec marker, got %d", len(recs))
	}
	if ex.argv0 != "" {
		t.Errorf("execer must not be called inside tmux, got argv0 %q", ex.argv0)
	}
}

// Mirrors the production handler's closed process-lifecycle message set.
var lifecycleBypassMessages = map[string]bool{
	"start":              true,
	"exit":               true,
	"exec":               true,
	"panic":              true,
	"log-level resolved": true,
}

// Models the production WARN gate plus the lifecycle bypass, without exporting
// the production handler. It gates, then forwards whatever survives to a
// logtest.Sink: the gate is the part a Sink cannot model, since a Sink admits
// every level. log.For binds the component attr through WithAttrs rather than
// putting it on the record, so the gate must remember its accumulated attrs to
// see it.
type warnBypassHandler struct {
	inner slog.Handler
	attrs []slog.Attr
}

func newWARNBypassHandler() (*warnBypassHandler, *logtest.Sink) {
	sink := &logtest.Sink{}
	return &warnBypassHandler{inner: sink}, sink
}

// Mirrors the production coarse INFO-floor pre-gate, so an INFO lifecycle
// record reaches Handle instead of being dropped by slog.
func (h *warnBypassHandler) Enabled(_ context.Context, level slog.Level) bool {
	floor := min(slog.LevelInfo, slog.LevelWarn)
	return level >= floor
}

func (h *warnBypassHandler) Handle(ctx context.Context, r slog.Record) error {
	bypass := h.component(r) == "process" && lifecycleBypassMessages[r.Message]
	if !bypass && r.Level < slog.LevelWarn {
		return nil
	}
	return h.inner.Handle(ctx, r)
}

func (h *warnBypassHandler) component(r slog.Record) string {
	var found string
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "component" {
			found = a.Value.Resolve().String()
			return false
		}
		return true
	})
	if found != "" {
		return found
	}
	for _, a := range h.attrs {
		if a.Key == "component" {
			return a.Value.Resolve().String()
		}
	}
	return ""
}

func (h *warnBypassHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(merged, h.attrs)
	copy(merged[len(h.attrs):], attrs)
	return &warnBypassHandler{inner: h.inner.WithAttrs(attrs), attrs: merged}
}

func (h *warnBypassHandler) WithGroup(string) slog.Handler { return h }

func TestExecMarker_VisibleAtWARN(t *testing.T) {
	// Not logtest.Install: the point of the test is the production level gate,
	// and a logtest.Sink admits every level. The gate forwards what survives
	// into the sink, so the records still read back through the Sink API.
	h, sink := newWARNBypassHandler()
	log.SetTestHandler(t, h)

	ex := &recordingExecer{}
	ac := &AttachConnector{execer: ex, tmuxPath: "/usr/bin/tmux"}

	if err := ac.Connect("foo"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r := sink.Records().Matching("process", "exec").Only(t, "process exec record")
	if target := r.AttrString(t, "target"); target != "tmux" {
		t.Errorf("target attr = %q, want %q", target, "tmux")
	}
	if gotArgs := r.AttrString(t, "args"); gotArgs != "attach-session -t =foo:" {
		t.Errorf("args attr = %q, want %q", gotArgs, "attach-session -t =foo:")
	}
}

type countingSessionLister struct {
	names []string
	calls int
}

func (c *countingSessionLister) ListSessionNames() ([]string, error) {
	c.calls++
	return c.names, nil
}

func TestOpenCommand_Ack_MalformedValue_UsageErrorBeforeTmux(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	lister := &countingSessionLister{names: []string{"dev"}}
	withOpenDeps(t, OpenDeps{
		SessionLister: lister,
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	})

	sessionCalled := false
	withFuncSeam(t, &openSessionFunc, func(_ *cobra.Command, _ string) error {
		sessionCalled = true
		return nil
	})

	pathCalled := false
	withFuncSeam(t, &openPathFunc, func(_ *cobra.Command, _ string, _ []string) error {
		pathCalled = true
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "dev", "--ack", "notcolon"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected a UsageError for a malformed --ack value, got nil")
	}
	if _, ok := errors.AsType[*UsageError](err); !ok {
		t.Errorf("error %v (%T) does not match *cmd.UsageError", err, err)
	}
	want := "open: --ack must be <batch>:<token>"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	if lister.calls != 0 {
		t.Errorf("ListSessionNames called %d times for a malformed --ack, want 0 (reject before tmux)", lister.calls)
	}
	if sessionCalled {
		t.Error("openSessionFunc must not be called for a malformed --ack")
	}
	if pathCalled {
		t.Error("openPathFunc must not be called for a malformed --ack")
	}
}

func TestOpenCommand_Ack_MarkerWrittenBeforeSessionAttach(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	var order []string
	ackWriter := &mockAckWriter{order: &order}
	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{names: []string{"dev"}},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
		AckWriter:     ackWriter,
	})

	var connectedTo string
	withFuncSeam(t, &openSessionFunc, func(_ *cobra.Command, name string) error {
		connectedTo = name
		order = append(order, "session")
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-s", "dev", "--ack", "b:t"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ackWriter.calls) != 1 {
		t.Fatalf("Write call count = %d, want 1", len(ackWriter.calls))
	}
	if got := ackWriter.calls[0]; got.batch != "b" || got.token != "t" {
		t.Errorf("Write(%q, %q), want (%q, %q)", got.batch, got.token, "b", "t")
	}
	if connectedTo != "dev" {
		t.Errorf("openSessionFunc called with %q, want %q", connectedTo, "dev")
	}
	wantOrder := []string{"write", "session"}
	if !slices.Equal(order, wantOrder) {
		t.Errorf("call order = %v, want %v (write strictly before attach)", order, wantOrder)
	}
}

func TestOpenCommand_Ack_MarkerWrittenBeforePathMint(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	var order []string
	ackWriter := &mockAckWriter{order: &order}
	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
		AckWriter:     ackWriter,
	})

	dir := t.TempDir()

	var mintedPath string
	withFuncSeam(t, &openPathFunc, func(_ *cobra.Command, path string, _ []string) error {
		mintedPath = path
		order = append(order, "path")
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-p", dir, "--ack", "b:t"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ackWriter.calls) != 1 {
		t.Fatalf("Write call count = %d, want 1", len(ackWriter.calls))
	}
	if got := ackWriter.calls[0]; got.batch != "b" || got.token != "t" {
		t.Errorf("Write(%q, %q), want (%q, %q)", got.batch, got.token, "b", "t")
	}
	if mintedPath != dir {
		t.Errorf("openPathFunc minted %q, want %q", mintedPath, dir)
	}
	wantOrder := []string{"write", "path"}
	if !slices.Equal(order, wantOrder) {
		t.Errorf("call order = %v, want %v (write strictly before mint)", order, wantOrder)
	}
}

func TestOpenCommand_Ack_WriteFailureStillConnects(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	t.Run("session attach", func(t *testing.T) {
		ackWriter := &mockAckWriter{err: fmt.Errorf("set-option failed")}
		withOpenDeps(t, OpenDeps{
			SessionLister: &testSessionLister{names: []string{"dev"}},
			AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
			Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
			DirValidator:  &testDirValidator{existing: map[string]bool{}},
			AckWriter:     ackWriter,
		})

		var connectedTo string
		withFuncSeam(t, &openSessionFunc, func(_ *cobra.Command, name string) error {
			connectedTo = name
			return nil
		})

		resetRootCmd()
		rootCmd.SetArgs([]string{"open", "-s", "dev", "--ack", "b:t"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("expected no error on best-effort write failure, got %v", err)
		}

		if len(ackWriter.calls) != 1 {
			t.Errorf("Write call count = %d, want 1", len(ackWriter.calls))
		}
		if connectedTo != "dev" {
			t.Errorf("openSessionFunc called with %q, want %q (best-effort must still connect)", connectedTo, "dev")
		}
	})

	t.Run("path mint", func(t *testing.T) {
		ackWriter := &mockAckWriter{err: fmt.Errorf("set-option failed")}
		withOpenDeps(t, OpenDeps{
			SessionLister: &testSessionLister{},
			AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
			Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
			DirValidator:  &testDirValidator{existing: map[string]bool{}},
			AckWriter:     ackWriter,
		})

		dir := t.TempDir()

		var mintedPath string
		withFuncSeam(t, &openPathFunc, func(_ *cobra.Command, path string, _ []string) error {
			mintedPath = path
			return nil
		})

		resetRootCmd()
		rootCmd.SetArgs([]string{"open", "-p", dir, "--ack", "b:t"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("expected no error on best-effort write failure, got %v", err)
		}

		if len(ackWriter.calls) != 1 {
			t.Errorf("Write call count = %d, want 1", len(ackWriter.calls))
		}
		if mintedPath != dir {
			t.Errorf("openPathFunc minted %q, want %q (best-effort must still mint)", mintedPath, dir)
		}
	})
}

func TestOpenCommand_Ack_CommandAttachGuardFiresBeforeWrite(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	ackWriter := &mockAckWriter{}
	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{names: []string{"dev"}},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
		AckWriter:     ackWriter,
	})

	withFuncSeam(t, &openSessionFunc, func(_ *cobra.Command, _ string) error {
		t.Error("openSessionFunc must not be called: a command targeting an existing session is a usage error")
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-s", "dev", "-e", "claude", "--ack", "b:t"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected a UsageError for a command targeting an attach session, got nil")
	}
	if _, ok := errors.AsType[*UsageError](err); !ok {
		t.Errorf("error %v (%T) does not match *cmd.UsageError", err, err)
	}
	if len(ackWriter.calls) != 0 {
		t.Errorf("marker written %d times despite the command+attach guard, want 0", len(ackWriter.calls))
	}
}

func TestOpenCommand_Ack_FlagIsHidden(t *testing.T) {
	f := openCmd.Flags().Lookup("ack")
	if f == nil {
		t.Fatal("open command has no --ack flag")
	}
	if !f.Hidden {
		t.Error("--ack flag must be hidden (MarkHidden), but Hidden == false")
	}
}
