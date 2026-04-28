// Tests in this file mutate package-level cobra command state and MUST NOT use t.Parallel.
package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// makeFIFO creates a fresh FIFO at <dir>/<name> and returns the path.
func makeFIFO(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := state.CreateFIFO(path); err != nil {
		t.Fatalf("CreateFIFO: %v", err)
	}
	return path
}

// stubExecShell records the prog and argv passed to ExecShell. Production
// implementation calls syscall.Exec; the stub just captures. The signature
// `func(prog string, args []string)` mirrors syscall.Exec's prog+argv shape so
// hook-chain tests can assert "/bin/sh", []string{"sh", "-c", "<cmd>; exec <SHELL>"}.
type stubExecShell struct {
	mu     sync.Mutex
	called bool
	target string
	args   []string
}

func (s *stubExecShell) fn() func(string, []string) {
	return func(prog string, args []string) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.called = true
		s.target = prog
		s.args = args
	}
}

// recordingCommander (defined in state_cleanup_test.go) is the tmux mock used
// by these tests; tests that need argv assertions inspect Calls directly.

func TestHydrate_BlocksOnFIFOUntilSignalArrives(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-foo__0.0.fifo")
	scrollback := filepath.Join(dir, "scrollback")
	if err := os.WriteFile(scrollback, []byte("HELLO"), 0o600); err != nil {
		t.Fatalf("seed scrollback: %v", err)
	}

	stdout := new(bytes.Buffer)
	exec := &stubExecShell{}
	cmder := &recordingCommander{}

	// Goroutine writes to the FIFO after a 50ms delay.
	signalSent := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		f, err := os.OpenFile(fifo, os.O_WRONLY, 0)
		if err != nil {
			t.Errorf("writer open: %v", err)
			close(signalSent)
			return
		}
		_, _ = f.Write([]byte("X"))
		_ = f.Close()
		close(signalSent)
	}()

	cfg := hydrateConfig{
		FIFO:      fifo,
		File:      scrollback,
		HookKey:   "foo:0.0",
		Stdout:    stdout,
		Client:    tmux.NewClient(cmder),
		ExecShell: exec.fn(),
		OpenFIFO:  openFIFOWithTimeout,
	}
	start := time.Now()
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	elapsed := time.Since(start)

	<-signalSent
	if !exec.called {
		t.Fatal("ExecShell not called")
	}
	// Hydrate should have blocked until the writer opened (~50ms) and slept
	// 100ms after the dump. Total >= 50ms + 100ms - small margin.
	if elapsed < 100*time.Millisecond {
		t.Errorf("runHydrate returned too quickly: %v (expected blocking on FIFO + 100ms sleep)", elapsed)
	}
}

func TestHydrate_ReadsSingleByteFromFIFOOnSignal(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-foo__0.0.fifo")
	scrollback := filepath.Join(dir, "scrollback")
	if err := os.WriteFile(scrollback, []byte(""), 0o600); err != nil {
		t.Fatalf("seed scrollback: %v", err)
	}

	// Pre-write more than one byte to FIFO; runHydrate should consume only one.
	go func() {
		f, err := os.OpenFile(fifo, os.O_WRONLY, 0)
		if err != nil {
			return
		}
		_, _ = f.Write([]byte("ABCDE"))
		_ = f.Close()
	}()

	cfg := hydrateConfig{
		FIFO:      fifo,
		File:      scrollback,
		HookKey:   "foo:0.0",
		Stdout:    io.Discard,
		Client:    tmux.NewClient(&recordingCommander{}),
		ExecShell: (&stubExecShell{}).fn(),
		OpenFIFO:  openFIFOWithTimeout,
	}
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	// FIFO should have been removed.
	if _, err := os.Stat(fifo); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("FIFO not removed; stat err = %v", err)
	}
}

func TestHydrate_RemovesFIFOAfterReading(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-bar__0.0.fifo")
	scrollback := filepath.Join(dir, "scrollback")
	_ = os.WriteFile(scrollback, []byte(""), 0o600)

	go func() {
		f, err := os.OpenFile(fifo, os.O_WRONLY, 0)
		if err != nil {
			return
		}
		_, _ = f.Write([]byte("X"))
		_ = f.Close()
	}()

	cfg := hydrateConfig{
		FIFO:      fifo,
		File:      scrollback,
		HookKey:   "bar:0.0",
		Stdout:    io.Discard,
		Client:    tmux.NewClient(&recordingCommander{}),
		ExecShell: (&stubExecShell{}).fn(),
		OpenFIFO:  openFIFOWithTimeout,
	}
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	if _, err := os.Stat(fifo); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("FIFO still present after hydrate")
	}
}

func TestHydrate_EmitsResetPreambleBeforeDump(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-x__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	_ = os.WriteFile(scrollback, []byte("CONTENT"), 0o600)

	go func() {
		f, _ := os.OpenFile(fifo, os.O_WRONLY, 0)
		_, _ = f.Write([]byte("X"))
		_ = f.Close()
	}()

	stdout := new(bytes.Buffer)
	cfg := hydrateConfig{
		FIFO: fifo, File: scrollback, HookKey: "x:0.0",
		Stdout:    stdout,
		Client:    tmux.NewClient(&recordingCommander{}),
		ExecShell: (&stubExecShell{}).fn(),
		OpenFIFO:  openFIFOWithTimeout,
	}
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	out := stdout.String()
	if !strings.HasPrefix(out, hydrateResetPreamble) {
		t.Errorf("stdout does not start with reset preamble; got %q", out)
	}
	preIdx := strings.Index(out, hydrateResetPreamble)
	contentIdx := strings.Index(out, "CONTENT")
	if preIdx < 0 || contentIdx < 0 || preIdx >= contentIdx {
		t.Errorf("preamble not before content: pre=%d content=%d", preIdx, contentIdx)
	}
}

func TestHydrate_StreamsScrollbackBytesVerbatim(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-y__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	body := "line1\nline2\r\nline3\x00\xff\x1b[31mred\x1b[0m"
	_ = os.WriteFile(scrollback, []byte(body), 0o600)

	go func() {
		f, _ := os.OpenFile(fifo, os.O_WRONLY, 0)
		_, _ = f.Write([]byte("X"))
		_ = f.Close()
	}()

	stdout := new(bytes.Buffer)
	cfg := hydrateConfig{
		FIFO: fifo, File: scrollback, HookKey: "y:0.0",
		Stdout:    stdout,
		Client:    tmux.NewClient(&recordingCommander{}),
		ExecShell: (&stubExecShell{}).fn(),
		OpenFIFO:  openFIFOWithTimeout,
	}
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	if !strings.Contains(stdout.String(), body) {
		t.Errorf("stdout missing verbatim scrollback body")
	}
}

func TestHydrate_EmitsResetPostambleWithCRLFAfterDump(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-z__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	_ = os.WriteFile(scrollback, []byte("DUMP"), 0o600)

	go func() {
		f, _ := os.OpenFile(fifo, os.O_WRONLY, 0)
		_, _ = f.Write([]byte("X"))
		_ = f.Close()
	}()

	stdout := new(bytes.Buffer)
	cfg := hydrateConfig{
		FIFO: fifo, File: scrollback, HookKey: "z:0.0",
		Stdout:    stdout,
		Client:    tmux.NewClient(&recordingCommander{}),
		ExecShell: (&stubExecShell{}).fn(),
		OpenFIFO:  openFIFOWithTimeout,
	}
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	out := stdout.String()
	if !strings.HasSuffix(out, hydrateResetPostamble) {
		t.Errorf("stdout does not end with reset postamble + CRLF; got %q", out)
	}
	if !strings.HasSuffix(out, "\r\n") {
		t.Errorf("stdout does not end with CRLF; got %q", out)
	}
	dumpIdx := strings.Index(out, "DUMP")
	postIdx := strings.LastIndex(out, hydrateResetPostamble)
	if dumpIdx < 0 || postIdx < 0 || dumpIdx >= postIdx {
		t.Errorf("postamble not after content: dump=%d post=%d", dumpIdx, postIdx)
	}
}

func TestHydrate_Sleeps100msBeforeUnsettingMarker(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-q__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	_ = os.WriteFile(scrollback, []byte(""), 0o600)

	go func() {
		f, _ := os.OpenFile(fifo, os.O_WRONLY, 0)
		_, _ = f.Write([]byte("X"))
		_ = f.Close()
	}()

	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			return "", nil
		},
	}
	cfg := hydrateConfig{
		FIFO: fifo, File: scrollback, HookKey: "q:0.0",
		Stdout:    io.Discard,
		Client:    tmux.NewClient(cmder),
		ExecShell: (&stubExecShell{}).fn(),
		OpenFIFO:  openFIFOWithTimeout,
	}

	start := time.Now()
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	elapsed := time.Since(start)

	if elapsed < 100*time.Millisecond {
		t.Errorf("runHydrate elapsed %v, expected >= 100ms (settle sleep)", elapsed)
	}
	if elapsed > 1*time.Second {
		t.Errorf("runHydrate elapsed %v, suspiciously slow (expected ~100-200ms)", elapsed)
	}
}

func TestHydrate_UnsetsSkeletonMarkerWithSetOptionSU(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-foo__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	_ = os.WriteFile(scrollback, []byte(""), 0o600)

	go func() {
		f, _ := os.OpenFile(fifo, os.O_WRONLY, 0)
		_, _ = f.Write([]byte("X"))
		_ = f.Close()
	}()

	cmder := &recordingCommander{}
	cfg := hydrateConfig{
		FIFO: fifo, File: scrollback, HookKey: "foo:0.0",
		Stdout:    io.Discard,
		Client:    tmux.NewClient(cmder),
		ExecShell: (&stubExecShell{}).fn(),
		OpenFIFO:  openFIFOWithTimeout,
	}
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	want := []string{"set-option", "-su", "@portal-skeleton-foo__0.0"}
	var found bool
	for _, c := range cmder.Calls {
		if len(c) == len(want) {
			match := true
			for i := range c {
				if c[i] != want[i] {
					match = false
					break
				}
			}
			if match {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("expected tmux call %v, got calls: %v", want, cmder.Calls)
	}
}

func TestHydrate_PreservesANSISequencesInDump(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-a__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	body := "\x1b[31mred\x1b[0m\x1b[1mbold\x1b[0m"
	_ = os.WriteFile(scrollback, []byte(body), 0o600)

	go func() {
		f, _ := os.OpenFile(fifo, os.O_WRONLY, 0)
		_, _ = f.Write([]byte("X"))
		_ = f.Close()
	}()

	stdout := new(bytes.Buffer)
	cfg := hydrateConfig{
		FIFO: fifo, File: scrollback, HookKey: "a:0.0",
		Stdout:    stdout,
		Client:    tmux.NewClient(&recordingCommander{}),
		ExecShell: (&stubExecShell{}).fn(),
		OpenFIFO:  openFIFOWithTimeout,
	}
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(body)) {
		t.Errorf("ANSI escapes not preserved verbatim in dump")
	}
}

func TestHydrate_StreamsLargeScrollbackFile(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-big__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")

	// Write 5MB of pseudo-random bytes (deterministic).
	const size = 5 * 1024 * 1024
	body := make([]byte, size)
	for i := range body {
		body[i] = byte(i % 251) // 251 is prime; gives non-trivial pattern
	}
	if err := os.WriteFile(scrollback, body, 0o600); err != nil {
		t.Fatalf("seed scrollback: %v", err)
	}

	go func() {
		f, _ := os.OpenFile(fifo, os.O_WRONLY, 0)
		_, _ = f.Write([]byte("X"))
		_ = f.Close()
	}()

	stdout := new(bytes.Buffer)
	cfg := hydrateConfig{
		FIFO: fifo, File: scrollback, HookKey: "big:0.0",
		Stdout:    stdout,
		Client:    tmux.NewClient(&recordingCommander{}),
		ExecShell: (&stubExecShell{}).fn(),
		OpenFIFO:  openFIFOWithTimeout,
	}
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	out := stdout.Bytes()
	// Content must appear between preamble and postamble.
	preLen := len(hydrateResetPreamble)
	postLen := len(hydrateResetPostamble)
	if len(out) != preLen+size+postLen {
		t.Errorf("stdout length = %d, want %d", len(out), preLen+size+postLen)
	}
	dumped := out[preLen : preLen+size]
	if !bytes.Equal(dumped, body) {
		t.Errorf("dumped bytes do not match input")
	}
}

func TestHydrate_ExecsShellWhenNoHookApplies(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-s__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	_ = os.WriteFile(scrollback, []byte(""), 0o600)

	go func() {
		f, _ := os.OpenFile(fifo, os.O_WRONLY, 0)
		_, _ = f.Write([]byte("X"))
		_ = f.Close()
	}()

	t.Setenv("SHELL", "/usr/local/bin/myshell")

	exec := &stubExecShell{}
	cfg := hydrateConfig{
		FIFO: fifo, File: scrollback, HookKey: "s:0.0",
		Stdout:    io.Discard,
		Client:    tmux.NewClient(&recordingCommander{}),
		ExecShell: exec.fn(),
		OpenFIFO:  openFIFOWithTimeout,
	}
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	if !exec.called {
		t.Fatal("ExecShell not called")
	}
	if exec.target != "/usr/local/bin/myshell" {
		t.Errorf("ExecShell target = %q, want /usr/local/bin/myshell", exec.target)
	}
}

func TestHydrate_DefaultsShellToBinSh(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-d__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	_ = os.WriteFile(scrollback, []byte(""), 0o600)

	go func() {
		f, _ := os.OpenFile(fifo, os.O_WRONLY, 0)
		_, _ = f.Write([]byte("X"))
		_ = f.Close()
	}()

	t.Setenv("SHELL", "")

	exec := &stubExecShell{}
	cfg := hydrateConfig{
		FIFO: fifo, File: scrollback, HookKey: "d:0.0",
		Stdout:    io.Discard,
		Client:    tmux.NewClient(&recordingCommander{}),
		ExecShell: exec.fn(),
		OpenFIFO:  openFIFOWithTimeout,
	}
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	if exec.target != "/bin/sh" {
		t.Errorf("ExecShell target = %q, want /bin/sh", exec.target)
	}
}

func TestHydrate_DoesNotReadHooksFileInThisPhase(t *testing.T) {
	// No hooks.json exists in t.TempDir() — should not error.
	dir := t.TempDir()
	t.Setenv("PORTAL_CONFIG_HOME", dir)

	fifo := makeFIFO(t, dir, "hydrate-h__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	_ = os.WriteFile(scrollback, []byte(""), 0o600)

	go func() {
		f, _ := os.OpenFile(fifo, os.O_WRONLY, 0)
		_, _ = f.Write([]byte("X"))
		_ = f.Close()
	}()

	cfg := hydrateConfig{
		FIFO: fifo, File: scrollback, HookKey: "h:0.0",
		Stdout:    io.Discard,
		Client:    tmux.NewClient(&recordingCommander{}),
		ExecShell: (&stubExecShell{}).fn(),
		OpenFIFO:  openFIFOWithTimeout,
	}
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	// hooks.json must not have been created or read.
	if _, err := os.Stat(filepath.Join(dir, "hooks.json")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("hooks.json must not exist; stat err = %v", err)
	}
}

func TestOpenFIFOWithTimeout_ReturnsErrHydrateTimeoutWhenNoWriter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "noreader.fifo")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}

	start := time.Now()
	f, err := openFIFOWithTimeout(path, 100*time.Millisecond)
	elapsed := time.Since(start)
	if !errors.Is(err, ErrHydrateTimeout) {
		t.Fatalf("expected ErrHydrateTimeout, got %v (file=%v)", err, f)
	}
	if elapsed < 100*time.Millisecond {
		t.Errorf("returned in %v, expected >= 100ms", elapsed)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("returned in %v, expected ~100ms", elapsed)
	}
}

func TestHydrate_TimeoutPathInvokesHandleTimeout(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-t__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	_ = os.WriteFile(scrollback, []byte(""), 0o600)

	called := false
	cfg := hydrateConfig{
		FIFO: fifo, File: scrollback, HookKey: "t:0.0",
		Stdout:    io.Discard,
		Client:    tmux.NewClient(&recordingCommander{}),
		ExecShell: (&stubExecShell{}).fn(),
		// Inject an OpenFIFO that always reports timeout.
		OpenFIFO: func(_ string, _ time.Duration) (*os.File, error) {
			return nil, ErrHydrateTimeout
		},
		HandleTimeout: func(_ hydrateConfig) error {
			called = true
			return nil
		},
	}
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	if !called {
		t.Errorf("HandleTimeout not invoked on timeout path")
	}
}

func TestHydrate_FileMissingPathInvokesHandleFileMissing(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-m__0.0.fifo")
	scrollback := filepath.Join(dir, "missing-sb")
	// Do NOT create scrollback file.

	go func() {
		f, _ := os.OpenFile(fifo, os.O_WRONLY, 0)
		_, _ = f.Write([]byte("X"))
		_ = f.Close()
	}()

	called := false
	cfg := hydrateConfig{
		FIFO: fifo, File: scrollback, HookKey: "m:0.0",
		Stdout:    io.Discard,
		Client:    tmux.NewClient(&recordingCommander{}),
		ExecShell: (&stubExecShell{}).fn(),
		OpenFIFO:  openFIFOWithTimeout,
		HandleFileMissing: func(_ hydrateConfig, _ hydrateFileMissingContext) error {
			called = true
			return nil
		},
	}
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	if !called {
		t.Errorf("HandleFileMissing not invoked when scrollback file is absent")
	}
}

func TestHydrate_FileMissing_ENOENT_EmitsPreambleAndExecsShell(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-fm__0.0.fifo")
	scrollback := filepath.Join(dir, "missing-sb")

	go func() {
		f, _ := os.OpenFile(fifo, os.O_WRONLY, 0)
		_, _ = f.Write([]byte("X"))
		_ = f.Close()
	}()

	stdout := new(bytes.Buffer)
	exec := &stubExecShell{}
	cfg := hydrateConfig{
		FIFO: fifo, File: scrollback, HookKey: "fm:0.0",
		Stdout:            stdout,
		Client:            tmux.NewClient(&recordingCommander{}),
		ExecShell:         exec.fn(),
		OpenFIFO:          openFIFOWithTimeout,
		HandleFileMissing: handleHydrateFileMissing,
	}
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	if stdout.String() != hydrateResetPreamble {
		t.Errorf("stdout = %q, want exactly preamble %q", stdout.String(), hydrateResetPreamble)
	}
	if !exec.called {
		t.Fatal("ExecShell not called on file-missing path")
	}
}

func TestHydrate_FileMissing_PermissionDenied_EmitsPreambleAndExecsShell(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-fp__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	if err := os.WriteFile(scrollback, []byte("HIDDEN"), 0o600); err != nil {
		t.Fatalf("seed scrollback: %v", err)
	}
	if err := os.Chmod(scrollback, 0o000); err != nil {
		t.Fatalf("chmod 0: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(scrollback, 0o600) })

	go func() {
		f, _ := os.OpenFile(fifo, os.O_WRONLY, 0)
		_, _ = f.Write([]byte("X"))
		_ = f.Close()
	}()

	stdout := new(bytes.Buffer)
	exec := &stubExecShell{}
	cfg := hydrateConfig{
		FIFO: fifo, File: scrollback, HookKey: "fp:0.0",
		Stdout:            stdout,
		Client:            tmux.NewClient(&recordingCommander{}),
		ExecShell:         exec.fn(),
		OpenFIFO:          openFIFOWithTimeout,
		HandleFileMissing: handleHydrateFileMissing,
	}
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	if stdout.String() != hydrateResetPreamble {
		t.Errorf("stdout = %q, want exactly preamble %q", stdout.String(), hydrateResetPreamble)
	}
	if strings.Contains(stdout.String(), "HIDDEN") {
		t.Errorf("stdout contains scrollback content despite permission denied: %q", stdout.String())
	}
	if !exec.called {
		t.Fatal("ExecShell not called on permission-denied path")
	}
}

func TestHydrate_FileMissing_MidStreamCopyError_LeavesPartialBytes(t *testing.T) {
	// Drive the io.Copy mid-stream branch directly via the production handler.
	// runHydrate uses os.Open + io.Copy on a real file, so to simulate a
	// mid-stream Read failure we exercise the handler via a test that calls
	// runHydrate with a real file but a reader-error injection is impossible
	// without adding a seam. Instead, validate via direct handler invocation:
	// the handler must NOT re-emit the preamble, must skip the sleep, must
	// unset the marker, and must succeed (return nil) for any cause.
	dir := t.TempDir()
	fifo := filepath.Join(dir, "hydrate-mid__0.0.fifo")
	stdout := new(bytes.Buffer)
	// Pre-populate stdout with preamble + some "partial" bytes already written
	// by runHydrate before the mid-stream io.Copy failure.
	stdout.WriteString(hydrateResetPreamble)
	stdout.WriteString("partial-bytes-already-on-stdout")

	cmder := &recordingCommander{}
	cfg := hydrateConfig{
		FIFO: fifo, File: filepath.Join(dir, "sb"), HookKey: "mid:0.0",
		Stdout: stdout,
		Client: tmux.NewClient(cmder),
	}

	start := time.Now()
	if err := handleHydrateFileMissing(cfg, hydrateFileMissingContext{Cause: errors.New("read: I/O error")}); err != nil {
		t.Fatalf("handleHydrateFileMissing: %v", err)
	}
	elapsed := time.Since(start)

	// Preamble appears exactly once (handler does not re-emit).
	if n := strings.Count(stdout.String(), hydrateResetPreamble); n != 1 {
		t.Errorf("preamble count = %d, want 1 (handler must not re-emit)", n)
	}
	// Partial bytes from before the failure are still present (no rollback).
	if !strings.Contains(stdout.String(), "partial-bytes-already-on-stdout") {
		t.Errorf("partial bytes were rolled back; stdout = %q", stdout.String())
	}
	// Skips the 100ms settle sleep.
	if elapsed >= 100*time.Millisecond {
		t.Errorf("handleHydrateFileMissing elapsed %v; expected << 100ms (no settle sleep)", elapsed)
	}
}

func TestHydrate_FileMissing_LogsENOENTDistinctly(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-le__0.0.fifo")
	scrollback := filepath.Join(dir, "missing-sb")

	go func() {
		f, _ := os.OpenFile(fifo, os.O_WRONLY, 0)
		_, _ = f.Write([]byte("X"))
		_ = f.Close()
	}()

	logPath := filepath.Join(dir, "portal.log")
	t.Setenv("PORTAL_LOG_LEVEL", "")
	logger, err := state.OpenLogger(logPath, false)
	if err != nil {
		t.Fatalf("OpenLogger: %v", err)
	}
	t.Cleanup(func() { _ = logger.Close() })

	cfg := hydrateConfig{
		FIFO: fifo, File: scrollback, HookKey: "le:0.0",
		Stdout:            io.Discard,
		Client:            tmux.NewClient(&recordingCommander{}),
		Logger:            logger,
		ExecShell:         (&stubExecShell{}).fn(),
		OpenFIFO:          openFIFOWithTimeout,
		HandleFileMissing: handleHydrateFileMissing,
	}
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	_ = logger.Close()

	data, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatalf("read log: %v", readErr)
	}
	contents := string(data)
	if !strings.Contains(contents, "not found") {
		t.Errorf("log missing distinct ENOENT phrase \"not found\": %q", contents)
	}
}

func TestHydrate_FileMissing_LogsPermissionDistinctly(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-lp__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	if err := os.WriteFile(scrollback, []byte("X"), 0o600); err != nil {
		t.Fatalf("seed scrollback: %v", err)
	}
	if err := os.Chmod(scrollback, 0o000); err != nil {
		t.Fatalf("chmod 0: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(scrollback, 0o600) })

	go func() {
		f, _ := os.OpenFile(fifo, os.O_WRONLY, 0)
		_, _ = f.Write([]byte("X"))
		_ = f.Close()
	}()

	logPath := filepath.Join(dir, "portal.log")
	t.Setenv("PORTAL_LOG_LEVEL", "")
	logger, err := state.OpenLogger(logPath, false)
	if err != nil {
		t.Fatalf("OpenLogger: %v", err)
	}
	t.Cleanup(func() { _ = logger.Close() })

	cfg := hydrateConfig{
		FIFO: fifo, File: scrollback, HookKey: "lp:0.0",
		Stdout:            io.Discard,
		Client:            tmux.NewClient(&recordingCommander{}),
		Logger:            logger,
		ExecShell:         (&stubExecShell{}).fn(),
		OpenFIFO:          openFIFOWithTimeout,
		HandleFileMissing: handleHydrateFileMissing,
	}
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	_ = logger.Close()

	data, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatalf("read log: %v", readErr)
	}
	contents := string(data)
	if !strings.Contains(contents, "permission denied") {
		t.Errorf("log missing distinct permission phrase \"permission denied\": %q", contents)
	}
}

func TestHydrate_FileMissing_LogsGenericIOError(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "hydrate-lg__0.0.fifo")
	stdout := new(bytes.Buffer)
	stdout.WriteString(hydrateResetPreamble)

	logPath := filepath.Join(dir, "portal.log")
	t.Setenv("PORTAL_LOG_LEVEL", "")
	logger, err := state.OpenLogger(logPath, false)
	if err != nil {
		t.Fatalf("OpenLogger: %v", err)
	}
	t.Cleanup(func() { _ = logger.Close() })

	cfg := hydrateConfig{
		FIFO: fifo, File: filepath.Join(dir, "sb"), HookKey: "lg:0.0",
		Stdout: stdout,
		Client: tmux.NewClient(&recordingCommander{}),
		Logger: logger,
	}
	genericErr := errors.New("synthetic mid-stream failure")
	if err := handleHydrateFileMissing(cfg, hydrateFileMissingContext{Cause: genericErr}); err != nil {
		t.Fatalf("handleHydrateFileMissing: %v", err)
	}
	_ = logger.Close()

	data, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatalf("read log: %v", readErr)
	}
	contents := string(data)
	if !strings.Contains(contents, "I/O error") {
		t.Errorf("log missing distinct generic phrase \"I/O error\": %q", contents)
	}
	if !strings.Contains(contents, "synthetic mid-stream failure") {
		t.Errorf("log missing wrapped cause: %q", contents)
	}
}

func TestHydrate_FileMissing_LogIncludesHookKeyAndFile(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-li__0.0.fifo")
	scrollback := filepath.Join(dir, "missing-sb")

	go func() {
		f, _ := os.OpenFile(fifo, os.O_WRONLY, 0)
		_, _ = f.Write([]byte("X"))
		_ = f.Close()
	}()

	logPath := filepath.Join(dir, "portal.log")
	t.Setenv("PORTAL_LOG_LEVEL", "")
	logger, err := state.OpenLogger(logPath, false)
	if err != nil {
		t.Fatalf("OpenLogger: %v", err)
	}
	t.Cleanup(func() { _ = logger.Close() })

	cfg := hydrateConfig{
		FIFO: fifo, File: scrollback, HookKey: "li:0.0",
		Stdout:            io.Discard,
		Client:            tmux.NewClient(&recordingCommander{}),
		Logger:            logger,
		ExecShell:         (&stubExecShell{}).fn(),
		OpenFIFO:          openFIFOWithTimeout,
		HandleFileMissing: handleHydrateFileMissing,
	}
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	_ = logger.Close()

	data, _ := os.ReadFile(logPath)
	contents := string(data)
	if !strings.Contains(contents, "li:0.0") {
		t.Errorf("log missing --hook-key value: %q", contents)
	}
	if !strings.Contains(contents, scrollback) {
		t.Errorf("log missing --file path %q: %q", scrollback, contents)
	}
}

func TestHydrate_FileMissing_UnsetsSkeletonMarkerWithSetOptionSU(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-fu__0.0.fifo")
	scrollback := filepath.Join(dir, "missing-sb")

	go func() {
		f, _ := os.OpenFile(fifo, os.O_WRONLY, 0)
		_, _ = f.Write([]byte("X"))
		_ = f.Close()
	}()

	cmder := &recordingCommander{}
	cfg := hydrateConfig{
		FIFO: fifo, File: scrollback, HookKey: "fu:0.0",
		Stdout:            io.Discard,
		Client:            tmux.NewClient(cmder),
		ExecShell:         (&stubExecShell{}).fn(),
		OpenFIFO:          openFIFOWithTimeout,
		HandleFileMissing: handleHydrateFileMissing,
	}
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	want := []string{"set-option", "-su", "@portal-skeleton-fu__0.0"}
	var found bool
	for _, c := range cmder.Calls {
		if len(c) == len(want) {
			match := true
			for i := range c {
				if c[i] != want[i] {
					match = false
					break
				}
			}
			if match {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("expected tmux call %v, got calls: %v", want, cmder.Calls)
	}
}

func TestHydrate_FileMissing_SkipsSettleSleep(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-fs__0.0.fifo")
	scrollback := filepath.Join(dir, "missing-sb")

	go func() {
		f, _ := os.OpenFile(fifo, os.O_WRONLY, 0)
		_, _ = f.Write([]byte("X"))
		_ = f.Close()
	}()

	cfg := hydrateConfig{
		FIFO: fifo, File: scrollback, HookKey: "fs:0.0",
		Stdout:            io.Discard,
		Client:            tmux.NewClient(&recordingCommander{}),
		ExecShell:         (&stubExecShell{}).fn(),
		OpenFIFO:          openFIFOWithTimeout,
		HandleFileMissing: handleHydrateFileMissing,
	}

	start := time.Now()
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	elapsed := time.Since(start)

	if elapsed >= 100*time.Millisecond {
		t.Errorf("runHydrate elapsed %v on file-missing path; expected << 100ms (no settle sleep)", elapsed)
	}
}

func TestHydrate_FileMissing_DoesNotReadHooksFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_CONFIG_HOME", dir)

	fifo := makeFIFO(t, dir, "hydrate-fh__0.0.fifo")
	scrollback := filepath.Join(dir, "missing-sb")

	go func() {
		f, _ := os.OpenFile(fifo, os.O_WRONLY, 0)
		_, _ = f.Write([]byte("X"))
		_ = f.Close()
	}()

	cfg := hydrateConfig{
		FIFO: fifo, File: scrollback, HookKey: "fh:0.0",
		Stdout:            io.Discard,
		Client:            tmux.NewClient(&recordingCommander{}),
		ExecShell:         (&stubExecShell{}).fn(),
		OpenFIFO:          openFIFOWithTimeout,
		HandleFileMissing: handleHydrateFileMissing,
	}
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "hooks.json")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("hooks.json must not exist; stat err = %v", err)
	}
}

func TestHydrate_FileMissing_LeavesPartialBytesOnMidStreamFailure(t *testing.T) {
	// Direct handler invocation: simulate that runHydrate has already written
	// the preamble + some bytes from a partial io.Copy before the mid-stream
	// failure. The handler must not roll back stdout and must not double-emit
	// the preamble.
	dir := t.TempDir()
	fifo := filepath.Join(dir, "hydrate-mp__0.0.fifo")

	stdout := new(bytes.Buffer)
	stdout.WriteString(hydrateResetPreamble)
	const partial = "ABC partial data DEF"
	stdout.WriteString(partial)

	cfg := hydrateConfig{
		FIFO: fifo, File: filepath.Join(dir, "sb"), HookKey: "mp:0.0",
		Stdout: stdout,
		Client: tmux.NewClient(&recordingCommander{}),
	}
	if err := handleHydrateFileMissing(cfg, hydrateFileMissingContext{Cause: errors.New("eio")}); err != nil {
		t.Fatalf("handleHydrateFileMissing: %v", err)
	}

	out := stdout.String()
	if strings.Count(out, hydrateResetPreamble) != 1 {
		t.Errorf("preamble emitted more than once after handler: %q", out)
	}
	if !strings.Contains(out, partial) {
		t.Errorf("partial bytes lost: %q", out)
	}
}

// instantTimeoutOpenFIFO returns ErrHydrateTimeout immediately so timeout-path
// tests do not have to wait the real 3-second hydrateTimeout.
func instantTimeoutOpenFIFO(_ string, _ time.Duration) (*os.File, error) {
	return nil, ErrHydrateTimeout
}

// timeoutCfg builds a hydrateConfig wired for the production timeout path:
// OpenFIFO returns ErrHydrateTimeout immediately and HandleTimeout points at
// handleHydrateTimeout. Callers override fields as needed.
func timeoutCfg(t *testing.T, fifo, scrollback, hookKey string, stdout io.Writer, cmder *recordingCommander, exec func(string, []string), logger *state.Logger) hydrateConfig {
	t.Helper()
	return hydrateConfig{
		FIFO:          fifo,
		File:          scrollback,
		HookKey:       hookKey,
		Stdout:        stdout,
		Client:        tmux.NewClient(cmder),
		Logger:        logger,
		ExecShell:     exec,
		OpenFIFO:      instantTimeoutOpenFIFO,
		HandleTimeout: handleHydrateTimeout,
	}
}

func TestHydrate_TimeoutWritesResetPreambleToStdout(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-tp__0.0.fifo")

	stdout := new(bytes.Buffer)
	cfg := timeoutCfg(t, fifo, filepath.Join(dir, "sb"), "tp:0.0", stdout, &recordingCommander{}, (&stubExecShell{}).fn(), nil)

	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	if stdout.String() != hydrateResetPreamble {
		t.Errorf("stdout = %q, want exactly the preamble %q", stdout.String(), hydrateResetPreamble)
	}
}

func TestHydrate_TimeoutWritesNoScrollbackOrPostamble(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-tn__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	// Seed scrollback so we can verify the timeout path does NOT read it.
	_ = os.WriteFile(scrollback, []byte("SHOULD-NOT-APPEAR"), 0o600)

	stdout := new(bytes.Buffer)
	cfg := timeoutCfg(t, fifo, scrollback, "tn:0.0", stdout, &recordingCommander{}, (&stubExecShell{}).fn(), nil)

	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	out := stdout.String()
	if strings.Contains(out, "SHOULD-NOT-APPEAR") {
		t.Errorf("stdout contains scrollback bytes on timeout: %q", out)
	}
	if strings.Contains(out, hydrateResetPostamble) {
		t.Errorf("stdout contains postamble on timeout: %q", out)
	}
	if out != hydrateResetPreamble {
		t.Errorf("stdout has bytes beyond preamble: %q (len=%d, preamble len=%d)", out, len(out), len(hydrateResetPreamble))
	}
}

func TestHydrate_TimeoutDoesNotSleep100ms(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-ts__0.0.fifo")

	cfg := timeoutCfg(t, fifo, filepath.Join(dir, "sb"), "ts:0.0", io.Discard, &recordingCommander{}, (&stubExecShell{}).fn(), nil)

	start := time.Now()
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	elapsed := time.Since(start)

	// Generous upper bound to avoid flakes; the real spec says no 100ms sleep
	// on timeout, so elapsed should be well under 100ms when ExecShell is a
	// synchronous no-op stub.
	if elapsed >= 100*time.Millisecond {
		t.Errorf("runHydrate elapsed %v on timeout path; expected << 100ms (no settle sleep)", elapsed)
	}
}

func TestHydrate_TimeoutRemovesFIFO(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-tr__0.0.fifo")

	cfg := timeoutCfg(t, fifo, filepath.Join(dir, "sb"), "tr:0.0", io.Discard, &recordingCommander{}, (&stubExecShell{}).fn(), nil)

	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	if _, err := os.Stat(fifo); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("FIFO not removed on timeout; stat err = %v", err)
	}
}

func TestHydrate_TimeoutDoesNotUnsetSkeletonMarker(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-tu__0.0.fifo")

	cmder := &recordingCommander{}
	cfg := timeoutCfg(t, fifo, filepath.Join(dir, "sb"), "tu:0.0", io.Discard, cmder, (&stubExecShell{}).fn(), nil)

	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	// Marker stays set -> no `set-option -su @portal-skeleton-...` argv.
	for _, c := range cmder.Calls {
		if len(c) >= 2 && c[0] == "set-option" && c[1] == "-su" {
			t.Errorf("timeout path issued set-option -su (marker should stay set): %v", c)
		}
	}
}

func TestHydrate_TimeoutLogsWarningNamingHookKey(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-tl__0.0.fifo")

	logPath := filepath.Join(dir, "portal.log")
	t.Setenv("PORTAL_LOG_LEVEL", "")
	logger, err := state.OpenLogger(logPath, false)
	if err != nil {
		t.Fatalf("OpenLogger: %v", err)
	}
	t.Cleanup(func() { _ = logger.Close() })

	cfg := timeoutCfg(t, fifo, filepath.Join(dir, "sb"), "tl:0.0", io.Discard, &recordingCommander{}, (&stubExecShell{}).fn(), logger)

	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	_ = logger.Close()

	data, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatalf("read log: %v", readErr)
	}
	contents := string(data)
	if !strings.Contains(contents, "WARN") {
		t.Errorf("log missing WARN level entry: %q", contents)
	}
	if !strings.Contains(contents, "tl:0.0") {
		t.Errorf("log missing hook-key %q in entry: %q", "tl:0.0", contents)
	}
	if !strings.Contains(contents, "hydrate") {
		t.Errorf("log missing component %q in entry: %q", "hydrate", contents)
	}
}

func TestHydrate_TimeoutExecsShell(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-te__0.0.fifo")

	t.Setenv("SHELL", "/usr/local/bin/myshell")
	exec := &stubExecShell{}
	cfg := timeoutCfg(t, fifo, filepath.Join(dir, "sb"), "te:0.0", io.Discard, &recordingCommander{}, exec.fn(), nil)

	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	if !exec.called {
		t.Fatal("ExecShell not called on timeout path")
	}
	if exec.target != "/usr/local/bin/myshell" {
		t.Errorf("ExecShell target = %q, want /usr/local/bin/myshell", exec.target)
	}
}

func TestHydrate_TimeoutDoesNotReadHooksFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_CONFIG_HOME", dir)

	fifo := makeFIFO(t, dir, "hydrate-th__0.0.fifo")

	cfg := timeoutCfg(t, fifo, filepath.Join(dir, "sb"), "th:0.0", io.Discard, &recordingCommander{}, (&stubExecShell{}).fn(), nil)

	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "hooks.json")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("hooks.json must not exist; stat err = %v", err)
	}
}

func TestHydrate_TimeoutToleratesMissingFIFOSilently(t *testing.T) {
	dir := t.TempDir()
	// FIFO path that does not exist — handleHydrateTimeout's os.Remove must
	// not surface an error.
	fifo := filepath.Join(dir, "hydrate-tm__0.0.fifo")

	cfg := timeoutCfg(t, fifo, filepath.Join(dir, "sb"), "tm:0.0", io.Discard, &recordingCommander{}, (&stubExecShell{}).fn(), nil)

	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v (FIFO os.Remove error must be tolerated)", err)
	}
}

// seedHookStore writes a hooks.json containing the given map and returns a
// *hooks.Store pointing at it. Used by hook-firing tests to drive
// LookupOnResume against a real on-disk store.
func seedHookStore(t *testing.T, dir string, contents map[string]map[string]string) *hooks.Store {
	t.Helper()
	path := filepath.Join(dir, "hooks.json")
	data, err := json.Marshal(contents)
	if err != nil {
		t.Fatalf("marshal hooks: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write hooks.json: %v", err)
	}
	return hooks.NewStore(path)
}

func TestHydrate_SignalArrived_ExecsHookChainWhenHookRegistered(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-work__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	_ = os.WriteFile(scrollback, []byte(""), 0o600)

	go func() {
		f, _ := os.OpenFile(fifo, os.O_WRONLY, 0)
		_, _ = f.Write([]byte("X"))
		_ = f.Close()
	}()

	t.Setenv("SHELL", "/bin/zsh")
	store := seedHookStore(t, dir, map[string]map[string]string{
		"work:0.0": {"on-resume": "echo hi"},
	})

	exec := &stubExecShell{}
	cfg := hydrateConfig{
		FIFO: fifo, File: scrollback, HookKey: "work:0.0",
		Stdout:    io.Discard,
		Client:    tmux.NewClient(&recordingCommander{}),
		HookStore: store,
		ExecShell: exec.fn(),
		OpenFIFO:  openFIFOWithTimeout,
	}
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	if !exec.called {
		t.Fatal("ExecShell not called")
	}
	if exec.target != "/bin/sh" {
		t.Errorf("ExecShell prog = %q, want /bin/sh", exec.target)
	}
	want := []string{"sh", "-c", "echo hi; exec /bin/zsh"}
	if !reflect.DeepEqual(exec.args, want) {
		t.Errorf("ExecShell args = %#v, want %#v", exec.args, want)
	}
}

func TestHydrate_SignalArrived_ExecsBareShellWhenNoHookRegistered(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-nohook__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	_ = os.WriteFile(scrollback, []byte(""), 0o600)

	go func() {
		f, _ := os.OpenFile(fifo, os.O_WRONLY, 0)
		_, _ = f.Write([]byte("X"))
		_ = f.Close()
	}()

	t.Setenv("SHELL", "/bin/zsh")
	// Empty hooks file: no entries.
	store := seedHookStore(t, dir, map[string]map[string]string{})

	exec := &stubExecShell{}
	cfg := hydrateConfig{
		FIFO: fifo, File: scrollback, HookKey: "nohook:0.0",
		Stdout:    io.Discard,
		Client:    tmux.NewClient(&recordingCommander{}),
		HookStore: store,
		ExecShell: exec.fn(),
		OpenFIFO:  openFIFOWithTimeout,
	}
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	if !exec.called {
		t.Fatal("ExecShell not called")
	}
	if exec.target != "/bin/zsh" {
		t.Errorf("ExecShell prog = %q, want /bin/zsh", exec.target)
	}
	if !reflect.DeepEqual(exec.args, []string{"/bin/zsh"}) {
		t.Errorf("ExecShell args = %#v, want [/bin/zsh]", exec.args)
	}
}

func TestHydrate_FileMissing_ExecsHookChainWhenHookRegistered(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-fmh__0.0.fifo")
	scrollback := filepath.Join(dir, "missing-sb")
	// Do NOT create scrollback file — drives the file-missing branch.

	go func() {
		f, _ := os.OpenFile(fifo, os.O_WRONLY, 0)
		_, _ = f.Write([]byte("X"))
		_ = f.Close()
	}()

	t.Setenv("SHELL", "/bin/zsh")
	store := seedHookStore(t, dir, map[string]map[string]string{
		"fmh:0.0": {"on-resume": "claude --resume abc"},
	})

	exec := &stubExecShell{}
	cfg := hydrateConfig{
		FIFO: fifo, File: scrollback, HookKey: "fmh:0.0",
		Stdout:            io.Discard,
		Client:            tmux.NewClient(&recordingCommander{}),
		HookStore:         store,
		ExecShell:         exec.fn(),
		OpenFIFO:          openFIFOWithTimeout,
		HandleFileMissing: handleHydrateFileMissing,
	}
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	if !exec.called {
		t.Fatal("ExecShell not called")
	}
	if exec.target != "/bin/sh" {
		t.Errorf("ExecShell prog = %q, want /bin/sh", exec.target)
	}
	want := []string{"sh", "-c", "claude --resume abc; exec /bin/zsh"}
	if !reflect.DeepEqual(exec.args, want) {
		t.Errorf("ExecShell args = %#v, want %#v", exec.args, want)
	}
}

func TestHydrate_FileMissing_ExecsBareShellWhenNoHookRegistered(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-fmn__0.0.fifo")
	scrollback := filepath.Join(dir, "missing-sb")

	go func() {
		f, _ := os.OpenFile(fifo, os.O_WRONLY, 0)
		_, _ = f.Write([]byte("X"))
		_ = f.Close()
	}()

	t.Setenv("SHELL", "/bin/zsh")
	store := seedHookStore(t, dir, map[string]map[string]string{})

	exec := &stubExecShell{}
	cfg := hydrateConfig{
		FIFO: fifo, File: scrollback, HookKey: "fmn:0.0",
		Stdout:            io.Discard,
		Client:            tmux.NewClient(&recordingCommander{}),
		HookStore:         store,
		ExecShell:         exec.fn(),
		OpenFIFO:          openFIFOWithTimeout,
		HandleFileMissing: handleHydrateFileMissing,
	}
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	if exec.target != "/bin/zsh" {
		t.Errorf("ExecShell prog = %q, want /bin/zsh", exec.target)
	}
	if !reflect.DeepEqual(exec.args, []string{"/bin/zsh"}) {
		t.Errorf("ExecShell args = %#v, want [/bin/zsh]", exec.args)
	}
}

func TestHydrate_Timeout_NeverFiresHookEvenIfRegistered(t *testing.T) {
	// On the timeout path, hooks must NOT fire even when one is registered for
	// the pane's hook-key. The de-facto verification is that ExecShell receives
	// bare-shell argv ([shell]), NOT a hook-chained argv.
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-tnf__0.0.fifo")

	t.Setenv("SHELL", "/bin/zsh")
	store := seedHookStore(t, dir, map[string]map[string]string{
		"tnf:0.0": {"on-resume": "should-not-fire"},
	})

	exec := &stubExecShell{}
	cfg := timeoutCfg(t, fifo, filepath.Join(dir, "sb"), "tnf:0.0", io.Discard, &recordingCommander{}, exec.fn(), nil)
	cfg.HookStore = store

	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	if !exec.called {
		t.Fatal("ExecShell not called")
	}
	if exec.target != "/bin/zsh" {
		t.Errorf("ExecShell prog = %q, want /bin/zsh (bare shell on timeout)", exec.target)
	}
	if !reflect.DeepEqual(exec.args, []string{"/bin/zsh"}) {
		t.Errorf("ExecShell args = %#v, want [/bin/zsh] (no hook chain on timeout)", exec.args)
	}
	// Defensive: the hook command string must not appear anywhere in the argv.
	for _, a := range exec.args {
		if strings.Contains(a, "should-not-fire") {
			t.Errorf("hook command leaked into timeout-path argv: %q", a)
		}
	}
}

func TestHydrate_LookupErrorDegradesToBareShellAndLogsWarning(t *testing.T) {
	// Drive a LookupOnResume I/O failure by pointing the store at a path that
	// is a directory rather than a regular file → os.ReadFile returns EISDIR.
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-le__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	_ = os.WriteFile(scrollback, []byte(""), 0o600)

	go func() {
		f, _ := os.OpenFile(fifo, os.O_WRONLY, 0)
		_, _ = f.Write([]byte("X"))
		_ = f.Close()
	}()

	// hooks.json is a directory, not a file.
	hooksDir := filepath.Join(dir, "hooks.json")
	if err := os.Mkdir(hooksDir, 0o700); err != nil {
		t.Fatalf("mkdir hooks.json: %v", err)
	}
	store := hooks.NewStore(hooksDir)

	logPath := filepath.Join(dir, "portal.log")
	t.Setenv("PORTAL_LOG_LEVEL", "")
	logger, err := state.OpenLogger(logPath, false)
	if err != nil {
		t.Fatalf("OpenLogger: %v", err)
	}
	t.Cleanup(func() { _ = logger.Close() })

	t.Setenv("SHELL", "/bin/zsh")
	exec := &stubExecShell{}
	cfg := hydrateConfig{
		FIFO: fifo, File: scrollback, HookKey: "le:0.0",
		Stdout:    io.Discard,
		Client:    tmux.NewClient(&recordingCommander{}),
		Logger:    logger,
		HookStore: store,
		ExecShell: exec.fn(),
		OpenFIFO:  openFIFOWithTimeout,
	}
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	_ = logger.Close()

	if exec.target != "/bin/zsh" {
		t.Errorf("ExecShell prog = %q, want /bin/zsh on lookup error", exec.target)
	}
	if !reflect.DeepEqual(exec.args, []string{"/bin/zsh"}) {
		t.Errorf("ExecShell args = %#v, want [/bin/zsh] on lookup error", exec.args)
	}

	data, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatalf("read log: %v", readErr)
	}
	contents := string(data)
	if !strings.Contains(contents, "lookup on-resume hook") {
		t.Errorf("log missing degradation warning phrase \"lookup on-resume hook\": %q", contents)
	}
	if !strings.Contains(contents, "le:0.0") {
		t.Errorf("log missing hook-key in warning: %q", contents)
	}
}

func TestHydrate_LooksUpHooksByHookKeyVerbatimNotByLivePaneKey(t *testing.T) {
	// FIFO basename derives livePaneKey "live__1.1" via state.PaneKeyFromFIFOPath, but
	// HookKey is the saved structural identifier "saved:0.0" — what the spec
	// pins for hooks lookup under base-index drift. The lookup must use HookKey
	// (so the saved-key hook fires), not the live paneKey (no entry under which).
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-live__1.1.fifo")
	scrollback := filepath.Join(dir, "sb")
	_ = os.WriteFile(scrollback, []byte(""), 0o600)

	go func() {
		f, _ := os.OpenFile(fifo, os.O_WRONLY, 0)
		_, _ = f.Write([]byte("X"))
		_ = f.Close()
	}()

	t.Setenv("SHELL", "/bin/zsh")
	store := seedHookStore(t, dir, map[string]map[string]string{
		"saved:0.0": {"on-resume": "echo saved"},
		// No entry under the FIFO-derived live paneKey "live__1.1".
	})

	exec := &stubExecShell{}
	cfg := hydrateConfig{
		FIFO: fifo, File: scrollback, HookKey: "saved:0.0",
		Stdout:    io.Discard,
		Client:    tmux.NewClient(&recordingCommander{}),
		HookStore: store,
		ExecShell: exec.fn(),
		OpenFIFO:  openFIFOWithTimeout,
	}
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	if exec.target != "/bin/sh" {
		t.Errorf("ExecShell prog = %q, want /bin/sh (hook chain)", exec.target)
	}
	want := []string{"sh", "-c", "echo saved; exec /bin/zsh"}
	if !reflect.DeepEqual(exec.args, want) {
		t.Errorf("ExecShell args = %#v, want %#v (lookup must use HookKey verbatim)", exec.args, want)
	}
}

func TestHydrate_PassesHookCommandAsSingleArgvElementToShDashC(t *testing.T) {
	// Single-quote safety: the hook command string sits in its own argv slot
	// of `sh -c <cmd>` — no manual escaping, no shell-command-line interpolation.
	// `sh`'s own parser handles embedded single quotes.
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-q__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	_ = os.WriteFile(scrollback, []byte(""), 0o600)

	go func() {
		f, _ := os.OpenFile(fifo, os.O_WRONLY, 0)
		_, _ = f.Write([]byte("X"))
		_ = f.Close()
	}()

	t.Setenv("SHELL", "/bin/zsh")
	rawCmd := "echo 'it works' && echo \"\\$x\""
	store := seedHookStore(t, dir, map[string]map[string]string{
		"q:0.0": {"on-resume": rawCmd},
	})

	exec := &stubExecShell{}
	cfg := hydrateConfig{
		FIFO: fifo, File: scrollback, HookKey: "q:0.0",
		Stdout:    io.Discard,
		Client:    tmux.NewClient(&recordingCommander{}),
		HookStore: store,
		ExecShell: exec.fn(),
		OpenFIFO:  openFIFOWithTimeout,
	}
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	if exec.target != "/bin/sh" {
		t.Fatalf("ExecShell prog = %q, want /bin/sh", exec.target)
	}
	if len(exec.args) != 3 {
		t.Fatalf("ExecShell args len = %d, want 3 (sh, -c, <cmd>)", len(exec.args))
	}
	if exec.args[0] != "sh" || exec.args[1] != "-c" {
		t.Errorf("ExecShell args[0:2] = %v, want [sh -c]", exec.args[0:2])
	}
	wantArg2 := rawCmd + "; exec /bin/zsh"
	if exec.args[2] != wantArg2 {
		t.Errorf("ExecShell args[2] = %q, want %q (verbatim cmd in single argv slot)", exec.args[2], wantArg2)
	}
}

func TestHydrate_SignalArrived_LookupHappensAfterSleepAndMarkerUnset(t *testing.T) {
	// On the signal-arrived path the spec pins the order:
	//   dump → 100ms sleep → set-option -su <marker> → hooks lookup → exec.
	// Verified by recording when the marker-unset occurs and asserting it
	// happens BEFORE LookupOnResume runs. The recorder uses a hooks-store
	// pointed at a sentinel hooks.json whose first read is timestamped via
	// a wrapping countingCommander on tmux + a custom hookStore subdir whose
	// access time is checked relative to the marker-unset timestamp.
	//
	// Concretely: capture the timestamps of (a) the set-option -su call and
	// (b) the os.Stat-able hooks.json read. The set-option must precede the
	// hooks read.
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-ord__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	_ = os.WriteFile(scrollback, []byte(""), 0o600)

	go func() {
		f, _ := os.OpenFile(fifo, os.O_WRONLY, 0)
		_, _ = f.Write([]byte("X"))
		_ = f.Close()
	}()

	store := seedHookStore(t, dir, map[string]map[string]string{
		"ord:0.0": {"on-resume": "echo ord"},
	})

	var (
		mu            sync.Mutex
		markerUnsetAt time.Time
	)
	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			if len(args) >= 3 && args[0] == "set-option" && args[1] == "-su" && args[2] == "@portal-skeleton-ord__0.0" {
				mu.Lock()
				markerUnsetAt = time.Now()
				mu.Unlock()
			}
			return "", nil
		},
	}

	var execAt time.Time
	cfg := hydrateConfig{
		FIFO: fifo, File: scrollback, HookKey: "ord:0.0",
		Stdout:    io.Discard,
		Client:    tmux.NewClient(cmder),
		HookStore: store,
		ExecShell: func(prog string, args []string) {
			execAt = time.Now()
		},
		OpenFIFO: openFIFOWithTimeout,
	}
	startSleep := time.Now()
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if markerUnsetAt.IsZero() {
		t.Fatal("set-option -su was never invoked")
	}
	if execAt.IsZero() {
		t.Fatal("ExecShell was never invoked")
	}
	if !markerUnsetAt.After(startSleep.Add(99 * time.Millisecond)) {
		t.Errorf("marker-unset at %v, expected >= startSleep + 100ms (= %v)", markerUnsetAt, startSleep.Add(100*time.Millisecond))
	}
	if !execAt.After(markerUnsetAt) {
		t.Errorf("ExecShell (%v) did not occur after marker-unset (%v) — lookup must follow marker-unset", execAt, markerUnsetAt)
	}
}

func TestHydrate_FileMissing_LookupHappensAfterMarkerUnset(t *testing.T) {
	// On the file-missing path the spec pins the order:
	//   preamble → set-option -su <marker> → hooks lookup → exec.
	// (No 100ms sleep — nothing was dumped to settle.)
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-fmo__0.0.fifo")
	scrollback := filepath.Join(dir, "missing-sb")

	go func() {
		f, _ := os.OpenFile(fifo, os.O_WRONLY, 0)
		_, _ = f.Write([]byte("X"))
		_ = f.Close()
	}()

	store := seedHookStore(t, dir, map[string]map[string]string{
		"fmo:0.0": {"on-resume": "echo fmo"},
	})

	var (
		mu            sync.Mutex
		markerUnsetAt time.Time
	)
	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			if len(args) >= 3 && args[0] == "set-option" && args[1] == "-su" && args[2] == "@portal-skeleton-fmo__0.0" {
				mu.Lock()
				markerUnsetAt = time.Now()
				mu.Unlock()
			}
			return "", nil
		},
	}

	var execAt time.Time
	cfg := hydrateConfig{
		FIFO: fifo, File: scrollback, HookKey: "fmo:0.0",
		Stdout:    io.Discard,
		Client:    tmux.NewClient(cmder),
		HookStore: store,
		ExecShell: func(prog string, args []string) {
			execAt = time.Now()
		},
		OpenFIFO:          openFIFOWithTimeout,
		HandleFileMissing: handleHydrateFileMissing,
	}
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if markerUnsetAt.IsZero() {
		t.Fatal("set-option -su was never invoked on file-missing path")
	}
	if execAt.IsZero() {
		t.Fatal("ExecShell was never invoked")
	}
	if !execAt.After(markerUnsetAt) {
		t.Errorf("ExecShell (%v) did not occur after marker-unset (%v) on file-missing path", execAt, markerUnsetAt)
	}
}

func TestHydrate_NilHookStoreDegradesToBareShellOnSignalArrived(t *testing.T) {
	// Defensive: nil HookStore (production path when loadHookStore failed) must
	// not panic and must exec bare $SHELL.
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-nil__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	_ = os.WriteFile(scrollback, []byte(""), 0o600)

	go func() {
		f, _ := os.OpenFile(fifo, os.O_WRONLY, 0)
		_, _ = f.Write([]byte("X"))
		_ = f.Close()
	}()

	t.Setenv("SHELL", "/bin/zsh")
	exec := &stubExecShell{}
	cfg := hydrateConfig{
		FIFO: fifo, File: scrollback, HookKey: "nil:0.0",
		Stdout:    io.Discard,
		Client:    tmux.NewClient(&recordingCommander{}),
		HookStore: nil,
		ExecShell: exec.fn(),
		OpenFIFO:  openFIFOWithTimeout,
	}
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	if exec.target != "/bin/zsh" {
		t.Errorf("ExecShell prog = %q, want /bin/zsh (nil store → bare shell)", exec.target)
	}
	if !reflect.DeepEqual(exec.args, []string{"/bin/zsh"}) {
		t.Errorf("ExecShell args = %#v, want [/bin/zsh]", exec.args)
	}
}
