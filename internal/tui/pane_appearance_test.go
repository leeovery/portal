package tui

import (
	"bytes"
	"errors"
	"io"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/bgreply"
	"github.com/leeovery/portal/internal/harnesstest"
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
	c1TerminatedLite = "\x1b]11;rgb:e1e1/e2e2/e7e7\x9c"
	truncatedReply   = "\x1b]11;rgb:0b0b/0c0c/14"
	unparseableReply = "\x1b]11;not-a-colour\a"
)

// Long enough that a scheduling hiccup cannot expire it before a buffered reply
// is read, short enough that the no-answer cases stay cheap.
const testProbeTimeout = 60 * time.Millisecond

var errSeam = errors.New("seam failure")

const (
	eventArmRead    = "arm read"
	eventWriteQuery = "write query"
	eventDrop       = "drop input"
)

// fakePaneReader reads the pipe the harness writes replies into, under the
// deadline the harness set on it when the read was armed, or, once scripted,
// one chunk of its script per read and then the deadline.
type fakePaneReader struct {
	f       *os.File
	readErr error
	reads   int

	scripted bool
	// A nil chunk is a deadline passing between the chunks either side of it.
	script [][]byte
}

func (r *fakePaneReader) Read(p []byte) (int, error) {
	r.reads++
	if r.readErr != nil {
		return 0, r.readErr
	}
	if r.scripted {
		return r.readScript(p)
	}
	return r.f.Read(p)
}

func (r *fakePaneReader) readScript(p []byte) (int, error) {
	if len(r.script) == 0 {
		return 0, os.ErrDeadlineExceeded
	}
	if r.script[0] == nil {
		r.script = r.script[1:]
		return 0, os.ErrDeadlineExceeded
	}
	n := copy(p, r.script[0])
	if r.script[0] = r.script[0][n:]; len(r.script[0]) == 0 {
		r.script = r.script[1:]
	}
	return n, nil
}

// deadlinePasses, among the chunks readsInChunks takes, is the read's deadline
// passing there.
const deadlinePasses = ""

// readsInChunks has every read of the probe take the next chunk alone, as a
// terminal's input does when its bytes arrive apart.
func (h *probeHarness) readsInChunks(chunks ...string) {
	h.reader.scripted = true
	for _, c := range chunks {
		chunk := []byte(c)
		if c == deadlinePasses {
			chunk = nil
		}
		h.reader.script = append(h.reader.script, chunk)
	}
}

// unread is what the scripted input still holds for whatever reads the pane
// next.
func (h *probeHarness) unread() string {
	var left []byte
	for _, c := range h.reader.script {
		left = append(left, c...)
	}
	return string(left)
}

// recordingWriter puts the query write into the same sequence as the read's
// arming and the drop, so their order is one assertion rather than three.
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
	timeout  time.Duration
	terminal bool
	armErr   error
	rawErr   error
	bounds   []time.Duration
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
	return &probeHarness{
		out:      &bytes.Buffer{},
		reader:   &fakePaneReader{f: read},
		replies:  write,
		timeout:  testProbeTimeout,
		terminal: true,
	}
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

// probeArmsRefusedPast is how many reads the harness arms before it refuses,
// so a probe that reads on without end fails on the bounds it armed rather
// than hanging.
const probeArmsRefusedPast = 8

// armRead records the bound as the read is armed, before anything is read
// under it, so the bound asserted on is the one the read was given.
func (h *probeHarness) armRead(bound time.Duration) (io.Reader, error) {
	h.bounds = append(h.bounds, bound)
	h.record(eventArmRead)
	if h.armErr != nil {
		return nil, h.armErr
	}
	if len(h.bounds) > probeArmsRefusedPast {
		return nil, errSeam
	}
	if err := h.reader.f.SetReadDeadline(time.Now().Add(bound)); err != nil {
		return nil, err
	}
	return h.reader, nil
}

func (h *probeHarness) probe() paneAppearanceProbe {
	return paneAppearanceProbe{
		out:        recordingWriter{buf: h.out, record: h.record},
		armRead:    h.armRead,
		isTerminal: func() bool { return h.terminal },
		makeRaw: func() (func(), error) {
			if h.rawErr != nil {
				return nil, h.rawErr
			}
			return func() { h.restores++ }, nil
		},
		timeout: h.timeout,
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

func (h *probeHarness) assertReadNothing(t *testing.T) {
	t.Helper()
	if len(h.bounds) != 0 || h.reader.reads != 0 {
		t.Errorf("armed %d reads and read %d times, want no read of the terminal on this path", len(h.bounds), h.reader.reads)
	}
}

func assertArmedWithin(t harnesstest.TestingT, bounds []time.Duration, limit time.Duration) {
	t.Helper()
	if len(bounds) != 1 {
		t.Errorf("armed %d reads, want exactly 1", len(bounds))
		return
	}
	if bounds[0] > limit {
		t.Errorf("armed the read with a bound of %v, want no more than the probe's timeout %v — a silent terminal would hold the pane that much longer", bounds[0], limit)
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

		found, err := resolvePaneTheme(theme.ConstantNomination(constant), false, 0, nil, h.probe())
		got := found.Theme

		if got != constant {
			t.Errorf("resolved %q, want the constant nomination's palette %q", got.Canvas.Value, constant.Canvas.Value)
		}
		assertNoDropError(t, err)
		h.assertWroteNothing(t)
		h.assertReadNothing(t)
	})

	t.Run("it returns the zero theme for a zero nomination", func(t *testing.T) {
		h := newProbeHarness(t)

		found, err := resolvePaneTheme(theme.Nomination{}, false, 0, nil, h.probe())
		got := found.Theme

		if got != (theme.Theme{}) {
			t.Errorf("resolved a theme with canvas %q for a zero nomination, want the zero theme", got.Canvas.Value)
		}
		assertNoDropError(t, err)
		h.assertWroteNothing(t)
		h.assertReadNothing(t)
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

				found, err := resolvePaneTheme(tc.n, true, 0, nil, h.probe())
				got := found.Theme

				if got != tc.want {
					t.Errorf("resolved canvas %q under NO_COLOR, want %q", got.Canvas.Value, tc.want.Canvas.Value)
				}
				assertNoDropError(t, err)
				h.assertWroteNothing(t)
				h.assertReadNothing(t)
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

				found, err := resolvePaneTheme(pair, false, 0, nil, h.probe())
				got := found.Theme

				if got != tc.want {
					t.Errorf("resolved canvas %q, want %q", got.Canvas.Value, tc.want.Canvas.Value)
				}
				assertNoDropError(t, err)
			})
		}
	})

	t.Run("it runs the drop after the appearance query resolves", func(t *testing.T) {
		pair, _, _ := adaptivePair(t)
		probed := []string{eventArmRead, eventWriteQuery, eventDrop}
		cases := []struct {
			name    string
			arrange func(h *probeHarness)
			want    []string
		}{
			{"a reply before the deadline", func(h *probeHarness) { h.reply(t, darkReply) }, probed},
			{"a reply that never comes", func(*probeHarness) {}, probed},
			{"a read that cannot be armed", func(h *probeHarness) { h.armErr = errSeam }, []string{eventArmRead, eventDrop}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				h := newProbeHarness(t)
				tc.arrange(h)

				_, _ = resolvePaneTheme(pair, false, 0, h.dropInput(nil), h.probe())

				if !slices.Equal(h.events, tc.want) {
					t.Errorf("ran %v with %s, want %v — the query waits on a ready read, and a drop taken before the query resolves cannot clear the query's own reply", h.events, tc.name, tc.want)
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

				_, _ = resolvePaneTheme(tc.n, tc.colourless, 0, h.dropInput(nil), h.probe())

				if h.drops != 1 {
					t.Errorf("ran the drop %d times for a %s resolution, want exactly 1 — every pane draw owes one drop", h.drops, tc.name)
				}
				if tc.detects {
					return
				}
				h.assertWroteNothing(t)
				h.assertReadNothing(t)
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

				found, err := resolvePaneTheme(tc.n, tc.colourless, 0, nil, bare.probe())
				got := found.Theme
				wanted, _ := resolvePaneTheme(tc.n, tc.colourless, 0, dropping.dropInput(nil), dropping.probe())
				want := wanted.Theme

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

		found, err := resolvePaneTheme(pair, false, 0, h.dropInput(errSeam), h.probe())
		got := found.Theme

		if got != light {
			t.Errorf("resolved canvas %q after a failed drop, want the probe's own answer %q — the caller still has a fallback screen to paint", got.Canvas.Value, light.Canvas.Value)
		}
		if err == nil {
			t.Error("returned a nil error after a failed drop, want the failure reported so the caller can say why")
		}
	})

	t.Run("it hands back the key the probe read for a pair, and none for a nomination it doesn't probe", func(t *testing.T) {
		pair, _, _ := adaptivePair(t)
		cases := []struct {
			name       string
			n          theme.Nomination
			colourless bool
			typed      string
		}{
			{"adaptive pair", pair, false, "\r"},
			{"constant", theme.ConstantNomination(themetest.DefaultLight(t)), false, ""},
			{"zero", theme.Nomination{}, false, ""},
			{"colourless", pair, true, ""},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				h := newProbeHarness(t)
				h.reply(t, "\r"+darkReply)

				found, _ := resolvePaneTheme(tc.n, tc.colourless, 0, nil, h.probe())

				if string(found.Typed) != tc.typed {
					t.Errorf("handed back %q as typed, want %q", found.Typed, tc.typed)
				}
			})
		}
	})

	t.Run("it reports an unanswered question only where it asked one, a drop changing nothing", func(t *testing.T) {
		pair, _, _ := adaptivePair(t)
		cases := []struct {
			name       string
			n          theme.Nomination
			colourless bool
			drop       bool
			unanswered bool
		}{
			{"adaptive pair", pair, false, false, true},
			{"adaptive pair, dropping its input", pair, false, true, true},
			{"constant", theme.ConstantNomination(themetest.DefaultLight(t)), false, false, false},
			{"zero", theme.Nomination{}, false, false, false},
			{"colourless", pair, true, false, false},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				h := newProbeHarness(t)
				var drop func() error
				if tc.drop {
					drop = h.dropInput(nil)
				}

				found, _ := resolvePaneTheme(tc.n, tc.colourless, 0, drop, h.probe())

				if found.Unanswered != tc.unanswered {
					t.Errorf("Unanswered = %v, want %v", found.Unanswered, tc.unanswered)
				}
			})
		}
	})

	t.Run("it hands the probe the questions owed, and settles none where it asks none", func(t *testing.T) {
		pair, _, _ := adaptivePair(t)
		cases := []struct {
			name       string
			n          theme.Nomination
			colourless bool
			settled    int
			unanswered bool
		}{
			{"adaptive pair", pair, false, 1, true},
			{"constant", theme.ConstantNomination(themetest.DefaultLight(t)), false, 0, false},
			{"colourless", pair, true, 0, false},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				h := newProbeHarness(t)
				h.reply(t, darkReply)

				found, _ := resolvePaneTheme(tc.n, tc.colourless, 1, nil, h.probe())

				if found.Settled != tc.settled || found.Unanswered != tc.unanswered {
					t.Errorf("Settled = %d, Unanswered = %v with one question owed; want %d, %v", found.Settled, found.Unanswered, tc.settled, tc.unanswered)
				}
			})
		}
	})

	t.Run("it hands back nothing typed when it drops the input", func(t *testing.T) {
		pair, _, _ := adaptivePair(t)
		for _, dropErr := range []error{nil, errSeam} {
			h := newProbeHarness(t)
			h.reply(t, "\r"+darkReply)

			found, _ := resolvePaneTheme(pair, false, 0, h.dropInput(dropErr), h.probe())

			if found.Typed != nil {
				t.Errorf("handed back %q as typed beside a drop failing with %v, want nothing: the drop discards it with the rest", found.Typed, dropErr)
			}
		}
	})

	t.Run("it returns the drop's own error", func(t *testing.T) {
		pair, _, _ := adaptivePair(t)
		h := newProbeHarness(t)
		h.reply(t, darkReply)

		_, err := resolvePaneTheme(pair, false, 0, h.dropInput(errSeam), h.probe())

		if err != errSeam {
			t.Errorf("returned %v, want the drop's own error %v unchanged — nothing here wraps it, retries it or swallows it", err, errSeam)
		}
	})
}

func TestPaneAppearanceProbe(t *testing.T) {
	t.Run("it writes the background-colour query exactly once for a pair", func(t *testing.T) {
		h := newProbeHarness(t)
		h.reply(t, darkReply)

		h.probe().detect(0)

		h.assertWroteQuery(t)
	})

	t.Run("it selects the light member for a light reply", func(t *testing.T) {
		h := newProbeHarness(t)
		h.reply(t, lightReply)

		if got, _ := h.probe().detect(0); got != theme.MemberLight {
			t.Errorf("classified a light reply as %v, want %v", got, theme.MemberLight)
		}
	})

	t.Run("it selects the dark member for a dark reply", func(t *testing.T) {
		cases := map[string]string{"BEL terminated": darkReply, "ST terminated": stTerminatedDark}
		for name, reply := range cases {
			t.Run(name, func(t *testing.T) {
				h := newProbeHarness(t)
				h.reply(t, reply)

				if got, _ := h.probe().detect(0); got != theme.MemberDark {
					t.Errorf("classified a dark reply as %v, want %v", got, theme.MemberDark)
				}
			})
		}
	})

	t.Run("it resolves dark when the terminal never answers", func(t *testing.T) {
		h := newProbeHarness(t)

		start := time.Now()
		got, _ := h.probe().detect(0)
		elapsed := time.Since(start)

		if got != theme.MemberDark {
			t.Errorf("resolved %v with no answer, want %v", got, theme.MemberDark)
		}
		if elapsed < testProbeTimeout {
			t.Errorf("returned after %v, want a wait of at least the probe's timeout %v", elapsed, testProbeTimeout)
		}
		assertArmedWithin(t, h.bounds, testProbeTimeout)
	})

	t.Run("it fails a probe that arms a bound ten times its timeout", func(t *testing.T) {
		h := newProbeHarness(t)
		h.timeout = testProbeTimeout * 10
		h.reply(t, darkReply)
		h.probe().detect(0)

		var rec harnesstest.Recorder
		rec.Run(func() { assertArmedWithin(&rec, h.bounds, testProbeTimeout) })

		if !rec.Failed() {
			t.Errorf("passed a read armed with %v against a timeout of %v, want the bound check to fail it", h.bounds, testProbeTimeout)
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

				if got, _ := h.probe().detect(0); got != theme.MemberDark {
					t.Errorf("classified %q as %v, want %v", reply, got, theme.MemberDark)
				}
				h.assertWroteQuery(t)
			})
		}
	})

	t.Run("it resolves dark for a read failure", func(t *testing.T) {
		h := newProbeHarness(t)
		h.reader.readErr = errSeam

		if got, _ := h.probe().detect(0); got != theme.MemberDark {
			t.Errorf("resolved %v after a read failure, want %v", got, theme.MemberDark)
		}
		h.assertWroteQuery(t)
	})

	t.Run("it resolves dark and writes nothing when stdin is not a terminal", func(t *testing.T) {
		h := newProbeHarness(t)
		h.terminal = false
		h.reply(t, lightReply)

		if got, _ := h.probe().detect(0); got != theme.MemberDark {
			t.Errorf("resolved %v off a terminal, want %v", got, theme.MemberDark)
		}
		h.assertWroteNothing(t)
		h.assertReadNothing(t)
	})

	t.Run("it resolves dark and writes nothing when raw mode cannot be entered", func(t *testing.T) {
		h := newProbeHarness(t)
		h.rawErr = errSeam
		h.reply(t, lightReply)

		if got, _ := h.probe().detect(0); got != theme.MemberDark {
			t.Errorf("resolved %v with raw mode refused, want %v", got, theme.MemberDark)
		}
		h.assertWroteNothing(t)
		h.assertReadNothing(t)
	})

	t.Run("it resolves dark without blocking and writes nothing when the read cannot be armed", func(t *testing.T) {
		h := newProbeHarness(t)
		h.armErr = errSeam
		h.reply(t, lightReply)

		// Off the test goroutine, so a probe that hangs fails this subtest rather
		// than hanging the package.
		answers := make(chan theme.Member, 1)
		go func() { member, _ := h.probe().detect(0); answers <- member }()

		select {
		case got := <-answers:
			if got != theme.MemberDark {
				t.Errorf("resolved %v with the read unarmed, want %v", got, theme.MemberDark)
			}
			h.assertWroteNothing(t)
			if h.reader.reads != 0 {
				t.Errorf("read %d times with no bound armed, want 0", h.reader.reads)
			}
		case <-time.After(testProbeTimeout * 4):
			t.Errorf("did not return within %v with the read unarmed", testProbeTimeout*4)
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

		got, _ := h.probe().detect(0)
		<-late

		if got != theme.MemberDark {
			t.Errorf("resolved %v, want %v — a reply that lost the race must never flip the answer", got, theme.MemberDark)
		}
	})

	t.Run("it hands back what was typed around the reply, and never the reply", func(t *testing.T) {
		cases := []struct {
			name  string
			input string
			want  theme.Member
			typed string
		}{
			{"a BEL-terminated reply alone", darkReply, theme.MemberDark, ""},
			{"an ST-terminated reply alone", stTerminatedDark, theme.MemberDark, ""},
			{"a light reply alone", lightReply, theme.MemberLight, ""},
			{"Enter before the reply", "\r" + lightReply, theme.MemberLight, "\r"},
			{"d after the reply", darkReply + "d", theme.MemberDark, "d"},
			{"Escape after an ST-terminated reply", stTerminatedDark + "\x1b", theme.MemberDark, "\x1b"},
			{"keys on both sides", "x" + lightReply + "y", theme.MemberLight, "xy"},
			{"Escape and no reply", "\x1b", theme.MemberDark, "\x1b"},
			{"a paste and no reply", "echo hi\r", theme.MemberDark, "echo hi\r"},
			{"a truncated reply alone", truncatedReply, theme.MemberDark, ""},
			{"a key before a truncated reply", "d" + truncatedReply, theme.MemberDark, "d"},
			{"a key and a reply cut off two bytes in", "q\x1b]", theme.MemberDark, "q"},
			{"a key and a reply cut off four bytes in", "q\x1b]11", theme.MemberDark, "q"},
			{"something that is no reply at all", "garbage\a", theme.MemberDark, "garbage\a"},
			{"a C1 ST-terminated reply alone", c1TerminatedLite, theme.MemberLight, ""},
			{"Enter before a C1 ST-terminated reply", "\r" + c1TerminatedLite, theme.MemberLight, "\r"},
			{"two terminals' replies with Enter between them", darkReply + "\r" + lightReply, theme.MemberDark, "\r"},
			{"two terminals' replies, then d", lightReply + stTerminatedDark + "d", theme.MemberLight, "d"},
			{"a reply, Enter, and a second cut off", darkReply + "\r\x1b]11;rgb:0b", theme.MemberDark, "\r"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				h := newProbeHarness(t)
				h.reply(t, tc.input)

				got, found := h.probe().detect(0)

				if got != tc.want {
					t.Errorf("resolved %v for %q, want %v", got, tc.input, tc.want)
				}
				if string(found.Typed) != tc.typed {
					t.Errorf("handed back %q as typed for %q, want %q", found.Typed, tc.input, tc.typed)
				}
			})
		}
	})

	t.Run("it reads on past a read that holds no whole reply", func(t *testing.T) {
		cases := []struct {
			name   string
			chunks []string
			want   theme.Member
			typed  string
		}{
			{"Enter, then the reply", []string{"\r", lightReply}, theme.MemberLight, "\r"},
			{"an ESC \\ reply split between its escape and backslash", []string{"\x1b]11;rgb:e1e1/e2e2/e7e7\x1b", "\\"}, theme.MemberLight, ""},
			{"a reply split in its payload", []string{"\x1b]11;rgb:e1e1/e2", "e2/e7e7\a"}, theme.MemberLight, ""},
			{"a second terminal's reply split, then d", []string{lightReply + "\x1b]11;rgb:0b0b", "/0c0c/1414\ad"}, theme.MemberLight, "d"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				h := newProbeHarness(t)
				h.readsInChunks(tc.chunks...)

				got, found := h.probe().detect(0)

				if got != tc.want {
					t.Errorf("resolved %v for reads %q, want %v", got, tc.chunks, tc.want)
				}
				if string(found.Typed) != tc.typed {
					t.Errorf("handed back %q as typed for reads %q, want %q", found.Typed, tc.chunks, tc.typed)
				}
			})
		}
	})

	t.Run("it reports a question no reply answered", func(t *testing.T) {
		cases := []struct {
			name       string
			input      string
			unanswered bool
		}{
			{"a reply", darkReply, false},
			{"Enter, then a reply", "\r" + lightReply, false},
			{"no reply", "", true},
			{"Enter and no reply", "\r", true},
			{"a truncated reply", truncatedReply, true},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				h := newProbeHarness(t)
				if tc.input != "" {
					h.reply(t, tc.input)
				}

				_, found := h.probe().detect(0)

				if found.Unanswered != tc.unanswered {
					t.Errorf("Unanswered = %v for %q, want %v", found.Unanswered, tc.input, tc.unanswered)
				}
			})
		}
	})

	// A terminal answers in the order it was asked, so the replies to questions
	// still owed from earlier draws come ahead of this one's.
	t.Run("it settles the questions owed before its own with the replies that come first", func(t *testing.T) {
		cases := []struct {
			name       string
			owed       int
			replies    []string
			want       theme.Member
			settled    int
			unanswered bool
		}{
			{"nothing owed before, and its own reply", 0, []string{darkReply}, theme.MemberDark, 0, false},
			{"nothing owed before, its own reply, and a second terminal's", 0, []string{lightReply, darkReply}, theme.MemberLight, 0, false},
			{"one owed before, and that one's reply alone", 1, []string{lightReply}, theme.MemberLight, 1, true},
			{"one owed before, that one's reply, then its own", 1, []string{darkReply, lightReply}, theme.MemberLight, 1, false},
			{"one owed before, both replies, and a second terminal's", 1, []string{darkReply, lightReply, darkReply}, theme.MemberLight, 1, false},
			{"two owed before, and one reply", 2, []string{lightReply}, theme.MemberLight, 1, true},
			{"one owed before, and no reply", 1, nil, theme.MemberDark, 0, true},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				h := newProbeHarness(t)
				h.readsInChunks(tc.replies...)

				got, found := h.probe().detect(tc.owed)

				if got != tc.want {
					t.Errorf("resolved %v, want %v", got, tc.want)
				}
				if found.Settled != tc.settled || found.Unanswered != tc.unanswered {
					t.Errorf("Settled = %d, Unanswered = %v; want %d, %v", found.Settled, found.Unanswered, tc.settled, tc.unanswered)
				}
				if len(found.Typed) != 0 {
					t.Errorf("handed back %q as typed, want no reply among it", found.Typed)
				}
			})
		}
	})

	t.Run("it reads on past its deadline, once, to finish a reply whose head has arrived", func(t *testing.T) {
		const head, tail = "\x1b]11;rgb:e1e1/e2", "e2/e7e7\a"
		cases := []struct {
			name       string
			chunks     []string
			want       theme.Member
			typed      string
			unanswered bool
			unread     string
		}{
			{"a reply split across the deadline", []string{head, deadlinePasses, tail}, theme.MemberLight, "", false, ""},
			{"Enter, then a reply split across the deadline", []string{"\r" + head, deadlinePasses, tail}, theme.MemberLight, "\r", false, ""},
			{"an introducer split across the deadline", []string{"\x1b]1", deadlinePasses, "1;rgb:e1e1/e2e2/e7e7\a"}, theme.MemberLight, "", false, ""},
			{"a reply's head and nothing more", []string{head}, theme.MemberDark, "", true, ""},
			{"a reply's rest past a second deadline", []string{head, deadlinePasses, "e2/e7", deadlinePasses, "e7\a"}, theme.MemberDark, "", true, "e7\a"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				h := newProbeHarness(t)
				h.readsInChunks(tc.chunks...)

				got, found := h.probe().detect(0)

				if got != tc.want || string(found.Typed) != tc.typed || found.Unanswered != tc.unanswered {
					t.Errorf("resolved %v, typed %q, Unanswered %v for reads %q; want %v, %q, %v", got, found.Typed, found.Unanswered, tc.chunks, tc.want, tc.typed, tc.unanswered)
				}
				if left := h.unread(); left != tc.unread {
					t.Errorf("left %q on the input for the next reader, want %q", left, tc.unread)
				}
				if want := []time.Duration{h.timeout, bgreply.TailWait}; !slices.Equal(h.bounds, want) {
					t.Errorf("armed reads bounded %v, want %v: the question's, then once more for the reply's rest", h.bounds, want)
				}
			})
		}
	})

	t.Run("it reads on past its deadline for nothing but a reply's head", func(t *testing.T) {
		for _, key := range []string{"\x1b", "q", "\r" + darkReply} {
			t.Run(strconv.Quote(key), func(t *testing.T) {
				h := newProbeHarness(t)
				h.readsInChunks(key, deadlinePasses, "[A")

				h.probe().detect(0)

				if want := []time.Duration{h.timeout}; !slices.Equal(h.bounds, want) {
					t.Errorf("armed reads bounded %v, want only the question's %v", h.bounds, want)
				}
				if left := h.unread(); left != "[A" {
					t.Errorf("left %q on the input for the next reader, want %q", left, "[A")
				}
			})
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
			{"a read that cannot be armed", func(h *probeHarness) { h.armErr = errSeam }},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				h := newProbeHarness(t)
				tc.arrange(h)

				h.probe().detect(0)

				if h.restores != 1 {
					t.Errorf("restored the terminal mode %d times after %s, want 1 — raw mode left on breaks the pane's keyboard", h.restores, tc.name)
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

	t.Run("it wires every seam", func(t *testing.T) {
		if p.out == nil || p.armRead == nil || p.isTerminal == nil || p.makeRaw == nil {
			t.Errorf("production probe has an unwired seam: out=%v armRead=%v isTerminal=%v makeRaw=%v",
				p.out != nil, p.armRead != nil, p.isTerminal != nil, p.makeRaw != nil)
		}
	})
}
