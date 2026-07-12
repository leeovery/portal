package spawn

import (
	"errors"
	"slices"
	"testing"
)

// recordingAdapter is a local spawn.Adapter double: it records a defensive copy
// of every OpenWindow argv (in call order) and replays a scripted Result per
// call, defaulting to Success once the script is exhausted. It lives in-package
// (not spawntest) so this internal test avoids the spawntest -> spawn import
// cycle while still asserting on the exact composed argv.
type recordingAdapter struct {
	calls   [][]string
	results []Result
}

func (r *recordingAdapter) OpenWindow(command []string) Result {
	r.calls = append(r.calls, slices.Clone(command))
	i := len(r.calls) - 1
	if i < len(r.results) {
		return r.results[i]
	}
	return Success("")
}

func TestSpawnWindows(t *testing.T) {
	const path = "/opt/homebrew/bin:/usr/bin"
	const exePath = "/abs/portal"

	t.Run("it resolves the executable once and composes an attach argv per session in list order", func(t *testing.T) {
		var exeCalls int
		exe := func() (string, error) { exeCalls++; return exePath, nil }
		adapter := &recordingAdapter{}
		sessions := []string{"s1", "s2", "s3"}

		outcomes, err := SpawnWindows(adapter, sessions, exe, mapGetenv(map[string]string{"PATH": path}))
		if err != nil {
			t.Fatalf("SpawnWindows error = %v, want nil", err)
		}
		if exeCalls != 1 {
			t.Errorf("executable resolved %d times, want exactly 1", exeCalls)
		}
		if len(adapter.calls) != len(sessions) {
			t.Fatalf("OpenWindow called %d times, want %d", len(adapter.calls), len(sessions))
		}
		for i, session := range sessions {
			want := composeAttachArgv(exePath, path, session)
			if !slices.Equal(adapter.calls[i], want) {
				t.Errorf("OpenWindow[%d] argv = %#v, want %#v", i, adapter.calls[i], want)
			}
		}
		if len(outcomes) != len(sessions) {
			t.Fatalf("outcomes len = %d, want %d", len(outcomes), len(sessions))
		}
		for i, session := range sessions {
			if outcomes[i].Session != session {
				t.Errorf("outcomes[%d].Session = %q, want %q", i, outcomes[i].Session, session)
			}
			if !outcomes[i].Result.OK() {
				t.Errorf("outcomes[%d] not OK, want success", i)
			}
		}
	})

	t.Run("it stops on the first non-success and returns the outcomes collected so far with the failed last", func(t *testing.T) {
		exe := func() (string, error) { return exePath, nil }
		adapter := &recordingAdapter{
			results: []Result{Success("ok"), SpawnFailed("osascript: -1743")},
		}
		sessions := []string{"s1", "s2", "s3"}

		outcomes, err := SpawnWindows(adapter, sessions, exe, mapGetenv(map[string]string{"PATH": path}))
		if err != nil {
			t.Fatalf("SpawnWindows error = %v, want nil (failure handling is via outcomes)", err)
		}
		if len(adapter.calls) != 2 {
			t.Errorf("OpenWindow called %d times, want 2 (stop after the failure)", len(adapter.calls))
		}
		if len(outcomes) != 2 {
			t.Fatalf("outcomes len = %d, want 2", len(outcomes))
		}
		if !outcomes[0].Result.OK() {
			t.Errorf("outcomes[0] not OK, want the first success")
		}
		last := outcomes[1]
		if last.Result.OK() {
			t.Errorf("outcomes[1] OK, want the failed one last")
		}
		if last.Session != "s2" {
			t.Errorf("failed outcome session = %q, want %q", last.Session, "s2")
		}
	})

	t.Run("it is a no-op that never resolves the executable for an empty session set", func(t *testing.T) {
		var exeCalls int
		exe := func() (string, error) { exeCalls++; return exePath, nil }
		adapter := &recordingAdapter{}

		outcomes, err := SpawnWindows(adapter, nil, exe, mapGetenv(map[string]string{"PATH": path}))
		if err != nil {
			t.Fatalf("SpawnWindows error = %v, want nil", err)
		}
		if outcomes != nil {
			t.Errorf("outcomes = %#v, want nil for an empty session set", outcomes)
		}
		if exeCalls != 0 {
			t.Errorf("executable resolved %d times, want 0 for an empty session set", exeCalls)
		}
		if len(adapter.calls) != 0 {
			t.Errorf("OpenWindow called %d times, want 0", len(adapter.calls))
		}
	})

	t.Run("it aborts before opening any window when the executable cannot be resolved", func(t *testing.T) {
		sentinel := errors.New("os.Executable: readlink /proc/self/exe: no such file")
		exe := func() (string, error) { return "", sentinel }
		adapter := &recordingAdapter{}

		outcomes, err := SpawnWindows(adapter, []string{"s1", "s2"}, exe, mapGetenv(map[string]string{"PATH": path}))
		if outcomes != nil {
			t.Errorf("outcomes = %#v, want nil on executable-resolution failure", outcomes)
		}
		if err == nil {
			t.Fatal("SpawnWindows error = nil, want a non-nil error")
		}
		if !errors.Is(err, sentinel) {
			t.Errorf("errors.Is(err, sentinel) = false, want true; err = %v", err)
		}
		if len(adapter.calls) != 0 {
			t.Errorf("OpenWindow called %d times, want 0 when the executable is unresolvable", len(adapter.calls))
		}
	})
}
