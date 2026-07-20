package cmd

// Tests in this file mutate package-level state (bootstrapDeps, openDeps, openTUIFunc) and MUST NOT use t.Parallel.

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
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/session"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
	"github.com/spf13/cobra"
)

// testAliasLookup implements resolver.AliasLookup for testing.
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

// testZoxideQuerier implements resolver.ZoxideQuerier for testing.
type testZoxideQuerier struct {
	result string
	err    error
}

func (t *testZoxideQuerier) Query(terms string) (string, error) {
	return t.result, t.err
}

// testDirValidator implements resolver.DirValidator for testing.
type testDirValidator struct {
	existing map[string]bool
}

func (t *testDirValidator) Exists(path string) bool {
	return t.existing[path]
}

// testSessionLister implements resolver.SessionLister for testing — it returns
// the user-visible (leading-underscore-filtered) session name set.
type testSessionLister struct {
	names []string
	err   error
}

func (t *testSessionLister) ListSessionNames() ([]string, error) {
	return t.names, t.err
}

func TestOpenCommand_PathArgument_NonExistentPath(t *testing.T) {
	// A Client is required in context: the session-domain pre-check consults it
	// via buildQueryResolver → tmuxClient(cmd) before path resolution runs.
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}, Client: tmux.NewClient(&stubCommander{})}
	t.Cleanup(func() { bootstrapDeps = nil })

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
	// A Client is required in context: the session-domain pre-check consults it
	// via buildQueryResolver → tmuxClient(cmd) before path resolution runs.
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}, Client: tmux.NewClient(&stubCommander{})}
	t.Cleanup(func() { bootstrapDeps = nil })

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
	// When a path argument is given, the TUI should not be launched.
	// We verify this by checking that IsPathArgument returns true for the arg,
	// and the command enters the path resolution branch.
	// A valid directory that exists will proceed to session creation, which
	// requires tmux -- so we test the path detection logic independently.
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
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	// When a non-path query resolves to an alias that points to a non-existent directory,
	// the error message should indicate the directory was not found.
	openDeps = &OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{"myapp": "/nonexistent/alias/path"}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	}
	t.Cleanup(func() { openDeps = nil })

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
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	// When a non-path query resolves via zoxide to a non-existent directory,
	// the error message should indicate the directory was not found.
	openDeps = &OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{result: "/gone/zoxide/dir"},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	}
	t.Cleanup(func() { openDeps = nil })

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
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	// An exact user-visible session-name hit must attach (openSessionFunc),
	// never mint (openPathFunc) or launch the picker (openTUIFunc).
	openDeps = &OpenDeps{
		SessionLister: &testSessionLister{names: []string{"api-x7Kd9a"}},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	}
	t.Cleanup(func() { openDeps = nil })

	var connectedTo string
	origSession := openSessionFunc
	openSessionFunc = func(_ *cobra.Command, name string) error {
		connectedTo = name
		return nil
	}
	t.Cleanup(func() { openSessionFunc = origSession })

	pathCalled := false
	origPath := openPathFunc
	openPathFunc = func(_ *cobra.Command, _ string, _ []string) error {
		pathCalled = true
		return nil
	}
	t.Cleanup(func() { openPathFunc = origPath })

	tuiCalled := false
	origTUI := openTUIFunc
	openTUIFunc = func(_ *cobra.Command, _ string, _ []string, _ bool) error {
		tuiCalled = true
		return nil
	}
	t.Cleanup(func() { openTUIFunc = origTUI })

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
	// `open -s <exact-user-visible-name>` resolves in the session domain only and
	// attaches (openSessionFunc) — never mints (openPathFunc) or opens the picker
	// (openTUIFunc). Spec § Domain-pinning flags: -s attaches, never mints.
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	openDeps = &OpenDeps{
		SessionLister: &testSessionLister{names: []string{"api-x7Kd9a"}},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	}
	t.Cleanup(func() { openDeps = nil })

	var connectedTo string
	origSession := openSessionFunc
	openSessionFunc = func(_ *cobra.Command, name string) error {
		connectedTo = name
		return nil
	}
	t.Cleanup(func() { openSessionFunc = origSession })

	pathCalled := false
	origPath := openPathFunc
	openPathFunc = func(_ *cobra.Command, _ string, _ []string) error {
		pathCalled = true
		return nil
	}
	t.Cleanup(func() { openPathFunc = origPath })

	tuiCalled := false
	origTUI := openTUIFunc
	openTUIFunc = func(_ *cobra.Command, _ string, _ []string, _ bool) error {
		tuiCalled = true
		return nil
	}
	t.Cleanup(func() { openTUIFunc = origTUI })

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
	// A command (-e/--) is mint-scoped: an existing (attach) session has no safe
	// command-injection channel, so `open <exact-session-name> -e <cmd>` is a
	// usage error (exit 2) and NO attach happens (spec § Command passthrough —
	// mint-scoped). The bare-positional session hit routes through the shared
	// openResolved dispatch, where the *SessionResult arm rejects a command.
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	openDeps = &OpenDeps{
		SessionLister: &testSessionLister{names: []string{"dev"}},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	}
	t.Cleanup(func() { openDeps = nil })

	sessionCalled := false
	origSession := openSessionFunc
	openSessionFunc = func(_ *cobra.Command, _ string) error {
		sessionCalled = true
		return nil
	}
	t.Cleanup(func() { openSessionFunc = origSession })

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
	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
		t.Errorf("expected *UsageError (exit 2), got %T", err)
	}
	if sessionCalled {
		t.Error("openSessionFunc must not be called: no attach may happen when a command targets an existing session")
	}
}

func TestOpenCommand_SessionPin_WithCommand_UsageError(t *testing.T) {
	// The same mint-scoped guard fires for the -s pin: `open -s <session> -e <cmd>`
	// is a usage error (exit 2), because the -s pin also dispatches its session hit
	// through the shared openResolved switch. No attach happens.
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	openDeps = &OpenDeps{
		SessionLister: &testSessionLister{names: []string{"dev"}},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	}
	t.Cleanup(func() { openDeps = nil })

	sessionCalled := false
	origSession := openSessionFunc
	openSessionFunc = func(_ *cobra.Command, _ string) error {
		sessionCalled = true
		return nil
	}
	t.Cleanup(func() { openSessionFunc = origSession })

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
	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
		t.Errorf("expected *UsageError (exit 2), got %T", err)
	}
	if sessionCalled {
		t.Error("openSessionFunc must not be called: no attach may happen when a command targets a -s pinned session")
	}
}

func TestOpenCommand_SessionPin_Glob_HardFailsNoFirstMatch(t *testing.T) {
	// Regression guard (report 13-1): a glob-bearing `-s` value reaching the single-pin
	// path must NEVER silently fork to the first match — glob fan-out is EXCLUSIVELY the
	// burst's job. In production the isMultiTarget/os.Args gate diverts a glob-bearing
	// `-s` value to the burst; but under `go test` that gate is inert (openOwnArgs()
	// finds no "open" token in the test binary's argv and returns nil), so this drives
	// the exact os.Args-assumption-break the report is about: `open -s 'api-*'` reaches
	// ResolveSessionPin directly and must hard-fail LOUDLY instead of attaching api-1.
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	openDeps = &OpenDeps{
		SessionLister: &testSessionLister{names: []string{"api-1", "api-2"}},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	}
	t.Cleanup(func() { openDeps = nil })

	attached := false
	origSession := openSessionFunc
	openSessionFunc = func(_ *cobra.Command, _ string) error {
		attached = true
		return nil
	}
	t.Cleanup(func() { openSessionFunc = origSession })

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
	// A -s miss (no exact, zero glob, or empty set) hard-fails with the verbatim
	// attach miss message and NEVER opens the picker or mints. Spec § Pinned-domain
	// contract: pins never fall back to the picker.
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	openDeps = &OpenDeps{
		SessionLister: &testSessionLister{names: []string{"web-abc"}},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	}
	t.Cleanup(func() { openDeps = nil })

	tuiCalled := false
	origTUI := openTUIFunc
	openTUIFunc = func(_ *cobra.Command, _ string, _ []string, _ bool) error {
		tuiCalled = true
		return nil
	}
	t.Cleanup(func() { openTUIFunc = origTUI })

	pathCalled := false
	origPath := openPathFunc
	openPathFunc = func(_ *cobra.Command, _ string, _ []string) error {
		pathCalled = true
		return nil
	}
	t.Cleanup(func() { openPathFunc = origPath })

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
	// A missing session is a runtime failure → plain error (exit 1), not UsageError.
	var usageErr *UsageError
	if errors.As(err, &usageErr) {
		t.Error("-s miss error must be a plain error, not a *UsageError")
	}
}

func TestOpenCommand_SessionPin_EmitsNoResolveLine(t *testing.T) {
	// A -s pin is deterministic (session-domain by construction), not a guess, so
	// it emits NO "resolve" decision line (spec § Wrong-guess feedback — pins emit
	// no resolve line; Phase 1 gates the line to the bare-positional path).
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	openDeps = &OpenDeps{
		SessionLister: &testSessionLister{names: []string{"dev"}},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	}
	t.Cleanup(func() { openDeps = nil })

	origSession := openSessionFunc
	openSessionFunc = func(_ *cobra.Command, _ string) error { return nil }
	t.Cleanup(func() { openSessionFunc = origSession })

	h := newCapturingHandler()
	log.SetTestHandler(t, h)

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-s", "dev"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if recs := h.resolveRecords(); len(recs) != 0 {
		t.Fatalf("expected no resolve records for a -s pin, got %d", len(recs))
	}
}

func TestOpenCommand_PathPin_Mints_NoPicker(t *testing.T) {
	// `open -p <existing-dir>` resolves in the path domain only and mints
	// (openPathFunc) — never attaches (openSessionFunc) and never opens the picker
	// (openTUIFunc). Spec § Domain-pinning flags: -p mints; dir must exist.
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	openDeps = &OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	}
	t.Cleanup(func() { openDeps = nil })

	dir := t.TempDir()

	var mintedPath string
	origPath := openPathFunc
	openPathFunc = func(_ *cobra.Command, path string, _ []string) error {
		mintedPath = path
		return nil
	}
	t.Cleanup(func() { openPathFunc = origPath })

	sessionCalled := false
	origSession := openSessionFunc
	openSessionFunc = func(_ *cobra.Command, _ string) error {
		sessionCalled = true
		return nil
	}
	t.Cleanup(func() { openSessionFunc = origSession })

	tuiCalled := false
	origTUI := openTUIFunc
	openTUIFunc = func(_ *cobra.Command, _ string, _ []string, _ bool) error {
		tuiCalled = true
		return nil
	}
	t.Cleanup(func() { openTUIFunc = origTUI })

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
	// A directory whose name contains glob metacharacters (foo[1]) is UNREACHABLE
	// as a bare positional (the glob pre-check hard-fails it), but -p reaches it:
	// ResolvePath stats the literal path, bypassing glob detection, and mints.
	// Spec § Glob targets (glob-named dir escape).
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	openDeps = &OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	}
	t.Cleanup(func() { openDeps = nil })

	tmp := t.TempDir()
	globDir := filepath.Join(tmp, "foo[1]")
	if err := os.Mkdir(globDir, 0o755); err != nil {
		t.Fatalf("failed to create glob-named dir: %v", err)
	}

	var mintedPath string
	origPath := openPathFunc
	openPathFunc = func(_ *cobra.Command, path string, _ []string) error {
		mintedPath = path
		return nil
	}
	t.Cleanup(func() { openPathFunc = origPath })

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-p", globDir})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mintedPath != globDir {
		t.Errorf("openPathFunc minted %q, want %q", mintedPath, globDir)
	}
}

func TestOpenCommand_PathPin_ThreadsCommandIntoMint(t *testing.T) {
	// A present -e/-- command threads into the minted session unchanged:
	// `open -p <dir> -e claude` → openPathFunc receives command == [claude].
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	openDeps = &OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	}
	t.Cleanup(func() { openDeps = nil })

	dir := t.TempDir()

	var gotPath string
	var gotCommand []string
	origPath := openPathFunc
	openPathFunc = func(_ *cobra.Command, path string, command []string) error {
		gotPath = path
		gotCommand = command
		return nil
	}
	t.Cleanup(func() { openPathFunc = origPath })

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
	// A -p pin is deterministic (path-domain by construction), not a guess, so it
	// emits NO "resolve" decision line (spec § Wrong-guess feedback — pins emit no
	// resolve line).
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	openDeps = &OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	}
	t.Cleanup(func() { openDeps = nil })

	dir := t.TempDir()

	origPath := openPathFunc
	openPathFunc = func(_ *cobra.Command, _ string, _ []string) error { return nil }
	t.Cleanup(func() { openPathFunc = origPath })

	h := newCapturingHandler()
	log.SetTestHandler(t, h)

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-p", dir})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if recs := h.resolveRecords(); len(recs) != 0 {
		t.Fatalf("expected no resolve records for a -p pin, got %d", len(recs))
	}
}

func TestOpenCommand_AliasPin_Mints_NoPicker(t *testing.T) {
	// `open -a <key>` resolves in the alias domain only and mints (openPathFunc) at
	// the aliased dir — even when a SAME-NAMED session exists (a session shadows the
	// key in the bare precedence chain). -a bypasses precedence: it never attaches
	// (openSessionFunc) and never opens the picker (openTUIFunc). Spec § Domain-
	// pinning flags: -a is the only way to reach a shadowed alias key.
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	dir := t.TempDir()

	openDeps = &OpenDeps{
		// A same-named session ("myapp") shadows the alias key in the bare chain;
		// -a must ignore it entirely and mint at the aliased dir.
		SessionLister: &testSessionLister{names: []string{"myapp"}},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{"myapp": dir}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{dir: true}},
	}
	t.Cleanup(func() { openDeps = nil })

	var mintedPath string
	origPath := openPathFunc
	openPathFunc = func(_ *cobra.Command, path string, _ []string) error {
		mintedPath = path
		return nil
	}
	t.Cleanup(func() { openPathFunc = origPath })

	sessionCalled := false
	origSession := openSessionFunc
	openSessionFunc = func(_ *cobra.Command, _ string) error {
		sessionCalled = true
		return nil
	}
	t.Cleanup(func() { openSessionFunc = origSession })

	tuiCalled := false
	origTUI := openTUIFunc
	openTUIFunc = func(_ *cobra.Command, _ string, _ []string, _ bool) error {
		tuiCalled = true
		return nil
	}
	t.Cleanup(func() { openTUIFunc = origTUI })

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
	// A -a miss (unknown key) hard-fails with "No alias found: <key>" and NEVER
	// opens the picker or mints. Spec § Pinned-domain contract: pins never fall back
	// to the picker.
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	openDeps = &OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{"known": "/code/known"}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	}
	t.Cleanup(func() { openDeps = nil })

	tuiCalled := false
	origTUI := openTUIFunc
	openTUIFunc = func(_ *cobra.Command, _ string, _ []string, _ bool) error {
		tuiCalled = true
		return nil
	}
	t.Cleanup(func() { openTUIFunc = origTUI })

	pathCalled := false
	origPath := openPathFunc
	openPathFunc = func(_ *cobra.Command, _ string, _ []string) error {
		pathCalled = true
		return nil
	}
	t.Cleanup(func() { openPathFunc = origPath })

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
	// An unknown alias key is a runtime failure → plain error (exit 1), not a UsageError.
	var usageErr *UsageError
	if errors.As(err, &usageErr) {
		t.Error("-a unknown-key error must be a plain error, not a *UsageError")
	}
}

func TestOpenCommand_AliasPin_ThreadsCommandIntoMint(t *testing.T) {
	// A present -e/-- command threads into the minted session unchanged:
	// `open -a <key> -e claude` → openPathFunc receives command == [claude].
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	dir := t.TempDir()

	openDeps = &OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{"myapp": dir}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{dir: true}},
	}
	t.Cleanup(func() { openDeps = nil })

	var gotPath string
	var gotCommand []string
	origPath := openPathFunc
	openPathFunc = func(_ *cobra.Command, path string, command []string) error {
		gotPath = path
		gotCommand = command
		return nil
	}
	t.Cleanup(func() { openPathFunc = origPath })

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
	// A -a pin is deterministic (alias-domain by construction), not a guess, so it
	// emits NO "resolve" decision line (spec § Wrong-guess feedback — pins emit no
	// resolve line).
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	dir := t.TempDir()

	openDeps = &OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{"myapp": dir}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{dir: true}},
	}
	t.Cleanup(func() { openDeps = nil })

	origPath := openPathFunc
	openPathFunc = func(_ *cobra.Command, _ string, _ []string) error { return nil }
	t.Cleanup(func() { openPathFunc = origPath })

	h := newCapturingHandler()
	log.SetTestHandler(t, h)

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-a", "myapp"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if recs := h.resolveRecords(); len(recs) != 0 {
		t.Fatalf("expected no resolve records for a -a pin, got %d", len(recs))
	}
}

func TestOpenCommand_ZoxidePin_Mints_NoPicker(t *testing.T) {
	// `open -z <query>` resolves in the zoxide domain only and mints (openPathFunc)
	// at zoxide's best-match dir — never attaches (openSessionFunc) and never opens
	// the picker (openTUIFunc). Spec § Domain-pinning flags: -z mints at zoxide's
	// best match.
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	dir := t.TempDir()

	openDeps = &OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{result: dir},
		DirValidator:  &testDirValidator{existing: map[string]bool{dir: true}},
	}
	t.Cleanup(func() { openDeps = nil })

	var mintedPath string
	origPath := openPathFunc
	openPathFunc = func(_ *cobra.Command, path string, _ []string) error {
		mintedPath = path
		return nil
	}
	t.Cleanup(func() { openPathFunc = origPath })

	sessionCalled := false
	origSession := openSessionFunc
	openSessionFunc = func(_ *cobra.Command, _ string) error {
		sessionCalled = true
		return nil
	}
	t.Cleanup(func() { openSessionFunc = origSession })

	tuiCalled := false
	origTUI := openTUIFunc
	openTUIFunc = func(_ *cobra.Command, _ string, _ []string, _ bool) error {
		tuiCalled = true
		return nil
	}
	t.Cleanup(func() { openTUIFunc = origTUI })

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
	// zoxide-absence is surfaced verbatim as ErrZoxideNotInstalled (a script sees
	// WHY), distinct from the bare chain's silent fall-through, and the pin NEVER
	// opens the picker. Spec § Domain-pinning flags: explicit error if zoxide is not
	// installed.
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	openDeps = &OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrZoxideNotInstalled},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	}
	t.Cleanup(func() { openDeps = nil })

	tuiCalled := false
	origTUI := openTUIFunc
	openTUIFunc = func(_ *cobra.Command, _ string, _ []string, _ bool) error {
		tuiCalled = true
		return nil
	}
	t.Cleanup(func() { openTUIFunc = origTUI })

	pathCalled := false
	origPath := openPathFunc
	openPathFunc = func(_ *cobra.Command, _ string, _ []string) error {
		pathCalled = true
		return nil
	}
	t.Cleanup(func() { openPathFunc = origPath })

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
	// A -z no-match hard-fails with "No zoxide match for: <query>" and NEVER opens
	// the picker or mints. Spec § Pinned-domain contract: pins never fall back to
	// the picker.
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	openDeps = &OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	}
	t.Cleanup(func() { openDeps = nil })

	tuiCalled := false
	origTUI := openTUIFunc
	openTUIFunc = func(_ *cobra.Command, _ string, _ []string, _ bool) error {
		tuiCalled = true
		return nil
	}
	t.Cleanup(func() { openTUIFunc = origTUI })

	pathCalled := false
	origPath := openPathFunc
	openPathFunc = func(_ *cobra.Command, _ string, _ []string) error {
		pathCalled = true
		return nil
	}
	t.Cleanup(func() { openPathFunc = origPath })

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
	// A no-match is a runtime failure → plain error (exit 1), not a UsageError.
	var usageErr *UsageError
	if errors.As(err, &usageErr) {
		t.Error("-z no-match error must be a plain error, not a *UsageError")
	}
}

func TestOpenCommand_ZoxidePin_ThreadsCommandIntoMint(t *testing.T) {
	// A present -e/-- command threads into the minted session unchanged:
	// `open -z <query> -e claude` → openPathFunc receives command == [claude].
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	dir := t.TempDir()

	openDeps = &OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{result: dir},
		DirValidator:  &testDirValidator{existing: map[string]bool{dir: true}},
	}
	t.Cleanup(func() { openDeps = nil })

	var gotPath string
	var gotCommand []string
	origPath := openPathFunc
	openPathFunc = func(_ *cobra.Command, path string, command []string) error {
		gotPath = path
		gotCommand = command
		return nil
	}
	t.Cleanup(func() { openPathFunc = origPath })

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
	// A -z pin is deterministic (zoxide-domain by construction), not a guess, so it
	// emits NO "resolve" decision line (spec § Wrong-guess feedback — pins emit no
	// resolve line).
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	dir := t.TempDir()

	openDeps = &OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{result: dir},
		DirValidator:  &testDirValidator{existing: map[string]bool{dir: true}},
	}
	t.Cleanup(func() { openDeps = nil })

	origPath := openPathFunc
	openPathFunc = func(_ *cobra.Command, _ string, _ []string) error { return nil }
	t.Cleanup(func() { openPathFunc = origPath })

	h := newCapturingHandler()
	log.SetTestHandler(t, h)

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-z", "proj"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if recs := h.resolveRecords(); len(recs) != 0 {
		t.Fatalf("expected no resolve records for a -z pin, got %d", len(recs))
	}
}

func TestOpenSession_DelegatesToBuildSessionConnector(t *testing.T) {
	// openSession must build the connector via buildSessionConnector and connect
	// through it — no real tmux. Inside tmux the connector is *SwitchConnector, so
	// Connect issues switch-client -t =<name> through the client's commander.
	t.Setenv("TMUX", "/tmp/fake-socket,1,0")

	cmder := &recordingCommander{}
	client := tmux.NewClient(cmder)
	cmd := cmdWithClient(client)

	if err := openSession(cmd, "api-x7Kd9a"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantCall := []string{"switch-client", "-t", "=api-x7Kd9a"}
	found := false
	for _, c := range cmder.Calls {
		if slices.Equal(c, wantCall) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected switch-client call %v, got calls %v", wantCall, cmder.Calls)
	}
}

// mockSwitchClient implements the SwitchClienter interface for testing.
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

// mockSessionCreator implements the sessionCreatorIface for testing.
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

// mockQuickStarter implements the quickStarter interface for testing.
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

// mockExecer implements the execer interface for testing.
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

		// Verify detached session creation
		if creator.createdDir != "/home/user/project" {
			t.Errorf("CreateFromDir called with %q, want %q", creator.createdDir, "/home/user/project")
		}

		// Verify switch-client called with correct session name
		if switcher.switchedTo != "myproject-abc123" {
			t.Errorf("SwitchClient called with %q, want %q", switcher.switchedTo, "myproject-abc123")
		}

		// Verify no exec happened
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

		// Verify QuickStart was called
		if qs.ranPath != "/home/user/project" {
			t.Errorf("QuickStart.Run called with %q, want %q", qs.ranPath, "/home/user/project")
		}

		// Verify exec was called with correct args
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

		// Verify CreateFromDir was NOT called (outside tmux uses QuickStart)
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

		// Verify switch-client was NOT called
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

// newTestOpenCmd creates a fresh cobra command with the -e/--exec flag for testing
// parseCommandArgs in isolation, avoiding state leaks between subtests.
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
			// Regression (Task 3-7): the Phase-2 -e/-- exclusivity guard still fires
			// with MULTIPLE positionals present (the multi-target burst path), before
			// any resolve/spawn.
			name:         "both -e and -- with multiple targets produces exit code 2",
			args:         []string{"open", "api", "web", "-e", "vim", "--", "claude"},
			wantErr:      "cannot use both -e/--exec and -- to specify a command",
			wantUsageErr: true,
		},
		{
			// Regression (Task 3-7): the Phase-2 empty -e guard still fires with
			// MULTIPLE positionals present.
			name:         "-e empty with multiple targets produces exit code 2",
			args:         []string{"open", "api", "web", "-e", ""},
			wantErr:      "-e/--exec value must not be empty",
			wantUsageErr: true,
		},
		{
			// Regression (Task 3-7): the Phase-2 empty -- guard still fires with
			// MULTIPLE positionals present.
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
					var usageErr *UsageError
					if !errors.As(checkErr, &usageErr) {
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

			// Check command slice
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

			// Check destination
			if gotDest != tt.wantDest {
				t.Errorf("destination = %q, want %q", gotDest, tt.wantDest)
			}
		})
	}
}

// stubProjectStore implements tui.ProjectStore for cmd-level testing.
type stubProjectStore struct {
	projects []project.Project
}

func (s *stubProjectStore) List() ([]project.Project, error) { return s.projects, nil }
func (s *stubProjectStore) CleanStale() ([]project.Project, error) {
	return s.projects, nil
}
func (s *stubProjectStore) Remove(_, _ string) error { return nil }

// stubSessionKiller implements tui.SessionKiller for cmd-level testing.
type stubSessionKiller struct{}

func (s *stubSessionKiller) KillSession(_ string) error { return nil }

// stubSessionRenamer implements tui.SessionRenamer for cmd-level testing.
type stubSessionRenamer struct{}

func (s *stubSessionRenamer) RenameSession(_, _ string) error { return nil }

// stubTUISessionCreator implements tui.SessionCreator for cmd-level testing.
type stubTUISessionCreator struct{}

func (s *stubTUISessionCreator) CreateFromDir(_ string, _ []string) (string, error) {
	return "stub-session", nil
}

// stubProjectEditor implements tui.ProjectEditor for cmd-level testing.
type stubProjectEditor struct{}

func (s *stubProjectEditor) Rename(_, _, _ string) error { return nil }

func (s *stubProjectEditor) AddTag(_, _ string) error { return nil }

func (s *stubProjectEditor) RemoveTag(_, _ string) error { return nil }

// stubAliasEditor implements tui.AliasEditor for cmd-level testing.
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

// stubCommander implements tmux.Commander for cmd-level testing.
// Returns a single session so list-sessions queries succeed during tests.
type stubCommander struct{}

func (s *stubCommander) Run(args ...string) (string, error) {
	if len(args) > 0 && args[0] == "list-sessions" {
		return "stub|1|0|", nil
	}
	return "", nil
}

// RunRaw satisfies tmux.Commander; the cmd-level stub never expects raw
// callers, so it returns the same empty defaults Run does.
func (s *stubCommander) RunRaw(args ...string) (string, error) {
	return "", nil
}

// defaultTestTUIConfig returns a tuiConfig with all stub dependencies wired.
// Tests override individual fields as needed for their specific scenario.
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

		m := buildTUIModel(cfg, "", nil)

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

		m := buildTUIModel(cfg, "", []string{"claude"})

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

		m := buildTUIModel(cfg, "myapp", nil)

		if m.InitialFilter() != "myapp" {
			t.Errorf("InitialFilter() = %q, want %q", m.InitialFilter(), "myapp")
		}
		if m.CommandPending() {
			t.Error("CommandPending() = true, want false")
		}
	})

	t.Run("command and filter combines both", func(t *testing.T) {
		cfg := defaultTestTUIConfig()

		m := buildTUIModel(cfg, "myapp", []string{"claude"})

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

		m := buildTUIModel(cfg, "", nil)

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

		m := buildTUIModel(cfg, "", nil)

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

		m := buildTUIModel(cfg, "", nil)

		// Navigate to projects page and populate
		var model tea.Model = m
		// Resolve the §2.6 detect-or-timeout first-paint gate: Build arms it for
		// the default (auto) appearance, so View renders the neutral blank frame
		// until OSC 11 detection (or the timeout) resolves the canvas mode. Deliver
		// the OSC 11 reply exactly as the live program does so View paints the real
		// content (the edit modal) this test asserts on.
		model, _ = model.Update(tea.BackgroundColorMsg{Color: color.RGBA{R: 0x0b, G: 0x0c, B: 0x14, A: 0xff}})
		// x is the sole Sessions↔Projects toggle (§12.2; the former p alias is
		// dropped). The model starts on Sessions, so x navigates to Projects.
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		model, _ = model.Update(tui.ProjectsLoadedMsg{Projects: projects})

		// Press e to open edit modal
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

		m := buildTUIModel(cfg, "", nil)

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

		m := buildTUIModel(cfg, "", nil)

		if m.ActivePage() != tui.PageSessions {
			t.Errorf("ActivePage() = %d, want PageSessions (%d)", m.ActivePage(), tui.PageSessions)
		}
		if m.ServerStarted() {
			t.Error("ServerStarted() = true, want false")
		}
	})

	t.Run("default serverStarted starts on sessions page", func(t *testing.T) {
		cfg := defaultTestTUIConfig()

		m := buildTUIModel(cfg, "", nil)

		if m.ActivePage() != tui.PageSessions {
			t.Errorf("ActivePage() = %d, want PageSessions (%d)", m.ActivePage(), tui.PageSessions)
		}
	})

	t.Run("serverStarted true preserves other options", func(t *testing.T) {
		cfg := defaultTestTUIConfig()
		cfg.insideTmux = true
		cfg.currentSession = "dev"
		cfg.serverStarted = true

		m := buildTUIModel(cfg, "", nil)

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
		// m.Selected() is "" by default
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
		// Simulate user selecting a session via Update with Enter
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
	// A target that resolves to nothing across every domain is a hard failure
	// carrying the escape-hatch message — the TUI picker is NEVER launched on a
	// miss (spec § Miss handling — total miss is a hard fail). The implicit
	// fallback-to-TUI-with-filter is removed.
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	openDeps = &OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	}
	t.Cleanup(func() { openDeps = nil })

	tuiCalled := false
	origFunc := openTUIFunc
	openTUIFunc = func(_ *cobra.Command, _ string, _ []string, _ bool) error {
		tuiCalled = true
		return nil
	}
	t.Cleanup(func() { openTUIFunc = origFunc })

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
	// A plain (non-usage) error → exit code 1, not the UsageError code 2.
	var usageErr *UsageError
	if errors.As(err, &usageErr) {
		t.Error("miss error must be a plain error, not a *UsageError")
	}
}

func TestOpenCommand_ResolveLog_SessionHit(t *testing.T) {
	// A session-domain hit emits exactly one INFO "resolve" decision line with
	// domain=session and resolved_path = the session name (the attr is overloaded
	// per the spec: resolved directory, or session name for a session hit).
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	openDeps = &OpenDeps{
		SessionLister: &testSessionLister{names: []string{"dev"}},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	}
	t.Cleanup(func() { openDeps = nil })

	origSession := openSessionFunc
	openSessionFunc = func(_ *cobra.Command, _ string) error { return nil }
	t.Cleanup(func() { openSessionFunc = origSession })

	h := newCapturingHandler()
	log.SetTestHandler(t, h)

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "dev"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	recs := h.resolveRecords()
	if len(recs) != 1 {
		t.Fatalf("expected exactly 1 resolve record, got %d", len(recs))
	}
	r := recs[0]
	if r.record.Level != slog.LevelInfo {
		t.Errorf("resolve record level = %v, want INFO", r.record.Level)
	}
	assertResolveAttr(t, r, "target", "dev")
	assertResolveAttr(t, r, "domain", "session")
	assertResolveAttr(t, r, "resolved_path", "dev")
}

func TestOpenCommand_ResolveLog_ZoxideMint(t *testing.T) {
	// A zoxide mint emits one INFO "resolve" line with domain=zoxide and
	// resolved_path = the resolved directory.
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	openDeps = &OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{result: "/Users/lee/Code/blog"},
		DirValidator:  &testDirValidator{existing: map[string]bool{"/Users/lee/Code/blog": true}},
	}
	t.Cleanup(func() { openDeps = nil })

	origPath := openPathFunc
	openPathFunc = func(_ *cobra.Command, _ string, _ []string) error { return nil }
	t.Cleanup(func() { openPathFunc = origPath })

	h := newCapturingHandler()
	log.SetTestHandler(t, h)

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "blog"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	recs := h.resolveRecords()
	if len(recs) != 1 {
		t.Fatalf("expected exactly 1 resolve record, got %d", len(recs))
	}
	r := recs[0]
	if r.record.Level != slog.LevelInfo {
		t.Errorf("resolve record level = %v, want INFO", r.record.Level)
	}
	assertResolveAttr(t, r, "target", "blog")
	assertResolveAttr(t, r, "domain", "zoxide")
	assertResolveAttr(t, r, "resolved_path", "/Users/lee/Code/blog")
}

func TestOpenCommand_ResolveLog_Miss(t *testing.T) {
	// A total miss emits one INFO "resolve" line with domain=miss and an empty
	// resolved_path — IN ADDITION to (not instead of) the separate stderr
	// hard-fail error, which the command still returns.
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	openDeps = &OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	}
	t.Cleanup(func() { openDeps = nil })

	h := newCapturingHandler()
	log.SetTestHandler(t, h)

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

	recs := h.resolveRecords()
	if len(recs) != 1 {
		t.Fatalf("expected exactly 1 resolve record, got %d", len(recs))
	}
	r := recs[0]
	if r.record.Level != slog.LevelInfo {
		t.Errorf("resolve record level = %v, want INFO", r.record.Level)
	}
	assertResolveAttr(t, r, "target", "blog")
	assertResolveAttr(t, r, "domain", "miss")
	assertResolveAttr(t, r, "resolved_path", "")
}

func TestOpenCommand_ResolveLog_GlobEmitsNoLine(t *testing.T) {
	// A glob target is deterministic (session-domain by construction, not a
	// guess), so it emits NO "resolve" decision line — the component stays
	// focused on guessing-chain resolutions. A glob routes to the burst (the
	// multi-target gate diverts any glob-bearing target), so this drives the
	// production path via an injected raw argv: the glob's resolve-line
	// suppression lives in resolveOpenSurfaces' emitResolveDecision gate, NOT in
	// the single-target Resolve (which is now non-glob-only — a glob reaching it
	// is a loud miss, never a first-match).
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	openDeps = &OpenDeps{
		SessionLister: &testSessionLister{names: []string{"dev-1", "dev-2"}},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	}
	t.Cleanup(func() { openDeps = nil })

	origBurst := runOpenBurstFunc
	runOpenBurstFunc = func(_ *cobra.Command, _ []spawn.Surface, _ []string) error { return nil }
	t.Cleanup(func() { runOpenBurstFunc = origBurst })

	origRaw := openRawArgs
	openRawArgs = func() []string { return []string{"portal", "open", "dev*"} }
	t.Cleanup(func() { openRawArgs = origRaw })

	h := newCapturingHandler()
	log.SetTestHandler(t, h)

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "dev*"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if recs := h.resolveRecords(); len(recs) != 0 {
		t.Fatalf("expected no resolve records for a glob target, got %d", len(recs))
	}
}

func TestEmitResolveDecision_Helper(t *testing.T) {
	// emitResolveDecision is the single-sourced resolve-decision emitter shared by
	// both bare paths (single-target open.go + resolveOpenSurfaces). The !HasGlobMeta
	// gate lives INSIDE it, so a glob target emits nothing from either call site.
	t.Run("non-glob target emits exactly one resolve line", func(t *testing.T) {
		h := newCapturingHandler()
		log.SetTestHandler(t, h)

		emitResolveDecision("dev", &resolver.SessionResult{Name: "dev", Domain: "session"})

		recs := h.resolveRecords()
		if len(recs) != 1 {
			t.Fatalf("expected exactly 1 resolve record, got %d", len(recs))
		}
		if recs[0].record.Level != slog.LevelInfo {
			t.Errorf("resolve record level = %v, want INFO", recs[0].record.Level)
		}
		assertResolveAttr(t, recs[0], "target", "dev")
		assertResolveAttr(t, recs[0], "domain", "session")
		assertResolveAttr(t, recs[0], "resolved_path", "dev")
	})

	t.Run("glob target emits no line (gate lives in the helper)", func(t *testing.T) {
		h := newCapturingHandler()
		log.SetTestHandler(t, h)

		emitResolveDecision("dev*", &resolver.SessionResult{Name: "dev-1", Domain: "glob"})

		if recs := h.resolveRecords(); len(recs) != 0 {
			t.Fatalf("expected no resolve records for a glob target, got %d", len(recs))
		}
	})
}

func TestLogExecHandoff_Helper(t *testing.T) {
	// logExecHandoff is the single-sourced exec-handoff emitter shared by both exec
	// paths (AttachConnector + PathOpener). It defensively strips argv[0] so both
	// sites render args=argv[1:] byte-identically.
	t.Run("strips argv[0] and joins the rest under target=tmux", func(t *testing.T) {
		h := newCapturingHandler()
		log.SetTestHandler(t, h)

		logExecHandoff([]string{"tmux", "attach-session", "-t", "=foo"})

		recs := h.execRecords()
		if len(recs) != 1 {
			t.Fatalf("expected exactly 1 process: exec record, got %d", len(recs))
		}
		if recs[0].record.Level != slog.LevelInfo {
			t.Errorf("exec marker level = %v, want INFO", recs[0].record.Level)
		}
		if target, ok := recordStringAttr(recs[0], "target"); !ok || target != "tmux" {
			t.Errorf("target attr = %q (ok=%v), want %q", target, ok, "tmux")
		}
		if gotArgs, ok := recordStringAttr(recs[0], "args"); !ok || gotArgs != "attach-session -t =foo" {
			t.Errorf("args attr = %q (ok=%v), want %q", gotArgs, ok, "attach-session -t =foo")
		}
	})

	t.Run("defensive: empty argv does not panic and logs empty args", func(t *testing.T) {
		h := newCapturingHandler()
		log.SetTestHandler(t, h)

		logExecHandoff(nil)

		recs := h.execRecords()
		if len(recs) != 1 {
			t.Fatalf("expected exactly 1 process: exec record, got %d", len(recs))
		}
		if gotArgs, ok := recordStringAttr(recs[0], "args"); !ok || gotArgs != "" {
			t.Errorf("args attr = %q (ok=%v), want empty", gotArgs, ok)
		}
	})
}

func TestOpenCommand_BareProjectName_MintsNeverAttaches(t *testing.T) {
	// 'open api' with a running api-x7Kd9a session must NOT attach it: the
	// exact-name check misses ({project}-{nanoid} names never equal the bare
	// project name), so api falls through the directory chain and mints
	// (spec § Bare project shorthand does not reattach).
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	openDeps = &OpenDeps{
		SessionLister: &testSessionLister{names: []string{"api-x7Kd9a"}},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{"api": "/Users/lee/Code/api"}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{"/Users/lee/Code/api": true}},
	}
	t.Cleanup(func() { openDeps = nil })

	var mintedPath string
	origPath := openPathFunc
	openPathFunc = func(_ *cobra.Command, path string, _ []string) error {
		mintedPath = path
		return nil
	}
	t.Cleanup(func() { openPathFunc = origPath })

	sessionCalled := false
	origSession := openSessionFunc
	openSessionFunc = func(_ *cobra.Command, _ string) error {
		sessionCalled = true
		return nil
	}
	t.Cleanup(func() { openSessionFunc = origSession })

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
	// The -- command must thread through openPath into the minted session,
	// unchanged from today's openPath(cmd, path, command) routing.
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	openDeps = &OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{"api": "/Users/lee/Code/api"}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{"/Users/lee/Code/api": true}},
	}
	t.Cleanup(func() { openDeps = nil })

	var gotPath string
	var gotCommand []string
	origPath := openPathFunc
	openPathFunc = func(_ *cobra.Command, path string, command []string) error {
		gotPath = path
		gotCommand = command
		return nil
	}
	t.Cleanup(func() { openPathFunc = origPath })

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
	// When no destination is provided, the direct TUI path must pass
	// the real serverWasStarted value so the TUI shows its loading interstitial.
	runner := &recordingRunner{started: true}
	bootstrapDeps = &BootstrapDeps{Orchestrator: runner}
	t.Cleanup(func() { bootstrapDeps = nil })

	var capturedServerStarted bool
	origFunc := openTUIFunc
	openTUIFunc = func(_ *cobra.Command, initialFilter string, command []string, serverStarted bool) error {
		capturedServerStarted = serverStarted
		return nil
	}
	t.Cleanup(func() { openTUIFunc = origFunc })

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

// recordingFilterLister records whether its ListSessionNames was consulted, so a
// -f invocation can assert the query resolver's session pre-check never ran (i.e.
// resolution was skipped entirely — -f is a picker redirect, not a target).
type recordingFilterLister struct {
	names  []string
	called bool
}

func (r *recordingFilterLister) ListSessionNames() ([]string, error) {
	r.called = true
	return r.names, nil
}

func TestOpenCommand_Filter_OpensPickerPrefilteredAndSkipsResolution(t *testing.T) {
	// -f <text> (no positional) skips resolution and launches the picker
	// pre-filled with the filter text (spec § -f/--filter is the sole
	// non-composing flag). The query resolver's session pre-check must never run.
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	lister := &recordingFilterLister{}
	openDeps = &OpenDeps{
		SessionLister: lister,
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	}
	t.Cleanup(func() { openDeps = nil })

	var gotFilter string
	var gotCommand []string
	tuiCalled := false
	origFunc := openTUIFunc
	openTUIFunc = func(_ *cobra.Command, initialFilter string, command []string, _ bool) error {
		tuiCalled = true
		gotFilter = initialFilter
		gotCommand = command
		return nil
	}
	t.Cleanup(func() { openTUIFunc = origFunc })

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-f", "blog"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !tuiCalled {
		t.Fatal("openTUIFunc must be called for -f")
	}
	if gotFilter != "blog" {
		t.Errorf("initialFilter = %q, want %q", gotFilter, "blog")
	}
	if gotCommand != nil {
		t.Errorf("command = %v, want nil", gotCommand)
	}
	if lister.called {
		t.Error("query resolver must not be consulted for a -f invocation (resolution skipped)")
	}
}

func TestOpenCommand_Filter_WithPositionalTarget_UsageError(t *testing.T) {
	// -f combined with a positional target is a usage error (exit 2); neither the
	// resolver nor the picker is invoked (spec § -f is mutually exclusive with a
	// positional target).
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	lister := &recordingFilterLister{}
	openDeps = &OpenDeps{
		SessionLister: lister,
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	}
	t.Cleanup(func() { openDeps = nil })

	tuiCalled := false
	origFunc := openTUIFunc
	openTUIFunc = func(_ *cobra.Command, _ string, _ []string, _ bool) error {
		tuiCalled = true
		return nil
	}
	t.Cleanup(func() { openTUIFunc = origFunc })

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
	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
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
	// -f combined with ANY domain pin (-s/-p/-z/-a) is a usage error (exit 2):
	// -f is the sole non-composing flag (spec § -f/--filter is the sole
	// non-composing flag; § Target-set composition). The guard runs BEFORE pin
	// dispatch, so neither the resolver nor the picker is invoked, and no
	// resolution outcome (attach/mint) fires.
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
			bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
			t.Cleanup(func() { bootstrapDeps = nil })

			lister := &recordingFilterLister{}
			openDeps = &OpenDeps{
				SessionLister: lister,
				AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
				Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
				DirValidator:  &testDirValidator{existing: map[string]bool{}},
			}
			t.Cleanup(func() { openDeps = nil })

			tuiCalled := false
			origTUI := openTUIFunc
			openTUIFunc = func(_ *cobra.Command, _ string, _ []string, _ bool) error {
				tuiCalled = true
				return nil
			}
			t.Cleanup(func() { openTUIFunc = origTUI })

			sessionCalled := false
			origSession := openSessionFunc
			openSessionFunc = func(_ *cobra.Command, _ string) error {
				sessionCalled = true
				return nil
			}
			t.Cleanup(func() { openSessionFunc = origSession })

			pathCalled := false
			origPath := openPathFunc
			openPathFunc = func(_ *cobra.Command, _ string, _ []string) error {
				pathCalled = true
				return nil
			}
			t.Cleanup(func() { openPathFunc = origPath })

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
			var usageErr *UsageError
			if !errors.As(err, &usageErr) {
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
	// -f combined with multiple pins is still a usage error — the guard rejects
	// on ANY pin, so -f + -s + -p is rejected the same as a single pin.
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	lister := &recordingFilterLister{}
	openDeps = &OpenDeps{
		SessionLister: lister,
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	}
	t.Cleanup(func() { openDeps = nil })

	tuiCalled := false
	origTUI := openTUIFunc
	openTUIFunc = func(_ *cobra.Command, _ string, _ []string, _ bool) error {
		tuiCalled = true
		return nil
	}
	t.Cleanup(func() { openTUIFunc = origTUI })

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
	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
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
	// An explicitly empty -f value is a usage error (exit 2), mirroring the
	// existing empty -e guard (planner decision).
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	tuiCalled := false
	origFunc := openTUIFunc
	openTUIFunc = func(_ *cobra.Command, _ string, _ []string, _ bool) error {
		tuiCalled = true
		return nil
	}
	t.Cleanup(func() { openTUIFunc = origFunc })

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
	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
		t.Errorf("expected *UsageError (exit 2), got %T", err)
	}
	if tuiCalled {
		t.Error("openTUIFunc must not be called for an empty -f value")
	}
}

func TestOpenCommand_NoArgs_NoFilter_LaunchesPicker(t *testing.T) {
	// Regression guard: no-arg open (no -f) still launches the picker with an
	// empty initial filter — the -f feature must not disturb this path.
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	var gotFilter string
	tuiCalled := false
	origFunc := openTUIFunc
	openTUIFunc = func(_ *cobra.Command, initialFilter string, _ []string, _ bool) error {
		tuiCalled = true
		gotFilter = initialFilter
		return nil
	}
	t.Cleanup(func() { openTUIFunc = origFunc })

	resetRootCmd()
	rootCmd.SetArgs([]string{"open"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !tuiCalled {
		t.Fatal("openTUIFunc must be called for no-arg open")
	}
	if gotFilter != "" {
		t.Errorf("initialFilter = %q, want empty", gotFilter)
	}
}

func TestOpenCommand_CommandNoTarget_ExecFlag_OpensProjectsPicker(t *testing.T) {
	// Regression guard (spec § Mint-only command with no target → picker in
	// Projects mode): `open -e <cmd>` with NO target and NO pin must reach the
	// no-target early-return and thread the command into the picker (initialFilter
	// == "", command == [claude]) — NOT the Task 2-6 usage error. The command is
	// threaded so the model lands in command-pending Projects mode; because there
	// is no positional and no pin, resolution + openResolved never run, so the
	// mint-scoped *SessionResult guard cannot fire on this path.
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	lister := &recordingFilterLister{}
	openDeps = &OpenDeps{
		SessionLister: lister,
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	}
	t.Cleanup(func() { openDeps = nil })

	var gotFilter string
	var gotCommand []string
	tuiCalled := false
	origTUI := openTUIFunc
	openTUIFunc = func(_ *cobra.Command, initialFilter string, command []string, _ bool) error {
		tuiCalled = true
		gotFilter = initialFilter
		gotCommand = command
		return nil
	}
	t.Cleanup(func() { openTUIFunc = origTUI })

	sessionCalled := false
	origSession := openSessionFunc
	openSessionFunc = func(_ *cobra.Command, _ string) error {
		sessionCalled = true
		return nil
	}
	t.Cleanup(func() { openSessionFunc = origSession })

	pathCalled := false
	origPath := openPathFunc
	openPathFunc = func(_ *cobra.Command, _ string, _ []string) error {
		pathCalled = true
		return nil
	}
	t.Cleanup(func() { openPathFunc = origPath })

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-e", "claude"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("open -e <cmd> with no target must NOT be a usage error, got: %v", err)
	}

	if !tuiCalled {
		t.Fatal("openTUIFunc must be called for a command with no target (Projects-mode picker)")
	}
	if gotFilter != "" {
		t.Errorf("initialFilter = %q, want empty (no -f)", gotFilter)
	}
	wantCmd := []string{"claude"}
	if !slices.Equal(gotCommand, wantCmd) {
		t.Errorf("command = %v, want %v (threaded into Projects mode)", gotCommand, wantCmd)
	}
	if sessionCalled || pathCalled {
		t.Error("no resolution outcome (attach/mint) may fire on the no-target command path — the Task 2-6 guard must not run")
	}
	if lister.called {
		t.Error("query resolver must not be consulted on the no-target command path (resolution skipped)")
	}
}

func TestOpenCommand_CommandNoTarget_DashDash_OpensProjectsPicker(t *testing.T) {
	// Same contract as the -e spelling via the -- spelling: `open -- <cmd>` with
	// no target and no pin threads the command into the Projects-mode picker
	// (initialFilter == "", command == [claude]) and is NOT a usage error.
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	lister := &recordingFilterLister{}
	openDeps = &OpenDeps{
		SessionLister: lister,
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	}
	t.Cleanup(func() { openDeps = nil })

	var gotFilter string
	var gotCommand []string
	tuiCalled := false
	origTUI := openTUIFunc
	openTUIFunc = func(_ *cobra.Command, initialFilter string, command []string, _ bool) error {
		tuiCalled = true
		gotFilter = initialFilter
		gotCommand = command
		return nil
	}
	t.Cleanup(func() { openTUIFunc = origTUI })

	sessionCalled := false
	origSession := openSessionFunc
	openSessionFunc = func(_ *cobra.Command, _ string) error {
		sessionCalled = true
		return nil
	}
	t.Cleanup(func() { openSessionFunc = origSession })

	pathCalled := false
	origPath := openPathFunc
	openPathFunc = func(_ *cobra.Command, _ string, _ []string) error {
		pathCalled = true
		return nil
	}
	t.Cleanup(func() { openPathFunc = origPath })

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "--", "claude"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("open -- <cmd> with no target must NOT be a usage error, got: %v", err)
	}

	if !tuiCalled {
		t.Fatal("openTUIFunc must be called for a -- command with no target (Projects-mode picker)")
	}
	if gotFilter != "" {
		t.Errorf("initialFilter = %q, want empty (no -f)", gotFilter)
	}
	wantCmd := []string{"claude"}
	if !slices.Equal(gotCommand, wantCmd) {
		t.Errorf("command = %v, want %v (threaded into Projects mode)", gotCommand, wantCmd)
	}
	if sessionCalled || pathCalled {
		t.Error("no resolution outcome (attach/mint) may fire on the no-target command path — the Task 2-6 guard must not run")
	}
	if lister.called {
		t.Error("query resolver must not be consulted on the no-target command path (resolution skipped)")
	}
}

func TestOpenCommand_Filter_ThreadsCommandToPicker(t *testing.T) {
	// -f threads any present -e/-- command straight through to the picker,
	// preserving the command-present ⇒ Projects specialization for free.
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	var gotFilter string
	var gotCommand []string
	origFunc := openTUIFunc
	openTUIFunc = func(_ *cobra.Command, initialFilter string, command []string, _ bool) error {
		gotFilter = initialFilter
		gotCommand = command
		return nil
	}
	t.Cleanup(func() { openTUIFunc = origFunc })

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-f", "web", "-e", "claude"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotFilter != "web" {
		t.Errorf("initialFilter = %q, want %q", gotFilter, "web")
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

// recordingExecer captures syscall.Exec arguments without replacing the
// test process. Used to verify AttachConnector's argv shape — the
// production AttachConnector hands off to tmux via syscall.Exec, which
// would otherwise destroy the test process.
type recordingExecer struct {
	argv0 string
	argv  []string
}

func (r *recordingExecer) Exec(argv0 string, argv []string, _ []string) error {
	r.argv0 = argv0
	r.argv = argv
	return nil
}

// TestAttachConnectorConnectArgv pins the argv passed to syscall.Exec at the
// AttachConnector boundary. The "=" prefix on the target forces tmux's
// exact-match resolution — without it, a killed session "foo" coexisting
// with a live "foo-2" would silently prefix-match the wrong session. See
// spec § Pre-select + attach sequence > step 4 and > Exact-match target
// syntax.
func TestAttachConnectorConnectArgv(t *testing.T) {
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
	want := []string{"tmux", "attach-session", "-t", "=foo"}
	if len(rec.argv) != len(want) {
		t.Fatalf("argv = %v, want %v", rec.argv, want)
	}
	for i := range want {
		if rec.argv[i] != want[i] {
			t.Errorf("argv[%d] = %q, want %q", i, rec.argv[i], want[i])
		}
	}
}

// capturedRecord pairs a captured slog.Record with the WithAttrs chain that was
// in force on the handler when it arrived. log.For("process") delivers the
// "component" attr via root.With(...) — i.e. through WithAttrs, not on the record
// itself — so a handler that wants to see the component must remember its
// accumulated attrs and merge them at lookup time.
type capturedRecord struct {
	record slog.Record
	attrs  []slog.Attr
}

// capturingHandler is an in-memory slog.Handler that records every Handle call
// together with the WithAttrs-accumulated context, so the exec-marker tests can
// assert on the For-delivered component via log.SetTestHandler.
type capturingHandler struct {
	mu       *sync.Mutex
	captured *[]capturedRecord
	attrs    []slog.Attr
}

func newCapturingHandler() *capturingHandler {
	return &capturingHandler{
		mu:       &sync.Mutex{},
		captured: &[]capturedRecord{},
	}
}

func (h *capturingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *capturingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	*h.captured = append(*h.captured, capturedRecord{record: r, attrs: h.attrs})
	return nil
}

// WithAttrs returns a derived handler sharing the same capture sink but carrying
// the extended attr chain, so For-delivered attrs (component) are remembered.
func (h *capturingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(merged, h.attrs)
	copy(merged[len(h.attrs):], attrs)
	return &capturingHandler{mu: h.mu, captured: h.captured, attrs: merged}
}

func (h *capturingHandler) WithGroup(string) slog.Handler { return h }

// snapshot returns a copy of the records captured so far. Used by the
// ordering-aware execer to prove the exec marker was already emitted at the
// instant Exec was invoked.
func (h *capturingHandler) snapshot() []capturedRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]capturedRecord, len(*h.captured))
	copy(out, *h.captured)
	return out
}

// execRecords returns every captured record whose component is "process" and
// whose message is "exec".
func (h *capturingHandler) execRecords() []capturedRecord {
	return filterExecRecords(h.snapshot())
}

func filterExecRecords(in []capturedRecord) []capturedRecord {
	var out []capturedRecord
	for _, cr := range in {
		if cr.record.Message != "exec" {
			continue
		}
		if recordComponent(cr) == "process" {
			out = append(out, cr)
		}
	}
	return out
}

// resolveRecords returns every captured record whose component is "resolve" and
// whose message is "resolved" — the resolution-decision receipt line.
func (h *capturingHandler) resolveRecords() []capturedRecord {
	var out []capturedRecord
	for _, cr := range h.snapshot() {
		if cr.record.Message != "resolved" {
			continue
		}
		if recordComponent(cr) == "resolve" {
			out = append(out, cr)
		}
	}
	return out
}

// assertResolveAttr asserts that a captured resolve record carries the named
// string attr with the expected value.
func assertResolveAttr(t *testing.T, cr capturedRecord, key, want string) {
	t.Helper()
	got, ok := recordStringAttr(cr, key)
	if !ok {
		t.Errorf("resolve record missing %q attr", key)
		return
	}
	if got != want {
		t.Errorf("resolve record %q = %q, want %q", key, got, want)
	}
}

// recordComponent extracts the "component" attr from a captured record, checking
// the record's own attrs first then the WithAttrs-accumulated chain.
func recordComponent(cr capturedRecord) string {
	if v, ok := recordStringAttr(cr, "component"); ok {
		return v
	}
	return ""
}

// recordStringAttr extracts a named string attr from a captured record, checking
// the record's own attrs first then the WithAttrs-accumulated chain.
func recordStringAttr(cr capturedRecord, key string) (string, bool) {
	var (
		found string
		ok    bool
	)
	cr.record.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			found = a.Value.Resolve().String()
			ok = true
			return false
		}
		return true
	})
	if ok {
		return found, true
	}
	for _, a := range cr.attrs {
		if a.Key == key {
			return a.Value.Resolve().String(), true
		}
	}
	return "", false
}

// orderingExecer is a recording execer that, at Exec invocation time, snapshots
// the records already captured by the handler. The exec-marker contract requires
// the marker to be emitted BEFORE syscall.Exec — this seam proves it by checking
// that a process: exec record exists in the snapshot taken at Exec time (i.e.
// before the real syscall.Exec would have replaced the image).
type orderingExecer struct {
	handler        *capturingHandler
	argv0          string
	argv           []string
	recordsAtCall  []capturedRecord
	execMarkerSeen bool
}

func (e *orderingExecer) Exec(argv0 string, argv []string, _ []string) error {
	e.argv0 = argv0
	e.argv = argv
	e.recordsAtCall = e.handler.snapshot()
	if len(filterExecRecords(e.recordsAtCall)) > 0 {
		e.execMarkerSeen = true
	}
	return nil
}

func TestAttachConnector_EmitsExecMarkerBeforeExec(t *testing.T) {
	t.Run("emits process: exec target=tmux args before exec", func(t *testing.T) {
		h := newCapturingHandler()
		log.SetTestHandler(t, h)

		ex := &orderingExecer{handler: h}
		ac := &AttachConnector{execer: ex, tmuxPath: "/usr/bin/tmux"}

		if err := ac.Connect("foo"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		recs := h.execRecords()
		if len(recs) != 1 {
			t.Fatalf("expected exactly 1 process: exec record, got %d", len(recs))
		}
		r := recs[0]

		if r.record.Level != slog.LevelInfo {
			t.Errorf("exec marker level = %v, want INFO", r.record.Level)
		}
		if target, ok := recordStringAttr(r, "target"); !ok || target != "tmux" {
			t.Errorf("target attr = %q (ok=%v), want %q", target, ok, "tmux")
		}
		gotArgs, ok := recordStringAttr(r, "args")
		if !ok {
			t.Fatal("exec marker missing args attr")
		}
		wantArgs := "attach-session -t =foo"
		if gotArgs != wantArgs {
			t.Errorf("args attr = %q, want %q", gotArgs, wantArgs)
		}
	})

	t.Run("marker emitted before the exec call (ordering)", func(t *testing.T) {
		h := newCapturingHandler()
		log.SetTestHandler(t, h)

		ex := &orderingExecer{handler: h}
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
		h := newCapturingHandler()
		log.SetTestHandler(t, h)

		ex := &orderingExecer{handler: h}
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

		recs := h.execRecords()
		if len(recs) != 1 {
			t.Fatalf("expected exactly 1 process: exec record, got %d", len(recs))
		}
		r := recs[0]

		if r.record.Level != slog.LevelInfo {
			t.Errorf("exec marker level = %v, want INFO", r.record.Level)
		}
		if target, ok := recordStringAttr(r, "target"); !ok || target != "tmux" {
			t.Errorf("target attr = %q (ok=%v), want %q", target, ok, "tmux")
		}
		gotArgs, ok := recordStringAttr(r, "args")
		if !ok {
			t.Fatal("exec marker missing args attr")
		}
		wantArgs := "new-session -A -s myproject-abc123 -c /home/user/project"
		if gotArgs != wantArgs {
			t.Errorf("args attr = %q, want %q", gotArgs, wantArgs)
		}
	})

	t.Run("marker emitted before the exec call (ordering)", func(t *testing.T) {
		h := newCapturingHandler()
		log.SetTestHandler(t, h)

		ex := &orderingExecer{handler: h}
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
	// Privacy posture: full args land in portal.log verbatim (single-user threat
	// model). A multi-word shell-command tail must survive unredacted.
	h := newCapturingHandler()
	log.SetTestHandler(t, h)

	ex := &orderingExecer{handler: h}
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

	recs := h.execRecords()
	if len(recs) != 1 {
		t.Fatalf("expected exactly 1 process: exec record, got %d", len(recs))
	}
	gotArgs, ok := recordStringAttr(recs[0], "args")
	if !ok {
		t.Fatal("exec marker missing args attr")
	}
	wantArgs := "new-session -A -s myproject-abc123 -c /home/user/project " + shellCmd
	if gotArgs != wantArgs {
		t.Errorf("args attr = %q, want %q (verbatim)", gotArgs, wantArgs)
	}
}

func TestSwitchConnector_EmitsNoExecMarker(t *testing.T) {
	h := newCapturingHandler()
	log.SetTestHandler(t, h)

	mock := &mockSwitchClient{}
	connector := &SwitchConnector{client: mock}

	if err := connector.Connect("my-session"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if recs := h.execRecords(); len(recs) != 0 {
		t.Errorf("SwitchConnector must emit no process: exec marker, got %d", len(recs))
	}
}

func TestPathOpener_InsideTmux_EmitsNoExecMarker(t *testing.T) {
	h := newCapturingHandler()
	log.SetTestHandler(t, h)

	ex := &orderingExecer{handler: h}
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

	if recs := h.execRecords(); len(recs) != 0 {
		t.Errorf("PathOpener inside-tmux must emit no process: exec marker, got %d", len(recs))
	}
	if ex.argv0 != "" {
		t.Errorf("execer must not be called inside tmux, got argv0 %q", ex.argv0)
	}
}

// lifecycleBypassMessages mirrors the production handler's closed
// process-lifecycle message set (spec § "Lifecycle markers bypass the level
// filter"); "exec" is the message under test here.
var lifecycleBypassMessages = map[string]bool{
	"start":              true,
	"exit":               true,
	"exec":               true,
	"panic":              true,
	"log-level resolved": true,
}

// warnBypassHandler models the production textHandler's WARN-level gate plus the
// process-lifecycle bypass: a record whose component is "process" and whose
// message is in the lifecycle set ("exec" among them) writes through even though
// it is INFO and the configured level is WARN. Any other INFO record is dropped.
// It tracks the WithAttrs chain so the For-delivered component is visible (same
// reasoning as capturingHandler). This lets the test prove the call-site emits
// the marker in a shape the production bypass admits at PORTAL_LOG_LEVEL=warn —
// without exporting the unexported production newTextHandler.
type warnBypassHandler struct {
	mu       *sync.Mutex
	captured *[]capturedRecord
	attrs    []slog.Attr
}

func newWARNBypassHandler() *warnBypassHandler {
	return &warnBypassHandler{mu: &sync.Mutex{}, captured: &[]capturedRecord{}}
}

// Enabled mirrors the production coarse INFO-floor pre-gate so an INFO lifecycle
// record is not skipped by slog before Handle can apply the bypass.
func (h *warnBypassHandler) Enabled(_ context.Context, level slog.Level) bool {
	floor := min(slog.LevelInfo, slog.LevelWarn)
	return level >= floor
}

func (h *warnBypassHandler) Handle(_ context.Context, r slog.Record) error {
	cr := capturedRecord{record: r, attrs: h.attrs}
	bypass := recordComponent(cr) == "process" && lifecycleBypassMessages[r.Message]
	if !bypass && r.Level < slog.LevelWarn {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	*h.captured = append(*h.captured, cr)
	return nil
}

func (h *warnBypassHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(merged, h.attrs)
	copy(merged[len(h.attrs):], attrs)
	return &warnBypassHandler{mu: h.mu, captured: h.captured, attrs: merged}
}

func (h *warnBypassHandler) WithGroup(string) slog.Handler { return h }

func (h *warnBypassHandler) execRecords() []capturedRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]capturedRecord, len(*h.captured))
	copy(out, *h.captured)
	return filterExecRecords(out)
}

func TestExecMarker_VisibleAtWARN(t *testing.T) {
	// The exec marker is a forensic tripwire: it must reach the sink even when
	// PORTAL_LOG_LEVEL filters out ordinary INFO. The call site emits an ordinary
	// INFO line under component=process with message=exec; the production handler
	// special-cases that lifecycle set to bypass the level gate. This handler
	// models the WARN gate + bypass and asserts the marker survives.
	h := newWARNBypassHandler()
	log.SetTestHandler(t, h)

	ex := &recordingExecer{}
	ac := &AttachConnector{execer: ex, tmuxPath: "/usr/bin/tmux"}

	if err := ac.Connect("foo"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	recs := h.execRecords()
	if len(recs) != 1 {
		t.Fatalf("exec marker not visible at WARN: expected 1 process: exec record, got %d", len(recs))
	}
	r := recs[0]
	if target, ok := recordStringAttr(r, "target"); !ok || target != "tmux" {
		t.Errorf("target attr = %q (ok=%v), want %q", target, ok, "tmux")
	}
	if gotArgs, ok := recordStringAttr(r, "args"); !ok || gotArgs != "attach-session -t =foo" {
		t.Errorf("args attr = %q (ok=%v), want %q", gotArgs, ok, "attach-session -t =foo")
	}
}

// countingSessionLister records how many times ListSessionNames is invoked, so a
// test can prove a fast-fail (e.g. a malformed --ack) short-circuits before any
// session-domain resolution touches tmux.
type countingSessionLister struct {
	names []string
	calls int
}

func (c *countingSessionLister) ListSessionNames() ([]string, error) {
	c.calls++
	return c.names, nil
}

func TestOpenCommand_Ack_MalformedValue_UsageErrorBeforeTmux(t *testing.T) {
	// `open <target> --ack <malformed>` is a usage error (exit 2) rejected at the
	// very top of RunE — BEFORE any resolver/tmux call — so the session lister is
	// never touched and neither connector seam fires (spec § Hidden --ack flag).
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	lister := &countingSessionLister{names: []string{"dev"}}
	openDeps = &OpenDeps{
		SessionLister: lister,
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	}
	t.Cleanup(func() { openDeps = nil })

	sessionCalled := false
	origSession := openSessionFunc
	openSessionFunc = func(_ *cobra.Command, _ string) error {
		sessionCalled = true
		return nil
	}
	t.Cleanup(func() { openSessionFunc = origSession })

	pathCalled := false
	origPath := openPathFunc
	openPathFunc = func(_ *cobra.Command, _ string, _ []string) error {
		pathCalled = true
		return nil
	}
	t.Cleanup(func() { openPathFunc = origPath })

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "dev", "--ack", "notcolon"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected a UsageError for a malformed --ack value, got nil")
	}
	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
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
	// `open -s <name> --ack <batch>:<token>` writes the @portal-spawn marker as the
	// last act before the attach handoff: the ack Write fires with (batch, token)
	// AND strictly before openSessionFunc (spec § Hidden --ack flag).
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	var order []string
	ackWriter := &mockAckWriter{order: &order}
	openDeps = &OpenDeps{
		SessionLister: &testSessionLister{names: []string{"dev"}},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
		AckWriter:     ackWriter,
	}
	t.Cleanup(func() { openDeps = nil })

	var connectedTo string
	origSession := openSessionFunc
	openSessionFunc = func(_ *cobra.Command, name string) error {
		connectedTo = name
		order = append(order, "session")
		return nil
	}
	t.Cleanup(func() { openSessionFunc = origSession })

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
	// `open -p <dir> --ack <batch>:<token>` writes the @portal-spawn marker as the
	// last act before the mint handoff: the ack Write fires with (batch, token) AND
	// strictly before openPathFunc (spec § Hidden --ack flag).
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	var order []string
	ackWriter := &mockAckWriter{order: &order}
	openDeps = &OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
		AckWriter:     ackWriter,
	}
	t.Cleanup(func() { openDeps = nil })

	dir := t.TempDir()

	var mintedPath string
	origPath := openPathFunc
	openPathFunc = func(_ *cobra.Command, path string, _ []string) error {
		mintedPath = path
		order = append(order, "path")
		return nil
	}
	t.Cleanup(func() { openPathFunc = origPath })

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
	// A best-effort marker-write failure must NOT abort the handoff: the connector
	// still runs (false negative, no orphan) and RunE returns nil (spec § Hidden
	// --ack flag — the write is best-effort).
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	t.Run("session attach", func(t *testing.T) {
		ackWriter := &mockAckWriter{err: fmt.Errorf("set-option failed")}
		openDeps = &OpenDeps{
			SessionLister: &testSessionLister{names: []string{"dev"}},
			AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
			Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
			DirValidator:  &testDirValidator{existing: map[string]bool{}},
			AckWriter:     ackWriter,
		}
		t.Cleanup(func() { openDeps = nil })

		var connectedTo string
		origSession := openSessionFunc
		openSessionFunc = func(_ *cobra.Command, name string) error {
			connectedTo = name
			return nil
		}
		t.Cleanup(func() { openSessionFunc = origSession })

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
		openDeps = &OpenDeps{
			SessionLister: &testSessionLister{},
			AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
			Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
			DirValidator:  &testDirValidator{existing: map[string]bool{}},
			AckWriter:     ackWriter,
		}
		t.Cleanup(func() { openDeps = nil })

		dir := t.TempDir()

		var mintedPath string
		origPath := openPathFunc
		openPathFunc = func(_ *cobra.Command, path string, _ []string) error {
			mintedPath = path
			return nil
		}
		t.Cleanup(func() { openPathFunc = origPath })

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
	// The command+attach usage guard must fire BEFORE the marker write on the
	// session arm: `open -s <name> -e <cmd> --ack b:t` is a usage error and NO
	// marker is written (spec § Command passthrough — mint-scoped).
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	ackWriter := &mockAckWriter{}
	openDeps = &OpenDeps{
		SessionLister: &testSessionLister{names: []string{"dev"}},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
		AckWriter:     ackWriter,
	}
	t.Cleanup(func() { openDeps = nil })

	origSession := openSessionFunc
	openSessionFunc = func(_ *cobra.Command, _ string) error {
		t.Error("openSessionFunc must not be called: a command targeting an existing session is a usage error")
		return nil
	}
	t.Cleanup(func() { openSessionFunc = origSession })

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-s", "dev", "-e", "claude", "--ack", "b:t"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected a UsageError for a command targeting an attach session, got nil")
	}
	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
		t.Errorf("error %v (%T) does not match *cmd.UsageError", err, err)
	}
	if len(ackWriter.calls) != 0 {
		t.Errorf("marker written %d times despite the command+attach guard, want 0", len(ackWriter.calls))
	}
}

func TestOpenCommand_Ack_FlagIsHidden(t *testing.T) {
	// --ack is an internal receipt flag: hidden from --help and completion via
	// Cobra MarkHidden (spec § Hidden --ack flag).
	f := openCmd.Flags().Lookup("ack")
	if f == nil {
		t.Fatal("open command has no --ack flag")
	}
	if !f.Hidden {
		t.Error("--ack flag must be hidden (MarkHidden), but Hidden == false")
	}
}
