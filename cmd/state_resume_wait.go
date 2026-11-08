package cmd

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/leeovery/portal/internal/bgreply"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/resumekeys"
	"github.com/spf13/cobra"
)

const (
	resumeKeyEnterCR = '\r'
	resumeKeyEnterLF = '\n'
	resumeKeyEscape  = 0x1b
)

// resumeResizeSettle is how long a size change waits for the next one before
// the panel is redrawn. A drag delivers changes every few tens of milliseconds,
// so the window closes when the drag stops rather than during it, and a single
// deliberate resize redraws with no perceptible pause.
const resumeResizeSettle = 150 * time.Millisecond

// resumeInputQuiet is how long a session's input must stay silent after a byte
// before the bytes that arrived together are judged. A keystroke arrives alone;
// a paste, an escape sequence and a burst each arrive as bytes far closer
// together than this.
const resumeInputQuiet = 50 * time.Millisecond

// resumeReplyTailHolds is how many quiet windows an arrival ending in a reply's
// head stays open for the rest of it, so a reply split across them is never
// judged as keys.
const resumeReplyTailHolds = int(bgreply.TailWait / resumeInputQuiet)

type resumeWaitConfig struct {
	resumeChainPayload

	Stdout     io.Writer
	In         io.Reader
	Logger     *slog.Logger
	IsTerminal func() bool
	MakeRaw    func() (restore func(), err error)
	ExecSelf   func(prog string, args []string)

	CatchSIGTERM func()

	// Winch and Settle are the redraw pair: a SIGWINCH arms the settle window
	// and each further one restarts it, so a drag of any length costs at most
	// one redraw. A SIGWINCH is the session's tty announcing a new size or zmx
	// attaching a client, so a panel painted at restore is repainted in the
	// theme as it is now, and nothing tells the two apart, so the window a
	// SIGWINCH opens redraws at whatever size it settles. Size reads the session
	// at start-up, and again when the window that start-up opened elapses.
	Winch  <-chan os.Signal
	Settle func(d time.Duration) <-chan time.Time
	Size   func() (int, int, error)

	// AwaitInput reports whether a byte arrived on the session within window,
	// reading nothing.
	AwaitInput func(window time.Duration) (bool, error)

	// Now is the wall clock the questions owed a reply were asked by.
	Now func() time.Time

	// ClearPending lowers the session's resume-pending flag; LookupResume
	// reads the registration at the moment the key is pressed rather than
	// carrying what the panel was drawn from, since the store can have changed
	// over an indefinite wait.
	ClearPending func() error
	LookupResume func(hookKey string) (hooks.OnResume, error)

	// DiscardRegistration removes the session's registration if it still
	// holds the command shown, reporting whether it removed one.
	DiscardRegistration func(hookKey, shown string) (bool, error)

	// EnableTTYSignals gives the kill keys back to a session being handed to a
	// hook or a shell. The raw restore cannot: it puts back the tty the waiter
	// found, which the chain keeps with signal generation off.
	EnableTTYSignals func() error

	// EnableEcho gives echo back to a session being handed on, which the draw
	// turned off and the raw restore therefore leaves off.
	EnableEcho func() error

	// BackToPicker sends the terminals showing the session to the session
	// picker, as Escape on the panel asks, reporting where they went.
	BackToPicker func() (backRoute, error)

	// Set by runResumeWait from MakeRaw, so an answer hands the session on in
	// a cooked tty. An answer reached without a wait restores nothing.
	restoreTerminal func()

	// replyHead is a reply cut off at the end of the arrival last judged, its
	// rest still to come, for the wait before a hand-off to finish.
	replyHead []byte
}

func (cfg resumeWaitConfig) restore() {
	if cfg.restoreTerminal != nil {
		cfg.restoreTerminal()
	}
}

// runResumeWait holds the session on whatever the draw painted until a key that
// screen offers answers it. A key answers only when it arrives alone, so a
// paste or a burst answers nothing whatever it carries, and every other byte is
// swallowed — the three keys that would signal a foreground process among them,
// since raw mode delivers them as bytes. A SIGTERM is caught and changes
// nothing; a hangup is not, so the session's end ends the waiter.
func runResumeWait(cfg resumeWaitConfig) error {
	cfg.CatchSIGTERM()
	cfg.Logger = hydrateLoggerOrDefault(cfg.Logger)

	// Spinning on a reader that is not a tty would burn a core for the life of
	// the session, and raw mode is what makes the kill keys arrive as bytes.
	if !cfg.IsTerminal() {
		return errors.New("resume wait: stdin is not a terminal")
	}
	restore, err := cfg.MakeRaw()
	if err != nil {
		return fmt.Errorf("resume wait: enter raw mode: %w", err)
	}
	cfg.restoreTerminal = sync.OnceFunc(restore)
	defer cfg.restore()

	return resumeWaitLoop(cfg)
}

// resumeKey is what one key does on one screen. ended reports whether it ended
// the wait by handing the session on, err being the hand-off's.
type resumeKey func(resumeWaitConfig) (ended bool, err error)

// handsOn is a key every outcome of which hands the session on — to another
// screen, a hook or a shell — and so ends the wait.
func handsOn(answer func(resumeWaitConfig) error) resumeKey {
	return func(cfg resumeWaitConfig) (bool, error) { return true, answer(cfg) }
}

// resumeKeys is what one screen does with a key that arrived alone. A key
// absent from it is swallowed.
type resumeKeys map[byte]resumeKey

// resumeKeysFor scopes the keys to the screen in front of the user: Enter
// resumes on the panel and must mean nothing on the confirmation one keystroke
// later. Escape backs out on both and changes nothing on either — out of the
// confirmation to the panel, and out of the panel to the session picker.
func resumeKeysFor(screen string) resumeKeys {
	if screen == resumeScreenDiscard {
		return resumeKeys{
			resumekeys.Confirm: handsOn(resumeAnswerDiscard),
			resumeKeyEscape:    handsOn(resumeCancelDiscardConfirm),
		}
	}
	return resumeKeys{
		resumeKeyEnterCR:   handsOn(resumeAnswerEnter),
		resumeKeyEnterLF:   handsOn(resumeAnswerEnter),
		resumekeys.Discard: handsOn(resumeOpenDiscardConfirm),
		resumeKeyEscape:    resumeBackToPicker,
	}
}

// answer does what a closed arrival means on this screen, reporting whether
// that ended the wait. Only a byte that arrived alone means anything.
func (k resumeKeys) answer(cfg resumeWaitConfig, judged resumeJudged) (ended bool, err error) {
	act := k[judged.key]
	if !judged.alone || act == nil {
		return false, nil
	}
	return act(cfg)
}

// resumeArrivalKept bounds the bytes an arrival keeps to be judged by: a key
// is one byte and a terminal's reply a few dozen, so an arrival longer than
// this holds no key alone.
const resumeArrivalKept = 256

// resumeArrival is the input that has reached the session since it was last
// quiet. A key is only a byte that arrived alone, with nothing behind it before
// the session fell quiet but the terminals' replies to a background question,
// which are no input of the user's.
type resumeArrival struct {
	kept     []byte
	overflow bool

	// carried marks input already arriving when the waiter started, whose
	// earlier bytes this waiter never read.
	carried bool

	// held counts the quiet windows it has been kept open past for a reply's
	// rest.
	held int
}

func (a *resumeArrival) add(b byte) {
	if len(a.kept) == resumeArrivalKept {
		a.overflow = true
		return
	}
	a.kept = append(a.kept, b)
}

func (a *resumeArrival) open() bool {
	return a.carried || len(a.kept) > 0
}

// hold keeps the arrival open through a quiet window while it ends in a reply
// whose rest is still to come, for as long as resumeReplyTailHolds allows.
func (a *resumeArrival) hold() bool {
	if a.held == resumeReplyTailHolds {
		return false
	}
	if _, _, head := bgreply.Split(a.kept); head == nil {
		return false
	}
	a.held++
	return true
}

// resumeJudged is an arrival closed: the byte left once every reply is taken
// out of it, whether that byte arrived alone, how many replies were among it,
// and a reply cut off at its end.
type resumeJudged struct {
	key     byte
	alone   bool
	replies int
	head    []byte
}

func (a *resumeArrival) end() resumeJudged {
	rest, payloads, head := bgreply.Split(a.kept)
	judged := resumeJudged{replies: len(payloads), head: head}
	if len(rest) == 1 && !a.carried && !a.overflow {
		judged.key, judged.alone = rest[0], true
	}
	*a = resumeArrival{}
	return judged
}

// judge closes the arrival. Each reply among it settles the oldest question
// owed, and a reply cut off at its end is kept for the wait before a hand-off
// to finish.
func (cfg *resumeWaitConfig) judge(arrival *resumeArrival) resumeJudged {
	judged := arrival.end()
	cfg.RepliesOwed = cfg.RepliesOwed.live(cfg.Now()).settle(judged.replies)
	cfg.replyHead = judged.head
	return judged
}

// The pending redraw dies with the process image a dispatched key execs, so a
// key answered inside a settle window cancels nothing. A redraw that comes due
// inside an arrival waits for the arrival to end and be judged: zmx's SIGWINCH
// on every attach opens the window, so a key typed as the session is opened
// may still be arriving as it closes, and a redraw then would drop it.
func resumeWaitLoop(cfg resumeWaitConfig) error {
	keys := resumeKeysFor(cfg.Screen)
	reader := startResumeReader(cfg.In, cfg.AwaitInput)
	defer close(reader.requests)

	settled := resumeStartupSettle(cfg)
	// Only the window start-up opened compares the size before it redraws: a
	// SIGWINCH may be zmx attaching a client at an unchanged size.
	signalled := false
	redrawDue := false
	arrival := resumeArrival{carried: cfg.InputArriving}
	for _, b := range []byte(cfg.Typed) {
		arrival.add(b)
	}
	cfg.InputArriving, cfg.Typed = false, ""
	outstanding := false
	for {
		if !outstanding {
			reader.requests <- resumeReadRequest{listen: arrival.open()}
			outstanding = true
		}

		select {
		case read := <-reader.results:
			outstanding = false
			if read.quiet {
				if arrival.hold() {
					continue
				}
				judged := cfg.judge(&arrival)
				if ended, err := keys.answer(cfg, judged); ended {
					return err
				}
				if redrawDue {
					return resumeRedraw(cfg)
				}
				continue
			}
			if read.n > 0 {
				arrival.add(read.b)
			}
			if read.err != nil {
				// No byte can follow the end of the input, so what arrived
				// last is judged as it stands.
				judged := cfg.judge(&arrival)
				if ended, err := keys.answer(cfg, judged); ended {
					return err
				}
				return fmt.Errorf("resume wait: read: %w", read.err)
			}

		case <-cfg.Winch:
			settled = cfg.Settle(resumeResizeSettle)
			signalled = true

		case <-settled:
			settled = nil
			if !signalled && resumeSizeUnchanged(cfg) {
				continue
			}
			if arrival.open() {
				redrawDue = true
				continue
			}
			return resumeRedraw(cfg)
		}
	}
}

// resumeStartupSettle answers a size change delivered before the watch was
// installed, which no SIGWINCH will announce, by opening the window one would
// have. A size the waiter cannot read, or a panel drawn at none, arms nothing:
// a read that keeps failing would otherwise bounce the session between draws.
func resumeStartupSettle(cfg resumeWaitConfig) <-chan time.Time {
	width, height, err := cfg.Size()
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return nil
	}
	if width == cfg.Width && height == cfg.Height {
		return nil
	}
	return cfg.Settle(resumeResizeSettle)
}

// A size that could not be read is not a size the panel was drawn at: the
// redraw resolves it to the renderer's own bounded fallback, where ending the
// wait would drop the panel over a transient failure.
func resumeSizeUnchanged(cfg resumeWaitConfig) bool {
	width, height, err := cfg.Size()
	return err == nil && width == cfg.Width && height == cfg.Height
}

type resumeRead struct {
	b     byte
	n     int
	err   error
	quiet bool
}

// resumeReadRequest asks for the next byte. A listen, made while an arrival is
// open, waits no longer than the quiet window and reads only a byte that has
// already arrived; any other request blocks until one does.
type resumeReadRequest struct {
	listen bool
}

type resumeReader struct {
	requests chan resumeReadRequest
	results  chan resumeRead
}

// startResumeReader reads one byte per request and never ahead, and a listen
// that finds the session quiet reads nothing, so an answer taken on that result
// runs with no read outstanding and input typed then is inherited by the
// process image the hand-off execs.
func startResumeReader(in io.Reader, awaitInput func(time.Duration) (bool, error)) resumeReader {
	reader := resumeReader{requests: make(chan resumeReadRequest), results: make(chan resumeRead, 1)}
	go func() {
		buf := make([]byte, 1)
		for request := range reader.requests {
			if request.listen {
				arrived, err := awaitInput(resumeInputQuiet)
				if err != nil || !arrived {
					reader.results <- resumeRead{err: err, quiet: err == nil}
					continue
				}
			}
			n, err := in.Read(buf)
			reader.results <- resumeRead{b: buf[0], n: n, err: err}
		}
	}()
	return reader
}

// resumeAnswerEnter hands the session back to its own transcript and starts
// what the store holds now over it, once its flag is lowered.
func resumeAnswerEnter(cfg resumeWaitConfig) error {
	cfg.awaitOwedReplies()
	if cleared, err := resumeUnfreeze(cfg); !cleared {
		return err
	}

	command := resumeRegistrationOrLog(cfg.Logger, cfg.LookupResume, cfg.Session).Command
	cfg.restore()
	enableTTYSignalsOrLog(cfg.Logger, cfg.EnableTTYSignals, cfg.Session)
	enableTTYEchoOrLog(cfg.Logger, cfg.EnableEcho, cfg.Session)

	handOffToHookOrShell(cfg.Logger, cfg.ExecSelf, command)
	return nil
}

// The rest of a burst that carried the d may still be arriving on the tty, so
// the draw is told to discard it before the confirmation goes up.
func resumeOpenDiscardConfirm(cfg resumeWaitConfig) error {
	cfg.DropInput = true
	return resumeShowScreen(cfg, resumeScreenDiscard)
}

func resumeCancelDiscardConfirm(cfg resumeWaitConfig) error {
	return resumeShowScreen(cfg, resumeScreenPanel)
}

// resumeBackToPicker is Escape on the panel. It backs out of the session
// rather than answering it: the terminals showing the session go back to the
// session picker, or to their shells with no runner to show one, and the
// session keeps waiting with its registration and its flag untouched, so the
// panel is there again the next time the session is opened. One the runners
// or the backend refused redraws the panel with the reason.
func resumeBackToPicker(cfg resumeWaitConfig) (ended bool, err error) {
	route, backErr := cfg.BackToPicker()
	if backErr != nil {
		cfg.Logger.Warn("back to picker failed", cfg.logAttrs("error", backErr)...)
		return true, resumeReport(cfg, resumeScreenPanel, resumeBackRefusal(backErr))
	}
	if route == backToShells {
		cfg.Logger.Info("detached to shell", cfg.logAttrs()...)
	} else {
		cfg.Logger.Info("back to picker", cfg.logAttrs()...)
	}
	return false, nil
}

// resumeAnswerDiscard is the confirmation's y. The removal runs while the
// confirmation is still on screen, so a refused write is reported there rather
// than closing the confirmation as a cancel would. A discard that found nothing
// to remove still proceeds: the store is already as the user asked for it.
func resumeAnswerDiscard(cfg resumeWaitConfig) error {
	if _, err := cfg.DiscardRegistration(cfg.Session, cfg.Command); err != nil {
		return resumeReport(cfg, resumeScreenDiscard, resumeDiscardRefusal(err))
	}

	cfg.awaitOwedReplies()
	if cleared, err := resumeUnfreeze(cfg); !cleared {
		return err
	}
	cfg.restore()
	enableTTYSignalsOrLog(cfg.Logger, cfg.EnableTTYSignals, cfg.Session)
	enableTTYEchoOrLog(cfg.Logger, cfg.EnableEcho, cfg.Session)

	handOffToHookOrShell(cfg.Logger, cfg.ExecSelf, "")
	return nil
}

// awaitOwedReplies holds a hand-off to the hook or a shell while a question
// is owed its reply within its window, reading the replies off the session's
// input as they come, each settling the oldest question owed, so neither ever
// reads one. It ends once nothing is owed or the newest question's window has
// closed, and with nothing owed the hand-off is immediate. An answer is taken
// with no read outstanding, so nothing else reads the input meanwhile. A key
// typed in the wait is lost whole, the one cost: once a byte no reply accounts
// for is read, the wait reads on to the quiet that ends what was typed, so no
// escape sequence's tail reaches what the session is handed to.
func (cfg *resumeWaitConfig) awaitOwedReplies() {
	now := cfg.Now()
	cfg.RepliesOwed = cfg.RepliesOwed.live(now)
	if !cfg.RepliesOwed.owing() {
		return
	}
	due := cfg.RepliesOwed.lastDue(now)
	read := replyRead{got: cfg.replyHead}
	cfg.replyHead = nil
	buf := make([]byte, 1)
	for read.n < resumeArrivalKept {
		window, ok := read.listen(cfg.RepliesOwed, due.Sub(now))
		if !ok {
			return
		}
		arrived, err := cfg.AwaitInput(window)
		if err != nil {
			return
		}
		now = cfg.Now()
		if !arrived {
			read.quiet()
			continue
		}
		if n, err := cfg.In.Read(buf); n == 0 || err != nil {
			return
		}
		cfg.RepliesOwed = cfg.RepliesOwed.settle(read.take(buf[0]))
	}
}

// replyRead is what the wait before a hand-off has read since the session was
// last quiet.
type replyRead struct {
	got []byte
	// settled counts the whole replies in got already settled.
	settled int
	// typed marks a byte in got that no reply accounts for.
	typed bool
	// n counts every byte the wait has read.
	n int
}

// listen is how long the wait listens next, ok false once it is over: a key
// typed is read on to the quiet that ends it, and otherwise the wait lasts
// while a question is owed and left of the time due.
func (r replyRead) listen(owed replyDebt, left time.Duration) (window time.Duration, ok bool) {
	if r.typed {
		return resumeInputQuiet, true
	}
	if !owed.owing() || left <= 0 {
		return 0, false
	}
	return min(resumeInputQuiet, left), true
}

// take adds a byte read, answering how many replies it finished.
func (r *replyRead) take(b byte) (finished int) {
	r.n++
	r.got = append(r.got, b)
	_, payloads, _ := bgreply.Split(r.got)
	finished, r.settled = len(payloads)-r.settled, len(payloads)
	r.typed = r.typed || bgreply.Typed(r.got)
	return finished
}

// quiet ends what had arrived: keys typed in it are dropped with the replies
// read, and only a reply still arriving is kept for its rest.
func (r *replyRead) quiet() {
	_, _, head := bgreply.Split(r.got)
	r.got, r.settled, r.typed = head, 0, false
}

// resumeUnfreeze hands the session back to its own transcript and then lowers
// its flag, in that order, so a saver tick landing between the two captures
// what is really in the session. A clear that is refused brings the waiting
// panel back with the reason, since a session handed on flagged is never
// captured again; cleared is false then, and err is the redraw's.
func resumeUnfreeze(cfg resumeWaitConfig) (cleared bool, err error) {
	_, _ = io.WriteString(cfg.Stdout, hydrateResetPreamble)

	if err := cfg.ClearPending(); err != nil {
		warnResumePendingClearFailed(cfg.Logger, cfg.Session, err)
		return false, resumeReport(cfg, resumeScreenPanel, resumeClearRefusal(err))
	}
	return true, nil
}

// resumeReport hands the session to a fresh draw of screen carrying row as the
// report. The draw paints what the payload names without consulting the store,
// so a registration already gone is still shown rather than a blank screen.
func resumeReport(cfg resumeWaitConfig, screen, row string) error {
	next := cfg
	next.Screen = screen
	next.Report = row
	next.DropInput = false
	return resumeRedraw(next)
}

// resumeShowScreen hands the session to a fresh draw of screen. The key press
// that moved it there ends any report the waiter was holding.
func resumeShowScreen(cfg resumeWaitConfig, screen string) error {
	next := cfg
	next.Screen = screen
	next.Report = ""
	return resumeRedraw(next)
}

// resumeRedraw hands the session to a fresh draw of the screen its payload
// names, carrying that payload whole. A binary that cannot be resolved ends the
// wait as a read error does: the flag is still raised, so the chain's tail
// recovers the session to a usable shell rather than leaving one whose keys
// silently do nothing.
func resumeRedraw(cfg resumeWaitConfig) error {
	cfg.restore()
	return resumeHandOff(cfg.Logger, cfg.ExecSelf, resumeDrawSubcommand, cfg.resumeChainPayload)
}

// A store that cannot even be resolved reads as no registration, which drops
// the session to a plain shell exactly as an unreadable one does.
func lookupResumeRegistration(hookKey string) (hooks.OnResume, error) {
	store, _ := loadHookStore()
	if store == nil {
		return hooks.OnResume{}, nil
	}
	return store.LookupOnResume(hookKey, hooks.ViaHydrate)
}

// discardResumeRegistration removes the session's registration when it still
// holds the command shown. Unlike the lookup, a store that cannot be resolved
// is an error: reading it as a miss would drop the panel while the registration
// it named survives. The store records its own refusals; one it was never
// reached for is recorded here.
func discardResumeRegistration(session, shown string) (bool, error) {
	store, err := loadHookStore()
	if err != nil {
		hydrateLogger.Warn("resolve hook store failed", resumeLogAttrs(session, "error", err)...)
		return false, hookStoreUnlocatedError{err: err}
	}
	return store.Discard(session, hooks.EventOnResume, shown, hooks.ViaPanel)
}

// A hangup keeps its default disposition, so the session's end ends the
// waiter with it.
func winchSignals() <-chan os.Signal {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	return ch
}

func awaitStdinInput(window time.Duration) (bool, error) {
	return awaitTTYInput(int(os.Stdin.Fd()), window)
}

func stdinIsTerminal() bool {
	return term.IsTerminal(os.Stdin.Fd())
}

func makeStdinRaw() (func(), error) {
	fd := os.Stdin.Fd()
	prior, err := term.MakeRaw(fd)
	if err != nil {
		return nil, err
	}
	return func() { _ = term.Restore(fd, prior) }, nil
}

var resumeWaitRunFunc = runResumeWait

// stateResumeWaitCmd holds a session between the screens of its resume panel.
var stateResumeWaitCmd = &cobra.Command{
	Use:    resumeWaitSubcommand,
	Short:  "Hold a session showing the resume panel until it is answered (internal)",
	Args:   cobra.NoArgs,
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		command, _ := cmd.Flags().GetString(resumeFlagCommand)
		report, _ := cmd.Flags().GetString(resumeFlagReport)
		session, _ := cmd.Flags().GetString(resumeFlagSession)
		width, _ := cmd.Flags().GetInt(resumeFlagWidth)
		height, _ := cmd.Flags().GetInt(resumeFlagHeight)
		screen, _ := cmd.Flags().GetString(resumeFlagScreen)
		inputArriving, _ := cmd.Flags().GetBool(resumeFlagInputArriving)
		typedHex, _ := cmd.Flags().GetString(resumeFlagTyped)
		typed, unnamed := typedFromFlag(typedHex)
		repliesOwed, _ := cmd.Flags().GetString(resumeFlagRepliesOwed)

		return resumeWaitRunFunc(resumeWaitConfig{
			resumeChainPayload: resumeChainPayload{
				Command: command,
				Report:  report,
				Session: session,
				Width:   width,
				Height:  height,
				Screen:  screen,

				InputArriving: inputArriving || unnamed,
				Typed:         typed,
				RepliesOwed:   replyDebtFromFlag(repliesOwed, time.Now()),
			},
			Stdout:              os.Stdout,
			In:                  os.Stdin,
			Logger:              hydrateLogger,
			IsTerminal:          stdinIsTerminal,
			MakeRaw:             makeStdinRaw,
			ExecSelf:            defaultExecShell,
			Winch:               winchSignals(),
			Settle:              time.After,
			Size:                sessionSizeFromStdin,
			AwaitInput:          awaitStdinInput,
			Now:                 time.Now,
			ClearPending:        func() error { return clearResumePending(session) },
			LookupResume:        lookupResumeRegistration,
			DiscardRegistration: discardResumeRegistration,
			EnableTTYSignals:    setStdinSignals,
			EnableEcho:          setStdinEcho,
			BackToPicker:        func() (backRoute, error) { return backToPicker(session) },
			CatchSIGTERM:        catchSIGTERM,
		})
	},
}

func init() {
	stateResumeWaitCmd.Flags().String(resumeFlagCommand, "", "The registered on-resume command the panel states")
	stateResumeWaitCmd.Flags().String(resumeFlagReport, "", "What an answer that could not be carried out reported")
	stateResumeWaitCmd.Flags().String(resumeFlagSession, "", "The waiting session: its resume hook's key and its resume-pending flag")
	stateResumeWaitCmd.Flags().Int(resumeFlagWidth, 0, "Width the screen the session is showing was drawn at")
	stateResumeWaitCmd.Flags().Int(resumeFlagHeight, 0, "Height the screen the session is showing was drawn at")
	stateResumeWaitCmd.Flags().String(resumeFlagScreen, resumeScreenPanel, "Which screen the session is showing: discard for the confirmation, else the panel")
	stateResumeWaitCmd.Flags().Bool(resumeFlagInputArriving, false, "Input was still arriving on the session when it was handed over")
	stateResumeWaitCmd.Flags().String(resumeFlagTyped, "", "The key, in hex, the draw read while it asked the terminal its background")
	stateResumeWaitCmd.Flags().String(resumeFlagRepliesOwed, "", "When each background question still owed its reply was asked, in unix milliseconds, oldest first, comma-separated")
	_ = stateResumeWaitCmd.MarkFlagRequired(resumeFlagCommand)
	_ = stateResumeWaitCmd.MarkFlagRequired(resumeFlagSession)

	stateCmd.AddCommand(stateResumeWaitCmd)
}
