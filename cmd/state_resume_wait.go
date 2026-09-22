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
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

const (
	resumeKeyEnterCR = '\r'
	resumeKeyEnterLF = '\n'
	resumeKeyDiscard = 'd'
)

// resumeResizeSettle is how long a size change waits for the next one before
// the panel is redrawn. A drag delivers changes every few tens of milliseconds,
// so the window closes when the drag stops rather than during it, and a single
// deliberate resize redraws with no perceptible pause.
const resumeResizeSettle = 150 * time.Millisecond

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
	// redraw. Size reads the pane at the moment the window elapses.
	Winch  <-chan os.Signal
	Settle func(d time.Duration) <-chan time.Time
	Size   func() (int, int, error)

	// ClearMarker lifts the pane's freeze; LookupResume reads the registration
	// at the moment the key is pressed rather than carrying what the panel was
	// drawn from, since the store can have changed over an indefinite wait.
	ClearMarker  func() error
	LookupResume func(hookKey string) (hooks.OnResume, error)

	// Set by runResumeWait from MakeRaw, so an answer hands the pane on in a
	// cooked tty. An answer reached without a wait restores nothing.
	restoreTerminal func()
}

func (cfg resumeWaitConfig) restore() {
	if cfg.restoreTerminal != nil {
		cfg.restoreTerminal()
	}
}

// runResumeWait holds the pane on whatever the draw painted until Enter or d
// answers it. Every other byte is discarded, so nothing the user did not send —
// a paste, a send-keys, a key aimed at another window — can answer the panel,
// and the three keys that would kill a foreground process are bytes like any
// other under raw mode. No signal is declined: tmux tearing the pane down ends
// the waiter.
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

// The pending redraw dies with the process image a dispatched key execs, so a
// key answered inside a settle window cancels nothing.
func resumeWaitLoop(cfg resumeWaitConfig) error {
	reader := startResumeReader(cfg.In)
	defer close(reader.requests)

	var settled <-chan time.Time
	outstanding := false
	for {
		if !outstanding {
			reader.requests <- struct{}{}
			outstanding = true
		}

		select {
		case read := <-reader.results:
			outstanding = false
			if read.n > 0 {
				switch read.b {
				case resumeKeyEnterCR, resumeKeyEnterLF:
					return resumeAnswerEnter(cfg)
				case resumeKeyDiscard:
					return resumeAnswerDiscard(cfg)
				}
			}
			if read.err != nil {
				return fmt.Errorf("resume wait: read: %w", read.err)
			}

		case <-cfg.Winch:
			settled = cfg.Settle(resumeResizeSettle)

		case <-settled:
			settled = nil
			if resumeSizeUnchanged(cfg) {
				continue
			}
			return resumeRedraw(cfg)
		}
	}
}

// A size that could not be read is not a size the panel was drawn at: the
// redraw resolves it to the renderer's own bounded fallback, where ending the
// wait would drop the panel over a transient failure.
func resumeSizeUnchanged(cfg resumeWaitConfig) bool {
	width, height, err := cfg.Size()
	return err == nil && width == cfg.Width && height == cfg.Height
}

type resumeRead struct {
	b   byte
	n   int
	err error
}

type resumeReader struct {
	requests chan struct{}
	results  chan resumeRead
}

// startResumeReader reads one byte per request and never ahead: exactly one read
// is outstanding at a time, so input still queued when a key is answered is
// inherited by the process image the hand-off execs.
func startResumeReader(in io.Reader) resumeReader {
	reader := resumeReader{requests: make(chan struct{}), results: make(chan resumeRead, 1)}
	go func() {
		buf := make([]byte, 1)
		for range reader.requests {
			n, err := in.Read(buf)
			reader.results <- resumeRead{b: buf[0], n: n, err: err}
		}
	}()
	return reader
}

// resumeAnswerEnter hands the pane back to its own transcript and starts what
// the store holds now over it. The order is the protection: the pane leaves the
// panel's screen first, so a saver tick landing mid-answer captures what is
// really in the pane, and nothing runs while the marker stands, since a pane
// handed over frozen stays frozen for the rest of its life with nothing left to
// report it.
func resumeAnswerEnter(cfg resumeWaitConfig) error {
	_, _ = io.WriteString(cfg.Stdout, hydrateResetPreamble)

	if err := cfg.ClearMarker(); err != nil {
		reported := cfg
		reported.Report = err.Error()
		return resumeRedraw(reported)
	}

	command := resumeRegistrationOrLog(cfg.Logger, cfg.LookupResume, cfg.HookKey).Command
	cfg.restore()

	shell := resolveShell()
	prog, args := shell, []string{shell}
	if command != "" {
		prog, args = hookExecArgs(command, shell)
	}

	execHandOff(cfg.Logger, cfg.ExecSelf, prog, args, command != "")
	return nil
}

func resumeAnswerDiscard(cfg resumeWaitConfig) error {
	return resumeRedraw(cfg)
}

// resumeRedraw hands the pane to a fresh draw of the screen it is already
// showing, carrying whatever payload its caller hands it. A binary that cannot
// be resolved ends the wait as a read error does: the pending marker is still
// set, so the chain's tail recovers the pane to a usable shell rather than
// leaving one whose keys silently do nothing.
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

// The pane's own resizes reach the waiter as a signal, which is the one signal
// it takes off nothing: every other keeps its default disposition, so tmux
// tearing the pane down ends the waiter with it.
func winchSignals() <-chan os.Signal {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	return ch
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

		return resumeWaitRunFunc(resumeWaitConfig{
			resumeChainPayload: resumeChainPayload{
				Command: command,
				Report:  report,
				HookKey: hookKey,
				Pane:    pane,
				PaneKey: paneKey,
				Width:   width,
				Height:  height,
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
			ClearMarker: func() error {
				return state.UnsetResumePendingMarker(tmux.DefaultClient(), tmux.PaneIDTarget(pane))
			},
			LookupResume: lookupResumeRegistration,
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
	_ = stateResumeWaitCmd.MarkFlagRequired(resumeFlagCommand)

	stateCmd.AddCommand(stateResumeWaitCmd)
}
