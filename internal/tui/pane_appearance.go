package tui

import (
	"bytes"
	"io"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
	"github.com/leeovery/portal/internal/theme"
)

// os.Stdin refuses a read deadline on a terminal — os.NewFile over a blocking tty
// descriptor, which is how the runtime builds it — so the reply is read from a
// fresh open of the pane's own pty instead.
const paneTTYPath = "/dev/tty"

const oscBackgroundPrefix = "\x1b]11;"

// A background reply is some 30 bytes; the cap bounds a terminal that answers
// something else entirely without ever terminating it.
const paneReplyCap = 256

// paneReader is the terminal the reply is read from: a deadline-bearing read the
// probe can abandon.
type paneReader interface {
	Read(p []byte) (int, error)
	SetReadDeadline(t time.Time) error
	Close() error
}

// paneAppearanceProbe is the non-Bubble-Tea OSC 11 read, for the process that
// paints a pane and then execs a waiter over itself — it has no program loop for
// tea.RequestBackgroundColor to resolve into.
type paneAppearanceProbe struct {
	out        io.Writer
	openReader func() (paneReader, error)
	isTerminal func() bool
	makeRaw    func() (restore func(), err error)
	timeout    time.Duration
}

func newPaneAppearanceProbe() paneAppearanceProbe {
	return paneAppearanceProbe{
		out:        os.Stdout,
		openReader: func() (paneReader, error) { return openTTYPath(paneTTYPath) },
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
// dropInput runs once the appearance question has resolved and never before it:
// the terminal's own answer can arrive after the probe's deadline, and a drop
// taken first cannot clear it. A nil dropInput is a no-op. The palette is
// returned whether or not the drop succeeded, alongside the drop's own error.
func ResolvePaneTheme(n theme.Nomination, colourless bool, dropInput func() error) (theme.Theme, error) {
	return resolvePaneTheme(n, colourless, dropInput, newPaneAppearanceProbe())
}

func resolvePaneTheme(n theme.Nomination, colourless bool, dropInput func() error, p paneAppearanceProbe) (theme.Theme, error) {
	resolved := selectPaneTheme(n, colourless, p)
	if dropInput == nil {
		return resolved, nil
	}
	return resolved, dropInput()
}

func selectPaneTheme(n theme.Nomination, colourless bool, p paneAppearanceProbe) theme.Theme {
	if n.IsConstant() {
		return n.Constant()
	}
	if n.IsZero() || colourless {
		return n.Select(theme.MemberDark)
	}
	return n.Select(p.detect())
}

func (p paneAppearanceProbe) detect() theme.Member {
	if !p.isTerminal() {
		return theme.MemberDark
	}
	reader, err := p.openReader()
	if err != nil {
		return theme.MemberDark
	}
	defer func() { _ = reader.Close() }()

	restore, err := p.makeRaw()
	if err != nil {
		return theme.MemberDark
	}
	defer restore()

	if _, err := io.WriteString(p.out, ansi.RequestBackgroundColor); err != nil {
		return theme.MemberDark
	}
	// Without a deadline the read blocks forever on a pane nobody is watching,
	// which is the common case rather than the edge.
	if err := reader.SetReadDeadline(time.Now().Add(p.timeout)); err != nil {
		return theme.MemberDark
	}
	payload, ok := readBackgroundReply(reader)
	if !ok {
		return theme.MemberDark
	}
	return terminalReplyFrom(tea.BackgroundColorMsg{Color: ansi.XParseColor(payload)}).member
}

func readBackgroundReply(r paneReader) (string, bool) {
	buf := make([]byte, paneReplyCap)
	filled := 0
	for filled < len(buf) {
		n, err := r.Read(buf[filled:])
		filled += n
		if payload, ok := backgroundPayload(buf[:filled]); ok {
			return payload, true
		}
		if err != nil {
			return "", false
		}
	}
	return "", false
}

// The payload between the OSC 11 introducer and its terminator, which a terminal
// writes as BEL or as either form of ST.
func backgroundPayload(buf []byte) (string, bool) {
	_, rest, found := bytes.Cut(buf, []byte(oscBackgroundPrefix))
	if !found {
		return "", false
	}
	for i, b := range rest {
		switch b {
		case ansi.BEL, ansi.ST:
			return string(rest[:i]), true
		case ansi.ESC:
			if i+1 < len(rest) && rest[i+1] == '\\' {
				return string(rest[:i]), true
			}
			return "", false
		}
	}
	return "", false
}

func openTTYPath(path string) (paneReader, error) {
	f, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// Raw mode is a property of the terminal rather than of a descriptor onto it, so
// the freshly opened reader sees it without a second MakeRaw.
func makeStdinRaw() (func(), error) {
	fd := os.Stdin.Fd()
	state, err := term.MakeRaw(fd)
	if err != nil {
		return nil, err
	}
	return func() { _ = term.Restore(fd, state) }, nil
}
