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

	// Unanswered reports a question written and given up on with no reply
	// read: the terminal may still answer, after the probe has stopped
	// reading, into whatever reads the pane's input next. A drop doesn't
	// change it, since the reply can arrive after the drop as well.
	Unanswered bool
}

// ResolvePaneTheme answers which palette a pane draw paints in. A constant
// nomination paints from the first frame and a colourless draw detects nothing,
// both without writing a byte to the terminal; a pair is raced against the
// picker's own detect timeout and resolves dark for every way the question can
// go unanswered.
//
// dropInput runs once the appearance question has resolved and never before it:
// the terminal's own answer can arrive after the probe's deadline, and a drop
// taken first cannot clear it. A nil dropInput is a no-op. The palette is
// returned whether or not the drop succeeded, alongside the drop's own error.
func ResolvePaneTheme(n theme.Nomination, colourless bool, dropInput func() error) (PaneAppearance, error) {
	return resolvePaneTheme(n, colourless, dropInput, newPaneAppearanceProbe())
}

func resolvePaneTheme(n theme.Nomination, colourless bool, dropInput func() error, p paneAppearanceProbe) (PaneAppearance, error) {
	found := selectPaneTheme(n, colourless, p)
	if dropInput == nil {
		return found, nil
	}
	found.Typed = nil
	return found, dropInput()
}

func selectPaneTheme(n theme.Nomination, colourless bool, p paneAppearanceProbe) PaneAppearance {
	if n.IsConstant() {
		return PaneAppearance{Theme: n.Constant()}
	}
	if n.IsZero() || colourless {
		return PaneAppearance{Theme: n.Select(theme.MemberDark)}
	}
	member, found := p.detect()
	found.Theme = n.Select(member)
	return found
}

// detect asks the terminal its background. The palette is left to the caller;
// what it read beside the answer, and whether one came, is in the appearance.
func (p paneAppearanceProbe) detect() (theme.Member, PaneAppearance) {
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
	payload, typed, ok := readBackgroundReply(reader)
	if !ok {
		return theme.MemberDark, PaneAppearance{Typed: typed, Unanswered: true}
	}
	return terminalReplyFrom(tea.BackgroundColorMsg{Color: ansi.XParseColor(payload)}).member, PaneAppearance{Typed: typed}
}

// readBackgroundReply reads until a whole reply has arrived with no other cut
// off behind it, as a second terminal on the same pane answering sends, or the
// read gives up. ok reports a reply read, payload being the first one's.
func readBackgroundReply(r io.Reader) (payload string, typed []byte, ok bool) {
	buf := make([]byte, paneReplyCap)
	filled := 0
	for {
		n, err := r.Read(buf[filled:])
		filled += n
		rest, payloads, cut := bgreply.Split(buf[:filled])
		if (len(payloads) > 0 && !cut) || err != nil || filled == len(buf) {
			if len(payloads) == 0 {
				return "", rest, false
			}
			return payloads[0], rest, true
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
