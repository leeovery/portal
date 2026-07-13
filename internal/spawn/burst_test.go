package spawn

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"
)

// manualClock is the deterministic fake clock the burst tests drive: Now reads
// the current instant and Sleep advances it (no real time passes). The burster
// runs sequentially in one goroutine, so no synchronisation is needed.
type manualClock struct{ t time.Time }

func (c *manualClock) now() time.Time         { return c.t }
func (c *manualClock) sleep(d time.Duration)  { c.t = c.t.Add(d) }
func (c *manualClock) elapsed() time.Duration { return c.t.Sub(time.Time{}) }

// delayingAck is an in-package AckCollector+AckWriter double. A Write records
// the token with a reveal instant of now()+delay, and Collect returns only the
// tokens whose reveal instant has arrived (now() >= revealAt). delay=0 reveals a
// token the instant it is written; a positive delay simulates a late-but-in-time
// ack under the manual clock. It cannot import spawntest (that would cycle back
// into spawn), so it mirrors FakeAckChannel in-package.
type delayingAck struct {
	now      func() time.Time
	delay    time.Duration
	revealAt map[string]map[string]time.Time // batch -> token -> reveal instant
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
		if !d.now().Before(revealAt) { // now >= revealAt
			out[token] = struct{}{}
		}
	}
	return out, nil
}

// writingAdapter records each composed argv (in call order) and replays a
// scripted Result per call (defaulting to Success once the script is exhausted).
// On a success Result whose per-window confirm flag is true (nil confirm ⇒ all
// true), it parses --spawn-ack <batch>:<token> out of the argv it was handed and
// writes that token to ack — simulating the spawned window's marker write. On a
// non-success Result (or a false confirm flag) it writes nothing, so the burster
// times that window out.
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

// confirmed reports whether window i should have its token written. A nil confirm
// slice (and any index beyond it) confirms; only an explicit false suppresses.
func (a *writingAdapter) confirmed(i int) bool {
	if i < len(a.confirm) {
		return a.confirm[i]
	}
	return true
}

// parseSpawnAckArgv finds the --spawn-ack <value> pair in an argv and splits its
// value back into (batch, token) via the real ParseSpawnAckFlag, so the fake
// stays honest to the wire format AttachCommand produced.
func parseSpawnAckArgv(argv []string) (batch, token string, ok bool) {
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == "--spawn-ack" {
			return ParseSpawnAckFlag(argv[i+1])
		}
	}
	return "", "", false
}

// seqIDGen returns a deterministic id generator yielding "id1", "id2", … —
// option-safe (alphanumeric, hyphen/colon-free) so NewSpawnID accepts them, and
// distinct so batch and per-window tokens never collide.
func seqIDGen() func() (string, error) {
	var n int
	return func() (string, error) {
		n++
		return fmt.Sprintf("id%d", n), nil
	}
}

const (
	testBurstPath = "/opt/homebrew/bin:/usr/bin"
	testBurstExe  = "/abs/portal"
)

func TestBurster_Run(t *testing.T) {
	t.Run("it resolves the executable once and composes an ack-flagged attach argv per session in list order", func(t *testing.T) {
		clock := &manualClock{}
		var exeCalls int
		exe := func() (string, error) { exeCalls++; return testBurstExe, nil }
		ack := newDelayingAck(clock.now, 0)
		adapter := &writingAdapter{ack: ack}
		b := &Burster{
			Adapter: adapter, Ack: ack, Exe: exe,
			Getenv:  mapGetenv(map[string]string{"PATH": testBurstPath}),
			NewID:   seqIDGen(),
			Timeout: 8 * time.Second, Poll: 75 * time.Millisecond,
			Now: clock.now, Sleep: clock.sleep,
		}
		sessions := []string{"s1", "s2", "s3"}

		batch, results, err := b.Run(sessions)
		if err != nil {
			t.Fatalf("Run error = %v, want nil", err)
		}
		if exeCalls != 1 {
			t.Errorf("executable resolved %d times, want exactly 1", exeCalls)
		}
		if batch == "" {
			t.Fatal("batch id is empty, want a generated id")
		}
		if len(results) != len(sessions) {
			t.Fatalf("results len = %d, want %d", len(results), len(sessions))
		}
		if len(adapter.calls) != len(sessions) {
			t.Fatalf("OpenWindow called %d times, want %d", len(adapter.calls), len(sessions))
		}
		for i, session := range sessions {
			if results[i].Session != session {
				t.Errorf("results[%d].Session = %q, want %q", i, results[i].Session, session)
			}
			if results[i].Ack != AckConfirmed {
				t.Errorf("results[%d].Ack = %q, want %q", i, results[i].Ack, AckConfirmed)
			}
			want := composeAttachArgv(testBurstExe, testBurstPath, session, batch, results[i].Token)
			if !slices.Equal(adapter.calls[i], want) {
				t.Errorf("OpenWindow[%d] argv = %#v, want %#v", i, adapter.calls[i], want)
			}
		}
	})

	t.Run("it starts each window's ack timer at its own spawn (per-window, not one global clock)", func(t *testing.T) {
		clock := &manualClock{}
		// window2Reveal is the delay between window 2's token being WRITTEN (at its
		// own spawn) and Collect returning it. It is deliberately >= Poll and <
		// Timeout so window 2 must poll at least once — crossing a timeout check —
		// before its token appears. This positive delay is what makes the test
		// DISCRIMINATING: with delay 0 the token is present at window 2's very first
		// Collect, so awaitToken returns before ever consulting the timer, and a
		// global-timer regression (start captured once before the window loop)
		// would pass too. With a positive delay window 2's timer value is load-
		// bearing to the outcome.
		const window2Reveal = 200 * time.Millisecond
		ack := newDelayingAck(clock.now, window2Reveal)
		// Window 1 never confirms (writes nothing) → it times out, advancing the
		// shared clock PAST Timeout (to ~800ms). Window 2 then spawns at ~800ms and
		// its token only appears ~200ms later (~1000ms). A per-window timer (reset
		// at window 2's spawn) sees elapsed 200ms < Timeout 750ms → AckConfirmed. A
		// single global timer from Enter is already ~900ms >= Timeout at window 2's
		// first timeout check → it would over-report AckTimeout. Only the per-window
		// timer confirms window 2, so this test now distinguishes the two.
		adapter := &writingAdapter{ack: ack, confirm: []bool{false, true}}
		b := &Burster{
			Adapter: adapter, Ack: ack, Exe: fixedExe(testBurstExe),
			Getenv:  mapGetenv(map[string]string{"PATH": testBurstPath}),
			NewID:   seqIDGen(),
			Timeout: 750 * time.Millisecond, Poll: 100 * time.Millisecond,
			Now: clock.now, Sleep: clock.sleep,
		}

		_, results, err := b.Run([]string{"w1", "w2"})
		if err != nil {
			t.Fatalf("Run error = %v, want nil", err)
		}
		if results[0].Ack != AckTimeout {
			t.Errorf("window 1 Ack = %q, want %q (never confirmed)", results[0].Ack, AckTimeout)
		}
		if results[1].Ack != AckConfirmed {
			t.Errorf("window 2 Ack = %q, want %q (its own budget, judged from its own spawn)", results[1].Ack, AckConfirmed)
		}
		// The independence proof: window 2's token only appeared >= Poll after its
		// own spawn AND the burst-wide elapsed clock was already past Timeout by
		// then — so a global timer would have expired before window 2's token
		// arrived. Only a per-window timer (reset at window 2's spawn) can explain
		// the confirm above.
		if clock.elapsed() < b.Timeout {
			t.Fatalf("clock advanced only %v, want >= Timeout %v so the global-clock independence proof is meaningful", clock.elapsed(), b.Timeout)
		}
	})

	t.Run("it confirms a token that arrives late but within the timeout", func(t *testing.T) {
		clock := &manualClock{}
		const revealDelay = 300 * time.Millisecond
		ack := newDelayingAck(clock.now, revealDelay) // token appears 300ms after write
		adapter := &writingAdapter{ack: ack}          // confirm nil ⇒ writes the token
		b := &Burster{
			Adapter: adapter, Ack: ack, Exe: fixedExe(testBurstExe),
			Getenv:  mapGetenv(map[string]string{"PATH": testBurstPath}),
			NewID:   seqIDGen(),
			Timeout: 8 * time.Second, Poll: 100 * time.Millisecond,
			Now: clock.now, Sleep: clock.sleep,
		}

		_, results, err := b.Run([]string{"w1"})
		if err != nil {
			t.Fatalf("Run error = %v, want nil", err)
		}
		if results[0].Ack != AckConfirmed {
			t.Errorf("Ack = %q, want %q (late but within the timeout)", results[0].Ack, AckConfirmed)
		}
		// It was genuinely late: at least one poll elapsed before the token showed.
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
		// Window 1 fails to open; window 2 opens cleanly (default Success).
		adapter := &writingAdapter{ack: ack, results: []Result{SpawnFailed("osascript: -1743")}}
		b := &Burster{
			Adapter: adapter, Ack: ack, Exe: fixedExe(testBurstExe),
			Getenv:  mapGetenv(map[string]string{"PATH": testBurstPath}),
			NewID:   seqIDGen(),
			Timeout: 8 * time.Second, Poll: 100 * time.Millisecond,
			Now: clock.now, Sleep: clock.sleep,
		}

		_, results, err := b.Run([]string{"w1", "w2"})
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
		// The failed window is never awaited: the clock never advanced for it.
		if clock.elapsed() != 0 {
			t.Errorf("clock advanced by %v, want 0 (a failed window is not awaited)", clock.elapsed())
		}
	})

	t.Run("it continues spawning the remaining windows after a middle window fails (no early stop)", func(t *testing.T) {
		clock := &manualClock{}
		ack := newDelayingAck(clock.now, 0)
		// Window 2 (the middle of three) fails to open; windows 1 and 3 open cleanly.
		// This proves the burst never short-circuits on a failed middle window — the
		// leave-what-opened contract needs every window that can open to open.
		adapter := &writingAdapter{ack: ack, results: []Result{Success(""), SpawnFailed("osascript: -1743"), Success("")}}
		b := &Burster{
			Adapter: adapter, Ack: ack, Exe: fixedExe(testBurstExe),
			Getenv:  mapGetenv(map[string]string{"PATH": testBurstPath}),
			NewID:   seqIDGen(),
			Timeout: 8 * time.Second, Poll: 100 * time.Millisecond,
			Now: clock.now, Sleep: clock.sleep,
		}

		_, results, err := b.Run([]string{"w1", "w2", "w3"})
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
		// Window 2 (of five) hits the permission wall. The macOS Automation grant
		// is per-(source, target), so every later window would hit the identical
		// wall — the burst must STOP at window 2 (the sole early-stop), never
		// composing or handing windows 3,4,5 to the adapter.
		adapter := &writingAdapter{ack: ack, results: []Result{Success(""), PermissionRequired("evt -1743", "grant Automation for Ghostty")}}
		b := &Burster{
			Adapter: adapter, Ack: ack, Exe: fixedExe(testBurstExe),
			Getenv:  mapGetenv(map[string]string{"PATH": testBurstPath}),
			NewID:   seqIDGen(),
			Timeout: 8 * time.Second, Poll: 100 * time.Millisecond,
			Now: clock.now, Sleep: clock.sleep,
		}

		batch, results, err := b.Run([]string{"w1", "w2", "w3", "w4", "w5"})
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
		// The two earlier windows were attempted, in order (windows 1 and 2).
		for i, session := range []string{"w1", "w2"} {
			want := composeAttachArgv(testBurstExe, testBurstPath, session, batch, results[i].Token)
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
			Getenv:  mapGetenv(map[string]string{"PATH": testBurstPath}),
			NewID:   seqIDGen(),
			Timeout: 8 * time.Second, Poll: 100 * time.Millisecond,
			Now: clock.now, Sleep: clock.sleep,
		}

		batch, results, err := b.Run([]string{"s1", "s2"})
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

	t.Run("it aborts before opening any window when an ack id cannot be generated", func(t *testing.T) {
		clock := &manualClock{}
		sentinel := errors.New("crypto/rand: read failed")
		adapter := &writingAdapter{}
		b := &Burster{
			Adapter: adapter, Ack: newDelayingAck(clock.now, 0),
			Exe:     fixedExe(testBurstExe),
			Getenv:  mapGetenv(map[string]string{"PATH": testBurstPath}),
			NewID:   func() (string, error) { return "", sentinel },
			Timeout: 8 * time.Second, Poll: 100 * time.Millisecond,
			Now: clock.now, Sleep: clock.sleep,
		}

		batch, results, err := b.Run([]string{"s1", "s2"})
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

func TestNewBurster_Defaults(t *testing.T) {
	adapter := &writingAdapter{}
	ack := newDelayingAck(time.Now, 0)
	b := NewBurster(adapter, ack, fixedExe(testBurstExe), mapGetenv(map[string]string{"PATH": testBurstPath}))

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
	// The default generator yields option-safe ids NewSpawnID accepts.
	if _, err := NewSpawnID(b.NewID); err != nil {
		t.Errorf("default NewID produced a non-option-safe id: %v", err)
	}
}
