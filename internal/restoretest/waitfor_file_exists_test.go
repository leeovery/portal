package restoretest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeFataller captures Fatalf invocations without aborting the
// test process. It satisfies the unexported fataller interface so
// the timeout branch of waitForFileExists can be exercised in-process.
type fakeFataller struct {
	helperCalls int
	fatalMsg    string
	fatalCalled bool
	name        string
}

func (f *fakeFataller) Helper()      { f.helperCalls++ }
func (f *fakeFataller) Name() string { return f.name }

func (f *fakeFataller) Fatalf(format string, args ...any) {
	f.fatalCalled = true
	f.fatalMsg = fmt.Sprintf(format, args...)
}

// TestWaitForFileExists_FilePresentImmediately asserts the helper
// returns promptly when the target path already exists when polling
// begins — no sleep, no spurious failure.
func TestWaitForFileExists_FilePresentImmediately(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ready")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	start := time.Now()
	WaitForFileExists(t, path, 1*time.Second, 50*time.Millisecond)
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Errorf("WaitForFileExists with present file took %v; expected near-immediate return", elapsed)
	}
}

// TestWaitForFileExists_FileAppearsMidPoll asserts the helper
// observes a file that is created after polling has begun, before
// the budget elapses.
func TestWaitForFileExists_FileAppearsMidPoll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "delayed")

	done := make(chan struct{})
	go func() {
		time.Sleep(80 * time.Millisecond)
		_ = os.WriteFile(path, []byte("x"), 0o600)
		close(done)
	}()

	WaitForFileExists(t, path, 2*time.Second, 25*time.Millisecond)
	<-done
}

// TestWaitForFileExists_TimeoutFatals asserts the helper calls
// Fatalf when the file never appears within the budget, and that
// the diagnostic includes the absolute path + the elapsed budget
// (the task's edge case). Driven via a fake fataller so the
// real test process is not aborted by t.Fatalf.
func TestWaitForFileExists_TimeoutFatals(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "never-appears")
	budget := 50 * time.Millisecond

	fake := &fakeFataller{}
	waitForFileExists(fake, path, budget, 10*time.Millisecond)

	if !fake.fatalCalled {
		t.Fatalf("expected Fatalf to be called when file never appears within %v", budget)
	}
	if !strings.Contains(fake.fatalMsg, path) {
		t.Errorf("diagnostic %q missing absolute path %q", fake.fatalMsg, path)
	}
	if !strings.Contains(fake.fatalMsg, budget.String()) {
		t.Errorf("diagnostic %q missing budget %v", fake.fatalMsg, budget)
	}
	if fake.helperCalls == 0 {
		t.Errorf("expected Helper() to be called at least once")
	}
}
