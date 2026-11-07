package tui

import (
	"bytes"
	"errors"
	"io"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
	"github.com/leeovery/portal/internal/theme"
	"golang.org/x/sys/unix"
)

const oscBackgroundPrefix = "\x1b]11;"

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

// ResolvePaneTheme answers which palette a pane draw paints in. A constant
// nomination paints from the first frame and a colourless draw detects nothing,
// both without writing a byte to the terminal; a pair is raced against the
// picker's own detect timeout and resolves dark for every way the question can
// go unanswered.
//
// typed is what the probe read of the pane's input that was not the terminal's
// reply, in the order it arrived: keys typed while the question was out, for
// the caller to judge as if typed after the draw. The reply, a partial one
// included, is never among them.
//
// dropInput runs once the appearance question has resolved and never before it:
// the terminal's own answer can arrive after the probe's deadline, and a drop
// taken first cannot clear it. A nil dropInput is a no-op. A drop discards what
// was typed with the rest of the input, so typed is nil when one is supplied.
// The palette is returned whether or not the drop succeeded, alongside the
// drop's own error.
func ResolvePaneTheme(n theme.Nomination, colourless bool, dropInput func() error) (resolved theme.Theme, typed []byte, err error) {
	return resolvePaneTheme(n, colourless, dropInput, newPaneAppearanceProbe())
}

func resolvePaneTheme(n theme.Nomination, colourless bool, dropInput func() error, p paneAppearanceProbe) (theme.Theme, []byte, error) {
	resolved, typed := selectPaneTheme(n, colourless, p)
	if dropInput == nil {
		return resolved, typed, nil
	}
	return resolved, nil, dropInput()
}

func selectPaneTheme(n theme.Nomination, colourless bool, p paneAppearanceProbe) (theme.Theme, []byte) {
	if n.IsConstant() {
		return n.Constant(), nil
	}
	if n.IsZero() || colourless {
		return n.Select(theme.MemberDark), nil
	}
	member, typed := p.detect()
	return n.Select(member), typed
}

func (p paneAppearanceProbe) detect() (theme.Member, []byte) {
	if !p.isTerminal() {
		return theme.MemberDark, nil
	}
	restore, err := p.makeRaw()
	if err != nil {
		return theme.MemberDark, nil
	}
	defer restore()

	// Armed before the query is written: a query with no bounded read behind it
	// leaves the terminal's reply on the pane's input for whatever reads it next.
	reader, err := p.armRead(p.timeout)
	if err != nil {
		return theme.MemberDark, nil
	}
	if _, err := io.WriteString(p.out, ansi.RequestBackgroundColor); err != nil {
		return theme.MemberDark, nil
	}
	payload, typed, ok := readBackgroundReply(reader)
	if !ok {
		return theme.MemberDark, typed
	}
	return terminalReplyFrom(tea.BackgroundColorMsg{Color: ansi.XParseColor(payload)}).member, typed
}

func readBackgroundReply(r io.Reader) (payload string, typed []byte, ok bool) {
	buf := make([]byte, paneReplyCap)
	filled := 0
	for filled < len(buf) {
		n, err := r.Read(buf[filled:])
		filled += n
		if payload, typed, ok := splitBackgroundReply(buf[:filled]); ok || err != nil {
			return payload, typed, ok
		}
	}
	return splitBackgroundReply(buf)
}

// splitBackgroundReply parts what the probe read into the reply's payload, the
// text between the OSC 11 introducer and its terminator (BEL or either form of
// ST), and the bytes on either side of the reply, which were typed. An
// introducer with no terminator after it is the reply still arriving, so
// nothing from it on is typed; nor is the start of one cut off at the end of
// what was read, though an escape alone there is the Escape key.
func splitBackgroundReply(buf []byte) (payload string, typed []byte, ok bool) {
	before, rest, found := bytes.Cut(buf, []byte(oscBackgroundPrefix))
	if !found {
		return "", withoutIntroducerStart(buf), false
	}
	typed = bytes.Clone(before)
	for i, b := range rest {
		switch b {
		case ansi.BEL, ansi.ST:
			return string(rest[:i]), append(typed, rest[i+1:]...), true
		case ansi.ESC:
			if i+1 < len(rest) && rest[i+1] == '\\' {
				return string(rest[:i]), append(typed, rest[i+2:]...), true
			}
			return "", typed, false
		}
	}
	return "", typed, false
}

func withoutIntroducerStart(buf []byte) []byte {
	for n := min(len(oscBackgroundPrefix)-1, len(buf)); n >= 2; n-- {
		if bytes.HasSuffix(buf, []byte(oscBackgroundPrefix[:n])) {
			return bytes.Clone(buf[:len(buf)-n])
		}
	}
	return bytes.Clone(buf)
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
