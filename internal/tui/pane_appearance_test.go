package tui

import (
	"bytes"
	"errors"
	"os"
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

type fakePaneReader struct {
	f           *os.File
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
	return r.f.Close()
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
	return &probeHarness{
		out:      &bytes.Buffer{},
		reader:   &fakePaneReader{f: read},
		replies:  write,
		terminal: true,
	}
}

func (h *probeHarness) probe() paneAppearanceProbe {
	return paneAppearanceProbe{
		out: h.out,
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

func adaptivePair(t *testing.T) (theme.Nomination, theme.Theme, theme.Theme) {
	t.Helper()
	light, dark := themetest.DefaultLight(t), themetest.DefaultDark(t)
	return theme.AdaptivePair(light, dark), light, dark
}

func TestResolvePaneTheme(t *testing.T) {
	t.Run("it paints a constant nomination with no query at all", func(t *testing.T) {
		h := newProbeHarness(t)
		constant := themetest.DefaultLight(t)

		got := resolvePaneTheme(theme.ConstantNomination(constant), false, h.probe())

		if got != constant {
			t.Errorf("resolved %q, want the constant nomination's palette %q", got.Canvas.Value, constant.Canvas.Value)
		}
		h.assertWroteNothing(t)
		if h.opens != 0 {
			t.Errorf("opened the terminal %d times for a constant nomination, want 0 — the gate is never consulted", h.opens)
		}
	})

	t.Run("it returns the zero theme for a zero nomination", func(t *testing.T) {
		h := newProbeHarness(t)

		got := resolvePaneTheme(theme.Nomination{}, false, h.probe())

		if got != (theme.Theme{}) {
			t.Errorf("resolved a theme with canvas %q for a zero nomination, want the zero theme", got.Canvas.Value)
		}
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

				got := resolvePaneTheme(tc.n, true, h.probe())

				if got != tc.want {
					t.Errorf("resolved canvas %q under NO_COLOR, want %q", got.Canvas.Value, tc.want.Canvas.Value)
				}
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

				got := resolvePaneTheme(pair, false, h.probe())

				if got != tc.want {
					t.Errorf("resolved canvas %q, want %q", got.Canvas.Value, tc.want.Canvas.Value)
				}
			})
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
