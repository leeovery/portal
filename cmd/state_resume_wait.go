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
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/resumekeys"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
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

// resumeInputQuiet is how long a pane's input must stay silent after a byte
// before the bytes that arrived together are judged. A keystroke arrives alone;
// a paste, an escape sequence and a burst each arrive as bytes far closer
// together than this.
const resumeInputQuiet = 50 * time.Millisecond

type resumeWaitConfig struct {
	resumeChainPayload

	Stdout     io.Writer
	In         io.Reader
	Logger     *slog.Logger
	IsTerminal func() bool
	MakeRaw    func() (restore func(), err error)
	ExecSelf   func(prog string, args []string)

	// Winch and Settle are the resize pair: a change arms the settle window and
	// each further change restarts it, so a drag of any length costs at most one
	// redraw. Size reads the pane at start-up and at the moment a window
	// elapses.
	Winch  <-chan os.Signal
	Settle func(d time.Duration) <-chan time.Time
	Size   func() (int, int, error)

	// AwaitInput reports whether a byte arrived on the pane within window,
	// reading nothing.
	AwaitInput func(window time.Duration) (bool, error)

	// ClearMarker lifts the pane's freeze; LookupResume reads the registration
	// at the moment the key is pressed rather than carrying what the panel was
	// drawn from, since the store can have changed over an indefinite wait.
	ClearMarker  func() error
	LookupResume func(hookKey string) (hooks.OnResume, error)

	// DiscardRegistration removes the pane's registration if it still holds
	// the command shown, reporting whether it removed one.
	DiscardRegistration func(hookKey, shown string) (bool, error)

	// EnableTTYSignals gives the kill keys back to a pane being handed to a
	// hook or a shell. The raw restore cannot: it puts back the tty the waiter
	// found, which the chain keeps with signal generation off.
	EnableTTYSignals func() error

	AltScreen altScreenPin

	// Set by runResumeWait from MakeRaw, so an answer hands the pane on in a
	// cooked tty. An answer reached without a wait restores nothing.
	restoreTerminal func()
}

func (cfg resumeWaitConfig) restore() {
	if cfg.restoreTerminal != nil {
		cfg.restoreTerminal()
	}
}

// runResumeWait holds the pane on whatever the draw painted until a key that
// screen offers answers it. A key answers only when it arrives alone, so a
// paste or a burst answers nothing whatever it carries, and every other byte is
// swallowed — the three keys that would signal a foreground process among them,
// since raw mode delivers them as bytes. No signal is declined: tmux tearing the
// pane down ends the waiter.
func runResumeWait(cfg resumeWaitConfig) error {
	cfg.Logger = hydrateLoggerOrDefault(cfg.Logger)

	// Spinning on a reader that is not a tty would burn a core for the life of
	// the pane, and raw mode is what makes the kill keys arrive as bytes.
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

// resumeKeys is what one screen answers to. A key absent from bytes is
// swallowed, and a nil escape leaves the Escape key inert.
type resumeKeys struct {
	bytes  map[byte]func(resumeWaitConfig) error
	escape func(resumeWaitConfig) error
}

// resumeKeysFor scopes the keys to the screen in front of the user: Enter
// resumes on the panel and must mean nothing on the confirmation one keystroke
// later, and Escape backs out of the confirmation but has nothing to back out
// of on the panel.
func resumeKeysFor(screen string) resumeKeys {
	if screen == resumeScreenDiscard {
		return resumeKeys{
			bytes:  map[byte]func(resumeWaitConfig) error{resumekeys.Confirm: resumeAnswerDiscard},
			escape: resumeCancelDiscardConfirm,
		}
	}
	return resumeKeys{bytes: map[byte]func(resumeWaitConfig) error{
		resumeKeyEnterCR:   resumeAnswerEnter,
		resumeKeyEnterLF:   resumeAnswerEnter,
		resumekeys.Discard: resumeOpenDiscardConfirm,
	}}
}

// answerTo is the answer an arrival gives, nil unless it was one byte alone.
func (k resumeKeys) answerTo(key byte, alone bool) func(resumeWaitConfig) error {
	switch {
	case !alone:
		return nil
	case key == resumeKeyEscape:
		return k.escape
	}
	return k.bytes[key]
}

// resumeArrival is the input that has reached the pane since it was last quiet.
// A key is only a byte that arrived alone, with nothing behind it before the
// pane fell quiet.
type resumeArrival struct {
	key   byte
	bytes int

	// carried marks input already arriving when the waiter started, whose
	// earlier bytes this waiter never read.
	carried bool
}

func (a *resumeArrival) add(b byte) {
	if a.bytes == 0 {
		a.key = b
	}
	a.bytes++
}

func (a *resumeArrival) open() bool {
	return a.carried || a.bytes > 0
}

// end closes the arrival, answering the byte it opened with and whether that
// byte arrived alone.
func (a *resumeArrival) end() (key byte, alone bool) {
	key, alone = a.key, a.bytes == 1 && !a.carried
	*a = resumeArrival{}
	return key, alone
}

// The pending redraw dies with the process image a dispatched key execs, so a
// key answered inside a settle window cancels nothing. A redraw inside an
// arrival hands the arrival to the next waiter, so the rest of it answers
// nothing there either.
func resumeWaitLoop(cfg resumeWaitConfig) error {
	keys := resumeKeysFor(cfg.Screen)
	reader := startResumeReader(cfg.In, cfg.AwaitInput)
	defer close(reader.requests)

	settled := resumeStartupSettle(cfg)
	arrival := resumeArrival{carried: cfg.InputArriving}
	cfg.InputArriving = false
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
				if answer := keys.answerTo(arrival.end()); answer != nil {
					return answer(cfg)
				}
				continue
			}
			if read.n > 0 {
				arrival.add(read.b)
			}
			if read.err != nil {
				// No byte can follow the end of the input, so what arrived
				// last is judged as it stands.
				if answer := keys.answerTo(arrival.end()); answer != nil {
					return answer(cfg)
				}
				return fmt.Errorf("resume wait: read: %w", read.err)
			}

		case <-cfg.Winch:
			settled = cfg.Settle(resumeResizeSettle)

		case <-settled:
			settled = nil
			if resumeSizeUnchanged(cfg) {
				continue
			}
			cfg.InputArriving = arrival.open()
			return resumeRedraw(cfg)
		}
	}
}

// resumeStartupSettle answers a size change delivered before the watch was
// installed, which no SIGWINCH will announce, by opening the window one would
// have. A size the waiter cannot read, or a panel drawn at none, arms nothing:
// a read that keeps failing would otherwise bounce the pane between draws.
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
// that finds the pane quiet reads nothing, so an answer taken on that result
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

// resumeAnswerEnter hands the pane back to its own transcript and starts what
// the store holds now over it, once the freeze has lifted.
func resumeAnswerEnter(cfg resumeWaitConfig) error {
	if cleared, err := resumeUnfreeze(cfg); !cleared {
		return err
	}

	command := resumeRegistrationOrLog(cfg.Logger, cfg.LookupResume, cfg.HookKey).Command
	cfg.restore()
	enableTTYSignalsOrLog(cfg.Logger, cfg.EnableTTYSignals, cfg.paneRef())

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

// resumeAnswerDiscard is the confirmation's y. The removal runs while the
// confirmation is still on screen, so a refused write is reported there rather
// than closing the confirmation as a cancel would. A discard that found nothing
// to remove still proceeds: the store is already as the user asked for it.
func resumeAnswerDiscard(cfg resumeWaitConfig) error {
	if _, err := cfg.DiscardRegistration(cfg.HookKey, cfg.Command); err != nil {
		return resumeReport(cfg, resumeScreenDiscard, resumeDiscardRefusal(err))
	}

	if cleared, err := resumeUnfreeze(cfg); !cleared {
		return err
	}
	cfg.restore()
	enableTTYSignalsOrLog(cfg.Logger, cfg.EnableTTYSignals, cfg.paneRef())

	handOffToHookOrShell(cfg.Logger, cfg.ExecSelf, "")
	return nil
}

// resumeUnfreeze hands the pane back to its own transcript and then lifts the
// freeze, in that order, so a saver tick landing between the two captures what
// is really in the pane. A clear that is refused brings the waiting panel back
// with the reason, since a pane handed on frozen stays frozen for the rest of
// its life; cleared is false then, err is the redraw's, and the alternate-screen
// pin stays for that redraw to paint onto.
func resumeUnfreeze(cfg resumeWaitConfig) (cleared bool, err error) {
	_, _ = io.WriteString(cfg.Stdout, hydrateResetPreamble)

	if err := cfg.ClearMarker(); err != nil {
		warnResumePendingClearFailed(cfg.Logger, cfg.paneRef(), err)
		return false, resumeReport(cfg, resumeScreenPanel, resumeClearRefusal(err))
	}
	releaseAltScreenPin(cfg.Logger, cfg.paneRef(), cfg.AltScreen)
	return true, nil
}

// resumeReport hands the pane to a fresh draw of screen carrying row as the
// report. The draw paints what the payload names without consulting the store,
// so a registration already gone is still shown rather than a blank pane.
func resumeReport(cfg resumeWaitConfig, screen, row string) error {
	next := cfg
	next.Screen = screen
	next.Report = row
	next.DropInput = false
	return resumeRedraw(next)
}

// resumeShowScreen hands the pane to a fresh draw of screen. The key press that
// moved it there ends any report the waiter was holding.
func resumeShowScreen(cfg resumeWaitConfig, screen string) error {
	next := cfg
	next.Screen = screen
	next.Report = ""
	return resumeRedraw(next)
}

// resumeRedraw hands the pane to a fresh draw of the screen its payload names,
// carrying that payload whole. A binary that cannot be resolved ends the wait as
// a read error does: the pending marker is still set, so the chain's tail
// recovers the pane to a usable shell rather than leaving one whose keys
// silently do nothing.
func resumeRedraw(cfg resumeWaitConfig) error {
	cfg.restore()
	return resumeHandOff(cfg.Logger, cfg.ExecSelf, resumeDrawSubcommand, cfg.resumeChainPayload)
}

// A store that cannot even be resolved reads as no registration, which drops
// the pane to a plain shell exactly as an unreadable one does.
func lookupResumeRegistration(hookKey string) (hooks.OnResume, error) {
	store, _ := loadHookStore()
	if store == nil {
		return hooks.OnResume{}, nil
	}
	return store.LookupOnResume(hookKey, hooks.ViaHydrate)
}

// discardResumeRegistration removes the pane's registration when it still
// holds the command shown. Unlike the lookup, a store that cannot be resolved
// is an error: reading it as a miss would drop the panel while the registration
// it named survives. The store records its own refusals; one it was never
// reached for is recorded here.
func discardResumeRegistration(pane resumePaneRef, shown string) (bool, error) {
	store, err := loadHookStore()
	if err != nil {
		hydrateLogger.Warn("resolve hook store failed", pane.logAttrs("error", err)...)
		return false, hookStoreUnlocatedError{err: err}
	}
	return store.Discard(pane.HookKey, hooks.EventOnResume, shown, hooks.ViaPanel)
}

// A hangup keeps its default disposition, so tmux tearing the pane down ends
// the waiter with it.
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

// stateResumeWaitCmd holds a pane between the screens of its resume panel.
var stateResumeWaitCmd = &cobra.Command{
	Use:    resumeWaitSubcommand,
	Short:  "Hold a pane showing the resume panel until it is answered (internal)",
	Args:   cobra.NoArgs,
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		command, _ := cmd.Flags().GetString(resumeFlagCommand)
		report, _ := cmd.Flags().GetString(resumeFlagReport)
		hookKey, _ := cmd.Flags().GetString(resumeFlagHookKey)
		pane, _ := cmd.Flags().GetString(resumeFlagPane)
		paneKey, _ := cmd.Flags().GetString(resumeFlagPaneKey)
		width, _ := cmd.Flags().GetInt(resumeFlagWidth)
		height, _ := cmd.Flags().GetInt(resumeFlagHeight)
		screen, _ := cmd.Flags().GetString(resumeFlagScreen)
		inputArriving, _ := cmd.Flags().GetBool(resumeFlagInputArriving)

		return resumeWaitRunFunc(resumeWaitConfig{
			resumeChainPayload: resumeChainPayload{
				Command: command,
				Report:  report,
				HookKey: hookKey,
				Pane:    pane,
				PaneKey: paneKey,
				Width:   width,
				Height:  height,
				Screen:  screen,

				InputArriving: inputArriving,
			},
			Stdout:     os.Stdout,
			In:         os.Stdin,
			Logger:     hydrateLogger,
			IsTerminal: stdinIsTerminal,
			MakeRaw:    makeStdinRaw,
			ExecSelf:   defaultExecShell,
			Winch:      winchSignals(),
			Settle:     time.After,
			Size:       paneSizeFromStdin,
			AwaitInput: awaitStdinInput,
			ClearMarker: func() error {
				return state.UnsetResumePendingMarker(tmux.DefaultClient(), tmux.PaneIDTarget(pane))
			},
			LookupResume: lookupResumeRegistration,
			DiscardRegistration: func(hookKey, shown string) (bool, error) {
				return discardResumeRegistration(resumePaneRef{HookKey: hookKey, PaneKey: paneKey}, shown)
			},
			EnableTTYSignals: setStdinSignals,
			AltScreen:        paneAltScreenPin(tmux.DefaultClient(), tmux.PaneIDTarget(pane)),
		})
	},
}

func init() {
	stateResumeWaitCmd.Flags().String(resumeFlagCommand, "", "The registered on-resume command the panel states")
	stateResumeWaitCmd.Flags().String(resumeFlagReport, "", "What an answer that could not be carried out reported")
	stateResumeWaitCmd.Flags().String(resumeFlagHookKey, "", "Saved pane token identifying the pane's resume hook")
	stateResumeWaitCmd.Flags().String(resumeFlagPane, "", "The pane id the marker writes address")
	stateResumeWaitCmd.Flags().String(resumeFlagPaneKey, "", "The pane key the chain's records name the pane by")
	stateResumeWaitCmd.Flags().Int(resumeFlagWidth, 0, "Width the screen the pane is showing was drawn at")
	stateResumeWaitCmd.Flags().Int(resumeFlagHeight, 0, "Height the screen the pane is showing was drawn at")
	stateResumeWaitCmd.Flags().String(resumeFlagScreen, resumeScreenPanel, "Which screen the pane is showing: discard for the confirmation, else the panel")
	stateResumeWaitCmd.Flags().Bool(resumeFlagInputArriving, false, "Input was still arriving on the pane when it was handed over")
	_ = stateResumeWaitCmd.MarkFlagRequired(resumeFlagCommand)

	stateCmd.AddCommand(stateResumeWaitCmd)
}
