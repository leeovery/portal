package spawn

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"
)

type manualClock struct{ t time.Time }

func (c *manualClock) now() time.Time         { return c.t }
func (c *manualClock) sleep(d time.Duration)  { c.t = c.t.Add(d) }
func (c *manualClock) elapsed() time.Duration { return c.t.Sub(time.Time{}) }

type delayingAck struct {
	now      func() time.Time
	delay    time.Duration
	revealAt map[string]map[string]time.Time
}

func newDelayingAck(now func() time.Time, delay time.Duration) *delayingAck {
	return &delayingAck{now: now, delay: delay, revealAt: map[string]map[string]time.Time{}}
}

func (d *delayingAck) Write(batch, token string) error {
	set := d.revealAt[batch]
	if set == nil {
		set = map[string]time.Time{}
		d.revealAt[batch] = set
	}
	set[token] = d.now().Add(d.delay)
	return nil
}

func (d *delayingAck) Collect(batch string) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	for token, revealAt := range d.revealAt[batch] {
		if !d.now().Before(revealAt) {
			out[token] = struct{}{}
		}
	}
	return out, nil
}

type erroringAck struct{ collectCalls int }

func (e *erroringAck) Write(batch, token string) error { return nil }

func (e *erroringAck) Collect(batch string) (map[string]struct{}, error) {
	e.collectCalls++
	return nil, errors.New("show-options: server not found")
}

type writingAdapter struct {
	calls   [][]string
	results []Result
	confirm []bool
	ack     AckWriter
}

func (a *writingAdapter) OpenWindow(command []string) Result {
	a.calls = append(a.calls, slices.Clone(command))
	i := len(a.calls) - 1

	result := Success("")
	if i < len(a.results) {
		result = a.results[i]
	}
	if result.OK() && a.ack != nil && a.confirmed(i) {
		if batch, token, ok := parseSpawnAckArgv(command); ok {
			_ = a.ack.Write(batch, token)
		}
	}
	return result
}

func (a *writingAdapter) confirmed(i int) bool {
	if i < len(a.confirm) {
		return a.confirm[i]
	}
	return true
}

func parseSpawnAckArgv(argv []string) (batch, token string, ok bool) {
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == "--ack" {
			return ParseSpawnAckFlag(argv[i+1])
		}
	}
	return "", "", false
}

func seqIDGen() func() (string, error) {
	var n int
	return func() (string, error) {
		n++
		return fmt.Sprintf("id%d", n), nil
	}
}

const testBurstExe = "/abs/portal"

func fixedEnv(env []string) WindowEnv {
	return func() ([]string, error) { return env, nil }
}

func TestBurster_Run(t *testing.T) {
	t.Run("it resolves the executable once and composes an ack-flagged open argv per surface in list order", func(t *testing.T) {
		clock := &manualClock{}
		var exeCalls int
		exe := func() (string, error) { exeCalls++; return testBurstExe, nil }
		ack := newDelayingAck(clock.now, 0)
		adapter := &writingAdapter{ack: ack}
		b := &Burster{
			Adapter: adapter, Ack: ack, Exe: exe,
			Env:     fixedEnv(testWindowEnv),
			NewID:   seqIDGen(),
			Timeout: 8 * time.Second, Poll: 75 * time.Millisecond,
			Now: clock.now, Sleep: clock.sleep,
		}
		surfaces := AttachSurfaces([]string{"s1", "s2", "s3"})

		batch, results, err := b.Run(context.Background(), surfaces, nil, nil)
		if err != nil {
			t.Fatalf("Run error = %v, want nil", err)
		}
		if exeCalls != 1 {
			t.Errorf("executable resolved %d times, want exactly 1", exeCalls)
		}
		if batch == "" {
			t.Fatal("batch id is empty, want a generated id")
		}
		if len(results) != len(surfaces) {
			t.Fatalf("results len = %d, want %d", len(results), len(surfaces))
		}
		if len(adapter.calls) != len(surfaces) {
			t.Fatalf("OpenWindow called %d times, want %d", len(adapter.calls), len(surfaces))
		}
		for i, surface := range surfaces {
			if results[i].Session != surface.Value {
				t.Errorf("results[%d].Session = %q, want %q", i, results[i].Session, surface.Value)
			}
			if results[i].Ack != AckConfirmed {
				t.Errorf("results[%d].Ack = %q, want %q", i, results[i].Ack, AckConfirmed)
			}
			want := composeOpenArgv(testBurstExe, testWindowEnv, surface, batch, results[i].Token, nil)
			if !slices.Equal(adapter.calls[i], want) {
				t.Errorf("OpenWindow[%d] argv = %#v, want %#v", i, adapter.calls[i], want)
			}
		}
	})

	t.Run("it composes the surface-matched open grammar per window over an attach+mint mix", func(t *testing.T) {
		clock := &manualClock{}
		ack := newDelayingAck(clock.now, 0)
		adapter := &writingAdapter{ack: ack}
		b := &Burster{
			Adapter: adapter, Ack: ack, Exe: fixedExe(testBurstExe),
			Env:     fixedEnv(testWindowEnv),
			NewID:   seqIDGen(),
			Timeout: 8 * time.Second, Poll: 75 * time.Millisecond,
			Now: clock.now, Sleep: clock.sleep,
		}
		surfaces := []Surface{
			{Kind: SurfaceAttach, Value: "proj-existing"},
			{Kind: SurfaceMint, Value: "/Users/me/projects/fresh"},
		}

		batch, results, err := b.Run(context.Background(), surfaces, nil, nil)
		if err != nil {
			t.Fatalf("Run error = %v, want nil", err)
		}
		if len(adapter.calls) != len(surfaces) {
			t.Fatalf("OpenWindow called %d times, want %d", len(adapter.calls), len(surfaces))
		}
		for i, surface := range surfaces {
			want := composeOpenArgv(testBurstExe, testWindowEnv, surface, batch, results[i].Token, nil)
			if !slices.Equal(adapter.calls[i], want) {
				t.Errorf("OpenWindow[%d] (%s) argv = %#v, want %#v", i, surface.Kind, adapter.calls[i], want)
			}
			if results[i].Ack != AckConfirmed {
				t.Errorf("results[%d].Ack = %q, want %q", i, results[i].Ack, AckConfirmed)
			}
		}
		mintArgv := adapter.calls[1]
		if !slices.Contains(mintArgv, "--path") {
			t.Errorf("mint window argv = %#v, want a --path flag", mintArgv)
		}
		if slices.Contains(mintArgv, "--session") {
			t.Errorf("mint window argv = %#v, must not carry --session", mintArgv)
		}
		flagIdx := slices.Index(mintArgv, "--path")
		if got := mintArgv[flagIdx+1]; got != "/Users/me/projects/fresh" {
			t.Errorf("mint --path value = %q, want the literal dir %q", got, "/Users/me/projects/fresh")
		}
	})

	t.Run("it threads the command to mint windows only, never attach windows", func(t *testing.T) {
		clock := &manualClock{}
		ack := newDelayingAck(clock.now, 0)
		adapter := &writingAdapter{ack: ack}
		b := &Burster{
			Adapter: adapter, Ack: ack, Exe: fixedExe(testBurstExe),
			Env:     fixedEnv(testWindowEnv),
			NewID:   seqIDGen(),
			Timeout: 8 * time.Second, Poll: 75 * time.Millisecond,
			Now: clock.now, Sleep: clock.sleep,
		}
		command := []string{"claude", "--resume"}
		surfaces := []Surface{
			{Kind: SurfaceAttach, Value: "proj-existing"},
			{Kind: SurfaceMint, Value: "/Users/me/projects/fresh"},
		}

		batch, results, err := b.Run(context.Background(), surfaces, command, nil)
		if err != nil {
			t.Fatalf("Run error = %v, want nil", err)
		}
		if len(adapter.calls) != len(surfaces) {
			t.Fatalf("OpenWindow called %d times, want %d", len(adapter.calls), len(surfaces))
		}
		for i, surface := range surfaces {
			want := composeOpenArgv(testBurstExe, testWindowEnv, surface, batch, results[i].Token, command)
			if !slices.Equal(adapter.calls[i], want) {
				t.Errorf("OpenWindow[%d] (%s) argv = %#v, want %#v", i, surface.Kind, adapter.calls[i], want)
			}
		}
		if slices.Contains(adapter.calls[0], "--") {
			t.Errorf("attach window argv carries the command; argv = %#v", adapter.calls[0])
		}
		mintArgv := adapter.calls[1]
		dashIdx := slices.Index(mintArgv, "--")
		if dashIdx < 0 {
			t.Fatalf("mint window argv missing the `--` passthrough terminator; argv = %#v", mintArgv)
		}
		if rest := mintArgv[dashIdx+1:]; !slices.Equal(rest, command) {
			t.Errorf("mint window post-`--` argv = %#v, want the command %#v verbatim", rest, command)
		}
	})

	t.Run("it starts each window's ack timer at its own spawn (per-window, not one global clock)", func(t *testing.T) {
		clock := &manualClock{}
		const window2Reveal = 200 * time.Millisecond
		ack := newDelayingAck(clock.now, window2Reveal)
		adapter := &writingAdapter{ack: ack, confirm: []bool{false, true}}
		b := &Burster{
			Adapter: adapter, Ack: ack, Exe: fixedExe(testBurstExe),
			Env:     fixedEnv(testWindowEnv),
			NewID:   seqIDGen(),
			Timeout: 750 * time.Millisecond, Poll: 100 * time.Millisecond,
			Now: clock.now, Sleep: clock.sleep,
		}

		_, results, err := b.Run(context.Background(), AttachSurfaces([]string{"w1", "w2"}), nil, nil)
		if err != nil {
			t.Fatalf("Run error = %v, want nil", err)
		}
		if results[0].Ack != AckTimeout {
			t.Errorf("window 1 Ack = %q, want %q (never confirmed)", results[0].Ack, AckTimeout)
		}
		if results[1].Ack != AckConfirmed {
			t.Errorf("window 2 Ack = %q, want %q (its own budget, judged from its own spawn)", results[1].Ack, AckConfirmed)
		}
		if clock.elapsed() < b.Timeout {
			t.Fatalf("clock advanced only %v, want >= Timeout %v so the global-clock independence proof is meaningful", clock.elapsed(), b.Timeout)
		}
	})

	t.Run("it times out a window when the ack enumeration persistently fails", func(t *testing.T) {
		clock := &manualClock{}
		ack := &erroringAck{}
		adapter := &writingAdapter{ack: ack}
		b := &Burster{
			Adapter: adapter, Ack: ack, Exe: fixedExe(testBurstExe),
			Env:     fixedEnv(testWindowEnv),
			NewID:   seqIDGen(),
			Timeout: 750 * time.Millisecond, Poll: 100 * time.Millisecond,
			Now: clock.now, Sleep: clock.sleep,
		}

		_, results, err := b.Run(context.Background(), AttachSurfaces([]string{"w1"}), nil, nil)
		if err != nil {
			t.Fatalf("Run error = %v, want nil (a failing Collect must not surface as a Run error)", err)
		}
		if len(results) != 1 {
			t.Fatalf("results len = %d, want 1", len(results))
		}
		if results[0].Ack != AckTimeout {
			t.Errorf("window Ack = %q, want %q (a persistently erroring Collect must time out, not confirm)", results[0].Ack, AckTimeout)
		}
		if ack.collectCalls == 0 {
			t.Error("Collect was never polled; the erroring branch was not exercised")
		}
	})

	t.Run("it confirms a token that arrives late but within the timeout", func(t *testing.T) {
		clock := &manualClock{}
		const revealDelay = 300 * time.Millisecond
		ack := newDelayingAck(clock.now, revealDelay)
		adapter := &writingAdapter{ack: ack}
		b := &Burster{
			Adapter: adapter, Ack: ack, Exe: fixedExe(testBurstExe),
			Env:     fixedEnv(testWindowEnv),
			NewID:   seqIDGen(),
			Timeout: 8 * time.Second, Poll: 100 * time.Millisecond,
			Now: clock.now, Sleep: clock.sleep,
		}

		_, results, err := b.Run(context.Background(), AttachSurfaces([]string{"w1"}), nil, nil)
		if err != nil {
			t.Fatalf("Run error = %v, want nil", err)
		}
		if results[0].Ack != AckConfirmed {
			t.Errorf("Ack = %q, want %q (late but within the timeout)", results[0].Ack, AckConfirmed)
		}
		if clock.elapsed() < revealDelay {
			t.Errorf("clock elapsed = %v, want >= the reveal delay %v (proving it polled)", clock.elapsed(), revealDelay)
		}
		if clock.elapsed() >= b.Timeout {
			t.Errorf("clock elapsed = %v, want < Timeout %v (in-time, not expired)", clock.elapsed(), b.Timeout)
		}
	})

	t.Run("it classifies a non-OK adapter result as failed and still spawns the remaining windows", func(t *testing.T) {
		clock := &manualClock{}
		ack := newDelayingAck(clock.now, 0)
		adapter := &writingAdapter{ack: ack, results: []Result{SpawnFailed("osascript: -1743")}}
		b := &Burster{
			Adapter: adapter, Ack: ack, Exe: fixedExe(testBurstExe),
			Env:     fixedEnv(testWindowEnv),
			NewID:   seqIDGen(),
			Timeout: 8 * time.Second, Poll: 100 * time.Millisecond,
			Now: clock.now, Sleep: clock.sleep,
		}

		_, results, err := b.Run(context.Background(), AttachSurfaces([]string{"w1", "w2"}), nil, nil)
		if err != nil {
			t.Fatalf("Run error = %v, want nil", err)
		}
		if len(adapter.calls) != 2 {
			t.Fatalf("OpenWindow called %d times, want 2 (no early stop on a failed window)", len(adapter.calls))
		}
		if results[0].Ack != AckFailed {
			t.Errorf("window 1 Ack = %q, want %q (adapter reported no window)", results[0].Ack, AckFailed)
		}
		if results[1].Ack != AckConfirmed {
			t.Errorf("window 2 Ack = %q, want %q", results[1].Ack, AckConfirmed)
		}
		if clock.elapsed() != 0 {
			t.Errorf("clock advanced by %v, want 0 (a failed window is not awaited)", clock.elapsed())
		}
	})

	t.Run("it continues spawning the remaining windows after a middle window fails (no early stop)", func(t *testing.T) {
		clock := &manualClock{}
		ack := newDelayingAck(clock.now, 0)
		adapter := &writingAdapter{ack: ack, results: []Result{Success(""), SpawnFailed("osascript: -1743"), Success("")}}
		b := &Burster{
			Adapter: adapter, Ack: ack, Exe: fixedExe(testBurstExe),
			Env:     fixedEnv(testWindowEnv),
			NewID:   seqIDGen(),
			Timeout: 8 * time.Second, Poll: 100 * time.Millisecond,
			Now: clock.now, Sleep: clock.sleep,
		}

		_, results, err := b.Run(context.Background(), AttachSurfaces([]string{"w1", "w2", "w3"}), nil, nil)
		if err != nil {
			t.Fatalf("Run error = %v, want nil", err)
		}
		if len(adapter.calls) != 3 {
			t.Fatalf("OpenWindow called %d times, want 3 (no early stop when a middle window fails)", len(adapter.calls))
		}
		if results[0].Ack != AckConfirmed {
			t.Errorf("window 1 Ack = %q, want %q", results[0].Ack, AckConfirmed)
		}
		if results[1].Ack != AckFailed {
			t.Errorf("window 2 Ack = %q, want %q (adapter reported no window)", results[1].Ack, AckFailed)
		}
		if results[2].Ack != AckConfirmed {
			t.Errorf("window 3 Ack = %q, want %q (spawned despite the middle window's failure)", results[2].Ack, AckConfirmed)
		}
	})

	t.Run("it stops the burst on permission-required so later windows are never spawned", func(t *testing.T) {
		clock := &manualClock{}
		ack := newDelayingAck(clock.now, 0)
		adapter := &writingAdapter{ack: ack, results: []Result{Success(""), PermissionRequired("evt -1743", "grant Automation for Ghostty")}}
		b := &Burster{
			Adapter: adapter, Ack: ack, Exe: fixedExe(testBurstExe),
			Env:     fixedEnv(testWindowEnv),
			NewID:   seqIDGen(),
			Timeout: 8 * time.Second, Poll: 100 * time.Millisecond,
			Now: clock.now, Sleep: clock.sleep,
		}

		batch, results, err := b.Run(context.Background(), AttachSurfaces([]string{"w1", "w2", "w3", "w4", "w5"}), nil, nil)
		if err != nil {
			t.Fatalf("Run error = %v, want nil", err)
		}
		if len(adapter.calls) != 2 {
			t.Fatalf("OpenWindow called %d times, want 2 (windows 3,4,5 never spawned after the permission wall)", len(adapter.calls))
		}
		if len(results) != 2 {
			t.Fatalf("results len = %d, want 2 (only windows 1,2 recorded before the stop)", len(results))
		}
		if results[1].Result.Outcome != OutcomePermissionRequired {
			t.Errorf("window 2 Outcome = %v, want OutcomePermissionRequired", results[1].Result.Outcome)
		}
		for i, surface := range AttachSurfaces([]string{"w1", "w2"}) {
			want := composeOpenArgv(testBurstExe, testWindowEnv, surface, batch, results[i].Token, nil)
			if !slices.Equal(adapter.calls[i], want) {
				t.Errorf("OpenWindow[%d] argv = %#v, want %#v", i, adapter.calls[i], want)
			}
		}
	})

	t.Run("it aborts before opening any window when the executable cannot be resolved", func(t *testing.T) {
		clock := &manualClock{}
		sentinel := errors.New("os.Executable: readlink /proc/self/exe: no such file")
		adapter := &writingAdapter{}
		b := &Burster{
			Adapter: adapter, Ack: newDelayingAck(clock.now, 0),
			Exe:     func() (string, error) { return "", sentinel },
			Env:     fixedEnv(testWindowEnv),
			NewID:   seqIDGen(),
			Timeout: 8 * time.Second, Poll: 100 * time.Millisecond,
			Now: clock.now, Sleep: clock.sleep,
		}

		batch, results, err := b.Run(context.Background(), AttachSurfaces([]string{"s1", "s2"}), nil, nil)
		if batch != "" {
			t.Errorf("batch = %q, want empty on executable-resolution failure", batch)
		}
		if results != nil {
			t.Errorf("results = %#v, want nil on executable-resolution failure", results)
		}
		if !errors.Is(err, sentinel) {
			t.Errorf("errors.Is(err, sentinel) = false, want true; err = %v", err)
		}
		if len(adapter.calls) != 0 {
			t.Errorf("OpenWindow called %d times, want 0", len(adapter.calls))
		}
	})

	t.Run("it aborts before opening any window when the window environment cannot be resolved", func(t *testing.T) {
		clock := &manualClock{}
		sentinel := errors.New("state dir: $HOME is not defined")
		adapter := &writingAdapter{}
		b := &Burster{
			Adapter: adapter, Ack: newDelayingAck(clock.now, 0),
			Exe:     fixedExe(testBurstExe),
			Env:     func() ([]string, error) { return nil, sentinel },
			NewID:   seqIDGen(),
			Timeout: 8 * time.Second, Poll: 100 * time.Millisecond,
			Now: clock.now, Sleep: clock.sleep,
		}

		batch, results, err := b.Run(context.Background(), AttachSurfaces([]string{"s1", "s2"}), nil, nil)
		if batch != "" || results != nil {
			t.Errorf("Run = (%q, %#v), want no batch and no results when the window environment fails", batch, results)
		}
		if !errors.Is(err, sentinel) {
			t.Errorf("errors.Is(err, sentinel) = false, want true; err = %v", err)
		}
		if len(adapter.calls) != 0 {
			t.Errorf("OpenWindow called %d times, want 0", len(adapter.calls))
		}
	})

	t.Run("it aborts before opening any window when an ack id cannot be generated", func(t *testing.T) {
		clock := &manualClock{}
		sentinel := errors.New("crypto/rand: read failed")
		adapter := &writingAdapter{}
		b := &Burster{
			Adapter: adapter, Ack: newDelayingAck(clock.now, 0),
			Exe:     fixedExe(testBurstExe),
			Env:     fixedEnv(testWindowEnv),
			NewID:   func() (string, error) { return "", sentinel },
			Timeout: 8 * time.Second, Poll: 100 * time.Millisecond,
			Now: clock.now, Sleep: clock.sleep,
		}

		batch, results, err := b.Run(context.Background(), AttachSurfaces([]string{"s1", "s2"}), nil, nil)
		if batch != "" {
			t.Errorf("batch = %q, want empty on id-generation failure", batch)
		}
		if results != nil {
			t.Errorf("results = %#v, want nil on id-generation failure", results)
		}
		if !errors.Is(err, sentinel) {
			t.Errorf("errors.Is(err, sentinel) = false, want true; err = %v", err)
		}
		if len(adapter.calls) != 0 {
			t.Errorf("OpenWindow called %d times, want 0 when an id cannot be generated", len(adapter.calls))
		}
	})
}

func TestBurster_Run_Progress(t *testing.T) {
	t.Run("it reports (i+1, len(external)) progress after each window in list order", func(t *testing.T) {
		clock := &manualClock{}
		ack := newDelayingAck(clock.now, 0)
		adapter := &writingAdapter{ack: ack}
		b := &Burster{
			Adapter: adapter, Ack: ack, Exe: fixedExe(testBurstExe),
			Env:     fixedEnv(testWindowEnv),
			NewID:   seqIDGen(),
			Timeout: 8 * time.Second, Poll: 100 * time.Millisecond,
			Now: clock.now, Sleep: clock.sleep,
		}

		var got [][2]int
		_, _, err := b.Run(context.Background(), AttachSurfaces([]string{"w1", "w2", "w3"}), nil, func(done, total int) {
			got = append(got, [2]int{done, total})
		})
		if err != nil {
			t.Fatalf("Run error = %v, want nil", err)
		}
		want := [][2]int{{1, 3}, {2, 3}, {3, 3}}
		if !slices.Equal(got, want) {
			t.Errorf("progress calls = %v, want %v", got, want)
		}
	})

	t.Run("it tolerates a nil progress callback (Phase-2/3 CLI parity)", func(t *testing.T) {
		clock := &manualClock{}
		ack := newDelayingAck(clock.now, 0)
		adapter := &writingAdapter{ack: ack}
		b := &Burster{
			Adapter: adapter, Ack: ack, Exe: fixedExe(testBurstExe),
			Env:     fixedEnv(testWindowEnv),
			NewID:   seqIDGen(),
			Timeout: 8 * time.Second, Poll: 100 * time.Millisecond,
			Now: clock.now, Sleep: clock.sleep,
		}

		_, results, err := b.Run(context.Background(), AttachSurfaces([]string{"w1", "w2"}), nil, nil)
		if err != nil {
			t.Fatalf("Run error = %v, want nil", err)
		}
		if len(results) != 2 || len(adapter.calls) != 2 {
			t.Fatalf("nil progress must not alter the burst: results=%d calls=%d, want 2/2", len(results), len(adapter.calls))
		}
	})

	t.Run("it stops iterating when the context is cancelled between windows", func(t *testing.T) {
		clock := &manualClock{}
		ack := newDelayingAck(clock.now, 0)
		adapter := &writingAdapter{ack: ack}
		ctx, cancel := context.WithCancel(context.Background())
		b := &Burster{
			Adapter: adapter, Ack: ack, Exe: fixedExe(testBurstExe),
			Env:     fixedEnv(testWindowEnv),
			NewID:   seqIDGen(),
			Timeout: 8 * time.Second, Poll: 100 * time.Millisecond,
			Now: clock.now, Sleep: clock.sleep,
		}

		_, results, err := b.Run(ctx, AttachSurfaces([]string{"w1", "w2", "w3"}), nil, func(done, _ int) {
			if done == 1 {
				cancel()
			}
		})
		if err != nil {
			t.Fatalf("Run error = %v, want nil (cancel returns what was collected)", err)
		}
		if len(adapter.calls) != 1 {
			t.Fatalf("OpenWindow called %d times, want 1 (cancel stops before window 2)", len(adapter.calls))
		}
		if len(results) != 1 {
			t.Errorf("results len = %d, want 1 (only the pre-cancel window)", len(results))
		}
	})
}

func TestNewBurster_Defaults(t *testing.T) {
	adapter := &writingAdapter{}
	ack := newDelayingAck(time.Now, 0)
	b := NewBurster(adapter, ack, fixedExe(testBurstExe), fixedEnv(testWindowEnv))

	if b.Timeout != spawnAckTimeout {
		t.Errorf("default Timeout = %v, want spawnAckTimeout %v", b.Timeout, spawnAckTimeout)
	}
	if b.Poll <= 0 {
		t.Errorf("default Poll = %v, want a positive interval", b.Poll)
	}
	if b.NewID == nil {
		t.Error("default NewID is nil, want a generator")
	}
	if b.Now == nil || b.Sleep == nil {
		t.Error("default Now/Sleep are nil, want the real clock seams")
	}
	if _, err := NewSpawnID(b.NewID); err != nil {
		t.Errorf("default NewID produced a non-option-safe id: %v", err)
	}
}
