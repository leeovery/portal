package cmd

import (
	"errors"
	"io"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

const resumeEscapeTestBudget = 2 * time.Second

// onScreen is the payload a hand-off to screen carries: the key press that
// moves between screens ends any report the waiter was launched holding.
func onScreen(p resumeChainPayload, screen string) resumeChainPayload {
	p.Screen = screen
	p.Report = ""
	return p
}

func reportedPayload(screen string) resumeChainPayload {
	p := samplePayload()
	p.Width, p.Height = 100, 30
	p.Report = "could not remove the resume command"
	p.Screen = screen
	return p
}

// swallowed runs the waiter over input that ends in EOF and asserts it acted on
// none of it: no hand-off, no write, no store or marker touched, and a read
// taken past the input so the loop was still going round when it ran dry.
func swallowed(t *testing.T, payload resumeChainPayload, input string) {
	t.Helper()
	var probe resumeWaitProbe
	reader := keystrokes(t, input)

	err := runResumeWait(queuedKeysConfig(t, &probe, payload, reader))

	if !errors.Is(err, io.EOF) {
		t.Fatalf("runResumeWait() error = %v, want the EOF that ended the wait after the swallowed input", err)
	}
	if want := len(input) + 1; reader.reads != want {
		t.Errorf("the wait read %d times, want %d: every byte and then the EOF", reader.reads, want)
	}
	assertNoHandOff(t, &probe)
	assertNoStateTouched(t, &probe)
}

func assertNoStateTouched(t *testing.T, p *resumeWaitProbe) {
	t.Helper()
	if p.clearCalls != 0 {
		t.Errorf("ClearMarker called %d times, want 0", p.clearCalls)
	}
	if len(p.lookupKeys) != 0 {
		t.Errorf("the store was read for %q, want it never opened", p.lookupKeys)
	}
	if len(p.discardKeys) != 0 {
		t.Errorf("a discard was written for %q, want the store never written", p.discardKeys)
	}
}

// assertDiscardAnswered asserts the confirmation's y reached the discard: one
// removal of the pane's registration and the pane handed to a plain shell.
func assertDiscardAnswered(t *testing.T, p *resumeWaitProbe, payload resumeChainPayload) {
	t.Helper()
	if !slices.Equal(p.discardKeys, []string{payload.HookKey}) {
		t.Errorf("discard written for %q, want one write for %q", p.discardKeys, payload.HookKey)
	}
	assertShellHandOff(t, p)
}

func answered(t *testing.T, payload resumeChainPayload, input string) *resumeWaitProbe {
	t.Helper()
	var probe resumeWaitProbe
	if err := runResumeWait(queuedKeysConfig(t, &probe, payload, keystrokes(t, input))); err != nil {
		t.Fatalf("runResumeWait() error = %v", err)
	}
	return &probe
}

// queuedKeysConfig is for input that is already queued in full, so every byte
// of it is read before any follow window could elapse. The windows never fire,
// leaving nothing raced against the clock, and a resize settle is refused: with
// no size change delivered and the pane at its drawn size, none can start.
func queuedKeysConfig(t *testing.T, p *resumeWaitProbe, payload resumeChainPayload, in io.Reader) resumeWaitConfig {
	t.Helper()
	cfg := newResumeWaitConfig(t, p, payload, in)
	cfg.Settle = func(d time.Duration) <-chan time.Time {
		if d != resumeEscapeFollow {
			t.Errorf("window = %v armed over queued keys, want only the escape follow", d)
		}
		return nil
	}
	return cfg
}

func assertArgvLacks(t *testing.T, argv []string, flag string) {
	t.Helper()
	if slices.Contains(argv, flagArg(flag)) {
		t.Errorf("hand-off argv = %q, want no %s", argv, flagArg(flag))
	}
}

// escapeHarness sends a lone ESC down a pipe and holds the follow window open
// until the test elapses it. The window is handed over unbuffered, so the
// elapse lands only once the loop is selecting on it.
type escapeHarness struct {
	t       *testing.T
	probe   *resumeWaitProbe
	writer  *io.PipeWriter
	reader  *oneByteReader
	windows chan escapeWindow
	done    chan error
}

type escapeWindow struct {
	d    time.Duration
	fire chan time.Time
}

func startBareEscape(t *testing.T, payload resumeChainPayload) *escapeHarness {
	t.Helper()
	pipeReader, pipeWriter := io.Pipe()
	h := &escapeHarness{
		t:       t,
		probe:   new(resumeWaitProbe),
		writer:  pipeWriter,
		reader:  &oneByteReader{t: t, inner: pipeReader},
		windows: make(chan escapeWindow),
		done:    make(chan error, 1),
	}
	t.Cleanup(func() { _ = pipeWriter.Close() })

	cfg := newResumeWaitConfig(t, h.probe, payload, h.reader)
	cfg.Settle = func(d time.Duration) <-chan time.Time {
		w := escapeWindow{d: d, fire: make(chan time.Time)}
		h.windows <- w
		return w.fire
	}

	go func() { h.done <- runResumeWait(cfg) }()
	go func() { _, _ = pipeWriter.Write([]byte{0x1b}) }()
	return h
}

func (h *escapeHarness) elapseFollow() {
	h.t.Helper()
	var w escapeWindow
	select {
	case w = <-h.windows:
	case <-time.After(resumeEscapeTestBudget):
		h.t.Fatal("the wait armed no follow window for the escape")
	}
	if w.d != resumeEscapeFollow {
		h.t.Errorf("follow window = %v, want %v", w.d, resumeEscapeFollow)
	}
	select {
	case w.fire <- time.Now():
	case <-time.After(resumeEscapeTestBudget):
		h.t.Fatal("the wait stopped waiting on the follow window")
	}
}

func (h *escapeHarness) wait() error {
	h.t.Helper()
	select {
	case err := <-h.done:
		return err
	case <-time.After(resumeEscapeTestBudget):
		h.t.Fatal("the wait never ended")
	}
	return nil
}

// backOut runs a bare Escape on the confirmation through to its hand-off.
func backOut(t *testing.T, payload resumeChainPayload) *resumeWaitProbe {
	t.Helper()
	h := startBareEscape(t, payload)
	h.elapseFollow()
	if err := h.wait(); err != nil {
		t.Fatalf("runResumeWait() error = %v", err)
	}
	return h.probe
}

// inertEscape runs a bare Escape on the waiting panel, then ends the input and
// asserts the loop was still reading when it did.
func inertEscape(t *testing.T, payload resumeChainPayload) *resumeWaitProbe {
	t.Helper()
	h := startBareEscape(t, payload)
	h.elapseFollow()
	_ = h.writer.Close()
	if err := h.wait(); !errors.Is(err, io.EOF) {
		t.Fatalf("runResumeWait() error = %v, want the EOF that ended the wait after the inert escape", err)
	}
	if h.reader.reads < 2 {
		t.Errorf("the wait read %d times, want it to read on past the escape", h.reader.reads)
	}
	assertNoHandOff(t, h.probe)
	assertNoStateTouched(t, h.probe)
	return h.probe
}

// pipedKeysHarness feeds keys down a pipe and hands every window the wait arms
// to the test, so a chord, the key after it and a resize are each ordered
// against the loop rather than against the clock. A press returns only once the
// wait has read every byte of it.
type pipedKeysHarness struct {
	t       *testing.T
	probe   *resumeWaitProbe
	writer  *io.PipeWriter
	reader  *trackedReader
	sizes   *paneResize
	winch   chan os.Signal
	follows chan chan time.Time
	settles chan chan time.Time
	done    chan error
}

func startPipedKeys(t *testing.T, payload resumeChainPayload) *pipedKeysHarness {
	t.Helper()
	pipeReader, pipeWriter := io.Pipe()
	h := &pipedKeysHarness{
		t:       t,
		probe:   new(resumeWaitProbe),
		writer:  pipeWriter,
		reader:  &trackedReader{t: t, inner: pipeReader, started: make(chan struct{}, 1)},
		sizes:   &paneResize{drawn: payload, to: resizedSize},
		winch:   make(chan os.Signal, 1),
		follows: make(chan chan time.Time, 8),
		settles: make(chan chan time.Time, 8),
		done:    make(chan error, 1),
	}
	t.Cleanup(func() { _ = pipeWriter.Close() })

	cfg := newResumeWaitConfig(t, h.probe, payload, h.reader)
	cfg.Winch = h.winch
	cfg.Size = h.sizes.size
	cfg.Settle = func(d time.Duration) <-chan time.Time {
		switch d {
		case resumeEscapeFollow:
			// Unbuffered, so a fire lands only while the wait is selecting on
			// it and can never race a key already queued behind it.
			timer := make(chan time.Time)
			h.follows <- timer
			return timer
		case resumeResizeSettle:
			timer := make(chan time.Time, 1)
			h.settles <- timer
			return timer
		default:
			t.Errorf("window = %v, want the escape follow or the resize settle", d)
			return nil
		}
	}

	go func() { h.done <- runResumeWait(cfg) }()
	return h
}

func (h *pipedKeysHarness) press(keys string) {
	h.t.Helper()
	written := make(chan struct{})
	go func() {
		_, _ = h.writer.Write([]byte(keys))
		close(written)
	}()
	select {
	case <-written:
	case <-time.After(resumeEscapeTestBudget):
		h.t.Fatalf("the wait never read all of %q", keys)
	}
}

func (h *pipedKeysHarness) resize() {
	h.sizes.deliver(h.winch)
}

// elapse fires a follow window once the wait is selecting on it.
func (h *pipedKeysHarness) elapse(timer chan time.Time) {
	h.t.Helper()
	select {
	case timer <- time.Now():
	case <-time.After(resumeEscapeTestBudget):
		h.t.Fatal("the wait stopped waiting on the follow window")
	}
}

// awaitWindow takes the next window of a kind the wait armed, failing when it
// arms none: a wait still consuming a sequence arms nothing.
func (h *pipedKeysHarness) awaitWindow(windows chan chan time.Time, what string) chan time.Time {
	h.t.Helper()
	select {
	case timer := <-windows:
		return timer
	case <-time.After(resumeEscapeTestBudget):
		h.t.Fatalf("the wait armed no %s", what)
	}
	return nil
}

func (h *pipedKeysHarness) wait() error {
	h.t.Helper()
	select {
	case err := <-h.done:
		return err
	case <-time.After(resumeEscapeTestBudget):
		h.t.Fatal("the wait never ended")
	}
	return nil
}

func TestRunResumeWait_Screens(t *testing.T) {
	t.Run("it opens the confirmation on d", func(t *testing.T) {
		payload := reportedPayload(resumeScreenPanel)

		probe := answered(t, payload, "d")

		assertHandOff(t, probe, opened(payload))
		assertArgvLacks(t, probe.execArgs, resumeFlagReport)
		assertNoStateTouched(t, probe)
	})

	t.Run("it backs out to the waiting panel on Escape", func(t *testing.T) {
		payload := reportedPayload(resumeScreenDiscard)

		probe := backOut(t, payload)

		assertHandOff(t, probe, onScreen(payload, resumeScreenPanel))
		assertArgvLacks(t, probe.execArgs, resumeFlagScreen)
		assertArgvLacks(t, probe.execArgs, resumeFlagReport)
		assertNoStateTouched(t, probe)
	})

	t.Run("it swallows Enter on the confirmation", func(t *testing.T) {
		for _, key := range []string{"\r", "\n"} {
			t.Run(keyName(key), func(t *testing.T) {
				swallowed(t, reportedPayload(resumeScreenDiscard), key)
			})
		}
	})

	t.Run("it swallows d on the confirmation", func(t *testing.T) {
		swallowed(t, reportedPayload(resumeScreenDiscard), "d")
	})

	t.Run("it swallows uppercase Y on the confirmation", func(t *testing.T) {
		swallowed(t, reportedPayload(resumeScreenDiscard), "Y")
	})

	t.Run("it leaves Escape inert on the waiting panel", func(t *testing.T) {
		inertEscape(t, reportedPayload(resumeScreenPanel))
	})

	t.Run("it swallows an escape sequence on the confirmation", func(t *testing.T) {
		for _, seq := range []string{"\x1b[A", "\x1b[3~", "\x1bOD"} {
			t.Run(strings.TrimPrefix(seq, "\x1b"), func(t *testing.T) {
				payload := reportedPayload(resumeScreenDiscard)
				swallowed(t, payload, seq)

				assertDiscardAnswered(t, answered(t, payload, seq+"y"), payload)
			})
		}
	})

	t.Run("it swallows an escape sequence on the waiting panel", func(t *testing.T) {
		for _, seq := range []string{
			"\x1b[A",
			"\x1b[3~",
			"\x1bOD",
			"\x1b[" + strings.Repeat("1", resumeEscapeSequenceCap-2),
			"\x1bO" + strings.Repeat("1", resumeEscapeSequenceCap-2),
			"\x1b]" + strings.Repeat("a", resumeOSCSequenceCap-2),
		} {
			t.Run(strings.TrimPrefix(seq, "\x1b"), func(t *testing.T) {
				payload := reportedPayload(resumeScreenPanel)
				swallowed(t, payload, seq)

				assertHandOff(t, answered(t, payload, seq+"d"), opened(payload))
			})
		}
	})

	t.Run("it swallows a sequence whose final byte is an acting key", func(t *testing.T) {
		cases := []struct {
			name   string
			screen string
			seq    string
		}{
			{"y on the confirmation", resumeScreenDiscard, "\x1b[?1;2y"},
			{"d on the waiting panel", resumeScreenPanel, "\x1b[5d"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				swallowed(t, reportedPayload(tc.screen), tc.seq)
			})
		}
	})

	t.Run("it swallows an SS3 whose final byte is an acting key", func(t *testing.T) {
		cases := []struct {
			name   string
			screen string
			seq    string
		}{
			{"y on the confirmation", resumeScreenDiscard, "\x1bOy"},
			{"d on the waiting panel", resumeScreenPanel, "\x1bOd"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				swallowed(t, reportedPayload(tc.screen), tc.seq)
			})
		}

		payload := reportedPayload(resumeScreenDiscard)
		assertDiscardAnswered(t, answered(t, payload, "\x1bOy"+"y"), payload)
	})

	t.Run("it swallows a terminal background-colour reply whole", func(t *testing.T) {
		// The 2d2d reply carries a d beyond the CSI cap, reachable only if the
		// OSC is cut short of its own terminator.
		replies := []string{
			"\x1b]11;rgb:1d1d/1f1f/2121",
			"\x1b]11;rgb:2d2d/2d2d/2d2d",
		}
		terminators := []struct {
			name string
			end  string
		}{
			{"BEL", "\x07"},
			{"ST", "\x1b\\"},
		}
		for _, screen := range []string{resumeScreenPanel, resumeScreenDiscard} {
			for _, reply := range replies {
				for _, term := range terminators {
					t.Run(screenName(screen)+"/"+reply[len(reply)-4:]+"/"+term.name, func(t *testing.T) {
						payload := reportedPayload(screen)
						swallowed(t, payload, reply+term.end)

						if screen == resumeScreenDiscard {
							assertDiscardAnswered(t, answered(t, payload, reply+term.end+"y"), payload)
							return
						}
						assertHandOff(t, answered(t, payload, reply+term.end+"d"), opened(payload))
					})
				}
			}
		}
	})

	t.Run("it backs out on a bare Escape", func(t *testing.T) {
		payload := reportedPayload(resumeScreenDiscard)
		probe := backOut(t, payload)
		assertHandOff(t, probe, onScreen(payload, resumeScreenPanel))

		inertEscape(t, reportedPayload(resumeScreenPanel))
	})

	t.Run("it swallows a sequence longer than the byte cap", func(t *testing.T) {
		// Each sequence fills its cap without a terminator, so a y after it
		// is only reachable by a loop that stopped consuming at the cap.
		cases := []struct {
			name string
			seq  string
		}{
			{"CSI", "\x1b[" + strings.Repeat("1", resumeEscapeSequenceCap-2)},
			{"SS3", "\x1bO" + strings.Repeat("1", resumeEscapeSequenceCap-2)},
			{"OSC", "\x1b]" + strings.Repeat("a", resumeOSCSequenceCap-2)},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				payload := reportedPayload(resumeScreenDiscard)
				swallowed(t, payload, tc.seq)

				assertDiscardAnswered(t, answered(t, payload, tc.seq+"y"), payload)
			})
		}
	})

	t.Run("it swallows an acting key at the last byte inside the cap", func(t *testing.T) {
		cases := []struct {
			name   string
			screen string
			seq    string
		}{
			{"CSI y on the confirmation", resumeScreenDiscard, "\x1b[" + strings.Repeat("1", resumeEscapeSequenceCap-3) + "y"},
			{"OSC d on the panel", resumeScreenPanel, "\x1b]" + strings.Repeat("a", resumeOSCSequenceCap-3) + "d"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				swallowed(t, reportedPayload(tc.screen), tc.seq)
			})
		}
	})

	t.Run("it resumes on the Enter after a two-byte chord on the waiting panel", func(t *testing.T) {
		payload := reportedPayload(resumeScreenPanel)
		probe := resumeWaitProbe{lookup: foundHook(payload.Command)}

		if err := runResumeWait(queuedKeysConfig(t, &probe, payload, keystrokes(t, "\x1bb\r"))); err != nil {
			t.Fatalf("runResumeWait() error = %v", err)
		}

		if probe.clearCalls != 1 {
			t.Errorf("ClearMarker called %d times, want 1", probe.clearCalls)
		}
		if !slices.Equal(probe.lookupKeys, []string{payload.HookKey}) {
			t.Errorf("the store was read for %q, want one read for %q", probe.lookupKeys, payload.HookKey)
		}
		assertHookHandOff(t, &probe, payload.Command)
	})

	t.Run("it confirms on the y after a two-byte chord on the confirmation", func(t *testing.T) {
		payload := reportedPayload(resumeScreenDiscard)

		assertDiscardAnswered(t, answered(t, payload, "\x1bxy"), payload)
	})

	t.Run("it backs out on a lone Escape after an Escape pair on the confirmation", func(t *testing.T) {
		payload := reportedPayload(resumeScreenDiscard)
		h := startPipedKeys(t, payload)

		h.press("\x1b\x1b")
		h.awaitWindow(h.follows, "follow window for the pair's Escape")
		h.press("\x1b")
		h.elapse(h.awaitWindow(h.follows, "follow window for the lone Escape"))

		if err := h.wait(); err != nil {
			t.Fatalf("runResumeWait() error = %v", err)
		}
		assertHandOff(t, h.probe, onScreen(payload, resumeScreenPanel))
		assertNoStateTouched(t, h.probe)
	})

	t.Run("it swallows a two-byte chord of an acting key", func(t *testing.T) {
		cases := []struct {
			name   string
			screen string
			seq    string
		}{
			{"y on the confirmation", resumeScreenDiscard, "\x1by"},
			{"d on the waiting panel", resumeScreenPanel, "\x1bd"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				swallowed(t, reportedPayload(tc.screen), tc.seq)
			})
		}
	})

	t.Run("it redraws on a resize that settles after a two-byte chord", func(t *testing.T) {
		payload := drawnPayload()
		h := startPipedKeys(t, payload)

		h.press("\x1bb")
		h.awaitWindow(h.follows, "follow window for the chord's Escape")
		h.resize()
		h.awaitWindow(h.settles, "settle window for the resize") <- time.Now()

		if err := h.wait(); err != nil {
			t.Fatalf("runResumeWait() error = %v", err)
		}
		assertHandOff(t, h.probe, payload)
		assertNoStateTouched(t, h.probe)
	})

	t.Run("it ends a chord of a string-introducer key at two bytes", func(t *testing.T) {
		t.Run("P then Enter on the waiting panel", func(t *testing.T) {
			payload := reportedPayload(resumeScreenPanel)
			probe := resumeWaitProbe{lookup: foundHook(payload.Command)}

			if err := runResumeWait(queuedKeysConfig(t, &probe, payload, keystrokes(t, "\x1bP\r"))); err != nil {
				t.Fatalf("runResumeWait() error = %v", err)
			}

			if probe.clearCalls != 1 {
				t.Errorf("ClearMarker called %d times, want 1", probe.clearCalls)
			}
			assertHookHandOff(t, &probe, payload.Command)
		})

		for _, seq := range []string{"\x1b_y", "\x1bXy"} {
			t.Run(seq[1:2]+" then y on the confirmation", func(t *testing.T) {
				payload := reportedPayload(resumeScreenDiscard)

				assertDiscardAnswered(t, answered(t, payload, seq), payload)
			})
		}
	})

	t.Run("it swallows every non-acting key on both screens", func(t *testing.T) {
		shared := []string{"\x03", "\x04", "\x1a", "\x1b", "D", "q", "a", " ", "cat README"}
		perScreen := map[string][]string{
			resumeScreenPanel:   append(slices.Clone(shared), "y", "Y"),
			resumeScreenDiscard: append(slices.Clone(shared), "\r", "\n", "d", "Y"),
		}
		for _, screen := range []string{resumeScreenPanel, resumeScreenDiscard} {
			for _, key := range perScreen[screen] {
				t.Run(screenName(screen)+"/"+keyName(key), func(t *testing.T) {
					swallowed(t, reportedPayload(screen), key)
				})
			}
		}
	})

	t.Run("it clears the report on both hand-offs", func(t *testing.T) {
		cases := []struct {
			name     string
			run      func(t *testing.T, payload resumeChainPayload) *resumeWaitProbe
			from     string
			wantTo   string
			wantDrop bool
		}{
			{
				name:     "d from a reported panel",
				run:      func(t *testing.T, p resumeChainPayload) *resumeWaitProbe { return answered(t, p, "d") },
				from:     resumeScreenPanel,
				wantTo:   resumeScreenDiscard,
				wantDrop: true,
			},
			{
				name:   "Escape from a reported confirmation",
				run:    backOut,
				from:   resumeScreenDiscard,
				wantTo: resumeScreenPanel,
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				payload := reportedPayload(tc.from)

				probe := tc.run(t, payload)

				want := onScreen(payload, tc.wantTo)
				want.DropInput = tc.wantDrop
				assertHandOff(t, probe, want)
				assertArgvLacks(t, probe.execArgs, resumeFlagReport)
			})
		}
	})

	t.Run("it restores the terminal before every hand-off", func(t *testing.T) {
		cases := []struct {
			name string
			run  func(t *testing.T) *resumeWaitProbe
		}{
			{"Enter on the panel", func(t *testing.T) *resumeWaitProbe {
				return answered(t, reportedPayload(resumeScreenPanel), "\r")
			}},
			{"d on the panel", func(t *testing.T) *resumeWaitProbe {
				return answered(t, reportedPayload(resumeScreenPanel), "d")
			}},
			{"y on the confirmation", func(t *testing.T) *resumeWaitProbe {
				return answered(t, reportedPayload(resumeScreenDiscard), "y")
			}},
			{"Escape on the confirmation", func(t *testing.T) *resumeWaitProbe {
				return backOut(t, reportedPayload(resumeScreenDiscard))
			}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				probe := tc.run(t)

				if probe.execCalls != 1 {
					t.Fatalf("ExecSelf called %d times, want exactly 1", probe.execCalls)
				}
				if probe.restoredAtExec != 1 {
					t.Errorf("%d restores had run when the hand-off exec'd, want 1: the next process image must inherit a cooked tty", probe.restoredAtExec)
				}
			})
		}
	})

	t.Run("it acts on both keys at a size below the card's", func(t *testing.T) {
		sizes := []struct {
			name string
			w, h int
		}{
			{"a single cell", 1, 1},
			{"narrower than the card", 20, 6},
			{"unmeasured", 0, 0},
			{"negative", -4, -2},
		}
		for _, size := range sizes {
			sized := func(screen string) resumeChainPayload {
				p := reportedPayload(screen)
				p.Width, p.Height = size.w, size.h
				return p
			}
			t.Run(size.name+"/panel/Enter", func(t *testing.T) {
				var probe resumeWaitProbe
				payload := sized(resumeScreenPanel)
				probe.lookup = foundHook(payload.Command)
				if err := runResumeWait(newResumeWaitConfig(t, &probe, payload, keystrokes(t, "\r"))); err != nil {
					t.Fatalf("runResumeWait() error = %v", err)
				}
				assertHookHandOff(t, &probe, payload.Command)
			})
			t.Run(size.name+"/panel/d", func(t *testing.T) {
				payload := sized(resumeScreenPanel)
				assertHandOff(t, answered(t, payload, "d"), opened(payload))
			})
			t.Run(size.name+"/discard/y", func(t *testing.T) {
				payload := sized(resumeScreenDiscard)
				assertDiscardAnswered(t, answered(t, payload, "y"), payload)
			})
			t.Run(size.name+"/discard/Escape", func(t *testing.T) {
				payload := sized(resumeScreenDiscard)
				assertHandOff(t, backOut(t, payload), onScreen(payload, resumeScreenPanel))
			})
		}
	})

	t.Run("it answers the confirmation's y with the discard", func(t *testing.T) {
		payload := reportedPayload(resumeScreenDiscard)

		assertDiscardAnswered(t, answered(t, payload, "y"), payload)
	})
}

// introducerChords are the Alt chords whose second byte opens a CSI, SS3 or OSC.
var introducerChords = []struct {
	name  string
	chord string
}{
	{"Alt-[", "\x1b["},
	{"Alt-Shift-O", "\x1bO"},
	{"Alt-]", "\x1b]"},
}

// chord presses keys that open a sequence and elapses the window its first
// byte after the introducer is read under, leaving that read outstanding.
func (h *pipedKeysHarness) chord(keys string) {
	h.t.Helper()
	h.press(keys)
	h.awaitWindow(h.follows, "follow window for the chord's Escape")
	h.elapse(h.awaitWindow(h.follows, "follow window for the byte after the introducer"))
}

func TestRunResumeWait_IntroducerChords(t *testing.T) {
	t.Run("it resumes on the Enter after an introducer chord on the waiting panel", func(t *testing.T) {
		for _, tc := range introducerChords {
			t.Run(tc.name, func(t *testing.T) {
				payload := reportedPayload(resumeScreenPanel)
				h := startPipedKeys(t, payload)
				h.probe.lookup = foundHook(payload.Command)

				h.chord(tc.chord)
				h.press("\r")

				if err := h.wait(); err != nil {
					t.Fatalf("runResumeWait() error = %v", err)
				}
				if h.probe.clearCalls != 1 {
					t.Errorf("ClearMarker called %d times, want 1", h.probe.clearCalls)
				}
				assertHookHandOff(t, h.probe, payload.Command)
			})
		}
	})

	t.Run("it backs out on a lone Escape after Alt-] on the confirmation", func(t *testing.T) {
		payload := reportedPayload(resumeScreenDiscard)
		h := startPipedKeys(t, payload)

		h.chord("\x1b]")
		h.press("\x1b")
		h.elapse(h.awaitWindow(h.follows, "follow window for the lone Escape"))

		if err := h.wait(); err != nil {
			t.Fatalf("runResumeWait() error = %v", err)
		}
		assertHandOff(t, h.probe, onScreen(payload, resumeScreenPanel))
		assertNoStateTouched(t, h.probe)
	})

	t.Run("it confirms on the y after Alt-[ on the confirmation", func(t *testing.T) {
		payload := reportedPayload(resumeScreenDiscard)
		h := startPipedKeys(t, payload)

		h.chord("\x1b[")
		h.press("y")

		if err := h.wait(); err != nil {
			t.Fatalf("runResumeWait() error = %v", err)
		}
		assertDiscardAnswered(t, h.probe, payload)
	})

	t.Run("it redraws on a resize delivered after Alt-[", func(t *testing.T) {
		payload := drawnPayload()
		h := startPipedKeys(t, payload)

		h.press("\x1b[")
		h.awaitWindow(h.follows, "follow window for the chord's Escape")
		follow := h.awaitWindow(h.follows, "follow window for the byte after the introducer")
		h.resize()
		h.elapse(follow)
		h.awaitWindow(h.settles, "settle window for the resize") <- time.Now()

		if err := h.wait(); err != nil {
			t.Fatalf("runResumeWait() error = %v", err)
		}
		assertHandOff(t, h.probe, payload)
		assertNoStateTouched(t, h.probe)
	})

	t.Run("it dispatches the key after a cut-short sequence from the read already outstanding", func(t *testing.T) {
		payload := reportedPayload(resumeScreenPanel)
		h := startPipedKeys(t, payload)

		h.chord("\x1b[")
		h.press("d")

		if err := h.wait(); err != nil {
			t.Fatalf("runResumeWait() error = %v", err)
		}
		assertHandOff(t, h.probe, opened(payload))
		if got := h.reader.overlaps.Load(); got != 0 {
			t.Errorf("%d reads overlapped another; input the hand-off should inherit is stranded in a buffer", got)
		}
		if got := h.reader.reads.Load(); got != 3 {
			t.Errorf("the wait read %d times, want 3: the Escape, the introducer, and the read the window left outstanding", got)
		}
	})
}

func screenName(screen string) string {
	if screen == resumeScreenDiscard {
		return "discard"
	}
	return "panel"
}
