package cmd

import (
	"errors"
	"io"
	"os"
	"slices"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

const resumeResizeTestBudget = 2 * time.Second

// trackedReader proves the loop never reads ahead: it counts every read and
// flags any two that overlap, so a byte pulled into a buffer the loop did not
// dispatch fails here rather than going missing from the next process image.
type trackedReader struct {
	t        *testing.T
	inner    io.Reader
	reads    atomic.Int64
	inFlight atomic.Int32
	overlaps atomic.Int64

	// Signalled non-blockingly so a caller counting reads can order itself
	// against one starting: taking the request off the channel is not yet the
	// read it leads to.
	started chan struct{}
}

func (r *trackedReader) Read(p []byte) (int, error) {
	if len(p) != 1 {
		r.t.Errorf("wait read into a %d-byte buffer, want 1: reading ahead strands input the hand-off should inherit", len(p))
	}
	select {
	case r.started <- struct{}{}:
	default:
	}
	r.reads.Add(1)
	if r.inFlight.Add(1) > 1 {
		r.overlaps.Add(1)
	}
	defer r.inFlight.Add(-1)
	return r.inner.Read(p)
}

// resumeResizeHarness drives the wait's resize seams as rendezvous channels, so
// every assertion is ordered against the loop rather than against the clock.
type resumeResizeHarness struct {
	t       *testing.T
	probe   *resumeWaitProbe
	reader  *trackedReader
	winch   chan os.Signal
	settles chan chan time.Time
	armed   atomic.Int64
	sizes   atomic.Int64

	// A settle window that elapsed is observed here rather than inferred from a
	// later rendezvous: the loop's select is free to take a winch arriving
	// alongside a fired timer first.
	sized  chan struct{}
	writer *io.PipeWriter
	done   chan error

	// The read the wait takes once at start-up is kept out of sizes and sized,
	// which count the reads a settle window's elapse takes.
	startupRead atomic.Bool
	startedUp   chan struct{}
}

// startResumeResize starts a wait whose start-up size read answers the size the
// panel was drawn at, and every later read answers size.
func startResumeResize(t *testing.T, payload resumeChainPayload, size func() (int, int, error)) *resumeResizeHarness {
	t.Helper()
	return startResumeResizeFrom(t, payload, fixedSize(payload.Width, payload.Height), size)
}

// startResumeResizeFrom returns only once the wait has taken its start-up size
// read, so no size change can be delivered before it.
func startResumeResizeFrom(t *testing.T, payload resumeChainPayload, startup, size func() (int, int, error)) *resumeResizeHarness {
	t.Helper()
	pipeReader, pipeWriter := io.Pipe()
	h := &resumeResizeHarness{
		t:         t,
		probe:     new(resumeWaitProbe),
		reader:    &trackedReader{t: t, inner: pipeReader, started: make(chan struct{}, 1)},
		winch:     make(chan os.Signal, 1),
		settles:   make(chan chan time.Time),
		writer:    pipeWriter,
		done:      make(chan error, 1),
		sized:     make(chan struct{}, 1),
		startedUp: make(chan struct{}),
	}
	t.Cleanup(func() { _ = pipeWriter.Close() })

	cfg := newResumeWaitConfig(t, h.probe, payload, h.reader)
	cfg.Winch = h.winch
	cfg.Settle = func(d time.Duration) <-chan time.Time {
		if d != resumeResizeSettle {
			t.Errorf("settle window = %v, want %v", d, resumeResizeSettle)
		}
		h.armed.Add(1)
		timer := make(chan time.Time, 1)
		h.settles <- timer
		return timer
	}
	cfg.Size = func() (int, int, error) {
		if !h.startupRead.Swap(true) {
			defer close(h.startedUp)
			return startup()
		}
		h.sizes.Add(1)
		// Never blocking: the redraw path reads the size and execs, so a test
		// that never receives must not hold the hand-off up.
		select {
		case h.sized <- struct{}{}:
		default:
		}
		return size()
	}

	go func() { h.done <- runResumeWait(cfg) }()
	select {
	case <-h.startedUp:
	case <-time.After(resumeResizeTestBudget):
		t.Fatal("the wait never read the pane size at start-up")
	}
	return h
}

// window takes the next settle window the wait arms of its own accord.
func (h *resumeResizeHarness) window() chan time.Time {
	h.t.Helper()
	select {
	case timer := <-h.settles:
		return timer
	case <-time.After(resumeResizeTestBudget):
		h.t.Fatal("the wait armed no settle window")
	}
	return nil
}

// resize delivers one size change and hands back the settle window it armed.
// Receiving that window is also the proof the loop is still going round, so a
// wait that ended is caught here instead of on a later assertion.
func (h *resumeResizeHarness) resize() chan time.Time {
	h.t.Helper()
	select {
	case h.winch <- syscall.SIGWINCH:
	case <-time.After(resumeResizeTestBudget):
		h.t.Fatal("the wait never took the size change")
	}
	select {
	case timer := <-h.settles:
		return timer
	case <-time.After(resumeResizeTestBudget):
		h.t.Fatal("the wait armed no settle window for the size change")
	}
	return nil
}

func (h *resumeResizeHarness) awaitSize() {
	h.t.Helper()
	select {
	case <-h.sized:
	case <-time.After(resumeResizeTestBudget):
		h.t.Fatal("the settle window elapsed without the wait reading the pane size")
	}
}

func (h *resumeResizeHarness) awaitRead() {
	h.t.Helper()
	select {
	case <-h.reader.started:
	case <-time.After(resumeResizeTestBudget):
		h.t.Fatal("the wait never started a read")
	}
}

func (h *resumeResizeHarness) elapse(timer chan time.Time) {
	h.t.Helper()
	timer <- time.Now()
}

func (h *resumeResizeHarness) press(key string) {
	go func() { _, _ = h.writer.Write([]byte(key)) }()
}

func (h *resumeResizeHarness) wait() error {
	h.t.Helper()
	select {
	case err := <-h.done:
		return err
	case <-time.After(resumeResizeTestBudget):
		h.t.Fatal("the wait never ended")
	}
	return nil
}

func drawnPayload() resumeChainPayload {
	payload := samplePayload()
	payload.Width, payload.Height = 80, 24
	return payload
}

func resizedSize() (int, int, error) { return 120, 40, nil }

// paneResize answers the size the panel was drawn at until a size change is
// delivered, and the resized size from then on.
type paneResize struct {
	drawn     resumeChainPayload
	to        func() (int, int, error)
	delivered atomic.Bool
}

func (r *paneResize) size() (int, int, error) {
	if !r.delivered.Load() {
		return r.drawn.Width, r.drawn.Height, nil
	}
	return r.to()
}

func (r *paneResize) deliver(winch chan<- os.Signal) {
	r.delivered.Store(true)
	winch <- syscall.SIGWINCH
}

func TestRunResumeWait_Resize(t *testing.T) {
	t.Run("it draws once for a burst of size changes", func(t *testing.T) {
		payload := drawnPayload()
		h := startResumeResize(t, payload, resizedSize)

		var last chan time.Time
		for range 10 {
			last = h.resize()
		}
		h.elapse(last)

		if err := h.wait(); err != nil {
			t.Fatalf("runResumeWait() error = %v", err)
		}
		if got := h.armed.Load(); got != 10 {
			t.Errorf("the wait armed %d settle windows for ten size changes, want 10", got)
		}
		if got := h.sizes.Load(); got != 1 {
			t.Errorf("the wait read the pane size %d times for one burst, want 1", got)
		}
		assertHandOff(t, h.probe, payload)
	})

	t.Run("it restarts the settle window for a change arriving inside it", func(t *testing.T) {
		payload := drawnPayload()
		h := startResumeResize(t, payload, resizedSize)

		first := h.resize()
		h.resize()
		h.elapse(first)

		latest := h.resize()
		if got := h.sizes.Load(); got != 0 {
			t.Fatalf("the wait read the pane size %d times after a re-armed window's first timer fired, want 0", got)
		}

		h.elapse(latest)
		if err := h.wait(); err != nil {
			t.Fatalf("runResumeWait() error = %v", err)
		}
		if got := h.armed.Load(); got != 3 {
			t.Errorf("the wait armed %d settle windows for three size changes, want 3", got)
		}
		if got := h.sizes.Load(); got != 1 {
			t.Errorf("the wait read the pane size %d times, want 1", got)
		}
		assertHandOff(t, h.probe, payload)
	})

	t.Run("it redraws nothing when the settled size matches the drawn size", func(t *testing.T) {
		payload := drawnPayload()
		h := startResumeResize(t, payload, func() (int, int, error) {
			return payload.Width, payload.Height, nil
		})

		h.elapse(h.resize())
		h.awaitSize()
		h.resize()

		if got := h.sizes.Load(); got != 1 {
			t.Errorf("the wait read the pane size %d times, want 1", got)
		}
		assertNoHandOff(t, h.probe)
		if h.probe.restores != 0 {
			t.Errorf("terminal restored %d times for a size that did not change, want 0", h.probe.restores)
		}
	})

	t.Run("it keeps waiting after a no-op settle", func(t *testing.T) {
		payload := drawnPayload()
		h := startResumeResize(t, payload, func() (int, int, error) {
			return payload.Width, payload.Height, nil
		})

		h.elapse(h.resize())
		h.awaitSize()
		h.press("d")

		if err := h.wait(); err != nil {
			t.Fatalf("runResumeWait() error = %v", err)
		}
		assertHandOff(t, h.probe, opened(payload))
	})

	t.Run("it hands over to a redraw when the settled size differs", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			w, h int
		}{
			{"width alone", 120, 24},
			{"height alone", 80, 40},
			{"both", 120, 40},
		} {
			t.Run(tc.name, func(t *testing.T) {
				payload := drawnPayload()
				h := startResumeResize(t, payload, func() (int, int, error) {
					return tc.w, tc.h, nil
				})

				h.elapse(h.resize())

				if err := h.wait(); err != nil {
					t.Fatalf("runResumeWait() error = %v", err)
				}
				assertHandOff(t, h.probe, payload)
				if h.probe.restoredAtExec != 1 {
					t.Errorf("%d restores had run when the redraw exec'd, want 1: the next process image must inherit a cooked tty", h.probe.restoredAtExec)
				}
			})
		}
	})

	t.Run("it carries the command and the report across a redraw", func(t *testing.T) {
		payload := drawnPayload()
		payload.Report = "could not clear the pending marker"
		h := startResumeResize(t, payload, resizedSize)

		h.elapse(h.resize())

		if err := h.wait(); err != nil {
			t.Fatalf("runResumeWait() error = %v", err)
		}
		assertHandOff(t, h.probe, payload)
	})

	t.Run("it redraws the confirmation on a settled resize", func(t *testing.T) {
		payload := drawnPayload()
		payload.Screen = resumeScreenDiscard
		payload.Report = "could not remove the resume command"
		h := startResumeResize(t, payload, resizedSize)

		h.elapse(h.resize())

		if err := h.wait(); err != nil {
			t.Fatalf("runResumeWait() error = %v", err)
		}
		assertHandOff(t, h.probe, payload)
		args := h.probe.execArgs
		if !slices.Contains(args, payload.Report) {
			t.Errorf("redraw argv = %q, want it to carry the report %q", args, payload.Report)
		}
		if len(args) < 2 || args[len(args)-2] != flagArg(resumeFlagScreen) || args[len(args)-1] != resumeScreenDiscard {
			t.Errorf("redraw argv = %q, want it to end with --%s %s", args, resumeFlagScreen, resumeScreenDiscard)
		}
	})

	t.Run("it dispatches a key pressed while the settle window is open", func(t *testing.T) {
		payload := drawnPayload()
		h := startResumeResize(t, payload, resizedSize)

		h.resize()
		h.press("d")

		if err := h.wait(); err != nil {
			t.Fatalf("runResumeWait() error = %v", err)
		}
		assertHandOff(t, h.probe, opened(payload))
		if got := h.sizes.Load(); got != 0 {
			t.Errorf("the wait read the pane size %d times, want 0: the key answered before the window elapsed", got)
		}
	})

	t.Run("it redraws at the bounded fallback when the size read fails", func(t *testing.T) {
		payload := drawnPayload()
		h := startResumeResize(t, payload, func() (int, int, error) {
			return 0, 0, errors.New("inappropriate ioctl for device")
		})

		h.elapse(h.resize())

		if err := h.wait(); err != nil {
			t.Fatalf("runResumeWait() error = %v, want a redraw rather than an ended wait", err)
		}
		assertHandOff(t, h.probe, payload)
	})

	t.Run("it keeps one read outstanding at a time", func(t *testing.T) {
		payload := drawnPayload()
		h := startResumeResize(t, payload, func() (int, int, error) {
			return payload.Width, payload.Height, nil
		})

		h.elapse(h.resize())
		h.awaitSize()
		h.resize()

		h.awaitRead()
		if got := h.reader.reads.Load(); got != 1 {
			t.Errorf("the wait started %d reads while one was still outstanding, want 1", got)
		}

		h.press("d")
		if err := h.wait(); err != nil {
			t.Fatalf("runResumeWait() error = %v", err)
		}
		if got := h.reader.overlaps.Load(); got != 0 {
			t.Errorf("%d reads overlapped another; input the hand-off should inherit is stranded in a buffer", got)
		}
		if got := h.reader.reads.Load(); got != 1 {
			t.Errorf("the wait read %d times, want 1: the dispatched byte ends the wait", got)
		}
	})
}

func failedSize() (int, int, error) {
	return 0, 0, errors.New("inappropriate ioctl for device")
}

func TestRunResumeWait_StartupSize(t *testing.T) {
	t.Run("it redraws once for a size change it missed before it started", func(t *testing.T) {
		payload := drawnPayload()
		h := startResumeResizeFrom(t, payload, resizedSize, resizedSize)

		h.elapse(h.window())

		if err := h.wait(); err != nil {
			t.Fatalf("runResumeWait() error = %v", err)
		}
		if got := h.armed.Load(); got != 1 {
			t.Errorf("the wait armed %d settle windows, want 1", got)
		}
		if got := h.sizes.Load(); got != 1 {
			t.Errorf("the wait read the pane size %d times when the window elapsed, want 1", got)
		}
		assertHandOff(t, h.probe, payload)
	})

	t.Run("it redraws the confirmation it started over at a missed size", func(t *testing.T) {
		payload := drawnPayload()
		payload.Screen = resumeScreenDiscard
		payload.Report = "could not remove the resume command"
		h := startResumeResizeFrom(t, payload, resizedSize, resizedSize)

		h.elapse(h.window())

		if err := h.wait(); err != nil {
			t.Fatalf("runResumeWait() error = %v", err)
		}
		assertHandOff(t, h.probe, payload)
	})

	t.Run("it restarts the start-up window for a change arriving inside it", func(t *testing.T) {
		payload := drawnPayload()
		h := startResumeResizeFrom(t, payload, resizedSize, resizedSize)

		startup := h.window()
		h.resize()
		h.elapse(startup)

		latest := h.resize()
		if got := h.sizes.Load(); got != 0 {
			t.Fatalf("the wait read the pane size %d times after the replaced start-up window fired, want 0", got)
		}

		h.elapse(latest)
		if err := h.wait(); err != nil {
			t.Fatalf("runResumeWait() error = %v", err)
		}
		if got := h.armed.Load(); got != 3 {
			t.Errorf("the wait armed %d settle windows, want 3: one at start-up and one per size change", got)
		}
		if got := h.sizes.Load(); got != 1 {
			t.Errorf("the wait read the pane size %d times, want 1", got)
		}
		assertHandOff(t, h.probe, payload)
	})

	t.Run("it dispatches a key pressed while the start-up window is open", func(t *testing.T) {
		payload := drawnPayload()
		h := startResumeResizeFrom(t, payload, resizedSize, resizedSize)

		h.window()
		h.press("d")

		if err := h.wait(); err != nil {
			t.Fatalf("runResumeWait() error = %v", err)
		}
		assertHandOff(t, h.probe, opened(payload))
		if got := h.sizes.Load(); got != 0 {
			t.Errorf("the wait read the pane size %d times, want 0: the key answered before the window elapsed", got)
		}
	})

	t.Run("it stays put when the start-up size read fails", func(t *testing.T) {
		payload := drawnPayload()
		h := startResumeResizeFrom(t, payload, failedSize, failedSize)

		h.awaitRead()
		if got := h.armed.Load(); got != 0 {
			t.Errorf("the wait armed %d settle windows over a failed start-up read, want 0", got)
		}
		h.press("d")

		if err := h.wait(); err != nil {
			t.Fatalf("runResumeWait() error = %v", err)
		}
		assertHandOff(t, h.probe, opened(payload))
		if got := h.armed.Load(); got != 0 {
			t.Errorf("the wait armed %d settle windows, want 0", got)
		}
	})

	t.Run("it still redraws on a settled resize after a failed start-up read", func(t *testing.T) {
		payload := drawnPayload()
		h := startResumeResizeFrom(t, payload, failedSize, failedSize)

		h.elapse(h.resize())

		if err := h.wait(); err != nil {
			t.Fatalf("runResumeWait() error = %v", err)
		}
		assertHandOff(t, h.probe, payload)
	})

	t.Run("it stays put when the panel was drawn at a non-positive size", func(t *testing.T) {
		for _, drawn := range []struct {
			name string
			w, h int
		}{
			{"unmeasured", 0, 0},
			{"zero width", 0, 24},
			{"zero height", 80, 0},
			{"negative", -4, -2},
		} {
			t.Run(drawn.name, func(t *testing.T) {
				payload := samplePayload()
				payload.Width, payload.Height = drawn.w, drawn.h
				h := startResumeResizeFrom(t, payload, resizedSize, resizedSize)

				h.awaitRead()
				_ = h.writer.Close()

				if err := h.wait(); !errors.Is(err, io.EOF) {
					t.Fatalf("runResumeWait() error = %v, want the EOF that ended the wait", err)
				}
				if got := h.armed.Load(); got != 0 {
					t.Errorf("the wait armed %d settle windows, want 0", got)
				}
				assertNoHandOff(t, h.probe)
			})
		}
	})

	t.Run("it arms nothing when started at the size it was drawn at", func(t *testing.T) {
		payload := drawnPayload()
		h := startResumeResize(t, payload, resizedSize)

		h.awaitRead()
		_ = h.writer.Close()

		if err := h.wait(); !errors.Is(err, io.EOF) {
			t.Fatalf("runResumeWait() error = %v, want the EOF that ended the wait", err)
		}
		if got := h.armed.Load(); got != 0 {
			t.Errorf("the wait armed %d settle windows, want 0", got)
		}
		assertNoHandOff(t, h.probe)
	})
}
