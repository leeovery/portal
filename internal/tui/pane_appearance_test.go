package tui

import (
	"bytes"
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
)

// Spelled out rather than taken from the constant the probe writes: the failure
// that matters is a pane draw writing an OSC 11 *set* instead of the query, and
// an expectation built from whatever the code writes could not see it.
const backgroundQuery = "\x1b]11;?\a"

const (
	darkReply        = "\x1b]11;rgb:0b0b/0c0c/1414\a"
	lightReply       = "\x1b]11;rgb:e1e1/e2e2/e7e7\a"
	stTerminatedDark = "\x1b]11;rgb:0b0b/0c0c/1414\x1b\\"
	truncatedReply   = "\x1b]11;rgb:0b0b/0c0c/14"
	unparseableReply = "\x1b]11;not-a-colour\a"
)

// Long enough that a scheduling hiccup cannot expire it before a buffered reply
// is read, short enough that the no-answer cases stay cheap.
const testProbeTimeout = 60 * time.Millisecond

var errSeam = errors.New("seam failure")

const (
	eventWriteQuery  = "write query"
	eventCloseReader = "close reader"
	eventDrop        = "drop input"
)

type fakePaneReader struct {
	f           *os.File
	record      func(string)
	readErr     error
	deadlineErr error
	reads       int
	closes      int
	deadlines   []time.Time
}

func (r *fakePaneReader) Read(p []byte) (int, error) {
	r.reads++
	if r.readErr != nil {
		return 0, r.readErr
	}
	return r.f.Read(p)
}

func (r *fakePaneReader) SetReadDeadline(t time.Time) error {
	r.deadlines = append(r.deadlines, t)
	if r.deadlineErr != nil {
		return r.deadlineErr
	}
	return r.f.SetReadDeadline(t)
}

func (r *fakePaneReader) Close() error {
	r.closes++
	r.record(eventCloseReader)
	return r.f.Close()
}

// recordingWriter puts the query write into the same sequence as the reader's
// close and the drop, so their order is one assertion rather than three.
type recordingWriter struct {
	buf    *bytes.Buffer
	record func(string)
}

func (w recordingWriter) Write(p []byte) (int, error) {
	w.record(eventWriteQuery)
	return w.buf.Write(p)
}

type probeHarness struct {
	out      *bytes.Buffer
	reader   *fakePaneReader
	replies  *os.File
	terminal bool
	openErr  error
	rawErr   error
	opens    int
	restores int
	drops    int
	events   []string
}

func newProbeHarness(t *testing.T) *probeHarness {
	t.Helper()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = read.Close()
		_ = write.Close()
	})
	h := &probeHarness{
		out:      &bytes.Buffer{},
		reader:   &fakePaneReader{f: read},
		replies:  write,
		terminal: true,
	}
	h.reader.record = h.record
	return h
}

func (h *probeHarness) record(event string) {
	h.events = append(h.events, event)
}

func (h *probeHarness) dropInput(err error) func() error {
	return func() error {
		h.drops++
		h.record(eventDrop)
		return err
	}
}

func (h *probeHarness) probe() paneAppearanceProbe {
	return paneAppearanceProbe{
		out: recordingWriter{buf: h.out, record: h.record},
		openReader: func() (paneReader, error) {
			h.opens++
			if h.openErr != nil {
				return nil, h.openErr
			}
			return h.reader, nil
		},
		isTerminal: func() bool { return h.terminal },
		makeRaw: func() (func(), error) {
			if h.rawErr != nil {
				return nil, h.rawErr
			}
			return func() { h.restores++ }, nil
		},
		timeout: testProbeTimeout,
	}
}

func (h *probeHarness) reply(t *testing.T, s string) {
	t.Helper()
	if _, err := h.replies.WriteString(s); err != nil {
		t.Fatalf("write reply: %v", err)
	}
}

func (h *probeHarness) assertWroteQuery(t *testing.T) {
	t.Helper()
	if got := h.out.String(); got != backgroundQuery {
		t.Errorf("wrote %q to the terminal, want exactly one background-colour query %q", got, backgroundQuery)
	}
}

func (h *probeHarness) assertWroteNothing(t *testing.T) {
	t.Helper()
	if got := h.out.String(); got != "" {
		t.Errorf("wrote %q to the terminal, want nothing written on this path", got)
	}
}

func assertNoDropError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("returned error %v, want nil — no drop was supplied to fail", err)
	}
}

func adaptivePair(t *testing.T) (theme.Nomination, theme.Theme, theme.Theme) {
	t.Helper()
	light, dark := themetest.DefaultLight(t), themetest.DefaultDark(t)
	return theme.AdaptivePair(light, dark), light, dark
}

func TestResolvePaneTheme(t *testing.T) {
	t.Run("it paints a constant nomination with no query at all", func(t *testing.T) {
		h := newProbeHarness(t)
		constant := themetest.DefaultLight(t)

		got, err := resolvePaneTheme(theme.ConstantNomination(constant), false, nil, h.probe())

		if got != constant {
			t.Errorf("resolved %q, want the constant nomination's palette %q", got.Canvas.Value, constant.Canvas.Value)
		}
		assertNoDropError(t, err)
		h.assertWroteNothing(t)
		if h.opens != 0 {
			t.Errorf("opened the terminal %d times for a constant nomination, want 0 — the gate is never consulted", h.opens)
		}
	})

	t.Run("it returns the zero theme for a zero nomination", func(t *testing.T) {
		h := newProbeHarness(t)

		got, err := resolvePaneTheme(theme.Nomination{}, false, nil, h.probe())

		if got != (theme.Theme{}) {
			t.Errorf("resolved a theme with canvas %q for a zero nomination, want the zero theme", got.Canvas.Value)
		}
		assertNoDropError(t, err)
		h.assertWroteNothing(t)
		if h.opens != 0 {
			t.Errorf("opened the terminal %d times for a zero nomination, want 0", h.opens)
		}
	})

	t.Run("it runs no detection and writes nothing under NO_COLOR", func(t *testing.T) {
		pair, _, dark := adaptivePair(t)
		constant := themetest.DefaultLight(t)
		cases := []struct {
			name string
			n    theme.Nomination
			want theme.Theme
		}{
			{"adaptive pair", pair, dark},
			{"constant", theme.ConstantNomination(constant), constant},
			{"zero", theme.Nomination{}, theme.Theme{}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				h := newProbeHarness(t)
				h.reply(t, lightReply)

				got, err := resolvePaneTheme(tc.n, true, nil, h.probe())

				if got != tc.want {
					t.Errorf("resolved canvas %q under NO_COLOR, want %q", got.Canvas.Value, tc.want.Canvas.Value)
				}
				assertNoDropError(t, err)
				h.assertWroteNothing(t)
				if h.opens != 0 {
					t.Errorf("opened the terminal %d times under NO_COLOR, want 0 — no detection runs at all", h.opens)
				}
			})
		}
	})

	t.Run("it selects the pair member the probe answers with", func(t *testing.T) {
		pair, light, dark := adaptivePair(t)
		cases := []struct {
			name  string
			reply string
			want  theme.Theme
		}{
			{"light reply", lightReply, light},
			{"dark reply", darkReply, dark},
			{"no reply", "", dark},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				h := newProbeHarness(t)
				if tc.reply != "" {
					h.reply(t, tc.reply)
				}

				got, err := resolvePaneTheme(pair, false, nil, h.probe())

				if got != tc.want {
					t.Errorf("resolved canvas %q, want %q", got.Canvas.Value, tc.want.Canvas.Value)
				}
				assertNoDropError(t, err)
			})
		}
	})

	t.Run("it runs the drop after the appearance query resolves", func(t *testing.T) {
		pair, _, _ := adaptivePair(t)
		probed := []string{eventWriteQuery, eventCloseReader, eventDrop}
		cases := []struct {
			name    string
			arrange func(h *probeHarness)
			want    []string
		}{
			{"a reply before the deadline", func(h *probeHarness) { h.reply(t, darkReply) }, probed},
			{"a reply that never comes", func(*probeHarness) {}, probed},
			{"a terminal that cannot be opened", func(h *probeHarness) { h.openErr = errSeam }, []string{eventDrop}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				h := newProbeHarness(t)
				tc.arrange(h)

				_, _ = resolvePaneTheme(pair, false, h.dropInput(nil), h.probe())

				if !slices.Equal(h.events, tc.want) {
					t.Errorf("ran %v with %s, want %v — a drop taken before the query resolves cannot clear the query's own reply", h.events, tc.name, tc.want)
				}
			})
		}
	})

	t.Run("it runs the drop exactly once for every nomination", func(t *testing.T) {
		pair, _, _ := adaptivePair(t)
		cases := []struct {
			name       string
			n          theme.Nomination
			colourless bool
			detects    bool
		}{
			{"constant", theme.ConstantNomination(themetest.DefaultLight(t)), false, false},
			{"zero", theme.Nomination{}, false, false},
			{"adaptive pair", pair, false, true},
			{"colourless", pair, true, false},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				h := newProbeHarness(t)
				h.reply(t, darkReply)

				_, _ = resolvePaneTheme(tc.n, tc.colourless, h.dropInput(nil), h.probe())

				if h.drops != 1 {
					t.Errorf("ran the drop %d times for a %s resolution, want exactly 1 — every pane draw owes one drop", h.drops, tc.name)
				}
				if tc.detects {
					return
				}
				h.assertWroteNothing(t)
				if h.opens != 0 {
					t.Errorf("opened the terminal %d times for a %s resolution, want 0 — the drop does not drag the gate in with it", h.opens, tc.name)
				}
			})
		}
	})

	t.Run("it resolves the same palette with no drop supplied", func(t *testing.T) {
		pair, _, _ := adaptivePair(t)
		cases := []struct {
			name       string
			n          theme.Nomination
			colourless bool
		}{
			{"constant", theme.ConstantNomination(themetest.DefaultLight(t)), false},
			{"zero", theme.Nomination{}, false},
			{"adaptive pair", pair, false},
			{"colourless", pair, true},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				bare, dropping := newProbeHarness(t), newProbeHarness(t)
				bare.reply(t, lightReply)
				dropping.reply(t, lightReply)

				got, err := resolvePaneTheme(tc.n, tc.colourless, nil, bare.probe())
				want, _ := resolvePaneTheme(tc.n, tc.colourless, dropping.dropInput(nil), dropping.probe())

				if got != want {
					t.Errorf("resolved canvas %q with no drop and %q with one, want a draw that owes no drop to paint what it paints today", got.Canvas.Value, want.Canvas.Value)
				}
				assertNoDropError(t, err)
			})
		}
	})

	t.Run("it returns the resolved palette when the drop fails", func(t *testing.T) {
		pair, light, _ := adaptivePair(t)
		h := newProbeHarness(t)
		h.reply(t, lightReply)

		got, err := resolvePaneTheme(pair, false, h.dropInput(errSeam), h.probe())

		if got != light {
			t.Errorf("resolved canvas %q after a failed drop, want the probe's own answer %q — the caller still has a fallback screen to paint", got.Canvas.Value, light.Canvas.Value)
		}
		if err == nil {
			t.Error("returned a nil error after a failed drop, want the failure reported so the caller can say why")
		}
	})

	t.Run("it returns the drop's own error", func(t *testing.T) {
		pair, _, _ := adaptivePair(t)
		h := newProbeHarness(t)
		h.reply(t, darkReply)

		_, err := resolvePaneTheme(pair, false, h.dropInput(errSeam), h.probe())

		if !errors.Is(err, errSeam) {
			t.Errorf("returned %v, want the drop's own error %v unchanged — nothing here wraps it, retries it or swallows it", err, errSeam)
		}
	})
}

func TestPaneAppearanceProbe(t *testing.T) {
	t.Run("it writes the background-colour query exactly once for a pair", func(t *testing.T) {
		h := newProbeHarness(t)
		h.reply(t, darkReply)

		h.probe().detect()

		h.assertWroteQuery(t)
	})

	t.Run("it selects the light member for a light reply", func(t *testing.T) {
		h := newProbeHarness(t)
		h.reply(t, lightReply)

		if got := h.probe().detect(); got != theme.MemberLight {
			t.Errorf("classified a light reply as %v, want %v", got, theme.MemberLight)
		}
	})

	t.Run("it selects the dark member for a dark reply", func(t *testing.T) {
		cases := map[string]string{"BEL terminated": darkReply, "ST terminated": stTerminatedDark}
		for name, reply := range cases {
			t.Run(name, func(t *testing.T) {
				h := newProbeHarness(t)
				h.reply(t, reply)

				if got := h.probe().detect(); got != theme.MemberDark {
					t.Errorf("classified a dark reply as %v, want %v", got, theme.MemberDark)
				}
			})
		}
	})

	t.Run("it resolves dark when the terminal never answers", func(t *testing.T) {
		h := newProbeHarness(t)

		start := time.Now()
		got := h.probe().detect()
		elapsed := time.Since(start)

		if got != theme.MemberDark {
			t.Errorf("resolved %v with no answer, want %v", got, theme.MemberDark)
		}
		if elapsed < testProbeTimeout {
			t.Errorf("returned after %v, want a wait of at least the probe's timeout %v", elapsed, testProbeTimeout)
		}
		if len(h.reader.deadlines) == 0 {
			t.Fatal("set no read deadline; the read would block forever on a silent terminal")
		}
		if remaining := time.Until(h.reader.deadlines[0]); remaining > testProbeTimeout {
			t.Errorf("set a deadline %v out at the time of the check, want no more than the probe's timeout %v", remaining, testProbeTimeout)
		}
	})

	t.Run("it resolves dark for a truncated or unparseable reply", func(t *testing.T) {
		cases := map[string]string{
			"truncated":                  truncatedReply,
			"unparseable":                unparseableReply,
			"not an OSC 11 reply at all": "garbage\a",
		}
		for name, reply := range cases {
			t.Run(name, func(t *testing.T) {
				h := newProbeHarness(t)
				h.reply(t, reply)

				if got := h.probe().detect(); got != theme.MemberDark {
					t.Errorf("classified %q as %v, want %v", reply, got, theme.MemberDark)
				}
				h.assertWroteQuery(t)
			})
		}
	})

	t.Run("it resolves dark for a read failure", func(t *testing.T) {
		h := newProbeHarness(t)
		h.reader.readErr = errSeam

		if got := h.probe().detect(); got != theme.MemberDark {
			t.Errorf("resolved %v after a read failure, want %v", got, theme.MemberDark)
		}
		h.assertWroteQuery(t)
	})

	t.Run("it resolves dark and writes nothing when stdin is not a terminal", func(t *testing.T) {
		h := newProbeHarness(t)
		h.terminal = false
		h.reply(t, lightReply)

		if got := h.probe().detect(); got != theme.MemberDark {
			t.Errorf("resolved %v off a terminal, want %v", got, theme.MemberDark)
		}
		h.assertWroteNothing(t)
		if h.opens != 0 {
			t.Errorf("opened the terminal %d times off a terminal, want 0", h.opens)
		}
	})

	t.Run("it resolves dark and writes nothing when raw mode cannot be entered", func(t *testing.T) {
		h := newProbeHarness(t)
		h.rawErr = errSeam
		h.reply(t, lightReply)

		if got := h.probe().detect(); got != theme.MemberDark {
			t.Errorf("resolved %v with raw mode refused, want %v", got, theme.MemberDark)
		}
		h.assertWroteNothing(t)
		if h.reader.closes != 1 {
			t.Errorf("closed the opened reader %d times with raw mode refused, want 1", h.reader.closes)
		}
	})

	t.Run("it resolves dark and writes nothing when the terminal cannot be opened", func(t *testing.T) {
		h := newProbeHarness(t)
		h.openErr = errSeam

		if got := h.probe().detect(); got != theme.MemberDark {
			t.Errorf("resolved %v with the terminal unopenable, want %v", got, theme.MemberDark)
		}
		h.assertWroteNothing(t)
		if h.restores != 0 {
			t.Errorf("entered raw mode %d times without a reader, want 0", h.restores)
		}
	})

	t.Run("it resolves dark without blocking when the reader refuses a deadline", func(t *testing.T) {
		h := newProbeHarness(t)
		h.reader.deadlineErr = errSeam

		// Off the test goroutine: a probe that reads with no deadline in force
		// blocks forever, and asserting on the answer inline would hang the
		// package rather than fail this subtest. The pipe's close on cleanup
		// releases a probe left behind.
		answers := make(chan theme.Member, 1)
		go func() { answers <- h.probe().detect() }()

		select {
		case got := <-answers:
			if got != theme.MemberDark {
				t.Errorf("resolved %v with the deadline refused, want %v", got, theme.MemberDark)
			}
			if h.reader.reads != 0 {
				t.Errorf("read %d times with no deadline in force, want 0 — the read would block forever", h.reader.reads)
			}
		case <-time.After(testProbeTimeout * 4):
			t.Errorf("did not return within %v with the deadline refused; a read with no deadline in force never returns at all", testProbeTimeout*4)
		}
	})

	t.Run("it ignores a reply that arrives after the deadline", func(t *testing.T) {
		h := newProbeHarness(t)
		late := make(chan struct{})
		go func() {
			time.Sleep(testProbeTimeout * 2)
			_, _ = h.replies.WriteString(lightReply)
			close(late)
		}()

		got := h.probe().detect()
		<-late

		if got != theme.MemberDark {
			t.Errorf("resolved %v, want %v — a reply that lost the race must never flip the answer", got, theme.MemberDark)
		}
	})

	t.Run("it restores the terminal mode on every path", func(t *testing.T) {
		cases := []struct {
			name    string
			arrange func(h *probeHarness)
		}{
			{"a dark reply", func(h *probeHarness) { h.reply(t, darkReply) }},
			{"a light reply", func(h *probeHarness) { h.reply(t, lightReply) }},
			{"no reply", func(*probeHarness) {}},
			{"a truncated reply", func(h *probeHarness) { h.reply(t, truncatedReply) }},
			{"an unparseable reply", func(h *probeHarness) { h.reply(t, unparseableReply) }},
			{"a read failure", func(h *probeHarness) { h.reader.readErr = errSeam }},
			{"a refused deadline", func(h *probeHarness) { h.reader.deadlineErr = errSeam }},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				h := newProbeHarness(t)
				tc.arrange(h)

				h.probe().detect()

				if h.restores != 1 {
					t.Errorf("restored the terminal mode %d times after %s, want 1 — raw mode left on breaks the pane's keyboard", h.restores, tc.name)
				}
				if h.reader.closes != 1 {
					t.Errorf("closed the reader %d times after %s, want 1", h.reader.closes, tc.name)
				}
			})
		}
	})
}

func TestProductionPaneAppearanceProbe(t *testing.T) {
	p := newPaneAppearanceProbe()

	t.Run("it races the picker's own detect timeout", func(t *testing.T) {
		if p.timeout != appearanceDetectTimeout {
			t.Errorf("probe timeout is %v, want the picker's %v — two timeouts that could drift would make the panel and the picker disagree", p.timeout, appearanceDetectTimeout)
		}
	})

	t.Run("it reads the pane's own terminal rather than stdin", func(t *testing.T) {
		if paneTTYPath != "/dev/tty" {
			t.Errorf("the probe opens %q; os.Stdin refuses a read deadline on a terminal, so the reply must come from a fresh /dev/tty open", paneTTYPath)
		}
	})

	t.Run("it wires every seam", func(t *testing.T) {
		if p.out == nil || p.openReader == nil || p.isTerminal == nil || p.makeRaw == nil {
			t.Errorf("production probe has an unwired seam: out=%v openReader=%v isTerminal=%v makeRaw=%v",
				p.out != nil, p.openReader != nil, p.isTerminal != nil, p.makeRaw != nil)
		}
	})
}
