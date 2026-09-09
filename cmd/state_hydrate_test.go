package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

func makeFIFO(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := state.CreateFIFO(path); err != nil {
		t.Fatalf("CreateFIFO: %v", err)
	}
	return path
}

// Errors are ignored: the read side is what is under test.
func signalFIFOAsync(t *testing.T, fifo string) {
	t.Helper()
	go func() {
		f, err := os.OpenFile(fifo, os.O_WRONLY, 0)
		if err != nil {
			return
		}
		_, _ = f.Write([]byte("X"))
		_ = f.Close()
	}()
}

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

func TestHydrate_BlocksOnFIFOUntilSignalArrives(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-foo__0.0.fifo")
	scrollback := filepath.Join(dir, "scrollback")
	if err := os.WriteFile(scrollback, []byte("HELLO"), 0o600); err != nil {
		t.Fatalf("seed scrollback: %v", err)
	}

	stdout := new(bytes.Buffer)
	exec := &stubExecShell{}
	cmder := commandertest.Quiet()

	// Inline rather than signalFIFOAsync: this test asserts elapsed-time
	// bounds, so it needs the embedded delay and a completion channel.
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

	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:      fifo,
		File:      scrollback,
		HookKey:   "foo:0.0",
		OpenFIFO:  openFIFOWithTimeout,
		Stdout:    stdout,
		Commander: cmder,
		ExecShell: exec.fn(),
	})
	start := time.Now()
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	elapsed := time.Since(start)

	<-signalSent
	if !exec.called {
		t.Fatal("ExecShell not called")
	}
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

	// Inline rather than signalFIFOAsync: the multi-byte payload is what makes a
	// second consumed byte visible.
	go func() {
		f, err := os.OpenFile(fifo, os.O_WRONLY, 0)
		if err != nil {
			return
		}
		_, _ = f.Write([]byte("ABCDE"))
		_ = f.Close()
	}()

	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:     fifo,
		File:     scrollback,
		HookKey:  "foo:0.0",
		OpenFIFO: openFIFOWithTimeout,
	})
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	if _, err := os.Stat(fifo); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("FIFO not removed; stat err = %v", err)
	}
}

func TestHydrate_RemovesFIFOAfterReading(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-bar__0.0.fifo")
	scrollback := filepath.Join(dir, "scrollback")
	_ = os.WriteFile(scrollback, []byte(""), 0o600)

	signalFIFOAsync(t, fifo)

	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:     fifo,
		File:     scrollback,
		HookKey:  "bar:0.0",
		OpenFIFO: openFIFOWithTimeout,
	})
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

	signalFIFOAsync(t, fifo)

	stdout := new(bytes.Buffer)
	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:     fifo,
		File:     scrollback,
		HookKey:  "x:0.0",
		OpenFIFO: openFIFOWithTimeout,
		Stdout:   stdout,
	})
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

	signalFIFOAsync(t, fifo)

	stdout := new(bytes.Buffer)
	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:     fifo,
		File:     scrollback,
		HookKey:  "y:0.0",
		OpenFIFO: openFIFOWithTimeout,
		Stdout:   stdout,
	})
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

	signalFIFOAsync(t, fifo)

	stdout := new(bytes.Buffer)
	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:     fifo,
		File:     scrollback,
		HookKey:  "z:0.0",
		OpenFIFO: openFIFOWithTimeout,
		Stdout:   stdout,
	})
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

	signalFIFOAsync(t, fifo)

	cmder := commandertest.Quiet()
	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:      fifo,
		File:      scrollback,
		HookKey:   "q:0.0",
		OpenFIFO:  openFIFOWithTimeout,
		Commander: cmder,
	})

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

	signalFIFOAsync(t, fifo)

	cmder := commandertest.Quiet()
	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:      fifo,
		File:      scrollback,
		HookKey:   "foo:0.0",
		OpenFIFO:  openFIFOWithTimeout,
		Commander: cmder,
	})
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	want := []string{"set-option", "-su", "@portal-skeleton-foo__0.0"}
	var found bool
	for _, c := range cmder.Calls() {
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
		t.Errorf("expected tmux call %v, got calls: %v", want, cmder.Calls())
	}
}

func TestHydrate_PreservesANSISequencesInDump(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-a__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	body := "\x1b[31mred\x1b[0m\x1b[1mbold\x1b[0m"
	_ = os.WriteFile(scrollback, []byte(body), 0o600)

	signalFIFOAsync(t, fifo)

	stdout := new(bytes.Buffer)
	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:     fifo,
		File:     scrollback,
		HookKey:  "a:0.0",
		OpenFIFO: openFIFOWithTimeout,
		Stdout:   stdout,
	})
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

	const size = 5 * 1024 * 1024
	body := make([]byte, size)
	for i := range body {
		body[i] = byte(i % 251)
	}
	if err := os.WriteFile(scrollback, body, 0o600); err != nil {
		t.Fatalf("seed scrollback: %v", err)
	}

	signalFIFOAsync(t, fifo)

	stdout := new(bytes.Buffer)
	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:     fifo,
		File:     scrollback,
		HookKey:  "big:0.0",
		OpenFIFO: openFIFOWithTimeout,
		Stdout:   stdout,
	})
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	out := stdout.Bytes()
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

	signalFIFOAsync(t, fifo)

	t.Setenv("SHELL", "/usr/local/bin/myshell")

	exec := &stubExecShell{}
	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:      fifo,
		File:      scrollback,
		HookKey:   "s:0.0",
		OpenFIFO:  openFIFOWithTimeout,
		ExecShell: exec.fn(),
		// The absent hook store is the subject: with none, the helper must
		// reach a bare shell.
		HookStore: nil,
	})
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

	signalFIFOAsync(t, fifo)

	t.Setenv("SHELL", "")

	exec := &stubExecShell{}
	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:      fifo,
		File:      scrollback,
		HookKey:   "d:0.0",
		OpenFIFO:  openFIFOWithTimeout,
		ExecShell: exec.fn(),
		// No hook store, so the bare shell whose default is under test is the
		// route taken.
		HookStore: nil,
	})
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	if exec.target != "/bin/sh" {
		t.Errorf("ExecShell target = %q, want /bin/sh", exec.target)
	}
}

func TestHydrate_DoesNotReadHooksFileInThisPhase(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_CONFIG_HOME", dir)

	fifo := makeFIFO(t, dir, "hydrate-h__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	_ = os.WriteFile(scrollback, []byte(""), 0o600)

	signalFIFOAsync(t, fifo)

	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:     fifo,
		File:     scrollback,
		HookKey:  "h:0.0",
		OpenFIFO: openFIFOWithTimeout,
		// The absent hook store is the subject: nothing may read hooks.json.
		HookStore: nil,
	})
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
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
	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:    fifo,
		File:    scrollback,
		HookKey: "t:0.0",
		OpenFIFO: func(_ string, _ time.Duration) (*os.File, error) {
			return nil, ErrHydrateTimeout
		},
		HandleTimeout: func(_ hydrateConfig) error {
			called = true
			return nil
		},
	})
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

	signalFIFOAsync(t, fifo)

	called := false
	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:     fifo,
		File:     scrollback,
		HookKey:  "m:0.0",
		OpenFIFO: openFIFOWithTimeout,
		HandleFileMissing: func(_ hydrateConfig, _ hydrateFileMissingContext) error {
			called = true
			return nil
		},
	})
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

	signalFIFOAsync(t, fifo)

	stdout := new(bytes.Buffer)
	exec := &stubExecShell{}
	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:              fifo,
		File:              scrollback,
		HookKey:           "fm:0.0",
		OpenFIFO:          openFIFOWithTimeout,
		Stdout:            stdout,
		ExecShell:         exec.fn(),
		HandleFileMissing: handleHydrateFileMissing,
	})
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

	signalFIFOAsync(t, fifo)

	stdout := new(bytes.Buffer)
	exec := &stubExecShell{}
	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:              fifo,
		File:              scrollback,
		HookKey:           "fp:0.0",
		OpenFIFO:          openFIFOWithTimeout,
		Stdout:            stdout,
		ExecShell:         exec.fn(),
		HandleFileMissing: handleHydrateFileMissing,
	})
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
	// The handler is invoked directly: injecting a mid-stream Read failure into
	// runHydrate's os.Open + io.Copy would need a seam that does not exist.
	dir := t.TempDir()
	fifo := filepath.Join(dir, "hydrate-mid__0.0.fifo")
	stdout := new(bytes.Buffer)
	// Stands in for what runHydrate had already written before the failure.
	stdout.WriteString(hydrateResetPreamble)
	stdout.WriteString("partial-bytes-already-on-stdout")

	cmder := commandertest.Quiet()
	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:      fifo,
		File:      filepath.Join(dir, "sb"),
		HookKey:   "mid:0.0",
		OpenFIFO:  unexpectedOpenFIFO(t),
		Stdout:    stdout,
		Commander: cmder,
	})

	start := time.Now()
	if err := handleHydrateFileMissing(cfg, hydrateFileMissingContext{Cause: errors.New("read: I/O error")}); err != nil {
		t.Fatalf("handleHydrateFileMissing: %v", err)
	}
	elapsed := time.Since(start)

	if n := strings.Count(stdout.String(), hydrateResetPreamble); n != 1 {
		t.Errorf("preamble count = %d, want 1 (handler must not re-emit)", n)
	}
	if !strings.Contains(stdout.String(), "partial-bytes-already-on-stdout") {
		t.Errorf("partial bytes were rolled back; stdout = %q", stdout.String())
	}
	if elapsed >= 100*time.Millisecond {
		t.Errorf("handleHydrateFileMissing elapsed %v; expected << 100ms (no settle sleep)", elapsed)
	}
}

func TestHydrate_FileMissing_LogsENOENTDistinctly(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-le__0.0.fifo")
	scrollback := filepath.Join(dir, "missing-sb")

	signalFIFOAsync(t, fifo)

	logger, sink := newCaptureLoggerForComponent(t, "hydrate")

	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:              fifo,
		File:              scrollback,
		HookKey:           "le:0.0",
		OpenFIFO:          openFIFOWithTimeout,
		Logger:            logger,
		HandleFileMissing: handleHydrateFileMissing,
	})
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	contents := sink.Body()
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

	signalFIFOAsync(t, fifo)

	logger, sink := newCaptureLoggerForComponent(t, "hydrate")

	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:              fifo,
		File:              scrollback,
		HookKey:           "lp:0.0",
		OpenFIFO:          openFIFOWithTimeout,
		Logger:            logger,
		HandleFileMissing: handleHydrateFileMissing,
	})
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	contents := sink.Body()
	if !strings.Contains(contents, "permission denied") {
		t.Errorf("log missing distinct permission phrase \"permission denied\": %q", contents)
	}
}

func TestHydrate_FileMissing_LogsGenericIOError(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "hydrate-lg__0.0.fifo")
	stdout := new(bytes.Buffer)
	stdout.WriteString(hydrateResetPreamble)

	logger, sink := newCaptureLoggerForComponent(t, "hydrate")

	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:     fifo,
		File:     filepath.Join(dir, "sb"),
		HookKey:  "lg:0.0",
		OpenFIFO: unexpectedOpenFIFO(t),
		Stdout:   stdout,
		Logger:   logger,
	})
	genericErr := errors.New("synthetic mid-stream failure")
	if err := handleHydrateFileMissing(cfg, hydrateFileMissingContext{Cause: genericErr}); err != nil {
		t.Fatalf("handleHydrateFileMissing: %v", err)
	}

	contents := sink.Body()
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

	signalFIFOAsync(t, fifo)

	logger, sink := newCaptureLoggerForComponent(t, "hydrate")

	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:              fifo,
		File:              scrollback,
		HookKey:           "li:0.0",
		OpenFIFO:          openFIFOWithTimeout,
		Logger:            logger,
		HandleFileMissing: handleHydrateFileMissing,
	})
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	contents := sink.Body()
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

	signalFIFOAsync(t, fifo)

	cmder := commandertest.Quiet()
	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:              fifo,
		File:              scrollback,
		HookKey:           "fu:0.0",
		OpenFIFO:          openFIFOWithTimeout,
		Commander:         cmder,
		HandleFileMissing: handleHydrateFileMissing,
	})
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	want := []string{"set-option", "-su", "@portal-skeleton-fu__0.0"}
	var found bool
	for _, c := range cmder.Calls() {
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
		t.Errorf("expected tmux call %v, got calls: %v", want, cmder.Calls())
	}
}

func TestHydrate_FileMissing_SkipsSettleSleep(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-fs__0.0.fifo")
	scrollback := filepath.Join(dir, "missing-sb")

	signalFIFOAsync(t, fifo)

	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:              fifo,
		File:              scrollback,
		HookKey:           "fs:0.0",
		OpenFIFO:          openFIFOWithTimeout,
		HandleFileMissing: handleHydrateFileMissing,
	})

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

	signalFIFOAsync(t, fifo)

	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:              fifo,
		File:              scrollback,
		HookKey:           "fh:0.0",
		OpenFIFO:          openFIFOWithTimeout,
		HandleFileMissing: handleHydrateFileMissing,
		// The absent hook store is the subject: nothing may read hooks.json.
		HookStore: nil,
	})
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "hooks.json")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("hooks.json must not exist; stat err = %v", err)
	}
}

func TestHydrate_FileMissing_LeavesPartialBytesOnMidStreamFailure(t *testing.T) {
	// Stands in for what runHydrate had already written before the failure.
	dir := t.TempDir()
	fifo := filepath.Join(dir, "hydrate-mp__0.0.fifo")

	stdout := new(bytes.Buffer)
	stdout.WriteString(hydrateResetPreamble)
	const partial = "ABC partial data DEF"
	stdout.WriteString(partial)

	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:     fifo,
		File:     filepath.Join(dir, "sb"),
		HookKey:  "mp:0.0",
		OpenFIFO: unexpectedOpenFIFO(t),
		Stdout:   stdout,
	})
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

// Returns immediately so timeout-path tests do not wait out hydrateTimeout.
func instantTimeoutOpenFIFO(_ string, _ time.Duration) (*os.File, error) {
	return nil, ErrHydrateTimeout
}

// unexpectedOpenFIFO stands in for a case that drives a handler or the exec
// chain directly and opens no FIFO. Reaching it means the case took a route it
// was not written for, so it says so rather than blocking on a real open.
func unexpectedOpenFIFO(t *testing.T) func(path string, timeout time.Duration) (*os.File, error) {
	t.Helper()
	return func(path string, _ time.Duration) (*os.File, error) {
		t.Errorf("OpenFIFO called on a case that opens no FIFO: %s", path)
		return nil, ErrHydrateTimeout
	}
}

// unexpectedTimeout is what an unnamed HandleTimeout becomes: returning an error
// makes runHydrate propagate rather than recover, so a stray timeout surfaces
// instead of quietly completing. Its error names the wrong route and does not
// wrap ErrHydrateTimeout.
func unexpectedTimeout(t *testing.T) func(cfg hydrateConfig) error {
	t.Helper()
	return func(cfg hydrateConfig) error {
		return fmt.Errorf("unexpected timeout path for a case that does not exercise it: %s", cfg.FIFO)
	}
}

// unexpectedFileMissing is the file-missing counterpart of unexpectedTimeout,
// with the same reasoning.
func unexpectedFileMissing(t *testing.T) func(cfg hydrateConfig, ctx hydrateFileMissingContext) error {
	t.Helper()
	return func(cfg hydrateConfig, ctx hydrateFileMissingContext) error {
		return fmt.Errorf("unexpected file-missing path for a case that does not exercise it: %s: %w", cfg.File, ctx.Cause)
	}
}

// hydrateAbsentHandler names the one handler a case deliberately leaves nil, so
// the fall-through that handler guards is reachable through the builder. Its
// zero value names none: an unnamed handler becomes the loud stand-in below, so
// a nil handler is never reached by omission.
type hydrateAbsentHandler uint8

const (
	hydrateHandlersWired hydrateAbsentHandler = iota
	hydrateAbsentTimeout
	hydrateAbsentFileMissing
)

// hydrateCfgOpts names the parts a hydrate case varies. Stdout, Commander and
// ExecShell default when unset — a discarded stdout, a fresh recording
// commander, and a stub exec whose recordings nobody reads. Logger and HookStore
// do not: a nil Logger falls through to the package logger, and a nil HookStore
// is the bare-shell scenario rather than an omission.
//
// Neither handler defaults to its production counterpart. The replay, timeout
// and file-missing routes share an end state — preamble on stdout, skeleton
// marker unset, shell or hook exec — so wiring a handler a case never named
// would let a stray excursion satisfy assertions written for a different route.
// An unnamed handler therefore becomes a stand-in that returns an error, so the
// excursion surfaces instead of completing. A case whose subject IS a nil
// handler names it through AbsentHandler.
type hydrateCfgOpts struct {
	FIFO              string
	File              string
	HookKey           string
	OpenFIFO          func(path string, timeout time.Duration) (*os.File, error)
	Stdout            io.Writer
	Commander         tmux.Commander
	Logger            *slog.Logger
	HookStore         *hooks.Store
	ExecShell         func(prog string, args []string)
	HandleFileMissing func(cfg hydrateConfig, ctx hydrateFileMissingContext) error
	HandleTimeout     func(cfg hydrateConfig) error
	AbsentHandler     hydrateAbsentHandler
}

// hydrateCfg builds the config a hydrate case runs against: the case names the
// seams that decide its route — the OpenFIFO seam and whichever handler its
// route ends in — and the rest defaults.
func hydrateCfg(t *testing.T, opts hydrateCfgOpts) hydrateConfig {
	t.Helper()
	if opts.OpenFIFO == nil {
		t.Fatal("hydrateCfg: OpenFIFO decides which path the case takes and must be named")
	}
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Commander == nil {
		opts.Commander = commandertest.Quiet()
	}
	if opts.ExecShell == nil {
		opts.ExecShell = (&stubExecShell{}).fn()
	}
	// Assigned aside rather than back into opts, which clearAbsentHandler reads
	// for what the case itself named.
	fileMissing := opts.HandleFileMissing
	if fileMissing == nil {
		fileMissing = unexpectedFileMissing(t)
	}
	timeout := opts.HandleTimeout
	if timeout == nil {
		timeout = unexpectedTimeout(t)
	}
	return clearAbsentHandler(t, hydrateConfig{
		FIFO:              opts.FIFO,
		File:              opts.File,
		HookKey:           opts.HookKey,
		Stdout:            opts.Stdout,
		Client:            tmux.NewClient(opts.Commander),
		Logger:            opts.Logger,
		HookStore:         opts.HookStore,
		ExecShell:         opts.ExecShell,
		OpenFIFO:          opts.OpenFIFO,
		HandleFileMissing: fileMissing,
		HandleTimeout:     timeout,
	}, opts)
}

// clearAbsentHandler runs after the stand-in defaults so the named handler ends
// up nil rather than loud. Naming a handler and declaring it absent is a case
// contradicting itself, so it fails rather than picking one.
func clearAbsentHandler(t *testing.T, cfg hydrateConfig, opts hydrateCfgOpts) hydrateConfig {
	t.Helper()
	switch opts.AbsentHandler {
	case hydrateHandlersWired:
	case hydrateAbsentTimeout:
		if opts.HandleTimeout != nil {
			t.Fatal("hydrateCfg: a case cannot both name a timeout handler and declare it absent")
		}
		cfg.HandleTimeout = nil
	case hydrateAbsentFileMissing:
		if opts.HandleFileMissing != nil {
			t.Fatal("hydrateCfg: a case cannot both name a file-missing handler and declare it absent")
		}
		cfg.HandleFileMissing = nil
	default:
		t.Fatalf("hydrateCfg: unknown absent handler %d", opts.AbsentHandler)
	}
	return cfg
}

func TestHydrate_TimeoutWritesResetPreambleToStdout(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-tp__0.0.fifo")

	stdout := new(bytes.Buffer)
	cfg := hydrateCfg(t, hydrateCfgOpts{FIFO: fifo, File: filepath.Join(dir, "sb"), HookKey: "tp:0.0",
		OpenFIFO: instantTimeoutOpenFIFO, HandleTimeout: handleHydrateTimeout, Stdout: stdout})

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
	// Seeded so a timeout path that read it would surface the content.
	_ = os.WriteFile(scrollback, []byte("SHOULD-NOT-APPEAR"), 0o600)

	stdout := new(bytes.Buffer)
	cfg := hydrateCfg(t, hydrateCfgOpts{FIFO: fifo, File: scrollback, HookKey: "tn:0.0",
		OpenFIFO: instantTimeoutOpenFIFO, HandleTimeout: handleHydrateTimeout, Stdout: stdout})

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

func TestHydrate_Timeout_PreservesSettleSleepBeforeExec(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-ts__0.0.fifo")

	cfg := hydrateCfg(t, hydrateCfgOpts{FIFO: fifo, File: filepath.Join(dir, "sb"), HookKey: "ts:0.0",
		OpenFIFO: instantTimeoutOpenFIFO, HandleTimeout: handleHydrateTimeout})

	start := time.Now()
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	elapsed := time.Since(start)

	if elapsed < hydrateSettleSleep {
		t.Errorf("runHydrate elapsed %v on timeout path; expected >= %v (settle sleep preserved)", elapsed, hydrateSettleSleep)
	}
}

func TestHydrate_TimeoutRemovesFIFO(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-tr__0.0.fifo")

	cfg := hydrateCfg(t, hydrateCfgOpts{FIFO: fifo, File: filepath.Join(dir, "sb"), HookKey: "tr:0.0",
		OpenFIFO: instantTimeoutOpenFIFO, HandleTimeout: handleHydrateTimeout})

	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	if _, err := os.Stat(fifo); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("FIFO not removed on timeout; stat err = %v", err)
	}
}

func TestHydrate_TimeoutUnsetsSkeletonMarkerWithSetOptionSU(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-tu__0.0.fifo")

	cmder := commandertest.Quiet()
	cfg := hydrateCfg(t, hydrateCfgOpts{FIFO: fifo, File: filepath.Join(dir, "sb"), HookKey: "tu:0.0",
		OpenFIFO: instantTimeoutOpenFIFO, HandleTimeout: handleHydrateTimeout, Commander: cmder})

	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	// The paneKey derives from the FIFO basename: hydrate-tu__0.0.fifo → tu__0.0.
	want := []string{"set-option", "-su", "@portal-skeleton-tu__0.0"}
	matches := 0
	for _, c := range cmder.Calls() {
		if reflect.DeepEqual(c, want) {
			matches++
		}
	}
	if matches != 1 {
		t.Errorf("expected tmux call %v exactly once, got %d matches; calls: %v", want, matches, cmder.Calls())
	}
}

func TestHydrate_TimeoutLogsWarningNamingHookKey(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-tl__0.0.fifo")

	logger, sink := newCaptureLoggerForComponent(t, "hydrate")

	cfg := hydrateCfg(t, hydrateCfgOpts{FIFO: fifo, File: filepath.Join(dir, "sb"), HookKey: "tl:0.0",
		OpenFIFO: instantTimeoutOpenFIFO, HandleTimeout: handleHydrateTimeout, Logger: logger})

	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	contents := sink.Body()
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
	cfg := hydrateCfg(t, hydrateCfgOpts{FIFO: fifo, File: filepath.Join(dir, "sb"), HookKey: "te:0.0",
		OpenFIFO: instantTimeoutOpenFIFO, HandleTimeout: handleHydrateTimeout, ExecShell: exec.fn(),
		// No hook store: the timeout path must reach a bare $SHELL.
		HookStore: nil})

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

	cfg := hydrateCfg(t, hydrateCfgOpts{FIFO: fifo, File: filepath.Join(dir, "sb"), HookKey: "th:0.0",
		OpenFIFO: instantTimeoutOpenFIFO, HandleTimeout: handleHydrateTimeout,
		// The absent hook store is the subject: nothing may read hooks.json.
		HookStore: nil})

	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "hooks.json")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("hooks.json must not exist; stat err = %v", err)
	}
}

func TestHydrate_TimeoutToleratesMissingFIFOSilently(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "hydrate-tm__0.0.fifo")

	cfg := hydrateCfg(t, hydrateCfgOpts{FIFO: fifo, File: filepath.Join(dir, "sb"), HookKey: "tm:0.0",
		OpenFIFO: instantTimeoutOpenFIFO, HandleTimeout: handleHydrateTimeout})

	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v (FIFO os.Remove error must be tolerated)", err)
	}
}

func TestHydrate_TimeoutHandler_OrderingAndTimingInvariants(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "hydrate-ord__0.0.fifo")

	cmder := commandertest.Quiet()
	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:      fifo,
		HookKey:   "ord:0.0",
		OpenFIFO:  unexpectedOpenFIFO(t),
		Commander: cmder,
	})

	start := time.Now()
	if err := handleHydrateTimeout(cfg); err != nil {
		t.Fatalf("handleHydrateTimeout: %v (must tolerate missing FIFO)", err)
	}
	elapsed := time.Since(start)

	// The settle sleep belongs to runHydrate, not the handler.
	if elapsed >= hydrateSettleSleep {
		t.Errorf("handleHydrateTimeout elapsed %v; expected << %v (handler must not own settle sleep)", elapsed, hydrateSettleSleep)
	}

	if _, statErr := os.Stat(fifo); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("FIFO unexpectedly present after handler; stat err = %v", statErr)
	}

	// The paneKey derives from the FIFO basename: hydrate-ord__0.0.fifo → ord__0.0.
	want := []string{"set-option", "-su", "@portal-skeleton-ord__0.0"}
	matched := false
	for _, c := range cmder.Calls() {
		if reflect.DeepEqual(c, want) {
			matched = true
			break
		}
	}
	if !matched {
		t.Errorf("expected tmux call %v before handler returned; calls: %v", want, cmder.Calls())
	}
}

func TestHydrate_SignalArrived_ExecsHookChainWhenHookRegistered(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-work__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	_ = os.WriteFile(scrollback, []byte(""), 0o600)

	signalFIFOAsync(t, fifo)

	t.Setenv("SHELL", "/bin/zsh")
	store, _ := hookstest.StageStore(t, hookstest.Staging{Dir: dir, SidecarAbsent: true, Body: map[string]map[string]string{
		"work:0.0": {"on-resume": "echo hi"},
	}})

	exec := &stubExecShell{}
	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:      fifo,
		File:      scrollback,
		HookKey:   "work:0.0",
		OpenFIFO:  openFIFOWithTimeout,
		HookStore: store,
		ExecShell: exec.fn(),
	})
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

	signalFIFOAsync(t, fifo)

	t.Setenv("SHELL", "/bin/zsh")
	store, _ := hookstest.StageStore(t, hookstest.Staging{Dir: dir, SidecarAbsent: true, Body: map[string]map[string]string{}})

	exec := &stubExecShell{}
	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:      fifo,
		File:      scrollback,
		HookKey:   "nohook:0.0",
		OpenFIFO:  openFIFOWithTimeout,
		HookStore: store,
		ExecShell: exec.fn(),
	})
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

	signalFIFOAsync(t, fifo)

	t.Setenv("SHELL", "/bin/zsh")
	store, _ := hookstest.StageStore(t, hookstest.Staging{Dir: dir, SidecarAbsent: true, Body: map[string]map[string]string{
		"fmh:0.0": {"on-resume": "claude --resume abc"},
	}})

	exec := &stubExecShell{}
	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:              fifo,
		File:              scrollback,
		HookKey:           "fmh:0.0",
		OpenFIFO:          openFIFOWithTimeout,
		HookStore:         store,
		ExecShell:         exec.fn(),
		HandleFileMissing: handleHydrateFileMissing,
	})
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

	signalFIFOAsync(t, fifo)

	t.Setenv("SHELL", "/bin/zsh")
	store, _ := hookstest.StageStore(t, hookstest.Staging{Dir: dir, SidecarAbsent: true, Body: map[string]map[string]string{}})

	exec := &stubExecShell{}
	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:              fifo,
		File:              scrollback,
		HookKey:           "fmn:0.0",
		OpenFIFO:          openFIFOWithTimeout,
		HookStore:         store,
		ExecShell:         exec.fn(),
		HandleFileMissing: handleHydrateFileMissing,
	})
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

func TestHydrate_Timeout_FiresHookWhenRegistered(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-tfh__0.0.fifo")

	t.Setenv("SHELL", "/bin/zsh")
	store, _ := hookstest.StageStore(t, hookstest.Staging{Dir: dir, SidecarAbsent: true, Body: map[string]map[string]string{
		"tfh:0.0": {"on-resume": "echo hi"},
	}})

	exec := &stubExecShell{}
	cfg := hydrateCfg(t, hydrateCfgOpts{FIFO: fifo, File: filepath.Join(dir, "sb"), HookKey: "tfh:0.0",
		OpenFIFO: instantTimeoutOpenFIFO, HandleTimeout: handleHydrateTimeout, ExecShell: exec.fn(), HookStore: store})

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

func TestHydrate_Timeout_NoHookStore_ExecsBareShell(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-tnh__0.0.fifo")

	t.Setenv("SHELL", "/bin/zsh")

	exec := &stubExecShell{}
	cfg := hydrateCfg(t, hydrateCfgOpts{FIFO: fifo, File: filepath.Join(dir, "sb"), HookKey: "tnh:0.0",
		OpenFIFO: instantTimeoutOpenFIFO, HandleTimeout: handleHydrateTimeout, ExecShell: exec.fn(),
		// The absent hook store is the subject the name states.
		HookStore: nil})

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

func TestHydrate_Timeout_LookupNotFound_ExecsBareShell(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-tlnf__0.0.fifo")

	t.Setenv("SHELL", "/bin/zsh")
	store, _ := hookstest.StageStore(t, hookstest.Staging{Dir: dir, SidecarAbsent: true, Body: map[string]map[string]string{}})

	exec := &stubExecShell{}
	cfg := hydrateCfg(t, hydrateCfgOpts{FIFO: fifo, File: filepath.Join(dir, "sb"), HookKey: "tlnf:0.0",
		OpenFIFO: instantTimeoutOpenFIFO, HandleTimeout: handleHydrateTimeout, ExecShell: exec.fn(), HookStore: store})

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

func TestHydrate_Timeout_LookupError_ExecsBareShellAndLogsWarning(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-tle__0.0.fifo")

	// An unreadable hooks.json forces EISDIR out of the store's read.
	store, _ := hookstest.StageStore(t, hookstest.Staging{Dir: dir, SidecarAbsent: true, Unreadable: true})

	logger, sink := newCaptureLoggerForComponent(t, "hydrate")

	t.Setenv("SHELL", "/bin/zsh")
	exec := &stubExecShell{}
	cfg := hydrateCfg(t, hydrateCfgOpts{FIFO: fifo, File: filepath.Join(dir, "sb"), HookKey: "tle:0.0",
		OpenFIFO: instantTimeoutOpenFIFO, HandleTimeout: handleHydrateTimeout, Logger: logger, ExecShell: exec.fn(), HookStore: store})

	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	if exec.target != "/bin/zsh" {
		t.Errorf("ExecShell prog = %q, want /bin/zsh on lookup error", exec.target)
	}
	if !reflect.DeepEqual(exec.args, []string{"/bin/zsh"}) {
		t.Errorf("ExecShell args = %#v, want [/bin/zsh] on lookup error", exec.args)
	}

	contents := sink.Body()
	// Counted, not merely present: the timeout handler logs its own WARN too.
	got := strings.Count(contents, "lookup on-resume hook failed")
	if got != 1 {
		t.Errorf("log has %d %q lines, want exactly 1: %q", got, "lookup on-resume hook failed", contents)
	}
	if !strings.Contains(contents, "tle:0.0") {
		t.Errorf("log missing hook-key in lookup-error warning: %q", contents)
	}
}

func TestHydrate_LookupErrorDegradesToBareShellAndLogsWarning(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-le__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	_ = os.WriteFile(scrollback, []byte(""), 0o600)

	signalFIFOAsync(t, fifo)

	// An unreadable hooks.json forces EISDIR out of the store's read.
	store, _ := hookstest.StageStore(t, hookstest.Staging{Dir: dir, SidecarAbsent: true, Unreadable: true})

	logger, sink := newCaptureLoggerForComponent(t, "hydrate")

	t.Setenv("SHELL", "/bin/zsh")
	exec := &stubExecShell{}
	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:      fifo,
		File:      scrollback,
		HookKey:   "le:0.0",
		OpenFIFO:  openFIFOWithTimeout,
		Logger:    logger,
		HookStore: store,
		ExecShell: exec.fn(),
	})
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	if exec.target != "/bin/zsh" {
		t.Errorf("ExecShell prog = %q, want /bin/zsh on lookup error", exec.target)
	}
	if !reflect.DeepEqual(exec.args, []string{"/bin/zsh"}) {
		t.Errorf("ExecShell args = %#v, want [/bin/zsh] on lookup error", exec.args)
	}

	contents := sink.Body()
	if !strings.Contains(contents, "lookup on-resume hook") {
		t.Errorf("log missing degradation warning phrase \"lookup on-resume hook\": %q", contents)
	}
	if !strings.Contains(contents, "le:0.0") {
		t.Errorf("log missing hook-key in warning: %q", contents)
	}
}

func TestHydrate_LooksUpHooksByHookKeyVerbatimNotByLivePaneKey(t *testing.T) {
	// The FIFO basename yields the live pane key, but the lookup must use the
	// saved HookKey so a hook still fires under base-index drift.
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-live__1.1.fifo")
	scrollback := filepath.Join(dir, "sb")
	_ = os.WriteFile(scrollback, []byte(""), 0o600)

	signalFIFOAsync(t, fifo)

	t.Setenv("SHELL", "/bin/zsh")
	store, _ := hookstest.StageStore(t, hookstest.Staging{Dir: dir, SidecarAbsent: true, Body: map[string]map[string]string{
		"saved:0.0": {"on-resume": "echo saved"},
	}})

	exec := &stubExecShell{}
	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:      fifo,
		File:      scrollback,
		HookKey:   "saved:0.0",
		OpenFIFO:  openFIFOWithTimeout,
		HookStore: store,
		ExecShell: exec.fn(),
	})
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
	// The hook command occupies its own argv slot of `sh -c <cmd>`, so sh's own
	// parser handles embedded quotes — never escape it here.
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-q__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	_ = os.WriteFile(scrollback, []byte(""), 0o600)

	signalFIFOAsync(t, fifo)

	t.Setenv("SHELL", "/bin/zsh")
	rawCmd := "echo 'it works' && echo \"\\$x\""
	store, _ := hookstest.StageStore(t, hookstest.Staging{Dir: dir, SidecarAbsent: true, Body: map[string]map[string]string{
		"q:0.0": {"on-resume": rawCmd},
	}})

	exec := &stubExecShell{}
	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:      fifo,
		File:      scrollback,
		HookKey:   "q:0.0",
		OpenFIFO:  openFIFOWithTimeout,
		HookStore: store,
		ExecShell: exec.fn(),
	})
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
	// Order matters: the marker unset must precede the hooks lookup, so the
	// test timestamps the set-option call and the hooks.json read and compares.
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-ord__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	_ = os.WriteFile(scrollback, []byte(""), 0o600)

	signalFIFOAsync(t, fifo)

	store, _ := hookstest.StageStore(t, hookstest.Staging{Dir: dir, SidecarAbsent: true, Body: map[string]map[string]string{
		"ord:0.0": {"on-resume": "echo ord"},
	}})

	var (
		mu            sync.Mutex
		markerUnsetAt time.Time
	)
	cmder := commandertest.Quiet(
		commandertest.Returns("", "set-option", "-su", "@portal-skeleton-ord__0.0").
			Doing(func(_ []string) {
				mu.Lock()
				markerUnsetAt = time.Now()
				mu.Unlock()
			}),
	)

	var execAt time.Time
	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:      fifo,
		File:      scrollback,
		HookKey:   "ord:0.0",
		OpenFIFO:  openFIFOWithTimeout,
		Commander: cmder,
		HookStore: store,
		ExecShell: func(prog string, args []string) {
			execAt = time.Now()
		},
	})
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
	// Order matters: the marker unset must precede the hooks lookup. No settle
	// sleep here — nothing was dumped.
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-fmo__0.0.fifo")
	scrollback := filepath.Join(dir, "missing-sb")

	signalFIFOAsync(t, fifo)

	store, _ := hookstest.StageStore(t, hookstest.Staging{Dir: dir, SidecarAbsent: true, Body: map[string]map[string]string{
		"fmo:0.0": {"on-resume": "echo fmo"},
	}})

	var (
		mu            sync.Mutex
		markerUnsetAt time.Time
	)
	cmder := commandertest.Quiet(
		commandertest.Returns("", "set-option", "-su", "@portal-skeleton-fmo__0.0").
			Doing(func(_ []string) {
				mu.Lock()
				markerUnsetAt = time.Now()
				mu.Unlock()
			}),
	)

	var execAt time.Time
	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:      fifo,
		File:      scrollback,
		HookKey:   "fmo:0.0",
		OpenFIFO:  openFIFOWithTimeout,
		Commander: cmder,
		HookStore: store,
		ExecShell: func(prog string, args []string) {
			execAt = time.Now()
		},
		HandleFileMissing: handleHydrateFileMissing,
	})
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
	// A nil HookStore is the production shape when loadHookStore failed.
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-nil__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	_ = os.WriteFile(scrollback, []byte(""), 0o600)

	signalFIFOAsync(t, fifo)

	t.Setenv("SHELL", "/bin/zsh")
	exec := &stubExecShell{}
	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:      fifo,
		File:      scrollback,
		HookKey:   "nil:0.0",
		OpenFIFO:  openFIFOWithTimeout,
		HookStore: nil,
		ExecShell: exec.fn(),
	})
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

// The fs.* cases use a wrapped *os.PathError — the shape runHydrate passes
// through verbatim — and the generic case a bare error, so classification must
// key off the unwrapped sentinel rather than the error's string form.
func TestHydrate_FileMissing_ClassifiesCauseFromRawChain(t *testing.T) {
	cases := []struct {
		name   string
		cause  error
		phrase string
	}{
		{
			name:   "ENOENT",
			cause:  &os.PathError{Op: "open", Path: "/x/sb.bin", Err: syscall.ENOENT},
			phrase: "not found",
		},
		{
			name:   "permission",
			cause:  &os.PathError{Op: "open", Path: "/x/sb.bin", Err: syscall.EACCES},
			phrase: "permission denied",
		},
		{
			name:   "generic",
			cause:  errors.New("synthetic mid-stream failure"),
			phrase: "I/O error",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			switch tc.name {
			case "ENOENT":
				if !errors.Is(tc.cause, fs.ErrNotExist) {
					t.Fatalf("test setup: cause does not traverse to fs.ErrNotExist: %v", tc.cause)
				}
			case "permission":
				if !errors.Is(tc.cause, fs.ErrPermission) {
					t.Fatalf("test setup: cause does not traverse to fs.ErrPermission: %v", tc.cause)
				}
			}

			logger, sink := newCaptureLoggerForComponent(t, "hydrate")
			cfg := hydrateCfg(t, hydrateCfgOpts{
				FIFO:     "/x/hydrate-c__0.0.fifo",
				File:     "/x/sb.bin",
				HookKey:  "c:0.0",
				OpenFIFO: unexpectedOpenFIFO(t),
				Logger:   logger,
			})
			if err := handleHydrateFileMissing(cfg, hydrateFileMissingContext{Cause: tc.cause}); err != nil {
				t.Fatalf("handleHydrateFileMissing: %v", err)
			}
			body := sink.Body()
			if !strings.Contains(body, tc.phrase) {
				t.Errorf("log missing classification phrase %q for %s cause; body = %q", tc.phrase, tc.name, body)
			}
		})
	}
}

// The *os.PathError must reach the handler's Cause unwrapped: a pre-wrap with
// %s would break errors.Is traversal in the classification switch.
func TestHydrate_FileMissing_PassesRawCauseVerbatim(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-vc__0.0.fifo")
	scrollback := filepath.Join(dir, "missing-sb")

	signalFIFOAsync(t, fifo)

	var captured error
	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:     fifo,
		File:     scrollback,
		HookKey:  "vc:0.0",
		OpenFIFO: openFIFOWithTimeout,
		HandleFileMissing: func(_ hydrateConfig, ctx hydrateFileMissingContext) error {
			captured = ctx.Cause
			return nil
		},
	})
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	if captured == nil {
		t.Fatal("HandleFileMissing was not invoked; Cause not captured")
	}
	if !errors.Is(captured, fs.ErrNotExist) {
		t.Fatalf("Cause does not traverse to fs.ErrNotExist (pre-wrapped?): %v", captured)
	}
	if _, ok := errors.AsType[*os.PathError](captured); !ok {
		t.Fatalf("Cause does not carry an *os.PathError verbatim: %v", captured)
	}
}

func TestHydrate_FileMissing_PassesPermissionCauseVerbatim(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses 0o000 mode bits")
	}
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-vp__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	if err := os.WriteFile(scrollback, []byte("HIDDEN"), 0o600); err != nil {
		t.Fatalf("seed scrollback: %v", err)
	}
	if err := os.Chmod(scrollback, 0o000); err != nil {
		t.Fatalf("chmod 0: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(scrollback, 0o600) })

	signalFIFOAsync(t, fifo)

	var captured error
	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:     fifo,
		File:     scrollback,
		HookKey:  "vp:0.0",
		OpenFIFO: openFIFOWithTimeout,
		HandleFileMissing: func(_ hydrateConfig, ctx hydrateFileMissingContext) error {
			captured = ctx.Cause
			return nil
		},
	})
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	if captured == nil {
		t.Fatal("HandleFileMissing was not invoked; Cause not captured")
	}
	if !errors.Is(captured, fs.ErrPermission) {
		t.Fatalf("Cause does not traverse to fs.ErrPermission (pre-wrapped?): %v", captured)
	}
}

func TestHydrate_MidStreamCopyError_CarriesUnderlyingCauseToHandler(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-ms__0.0.fifo")
	// A directory is the deterministic open-succeeds-then-Read-fails shape: it
	// opens cleanly and io.Copy's first Read fails (EISDIR / "is a directory").
	scrollbackDir := filepath.Join(dir, "sb-as-dir")
	if err := os.Mkdir(scrollbackDir, 0o700); err != nil {
		t.Fatalf("mkdir scrollback dir: %v", err)
	}

	signalFIFOAsync(t, fifo)

	var captured error
	invoked := false
	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:     fifo,
		File:     scrollbackDir,
		HookKey:  "ms:0.0",
		OpenFIFO: openFIFOWithTimeout,
		HandleFileMissing: func(_ hydrateConfig, ctx hydrateFileMissingContext) error {
			invoked = true
			captured = ctx.Cause
			return nil
		},
	})
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	if !invoked {
		t.Fatal("HandleFileMissing not invoked on mid-stream read failure")
	}
	if captured == nil {
		t.Fatal("mid-stream failure carried a nil Cause to the handler")
	}
}
