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

	// attached reports whether a terminal shows the pane to answer a
	// question, asked only once one would be written.
	attached func() bool

	// now dates the question as it is written.
	now func() time.Time
}

func newPaneAppearanceProbe(attached func() bool) paneAppearanceProbe {
	return paneAppearanceProbe{
		out:        os.Stdout,
		armRead:    armStdinRead,
		isTerminal: func() bool { return term.IsTerminal(os.Stdin.Fd()) },
		makeRaw:    makeStdinRaw,
		timeout:    appearanceDetectTimeout,
		attached:   attached,
		now:        time.Now,
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

	// AskedAt is when the question was written, by the wall clock, and zero
	// when none was: the terminal answers it, after the probe has stopped
	// reading if not before, into whatever reads the pane's input next, and
	// the reply's window runs from here.
	AskedAt time.Time

	// Replies is how many whole replies were read off the pane's input, the
	// probe's and the drop's, each answering this question or one an earlier
	// draw asked.
	Replies int
}

// Asked reports whether a question was written.
func (a PaneAppearance) Asked() bool { return !a.AskedAt.IsZero() }

// ResolvePaneTheme answers which palette a pane draw paints in. A constant
// nomination paints from the first frame and a colourless draw detects nothing,
// both without writing a byte to the terminal; a pair is raced against the
// picker's own detect timeout and resolves dark for every way the question can
// go unanswered. A pane no terminal shows, as attached reports, is asked
// nothing and painted as for a question unanswered: no terminal would ever
// answer it.
//
// owed is how many questions earlier draws asked are still owed their replies,
// which the terminal sends ahead of this one's.
//
// dropInput runs once the appearance question has resolved and never before it:
// the terminal's own answer can arrive after the probe's deadline, and a drop
// taken first cannot clear it. It answers how many replies it read off with
// what it discarded. A nil dropInput is a no-op. The palette is returned
// whether or not the drop succeeded, alongside the drop's own error.
func ResolvePaneTheme(n theme.Nomination, colourless bool, owed int, attached func() bool, dropInput func() (replies int, err error)) (PaneAppearance, error) {
	return resolvePaneTheme(n, colourless, owed, dropInput, newPaneAppearanceProbe(attached))
}

func resolvePaneTheme(n theme.Nomination, colourless bool, owed int, dropInput func() (int, error), p paneAppearanceProbe) (PaneAppearance, error) {
	found := selectPaneTheme(n, colourless, owed, p)
	if dropInput == nil {
		return found, nil
	}
	found.Typed = nil
	replies, err := dropInput()
	found.Replies += replies
	return found, err
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
// replies ahead of it, reading until as many replies as questions are owed
// have arrived or the read gives up. The member is its own reply's or, short
// of it, the latest one's it read, which a terminal attached a moment ago
// sent; whether it asked, how many replies it read and what it read beside
// them is in the appearance.
func (p paneAppearanceProbe) detect(owed int) (theme.Member, PaneAppearance) {
	if !p.isTerminal() || !p.attached() {
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
	asked := p.now()
	replies, typed := p.readBackgroundReplies(reader, owed+1)
	found := PaneAppearance{Typed: typed, AskedAt: asked, Replies: len(replies)}
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
