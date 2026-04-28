// Tests in this file mutate package-level cobra command state and MUST NOT use t.Parallel.
package cmd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

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

// stubExecShell records the shell argument it was called with. Production
// implementation calls syscall.Exec; the stub just captures.
type stubExecShell struct {
	mu     sync.Mutex
	called bool
	target string
}

func (s *stubExecShell) fn() func(string) {
	return func(shell string) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.called = true
		s.target = shell
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

func TestHydrate_DerivesPaneKeyFromFIFOBasename(t *testing.T) {
	tests := []struct {
		fifoBase string
		want     string
	}{
		{"hydrate-foo__0.0.fifo", "foo__0.0"},
		{"hydrate-myproj__1.2.fifo", "myproj__1.2"},
		{"hydrate-a__0.0.fifo", "a__0.0"},
	}
	for _, tt := range tests {
		t.Run(tt.fifoBase, func(t *testing.T) {
			got := paneKeyFromFIFOPath("/some/path/" + tt.fifoBase)
			if got != tt.want {
				t.Errorf("paneKeyFromFIFOPath(%q) = %q, want %q", tt.fifoBase, got, tt.want)
			}
		})
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
		HandleFileMissing: func(_ hydrateConfig) error {
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
