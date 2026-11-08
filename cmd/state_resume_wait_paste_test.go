package cmd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/bgreply"
)

const resumeArrivalTestBudget = 2 * time.Second

// arrivalHarness delivers input to a wait in arrivals — bytes landing together,
// then the pane falling quiet — and holds every listen and window the wait
// takes until the test answers it, so which bytes arrived together is decided
// by the test and never by the clock. Each wake of the idle listen is the
// test's to take before it answers that listen woken. The wait's wall clock
// moves on only by the windows it listened through in quiet.
type arrivalHarness struct {
	t       *testing.T
	probe   *resumeWaitProbe
	writer  *io.PipeWriter
	reader  *trackedReader
	sizes   *paneResize
	winch   chan os.Signal
	listens chan listen
	wakes   chan struct{}
	settles chan chan time.Time
	done    chan error
	ended   chan struct{}
	clock   *testClock

	// The wait's listen in flight while it waits between arrivals, past the
	// latest byte or for the next, answered by whether a byte arrived.
	live *listen

	// The bytes of the arrival the wait has yet to judge, and the quiet
	// windows it has held that arrival open for a reply's rest.
	pending []byte
	heldFor int
}

// listen is one listen the wait takes, held until the test answers it: for the
// quiet that ends an arrival, or, idle, with no arrival open, for as long as
// nothing arrives and the wait does not wake it.
type listen struct {
	idle  bool
	reply chan bool
}

func startArrivals(t *testing.T, payload resumeChainPayload) *arrivalHarness {
	t.Helper()
	pipeReader, pipeWriter := io.Pipe()
	h := &arrivalHarness{
		t:       t,
		probe:   new(resumeWaitProbe),
		writer:  pipeWriter,
		reader:  &trackedReader{t: t, inner: pipeReader, started: make(chan struct{}, 1)},
		sizes:   &paneResize{drawn: payload, to: resizedSize},
		winch:   make(chan os.Signal, 1),
		listens: make(chan listen),
		wakes:   make(chan struct{}, 8),
		settles: make(chan chan time.Time, 8),
		done:    make(chan error, 1),
		ended:   make(chan struct{}),
		clock:   newTestClock(),
		pending: []byte(payload.Typed),
	}
	t.Cleanup(func() { _ = pipeWriter.Close() })

	cfg := newResumeWaitConfig(t, h.probe, payload, h.reader)
	cfg.Winch = h.winch
	cfg.Size = h.sizes.size
	cfg.Now = h.clock.now
	cfg.AwaitInput = func(d time.Duration) (bool, error) {
		if d != resumeInputQuiet {
			t.Errorf("listen window = %v, want the quiet %v", d, resumeInputQuiet)
		}
		arrived := h.listened(false)
		if !arrived {
			h.clock.advance(d)
		}
		return arrived, nil
	}
	cfg.OpenWaker = func() (resumeWaker, error) { return arrivalWaker{h}, nil }
	cfg.Settle = func(d time.Duration) <-chan time.Time {
		if d != resumeResizeSettle {
			t.Errorf("window = %v, want the resize settle", d)
			return nil
		}
		timer := make(chan time.Time, 1)
		h.settles <- timer
		return timer
	}

	go func() {
		h.done <- runResumeWait(cfg)
		close(h.ended)
	}()
	h.live = h.awaitListen("the wait's first listen")
	return h
}

// listened takes one listen until the test answers it, reporting the answer.
func (h *arrivalHarness) listened(idle bool) bool {
	l := listen{idle: idle, reply: make(chan bool)}
	h.listens <- l
	return <-l.reply
}

// arrivalWaker holds the wait's idle listen as it holds every other, and hands
// each wake to the test, which answers the listen woken once it has seen one.
type arrivalWaker struct{ h *arrivalHarness }

func (w arrivalWaker) Await() (bool, error) { return w.h.listened(true), nil }

func (w arrivalWaker) Wake() {
	select {
	case w.h.wakes <- struct{}{}:
	default:
	}
}

func (arrivalWaker) Close() error { return nil }

func (h *arrivalHarness) awaitListen(what string) *listen {
	h.t.Helper()
	select {
	case l := <-h.listens:
		return &l
	case err := <-h.done:
		h.t.Fatalf("the wait ended while waiting for %s: %v", what, err)
	case <-time.After(resumeArrivalTestBudget):
		h.t.Fatalf("the wait took no %s", what)
	}
	return nil
}

// arrive delivers keys as one arrival and returns once the wait has read every
// byte of it and is listening for one more.
func (h *arrivalHarness) arrive(keys string) {
	h.t.Helper()
	h.pending = append(h.pending, keys...)
	go func() { _, _ = h.writer.Write([]byte(keys)) }()
	for range len(keys) {
		if h.live == nil {
			h.live = h.awaitListen("a listen for a byte of " + strconv.Quote(keys))
		}
		h.answerListen(true)
		h.live = h.awaitListen("a listen past a byte of " + strconv.Quote(keys))
	}
}

func (h *arrivalHarness) answerListen(arrived bool) {
	h.t.Helper()
	select {
	case h.live.reply <- arrived:
		h.live = nil
	case <-time.After(resumeArrivalTestBudget):
		h.t.Fatal("the wait stopped listening")
	}
}

// quiet lets the pane fall silent after the latest arrival, through every
// quiet window the wait holds an arrival ending in a reply's head open for,
// and returns once the wait has acted on it: taken its next listen, or ended.
func (h *arrivalHarness) quiet() {
	h.t.Helper()
	if h.live != nil && h.live.idle {
		h.t.Fatal("no arrival is open to fall quiet after: the idle listen ends only on a byte or a wake")
	}
	if _, _, head := bgreply.Split(h.pending); head != nil {
		for h.heldFor < resumeReplyTailHolds {
			h.held()
		}
	}
	h.pending, h.heldFor = nil, 0
	h.answerListen(false)
	select {
	case l := <-h.listens:
		h.live = &l
	case <-h.ended:
	case <-time.After(resumeArrivalTestBudget):
		h.t.Fatal("the wait never acted on the pane falling quiet")
	}
}

// typed delivers keys as a keystroke does: alone, with the pane quiet after.
func (h *arrivalHarness) typed(key string) {
	h.t.Helper()
	h.arrive(key)
	h.quiet()
}

func (h *arrivalHarness) wait() error {
	h.t.Helper()
	select {
	case err := <-h.done:
		return err
	case <-time.After(resumeArrivalTestBudget):
		h.t.Fatal("the wait never ended")
	}
	return nil
}

// endInput ends the input under a wait still listening and returns what the
// wait ended with.
func (h *arrivalHarness) endInput() error {
	h.t.Helper()
	_ = h.writer.Close()
	if h.live == nil {
		h.live = h.awaitListen("a listen to find the input ended")
	}
	h.answerListen(true)
	return h.wait()
}

// stillWaiting ends the input and asserts the wait had answered nothing and
// touched nothing before it did.
func (h *arrivalHarness) stillWaiting() {
	h.t.Helper()
	if err := h.endInput(); !errors.Is(err, io.EOF) {
		h.t.Fatalf("runResumeWait() error = %v, want the EOF that ended a wait still going", err)
	}
	assertNoHandOff(h.t, h.probe)
	assertNoStateTouched(h.t, h.probe)
}

func (h *arrivalHarness) answered() {
	h.t.Helper()
	if err := h.wait(); err != nil {
		h.t.Fatalf("runResumeWait() error = %v", err)
	}
}

// A terminal's paste reaches the session with its line feeds as carriage
// returns; one case keeps them as line feeds regardless.
var panelPastes = []struct {
	name  string
	paste string
}{
	{"a line ending in its line break", "echo hi >> notes\r"},
	{"a line break alone between two lines", "echo one\recho two"},
	{"a paste opening with a line break", "\recho hi"},
	{"two line breaks", "\r\r"},
	{"line feeds as they are", "echo one\necho two\n"},
	{"a paste opening with the discard key", "d\recho hi\r"},
	{"the discard key and a line break", "d\r"},
	{"an escape sequence and a line break", "\x1b[A\r"},
}

func TestRunResumeWait_Pastes(t *testing.T) {
	t.Run("a paste never answers the waiting panel", func(t *testing.T) {
		for _, tc := range panelPastes {
			t.Run(tc.name, func(t *testing.T) {
				h := startArrivals(t, reportedPayload(resumeScreenPanel))

				h.arrive(tc.paste)
				h.quiet()

				h.stillWaiting()
			})
		}
	})

	t.Run("a paste carrying the confirm key never discards", func(t *testing.T) {
		for _, paste := range []string{"yes", "y\r", "cd ~/dev && yarn\r", "\x1by", "yy"} {
			t.Run(strconv.Quote(paste), func(t *testing.T) {
				h := startArrivals(t, reportedPayload(resumeScreenDiscard))

				h.arrive(paste)
				h.quiet()

				h.stillWaiting()
			})
		}
	})

	t.Run("a key typed after a paste answers the waiting panel as it would have", func(t *testing.T) {
		for _, tc := range panelPastes {
			t.Run(tc.name+"/Enter", func(t *testing.T) {
				payload := reportedPayload(resumeScreenPanel)
				h := startArrivals(t, payload)
				h.probe.lookup = foundHook(payload.Command)

				h.arrive(tc.paste)
				h.quiet()
				h.typed("\r")

				h.answered()
				if h.probe.clearCalls != 1 {
					t.Errorf("ClearPending called %d times, want 1", h.probe.clearCalls)
				}
				assertHookHandOff(t, h.probe, payload.Command)
			})
			t.Run(tc.name+"/d", func(t *testing.T) {
				payload := reportedPayload(resumeScreenPanel)
				h := startArrivals(t, payload)

				h.arrive(tc.paste)
				h.quiet()
				h.typed("d")

				h.answered()
				assertHandOff(t, h.probe, opened(payload))
				assertNoStateTouched(t, h.probe)
			})
		}
	})

	t.Run("a key typed after a paste answers the confirmation as it would have", func(t *testing.T) {
		t.Run("y discards", func(t *testing.T) {
			payload := reportedPayload(resumeScreenDiscard)
			h := startArrivals(t, payload)

			h.arrive("yes\r")
			h.quiet()
			h.typed("y")

			h.answered()
			assertDiscardAnswered(t, h.probe, payload)
		})
		t.Run("Escape backs out", func(t *testing.T) {
			payload := reportedPayload(resumeScreenDiscard)
			h := startArrivals(t, payload)

			h.arrive("yes\r")
			h.quiet()
			h.typed("\x1b")

			h.answered()
			assertHandOff(t, h.probe, onScreen(payload, resumeScreenPanel))
			assertNoStateTouched(t, h.probe)
		})
	})

	t.Run("a key answers only once the pane has gone quiet after it", func(t *testing.T) {
		payload := reportedPayload(resumeScreenPanel)
		h := startArrivals(t, payload)
		h.probe.lookup = foundHook(payload.Command)

		h.arrive("\r")
		select {
		case err := <-h.done:
			t.Fatalf("the wait answered before the pane went quiet after the key: %v", err)
		case <-time.After(50 * time.Millisecond):
		}
		if h.probe.execCalls != 0 || h.probe.clearCalls != 0 {
			t.Fatalf("the wait acted on the key before the pane went quiet: %d execs, %d clears",
				h.probe.execCalls, h.probe.clearCalls)
		}
		h.quiet()

		h.answered()
		assertHookHandOff(t, h.probe, payload.Command)
	})

	t.Run("it keeps one read outstanding at a time over a paste", func(t *testing.T) {
		h := startArrivals(t, reportedPayload(resumeScreenPanel))

		h.arrive("echo one\recho two\r")
		h.quiet()
		h.stillWaiting()

		if got := h.reader.overlaps.Load(); got != 0 {
			t.Errorf("%d reads overlapped another; input the hand-off should inherit is stranded in a buffer", got)
		}
	})
}

func TestRunResumeWait_InputStillArriving(t *testing.T) {
	arriving := func(screen string) resumeChainPayload {
		p := reportedPayload(screen)
		p.InputArriving = true
		return p
	}

	t.Run("it answers nothing that arrives before the pane goes quiet", func(t *testing.T) {
		for _, screen := range []string{resumeScreenPanel, resumeScreenDiscard} {
			for _, key := range []string{"\r", "\n", "d", "y", "\x1b"} {
				t.Run(screenName(screen)+"/"+strconv.Quote(key), func(t *testing.T) {
					h := startArrivals(t, arriving(screen))

					h.arrive(key)
					h.quiet()

					h.stillWaiting()
				})
			}
		}
	})

	t.Run("it answers a key typed once the pane has gone quiet", func(t *testing.T) {
		cases := []struct {
			name   string
			before func(h *arrivalHarness)
		}{
			{"with nothing more arriving", func(h *arrivalHarness) { h.quiet() }},
			{"after the rest of the input", func(h *arrivalHarness) {
				h.arrive("\r")
				h.quiet()
			}},
		}
		for _, tc := range cases {
			t.Run(tc.name+"/Enter", func(t *testing.T) {
				payload := arriving(resumeScreenPanel)
				h := startArrivals(t, payload)
				h.probe.lookup = foundHook(payload.Command)

				tc.before(h)
				h.typed("\r")

				h.answered()
				assertHookHandOff(t, h.probe, payload.Command)
			})
			t.Run(tc.name+"/d", func(t *testing.T) {
				payload := arriving(resumeScreenPanel)
				h := startArrivals(t, payload)

				tc.before(h)
				h.typed("d")

				h.answered()
				want := opened(payload)
				want.InputArriving = false
				assertHandOff(t, h.probe, want)
				assertArgvLacks(t, h.probe.execArgs, resumeFlagInputArriving)
			})
		}
	})

	// zmx's SIGWINCH on every attach opens the settle window, so a key typed
	// as a session is opened can still be arriving when the redraw comes due.
	t.Run("a redraw that comes due inside an arrival waits for it, and a key alone still answers", func(t *testing.T) {
		t.Run("Enter", func(t *testing.T) {
			payload := drawnPayload()
			h := startArrivals(t, payload)
			h.probe.lookup = foundHook(payload.Command)

			h.arrive("\r")
			h.resizeSettled()
			h.quiet()

			h.answered()
			assertHookHandOff(t, h.probe, payload.Command)
		})
		t.Run("d", func(t *testing.T) {
			payload := drawnPayload()
			h := startArrivals(t, payload)

			h.arrive("d")
			h.resizeSettled()
			h.quiet()

			h.answered()
			assertHandOff(t, h.probe, opened(payload))
			assertNoStateTouched(t, h.probe)
		})
		t.Run("Escape, then the redraw", func(t *testing.T) {
			payload := drawnPayload()
			h := startArrivals(t, payload)

			h.arrive("\x1b")
			h.resizeSettled()
			h.quiet()

			h.answered()
			if h.probe.backCalls != 1 {
				t.Errorf("BackToPicker called %d times, want 1", h.probe.backCalls)
			}
			assertHandOff(t, h.probe, payload)
		})
	})

	t.Run("a redraw that comes due inside a paste answers nothing, and redraws once it ends", func(t *testing.T) {
		for _, tc := range []struct {
			name     string
			arriving bool
		}{
			{"started with the pane quiet", false},
			{"started with input still arriving", true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				start := drawnPayload()
				start.InputArriving = tc.arriving
				h := startArrivals(t, start)

				h.arrive("echo o")
				h.resizeSettled()
				h.arrive("ne\r")
				h.quiet()

				h.answered()
				want := start
				want.InputArriving = false
				assertHandOff(t, h.probe, want)
				assertNoStateTouched(t, h.probe)
			})
		}
	})

	t.Run("a redraw once the pane has gone quiet hands no arrival on", func(t *testing.T) {
		start := drawnPayload()
		start.InputArriving = true
		h := startArrivals(t, start)

		h.arrive("echo o")
		h.quiet()
		h.resizeSettled()

		h.answered()
		want := start
		want.InputArriving = false
		assertHandOff(t, h.probe, want)
	})

	// The redraw an attach's SIGWINCH asks for comes due about as the user
	// reaches for a key: the listen the redraw wakes finds the key, and the key
	// answers as it would have with no redraw due.
	t.Run("a key arriving as a redraw comes due answers, the redraw waiting on the listen it woke", func(t *testing.T) {
		payload := drawnPayload()
		h := startArrivals(t, payload)
		if !h.live.idle {
			t.Fatal("the wait's first listen is for the quiet that ends an arrival, want the idle listen")
		}

		h.redrawDue()
		h.arrive("d")
		h.quiet()

		h.answered()
		assertHandOff(t, h.probe, opened(payload))
	})
}

// redrawDue delivers a size change and elapses the settle window it opens,
// returning once the wait, the redraw due, has woken its listen, and so before
// any answer to the listen in flight.
func (h *arrivalHarness) redrawDue() {
	h.t.Helper()
	h.sizes.deliver(h.winch)
	h.awaitSettle() <- time.Now()
	select {
	case <-h.wakes:
	case <-time.After(resumeArrivalTestBudget):
		h.t.Fatal("the redraw came due and the wait never woke its listen")
	}
}

// resizeSettled makes a redraw due, the listen in flight ending as the wake
// ends it: with no arrival open, at once, and inside one, at the quiet that
// ends it, which is the test's to deliver.
func (h *arrivalHarness) resizeSettled() {
	h.t.Helper()
	h.redrawDue()
	if h.live != nil && h.live.idle {
		h.answerListen(false)
	}
}

func (h *arrivalHarness) awaitSettle() chan time.Time {
	h.t.Helper()
	select {
	case timer := <-h.settles:
		return timer
	case <-time.After(resumeArrivalTestBudget):
		h.t.Fatal("the wait armed no settle window for the size change")
	}
	return nil
}

func TestResumeInputArriving_Draw(t *testing.T) {
	t.Run("it hands the waiter the input the drop could not discard", func(t *testing.T) {
		for _, dropErr := range []error{errInputKeptArriving, errUnselectableTTY, errors.New("ioctl refused")} {
			t.Run(dropErr.Error(), func(t *testing.T) {
				d := runDropDraw(t, openingPayload(), dropErr)

				if !slices.Contains(d.execArgs, flagArg(resumeFlagInputArriving)) {
					t.Errorf("waiter argv = %q, want %s", d.execArgs, flagArg(resumeFlagInputArriving))
				}
			})
		}
	})

	t.Run("it hands the waiter no arrival when the drop succeeds or none was asked for", func(t *testing.T) {
		cases := []struct {
			name    string
			payload resumeChainPayload
		}{
			{"a drop that succeeded", openingPayload()},
			{"a panel draw", samplePayload()},
			{"a report redraw", reportedPayload(resumeScreenPanel)},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				d := runDropDraw(t, tc.payload, nil)

				assertArgvLacks(t, d.execArgs, resumeFlagInputArriving)
			})
		}
	})

	t.Run("it hands on an arrival a waiter from before the upgrade carried to it", func(t *testing.T) {
		payload := drawnPayload()
		payload.InputArriving = true

		d := runDropDraw(t, payload, errors.New("must not be reached"))

		if !slices.Contains(d.execArgs, flagArg(resumeFlagInputArriving)) {
			t.Errorf("waiter argv = %q, want %s", d.execArgs, flagArg(resumeFlagInputArriving))
		}
	})
}

func TestResumeInputArriving_Commands(t *testing.T) {
	for _, subcommand := range []string{resumeDrawSubcommand, resumeWaitSubcommand} {
		t.Run(subcommand+" parses the arrival flag", func(t *testing.T) {
			payload := samplePayload()
			payload.InputArriving = true

			var got resumeChainPayload
			withFuncSeam(t, &resumeDrawRunFunc, func(cfg resumeDrawConfig) error {
				got = cfg.resumeChainPayload
				return nil
			})
			withFuncSeam(t, &resumeWaitRunFunc, func(cfg resumeWaitConfig) error {
				got = cfg.resumeChainPayload
				return nil
			})

			executeChainArgv(t, resumeChainArgv("portal", subcommand, payload)[1:])
			if got != payload {
				t.Errorf("payload = %+v, want %+v", got, payload)
			}
		})
	}
}

func executeChainArgv(t *testing.T, args []string) {
	t.Helper()
	resetRootCmd()
	rootCmd.SetOut(new(bytes.Buffer))
	errBuf := new(bytes.Buffer)
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs(args)
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("executing the composed argv: %v\nstderr: %s", err, errBuf)
	}
}

// typeAhead is input a user is still typing: bytes join it while the wait runs,
// and a byte nobody has read stays for whichever program reads the pane next.
// It answers the wait's listens as a terminal does, the one with no arrival
// open with no timeout.
type typeAhead struct {
	mu     sync.Mutex
	cond   *sync.Cond
	bytes  []byte
	closed bool
	woken  bool

	// listening is signalled, never blocking, as each listen begins.
	listening chan struct{}
	// listens counts the listens in flight, and idle the listens with no
	// arrival open the wait has begun.
	listens atomic.Int32
	idle    atomic.Int64
}

func newTypeAhead(typed string) *typeAhead {
	q := &typeAhead{bytes: []byte(typed), listening: make(chan struct{}, 1)}
	q.cond = sync.NewCond(&q.mu)
	return q
}

// listenOn hands every listen the wait takes to input.
func listenOn(cfg *resumeWaitConfig, input *typeAhead) {
	cfg.AwaitInput = input.await
	cfg.OpenWaker = func() (resumeWaker, error) { return input, nil }
}

func (q *typeAhead) Read(p []byte) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.bytes) == 0 && !q.closed {
		q.cond.Wait()
	}
	if len(q.bytes) == 0 {
		return 0, io.EOF
	}
	n := copy(p, q.bytes)
	q.bytes = q.bytes[n:]
	return n, nil
}

func (q *typeAhead) typeKey(b byte) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.bytes = append(q.bytes, b)
	q.cond.Broadcast()
}

func (q *typeAhead) typeKeys(keys string) {
	for _, b := range []byte(keys) {
		q.typeKey(b)
	}
}

// end ends the input, as a session's tty hanging up does.
func (q *typeAhead) end() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.closed = true
	q.cond.Broadcast()
}

// arrived answers at once: a key typed alone has nothing behind it yet.
func (q *typeAhead) arrived(time.Duration) (bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.bytes) > 0, nil
}

// listen begins a listen, returning its end.
func (q *typeAhead) listen() (done func()) {
	q.listens.Add(1)
	select {
	case q.listening <- struct{}{}:
	default:
	}
	return func() { q.listens.Add(-1) }
}

// await answers as a terminal's select does: at once for a byte unread or the
// input ended, and otherwise once a key is typed or window has passed.
func (q *typeAhead) await(window time.Duration) (bool, error) {
	defer q.listen()()
	passed := time.AfterFunc(window, func() {
		q.mu.Lock()
		defer q.mu.Unlock()
		q.cond.Broadcast()
	})
	defer passed.Stop()
	deadline := time.Now().Add(window)
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.bytes) == 0 && !q.closed {
		if !time.Now().Before(deadline) {
			return false, nil
		}
		q.cond.Wait()
	}
	return true, nil
}

// Await answers as a select with no timeout over the terminal and the wake
// pipe does: at once for a byte unread or the input ended, and otherwise once
// a key is typed or the wait wakes it, a byte winning over a wake.
func (q *typeAhead) Await() (bool, error) {
	defer q.listen()()
	q.idle.Add(1)
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.bytes) == 0 && !q.closed && !q.woken {
		q.cond.Wait()
	}
	q.woken = false
	return len(q.bytes) > 0 || q.closed, nil
}

func (q *typeAhead) Wake() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.woken = true
	q.cond.Broadcast()
}

func (q *typeAhead) Close() error { return nil }

func (q *typeAhead) unread() string {
	q.mu.Lock()
	defer q.mu.Unlock()
	return string(q.bytes)
}

func TestRunResumeWait_KeysTypedDuringAnAnswer(t *testing.T) {
	const typedNext = 'k'
	cases := []struct {
		name    string
		payload resumeChainPayload
		answer  string
		during  func(cfg *resumeWaitConfig, typeKey func())
	}{
		{"Enter, typed into while the marker clears", samplePayload(), "\r", func(cfg *resumeWaitConfig, typeKey func()) {
			clear := cfg.ClearPending
			cfg.ClearPending = func() error {
				typeKey()
				return clear()
			}
		}},
		{"y, typed into while the registration is removed", confirmationPayload(), "y", func(cfg *resumeWaitConfig, typeKey func()) {
			discard := cfg.DiscardRegistration
			cfg.DiscardRegistration = func(hookKey, shown string) (bool, error) {
				typeKey()
				return discard(hookKey, shown)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name+" leaves the key for the program the pane is handed to", func(t *testing.T) {
			var probe resumeWaitProbe
			input := newTypeAhead(tc.answer)
			cfg := newResumeWaitConfig(t, &probe, tc.payload, input)
			cfg.AwaitInput = input.arrived
			tc.during(&cfg, func() { input.typeKey(typedNext) })

			if err := runResumeWait(cfg); err != nil {
				t.Fatalf("runResumeWait() error = %v", err)
			}

			if probe.execCalls != 1 {
				t.Fatalf("ExecSelf called %d times, want the pane handed on once", probe.execCalls)
			}
			// Any read the wait left behind would take the key within this.
			time.Sleep(20 * time.Millisecond)
			if got := input.unread(); got != string(typedNext) {
				t.Errorf("input left unread after the hand-off = %q, want %q", got, string(typedNext))
			}
		})
	}
}
