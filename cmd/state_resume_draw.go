package cmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tui"
	"github.com/spf13/cobra"
)

const resumeCursorHome = "\x1b[H"

type resumeDrawConfig struct {
	resumeChainPayload

	Stdout     io.Writer
	Logger     *slog.Logger
	Colourless bool
	Size       func() (int, int, error)
	ExecSelf   func(prog string, args []string)

	// Now is the wall clock the questions earlier draws asked are judged by,
	// as the probe dates its own: each rides the hand-off to another process,
	// which no monotonic reading survives.
	Now func() time.Time

	CatchSIGTERM func()

	// DisableEcho runs ahead of the appearance query and the input drop, so the
	// modes each puts back on its way out have echo off.
	DisableEcho func() error

	// ResolveTheme asks the terminal its background, with owed questions still
	// owed their replies ahead of it, when a terminal shows the session to
	// answer, and runs dropInput, when handed one, after the question and
	// before it returns, answering the drop's error beside the palette, when
	// it asked, what it read of the input that wasn't a terminal's reply, and
	// how many replies it read.
	ResolveTheme func(colourless bool, owed int, dropInput func() (int, error)) (tui.PaneAppearance, error)

	// DropInputQueue discards the session's input, answering how many replies
	// were among it.
	DropInputQueue func() (int, error)
}

// runResumeDraw paints one screen of the waiting session and replaces its own
// process image with the waiter, so nothing the draw touched stays resident for
// as long as the session waits. The leave sequence is never written here: it
// belongs to whatever answers the panel.
func runResumeDraw(cfg resumeDrawConfig) error {
	cfg.CatchSIGTERM()
	cfg.Logger = hydrateLoggerOrDefault(cfg.Logger)

	// A refusal still paints: a panel over an echoing tty is better than none.
	if err := cfg.DisableEcho(); err != nil {
		cfg.Logger.Warn("disable terminal echo failed", cfg.logAttrs("error", err)...)
	}

	// The read's error is not consulted: a failed or non-positive size reaches
	// the renderer as it stands and resolves to its own bounded fallback, and a
	// session that drew nothing would read as restored with a dead keyboard.
	width, height, _ := cfg.Size()

	var dropInput func() (int, error)
	if cfg.DropInput {
		dropInput = cfg.DropInputQueue
	}
	owed := cfg.RepliesOwed.live(cfg.Now())
	found, dropErr := cfg.ResolveTheme(cfg.Colourless, owed.count(), dropInput)
	if found.Asked() {
		owed = owed.owe(found.AskedAt)
	}

	shown := cfg.resumeChainPayload
	shown.DropInput = false
	// A key typed while the query was out was read with it; the waiter judges
	// it as if typed after the draw.
	shown.carryTyped(found.Typed)
	shown.RepliesOwed = owed.settle(found.Replies)
	// Input the drop could not discard may still answer the confirmation, so it
	// is never put up over that input, and the waiter is told the input is its
	// own to swallow.
	if dropErr != nil {
		cfg.Logger.Warn("clear pending input failed", shown.logAttrs("error", dropErr)...)
		shown.Screen = resumeScreenPanel
		shown.Report = resumeDropRefusal(dropErr)
		shown.InputArriving = true
	}

	_, _ = io.WriteString(cfg.Stdout, hydrateAltScreenEnter)
	_, _ = io.WriteString(cfg.Stdout, resumeCursorHome)
	_, _ = io.WriteString(cfg.Stdout, renderResumeScreen(tui.ResumeScreen{
		Command:    shown.Command,
		Report:     shown.Report,
		Width:      width,
		Height:     height,
		Theme:      found.Theme,
		Colourless: cfg.Colourless,
	}, shown.Screen))

	shown.Width, shown.Height = width, height
	return resumeHandOff(cfg.Logger, cfg.ExecSelf, resumeWaitSubcommand, shown)
}

func renderResumeScreen(s tui.ResumeScreen, screen string) string {
	if screen == resumeScreenDiscard {
		return tui.RenderResumeDiscardConfirm(s)
	}
	return tui.RenderResumePanel(s)
}

// sessionDrawTheme is the palette one draw of session paints in, read through
// the non-migrating prefs route: the migrating one performs the one-shot
// appearance translation and writes, and a boot's worth of sessions taking it
// at once is a write storm over one file for a value none of them is setting.
func sessionDrawTheme(session string) func(colourless bool, owed int, dropInput func() (int, error)) (tui.PaneAppearance, error) {
	attached := func() bool { return sessionAttached(session) }
	return func(colourless bool, owed int, dropInput func() (int, error)) (tui.PaneAppearance, error) {
		nomination := sessionDrawNomination(loadPrefsStoreNoMigrate, newThemeLoader())
		return resolvePaneThemeFunc(nomination, colourless, owed, attached, dropInput)
	}
}

var resolvePaneThemeFunc = tui.ResolvePaneTheme

// resumeAttachedBound bounds a draw's asks of its session's zmx daemon, the
// leader's environment and the session's Info, so one slow to answer delays
// the panel by this at most. The process table's read between them takes no
// context, so this bound doesn't cover it: it takes about a millisecond, and
// at worst about a tenth of a second more for each zmx it finds mid-exec,
// which it reads again until it can, or gives up on and fails.
const resumeAttachedBound = 250 * time.Millisecond

// sessionAttached reports whether a terminal shows session to answer a
// question, as backend.Held judges it, so a reader connected to the session
// asks none. A session that can't be judged is taken to have one, so a reply
// that may come is still owed.
func sessionAttached(session string) bool {
	b, err := sessionBackend()
	if err != nil {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), resumeAttachedBound)
	defer cancel()
	held, err := backend.Held(ctx, b, session)
	return err != nil || held
}

// Every failure along the route degrades to the shipped pair rather than
// aborting: a session that could not be painted is worse than one painted in the
// wrong half of a pair.
func sessionDrawNomination(openStore func() (*prefs.Store, error), loader theme.Loader) theme.Nomination {
	store, err := openStore()
	if err != nil {
		return shippedSessionThemePair()
	}
	keys, err := store.LoadThemeKeys()
	if err != nil {
		return shippedSessionThemePair()
	}
	resolution, _, err := themeResolution(keys, loader)
	if err != nil {
		return shippedSessionThemePair()
	}
	return resolution.Nomination
}

// Silent because seeding a fallback is not a use of a theme, which is what the
// theme component records.
func shippedSessionThemePair() theme.Nomination {
	loader := theme.NewSilentLoader()
	light, _, _ := loader.LoadBuiltin(theme.DefaultLightSlug)
	dark, _, _ := loader.LoadBuiltin(theme.DefaultDarkSlug)
	return theme.AdaptivePair(light.Theme, dark.Theme)
}

func dropStdinInputQueue() (replies int, err error) {
	replies, err = resumeInputDrain.drain(int(os.Stdin.Fd()))
	if err != nil {
		return replies, fmt.Errorf("could not clear pending input: %w", err)
	}
	return replies, nil
}

// The session's own tty, so a boot's worth of waiting sessions asks the
// backend nothing.
func sessionSizeFromStdin() (int, int, error) {
	return term.GetSize(os.Stdin.Fd())
}

var resumeDrawRunFunc = runResumeDraw

// stateResumeDrawCmd paints one screen of a waiting session and hands the
// session to the waiter.
var stateResumeDrawCmd = &cobra.Command{
	Use:    resumeDrawSubcommand,
	Short:  "Draw the resume panel into a waiting session (internal)",
	Args:   cobra.NoArgs,
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		command, _ := cmd.Flags().GetString(resumeFlagCommand)
		report, _ := cmd.Flags().GetString(resumeFlagReport)
		session, _ := cmd.Flags().GetString(resumeFlagSession)
		screen, _ := cmd.Flags().GetString(resumeFlagScreen)
		dropInput, _ := cmd.Flags().GetBool(resumeFlagDropInput)
		inputArriving, _ := cmd.Flags().GetBool(resumeFlagInputArriving)
		repliesOwed, _ := cmd.Flags().GetString(resumeFlagRepliesOwed)

		return resumeDrawRunFunc(resumeDrawConfig{
			resumeChainPayload: resumeChainPayload{
				Command:       command,
				Report:        report,
				Session:       session,
				Screen:        screen,
				DropInput:     dropInput,
				InputArriving: inputArriving,
				RepliesOwed:   replyDebtFromFlag(repliesOwed, time.Now()),
			},
			Stdout:         os.Stdout,
			Logger:         hydrateLogger,
			Colourless:     noColorEnabled(),
			Now:            time.Now,
			Size:           sessionSizeFromStdin,
			DisableEcho:    clearStdinEcho,
			ResolveTheme:   sessionDrawTheme(session),
			DropInputQueue: dropStdinInputQueue,
			ExecSelf:       defaultExecShell,
			CatchSIGTERM:   catchSIGTERM,
		})
	},
}

func init() {
	stateResumeDrawCmd.Flags().String(resumeFlagCommand, "", "The registered on-resume command the panel states")
	stateResumeDrawCmd.Flags().String(resumeFlagReport, "", "What an answer that could not be carried out reported")
	stateResumeDrawCmd.Flags().String(resumeFlagSession, "", "The waiting session: its resume hook's key and its resume-pending flag")
	// Width and height are registered but not read: a draw measures its own tty,
	// and a flag the waiter hands back that this command did not register would
	// fail its parse.
	stateResumeDrawCmd.Flags().Int(resumeFlagWidth, 0, "Width the previous screen was drawn at")
	stateResumeDrawCmd.Flags().Int(resumeFlagHeight, 0, "Height the previous screen was drawn at")
	stateResumeDrawCmd.Flags().String(resumeFlagScreen, resumeScreenPanel, "Which screen to draw: discard for the confirmation, else the panel")
	stateResumeDrawCmd.Flags().Bool(resumeFlagDropInput, false, "Discard input already queued on the session before drawing")
	stateResumeDrawCmd.Flags().Bool(resumeFlagInputArriving, false, "Input was still arriving on the session when it was handed over")
	stateResumeDrawCmd.Flags().String(resumeFlagRepliesOwed, "", "When each background question still owed its reply was asked, in unix milliseconds, oldest first, comma-separated")
	_ = stateResumeDrawCmd.MarkFlagRequired(resumeFlagCommand)
	_ = stateResumeDrawCmd.MarkFlagRequired(resumeFlagSession)

	stateCmd.AddCommand(stateResumeDrawCmd)
}
