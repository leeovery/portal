package tui

import (
	"errors"
	"io"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
	"github.com/leeovery/portal/internal/bgreply"
	"github.com/leeovery/portal/internal/theme"
	"golang.org/x/sys/unix"
)

// A background reply is some 30 bytes; the cap bounds a terminal that answers
// something else entirely without ever terminating it.
const paneReplyCap = 256

var errUnselectable = errors.New("stdin descriptor is outside the select set")

// paneAppearanceProbe is the non-Bubble-Tea OSC 11 read, for the process that
// paints a pane and then execs a waiter over itself — it has no program loop for
// tea.RequestBackgroundColor to resolve into.
type paneAppearanceProbe struct {
	out io.Writer
	// armRead readies a read of the terminal that gives up once bound has
	// elapsed from the moment it was armed.
	armRead    func(bound time.Duration) (io.Reader, error)
	isTerminal func() bool
	makeRaw    func() (restore func(), err error)
	timeout    time.Duration
}

func newPaneAppearanceProbe() paneAppearanceProbe {
	return paneAppearanceProbe{
		out:        os.Stdout,
		armRead:    armStdinRead,
		isTerminal: func() bool { return term.IsTerminal(os.Stdin.Fd()) },
		makeRaw:    makeStdinRaw,
		timeout:    appearanceDetectTimeout,
	}
}

// PaneAppearance is what resolving a pane draw's palette found.
type PaneAppearance struct {
	Theme theme.Theme

	// Typed is what the probe read of the pane's input that was not a
	// terminal's reply, in the order it arrived: keys typed while the question
	// was out, for the caller to judge as if typed after the draw. No reply,
	// whole or cut off, is ever among it. A drop discards it with the rest of
	// the input, so it is nil when one was supplied.
	Typed []byte

	// Settled is how many of the questions owed a reply before this one the
	// replies the probe read answered, oldest first: a terminal answers in the
	// order it was asked, so their replies come ahead of this question's.
	Settled int

	// Unanswered reports a question written and given up on with its own
	// reply unread: the terminal may still answer, after the probe has
	// stopped reading, into whatever reads the pane's input next. A drop
	// doesn't change it, since the reply can arrive after the drop as well.
	Unanswered bool
}

// ResolvePaneTheme answers which palette a pane draw paints in. A constant
// nomination paints from the first frame and a colourless draw detects nothing,
// both without writing a byte to the terminal; a pair is raced against the
// picker's own detect timeout and resolves dark for every way the question can
// go unanswered.
//
// owed is how many questions earlier draws asked are still owed their replies,
// which the terminal sends ahead of this one's.
//
// dropInput runs once the appearance question has resolved and never before it:
// the terminal's own answer can arrive after the probe's deadline, and a drop
// taken first cannot clear it. A nil dropInput is a no-op. The palette is
// returned whether or not the drop succeeded, alongside the drop's own error.
func ResolvePaneTheme(n theme.Nomination, colourless bool, owed int, dropInput func() error) (PaneAppearance, error) {
	return resolvePaneTheme(n, colourless, owed, dropInput, newPaneAppearanceProbe())
}

func resolvePaneTheme(n theme.Nomination, colourless bool, owed int, dropInput func() error, p paneAppearanceProbe) (PaneAppearance, error) {
	found := selectPaneTheme(n, colourless, owed, p)
	if dropInput == nil {
		return found, nil
	}
	found.Typed = nil
	return found, dropInput()
}

func selectPaneTheme(n theme.Nomination, colourless bool, owed int, p paneAppearanceProbe) PaneAppearance {
	if n.IsConstant() {
		return PaneAppearance{Theme: n.Constant()}
	}
	if n.IsZero() || colourless {
		return PaneAppearance{Theme: n.Select(theme.MemberDark)}
	}
	member, found := p.detect(owed)
	found.Theme = n.Select(member)
	return found
}

// detect asks the terminal its background, owed questions still owed their
// replies ahead of it. The member is its own reply's or, unanswered, the latest
// earlier one's it read, which a terminal attached a moment ago sent; what it
// read beside the replies, and which questions they answered, is in the
// appearance.
func (p paneAppearanceProbe) detect(owed int) (theme.Member, PaneAppearance) {
	if !p.isTerminal() {
		return theme.MemberDark, PaneAppearance{}
	}
	restore, err := p.makeRaw()
	if err != nil {
		return theme.MemberDark, PaneAppearance{}
	}
	defer restore()

	// Armed before the query is written: a query with no bounded read behind it
	// leaves the terminal's reply on the pane's input for whatever reads it next.
	reader, err := p.armRead(p.timeout)
	if err != nil {
		return theme.MemberDark, PaneAppearance{}
	}
	if _, err := io.WriteString(p.out, ansi.RequestBackgroundColor); err != nil {
		return theme.MemberDark, PaneAppearance{}
	}
	replies, typed := p.readBackgroundReplies(reader, owed+1)
	found := PaneAppearance{Typed: typed, Settled: min(len(replies), owed), Unanswered: len(replies) <= owed}
	if len(replies) == 0 {
		return theme.MemberDark, found
	}
	payload := replies[min(owed, len(replies)-1)]
	return terminalReplyFrom(tea.BackgroundColorMsg{Color: ansi.XParseColor(payload)}).member, found
}

// readBackgroundReplies reads until want replies have arrived whole with no
// other cut off behind them, as a second terminal on the same pane answering
// sends, or the read gives up. A reply whose head has arrived as it gives up is
// read on for bgreply.TailWait more, once, so its rest is never left on the
// pane's input for the next reader to take as keys.
func (p paneAppearanceProbe) readBackgroundReplies(r io.Reader, want int) (replies []string, typed []byte) {
	buf := make([]byte, paneReplyCap)
	filled := 0
	finishing := false
	for {
		n, err := r.Read(buf[filled:])
		filled += n
		rest, payloads, head := bgreply.Split(buf[:filled])
		if (len(payloads) >= want && head == nil) || filled == len(buf) {
			return payloads, rest
		}
		if err == nil {
			continue
		}
		if finishing || head == nil || !errors.Is(err, os.ErrDeadlineExceeded) {
			return payloads, rest
		}
		finishing = true
		if r, err = p.armRead(bgreply.TailWait); err != nil {
			return payloads, rest
		}
	}
}

// armStdinRead bounds reads of stdin with select rather than a read deadline:
// os.Stdin is a blocking descriptor outside the runtime poller, so it takes no
// deadline, and a fresh /dev/tty open takes none on macOS either, since kqueue
// cannot poll that device.
func armStdinRead(bound time.Duration) (io.Reader, error) {
	fd := int(os.Stdin.Fd())
	if fd < 0 || fd >= unix.FD_SETSIZE {
		return nil, errUnselectable
	}
	return &selectBoundedReader{fd: fd, deadline: time.Now().Add(bound)}, nil
}

type selectBoundedReader struct {
	fd       int
	deadline time.Time
}

func (r *selectBoundedReader) Read(p []byte) (int, error) {
	if err := r.awaitReadable(); err != nil {
		return 0, err
	}
	n, err := unix.Read(r.fd, p)
	switch {
	case err != nil:
		return 0, err
	case n == 0:
		return 0, io.EOF
	}
	return n, nil
}

func (r *selectBoundedReader) awaitReadable() error {
	for {
		remaining := time.Until(r.deadline)
		if remaining <= 0 {
			return os.ErrDeadlineExceeded
		}
		var readable unix.FdSet
		readable.Set(r.fd)
		timeout := unix.NsecToTimeval(remaining.Nanoseconds())
		n, err := unix.Select(r.fd+1, &readable, nil, nil, &timeout)
		switch {
		case errors.Is(err, unix.EINTR):
			continue
		case err != nil:
			return err
		case n == 0:
			return os.ErrDeadlineExceeded
		}
		return nil
	}
}

func makeStdinRaw() (func(), error) {
	fd := os.Stdin.Fd()
	state, err := term.MakeRaw(fd)
	if err != nil {
		return nil, err
	}
	return func() { _ = term.Restore(fd, state) }, nil
}
