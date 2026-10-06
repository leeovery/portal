package cmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/resumemode"
	"github.com/leeovery/portal/internal/shellquote"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// The reset sequences make the cursor visible, leave the alternate screen and
// reset SGR; the postamble adds CRLF so the shell prompt lands at column 0. The
// entry is their pair: a second one written to a pane already on the alternate
// screen does not nest, so a redraw may re-write it.
const (
	hydrateAltScreenEnter = "\x1b[?1049h\x1b[?25l"
	hydrateResetPreamble  = "\x1b[?25h\x1b[?1049l\x1b[0m"
	hydrateResetPostamble = "\x1b[?25h\x1b[?1049l\x1b[0m\r\n"
	hydrateTimeout        = 3 * time.Second
	hydrateSettleSleep    = 100 * time.Millisecond
)

var ErrHydrateTimeout = errors.New("fifo open timeout")

type hydrateConfig struct {
	FIFO              string
	File              string
	HookKey           string
	Stdout            io.Writer
	Client            *tmux.Client
	Logger            *slog.Logger
	HookStore         *hooks.Store
	ExecShell         func(prog string, args []string)
	OpenFIFO          func(path string, timeout time.Duration) (*os.File, error)
	HandleFileMissing func(cfg hydrateConfig, ctx hydrateFileMissingContext) error
	HandleTimeout     func(cfg hydrateConfig) error

	// LoadPrefsStore and ResolveExe are nil-tolerant: unset takes the
	// non-migrating prefs route and the running binary's own path.
	LoadPrefsStore func() (*prefs.Store, error)
	ResolveExe     func() (string, error)

	// DisableTTYSignals turns the pane's kill keys into bytes before a waiting
	// pane's chain is parked; the chain hands them back on whatever path gives
	// the pane to a hook or a shell.
	DisableTTYSignals func() error
}

// resumeDecision carries one pane's resolved resume mode from the top of the
// helper to whichever tail it ends on.
type resumeDecision struct {
	Wait   bool
	Lookup hooks.OnResume
}

// parkedPane is a pane the mark step wrote the pending marker on, with the
// executable its chain is composed from.
type parkedPane struct {
	Pane string
	Exe  string
}

func hydrateLoggerOrDefault(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return hydrateLogger
	}
	return logger
}

type hydrateFileMissingContext struct {
	Cause error
}

// An abandoned open leaks its goroutine and the eventual *os.File - tolerable
// because the helper exec's a shell straight after and the process is replaced.
func openFIFOWithTimeout(path string, timeout time.Duration) (*os.File, error) {
	type result struct {
		f   *os.File
		err error
	}
	ch := make(chan result, 1)
	go func() {
		f, err := os.OpenFile(path, os.O_RDONLY, 0)
		ch <- result{f, err}
	}()
	select {
	case r := <-ch:
		return r.f, r.err
	case <-time.After(timeout):
		return nil, ErrHydrateTimeout
	}
}

// The trailing return is unreachable in production: ExecShell replaces the
// process image.
func runHydrate(cfg hydrateConfig) error {
	cfg.Logger = hydrateLoggerOrDefault(cfg.Logger)
	decision := resolveResumeDecision(cfg)
	f, err := cfg.OpenFIFO(cfg.FIFO, hydrateTimeout)
	if err != nil {
		if errors.Is(err, ErrHydrateTimeout) {
			if cfg.HandleTimeout != nil {
				if err := cfg.HandleTimeout(cfg); err != nil {
					return err
				}
				// Clearing the skeleton marker is the recovery: the FIFO is
				// already unlinked, so leaving it set would offer no retry, only
				// a re-fired ENOENT on the next attach.
				parked := markPendingThenUnsetSkeletonMarker(cfg, decision)
				time.Sleep(hydrateSettleSleep)
				execShellOrHookAndExit(cfg, decision.Lookup, parked)
				return nil
			}
			return err
		}
		// A missing FIFO surfaces here rather than as a timeout: os.OpenFile
		// returns ENOENT immediately instead of blocking. There is no exec to
		// fall through to, so the error is returned.
		cfg.Logger.Info("fifo missing", "path", cfg.FIFO)
		return fmt.Errorf("open fifo %s: %w", cfg.FIFO, err)
	}

	// Any read counts as the signal, errors included: a 0-byte read means the
	// writer closed, which is still arrival.
	buf := make([]byte, 1)
	_, _ = f.Read(buf)
	_ = f.Close()
	// Best-effort unlink; a residual FIFO is reclaimed by the next bootstrap sweep.
	_ = os.Remove(cfg.FIFO)

	// Emitted before os.Open so the file-missing path inherits a written
	// preamble and its handler need not re-emit one.
	_, _ = io.WriteString(cfg.Stdout, hydrateResetPreamble)

	sb, err := os.Open(cfg.File)
	if err != nil {
		if cfg.HandleFileMissing != nil {
			return runFileMissingTail(cfg, decision, err)
		}
		return fmt.Errorf("open scrollback %s: %w", cfg.File, err)
	}
	defer func() { _ = sb.Close() }()

	start := time.Now()
	n, err := io.Copy(cfg.Stdout, sb)
	took := time.Since(start)
	if err != nil {
		if cfg.HandleFileMissing != nil {
			return runFileMissingTail(cfg, decision, err)
		}
		return err
	}

	_, _ = io.WriteString(cfg.Stdout, hydrateResetPostamble)

	// Let tmux's PTY parser finish ingesting the dump before the skeleton marker is unset.
	time.Sleep(hydrateSettleSleep)

	parked := markPendingThenUnsetSkeletonMarker(cfg, decision)

	cfg.Logger.Info("scrollback replayed", "bytes", n, "took", took)

	execShellOrHookAndExit(cfg, decision.Lookup, parked)
	return nil
}

// There is no settle sleep: nothing was fully dumped.
func runFileMissingTail(cfg hydrateConfig, decision resumeDecision, cause error) error {
	if err := cfg.HandleFileMissing(cfg, hydrateFileMissingContext{Cause: cause}); err != nil {
		return err
	}
	parked := markPendingThenUnsetSkeletonMarker(cfg, decision)
	execShellOrHookAndExit(cfg, decision.Lookup, parked)
	return nil
}

func resolveShell() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	return shell
}

// resolveResumeDecision reads the pane's registration and the install-wide
// default once, at the top of the helper, so every tail it can end on decides
// the same way and pays for the reads once.
func resolveResumeDecision(cfg hydrateConfig) resumeDecision {
	lookup := lookupOnResumeOrLog(cfg)
	wait := lookup.Found && resumemode.Resolve(lookup.Mode, installResumeMode(cfg)) == resumemode.Lazy
	return resumeDecision{Wait: wait, Lookup: lookup}
}

// A prefs store that cannot be built, and a read that fails, both take the
// shipped default: a panel is answered in a keystroke, while a resume the user
// did not want cannot be taken back.
func installResumeMode(cfg hydrateConfig) resumemode.Mode {
	load := cfg.LoadPrefsStore
	if load == nil {
		load = loadPrefsStoreNoMigrate
	}
	store, err := load()
	if err != nil {
		return installResumeModeOf(nil)
	}
	return installResumeModeOf(store)
}

// The read error is discarded: LoadResumeMode answers resumemode.Default beside it.
func installResumeModeOf(store *prefs.Store) resumemode.Mode {
	if store == nil {
		return resumemode.Default
	}
	mode, _ := store.LoadResumeMode()
	return mode
}

// An absent store reads as an unregistered key, which the shared rule records
// and answers as any other miss.
func lookupOnResumeOrLog(cfg hydrateConfig) hooks.OnResume {
	return resumeRegistrationOrLog(cfg.Logger, func(hookKey string) (hooks.OnResume, error) {
		if cfg.HookStore == nil {
			return hooks.OnResume{}, nil
		}
		return cfg.HookStore.LookupOnResume(hookKey, hooks.ViaHydrate)
	}, cfg.HookKey)
}

// Only a pane the mark step returned parks; any other runs the registration's
// command, whatever mode it resolved to.
func execShellOrHookAndExit(cfg hydrateConfig, registration hooks.OnResume, parked *parkedPane) {
	cfg.Logger = hydrateLoggerOrDefault(cfg.Logger)
	if parked != nil {
		execResumeChainAndExit(cfg, registration.Command, *parked)
		return
	}
	handOffToHookOrShell(cfg.Logger, cfg.ExecShell, registration.Command)
}

// execResumeChainAndExit parks the pane on the draw followed by the chain's
// tail, so a waiter that ends without answering leaves a pane its user can
// still type into. Both halves are composed from the marked pane, which
// resolves nothing further: a resolution failing after the marker is written
// would leave the pane frozen for life.
//
// A pane whose signal generation could not be turned off still parks: the
// chain's trap turns a kill key that lands into the recovery tail.
func execResumeChainAndExit(cfg hydrateConfig, command string, parked parkedPane) {
	payload := resumeChainPayload{
		Command: command,
		HookKey: cfg.HookKey,
		Pane:    parked.Pane,
		PaneKey: state.PaneKeyFromFIFOPath(cfg.FIFO),
	}
	if err := cfg.DisableTTYSignals(); err != nil {
		cfg.Logger.Warn("disable terminal signals failed", payload.paneRef().logAttrs("error", err)...)
	}
	args := []string{"sh", "-c", parkedResumeChain(parked.Exe, payload)}
	execHandOff(cfg.Logger, cfg.ExecShell, "/bin/sh", args, true)
}

// parkedChainTrap keeps the parked shell alive through a group interrupt or
// quit so its recovery tail still runs, and through a SIGTERM so the pane keeps
// its session until tmux itself exits. It must stay a caught trap: an ignored
// signal stays ignored across exec, and the hook and the user's shell would
// inherit it. SIGHUP stays uncaught, so a kill still ends the pane.
const parkedChainTrap = "trap : INT QUIT TERM; "

// parkedChainBackstop leaves the pane at a shell when the tail could not start
// at all. It keys on the shell's could-not-run statuses, not on any failure: a
// tail that recovered the pane exec'd the user's shell, whose exit status
// arrives here, and a second shell after it would make the pane take two exits
// to close. The executable check tells a tail that never started from a shell
// that exited 126 or 127 itself. The binary can leave the baked path while an
// answered pane's shell runs, so the tail's own gate comes first: a marker that
// reads back clear ends the chain, and a read that fails counts as still
// pending. Past the gate it takes the tail's own steps in the tail's order,
// since no Portal binary is left to take them.
func parkedChainBackstop(exe string, payload resumeChainPayload) string {
	target := string(tmux.PaneIDTarget(payload.Pane))
	answeredGate := `if ! ` + paneStillPending(payload.Pane) + `; then exit $s; fi`
	reset := `printf '%s' ` + shellquote.Single(hydrateResetPreamble)
	clearMarker := shellquote.Join([]string{
		"tmux", "set-option", "-pu", "-t", target, state.ResumePendingOption,
	}) + ` 2>/dev/null`
	return `; s=$?; case $s in 126|127) if [ ! -x ` + shellquote.Single(exe) + ` ]; then ` +
		answeredGate + `; ` + reset + `; ` + clearMarker + `; ` + backstopReleasePin(payload.Pane) +
		`; stty sane 2>/dev/null; exec "${SHELL:-/bin/sh}"; fi;; esac; exit $s`
}

// backstopReleasePin is releaseAltScreenPin in shell form: the pin is unset only
// once #{alternate_on} reads 0 within the bound, and nothing reaches stderr.
func backstopReleasePin(pane string) string {
	readAlternateOn := shellquote.Join([]string{
		"tmux", "display-message", "-p", "-t", string(tmux.PaneIDTarget(pane)), "-F", "#{" + alternateOnFormat + "}",
	}) + ` 2>/dev/null`
	unpin := shellquote.Join([]string{
		"tmux", "set-option", "-pu", "-t", string(tmux.PaneIDTarget(pane)), altScreenOption,
	}) + ` 2>/dev/null`
	attempts := strconv.Itoa(altScreenLeaveAttempts)
	pause := strconv.FormatFloat(altScreenLeavePoll.Seconds(), 'f', -1, 64)
	return `n=1; until [ "$(` + readAlternateOn + `)" = 0 ]; do ` +
		`if [ $n -ge ` + attempts + ` ]; then n=0; break; fi; n=$((n+1)); sleep ` + pause + ` 2>/dev/null; done; ` +
		`if [ $n -gt 0 ]; then ` + unpin + `; fi`
}

// paneStillPending is a shell test that succeeds unless the pane's pending
// marker reads back clear; a read that fails counts as still pending. A plain
// format read serves: the one pane it misreads as clear is a gone one, which
// wants nothing more from the chain.
func paneStillPending(pane string) string {
	readMarker := shellquote.Join([]string{
		"tmux", "display-message", "-p", "-t", string(tmux.PaneIDTarget(pane)), "-F", "#{" + state.ResumePendingOption + "}",
	}) + ` 2>/dev/null`
	return `{ ! m=$(` + readMarker + `) || [ -n "$m" ]; }`
}

// sigtermStatus is the status the shell reports for a child a SIGTERM ended.
const sigtermStatus = 128 + int(syscall.SIGTERM)

// parkedChainDraw starts the draw again when the draw, or the waiter it became,
// was ended by a SIGTERM while the pane still waits: a catch is not inherited
// across fork or exec and a Go image cannot install one before its runtime has
// started, so each hand-off opens a moment only the parked shell can close. The
// marker gate keeps an answered pane, whose own shell now runs as that process,
// off the panel.
func parkedChainDraw(exe string, payload resumeChainPayload) string {
	return `while ` + shellquote.Join(resumeChainArgv(exe, resumeDrawSubcommand, payload)) +
		`; [ $? -eq ` + strconv.Itoa(sigtermStatus) + ` ] && ` + paneStillPending(payload.Pane) + `; do :; done`
}

func parkedResumeChain(exe string, payload resumeChainPayload) string {
	return parkedChainTrap +
		parkedChainDraw(exe, payload) + "; " +
		shellquote.Join(resumeChainArgv(exe, resumeRecoverSubcommand, payload)) +
		parkedChainBackstop(exe, payload)
}

func handleHydrateTimeout(cfg hydrateConfig) error {
	cfg.Logger = hydrateLoggerOrDefault(cfg.Logger)
	_, _ = io.WriteString(cfg.Stdout, hydrateResetPreamble)

	// Best-effort: an already-removed FIFO or a permission error must not block
	// the shell exec the helper falls through to.
	_ = os.Remove(cfg.FIFO)

	cfg.Logger.Warn("timeout waiting for hydrate signal", "hook_key", cfg.HookKey, "path", cfg.FIFO)

	cfg.Logger.Info("signal timeout", "took", hydrateTimeout)
	return nil
}

// The preamble is already on stdout and must not be re-emitted, and partial
// bytes already streamed are left in place.
func handleHydrateFileMissing(cfg hydrateConfig, ctx hydrateFileMissingContext) error {
	cfg.Logger = hydrateLoggerOrDefault(cfg.Logger)
	switch {
	case errors.Is(ctx.Cause, fs.ErrNotExist):
		cfg.Logger.Warn("scrollback file not found", "hook_key", cfg.HookKey, "path", cfg.File)
	case errors.Is(ctx.Cause, fs.ErrPermission):
		cfg.Logger.Warn("scrollback file unreadable (permission denied)", "hook_key", cfg.HookKey, "path", cfg.File)
	default:
		cfg.Logger.Warn("scrollback file I/O error", "hook_key", cfg.HookKey, "path", cfg.File, "error", ctx.Cause)
	}

	cfg.Logger.Info("scrollback missing", "path", cfg.File)
	return nil
}

// markPendingThenUnsetSkeletonMarker holds the ordering the pane's saved
// transcript depends on: a pane that is going to wait carries its own marker
// before the mid-restore one is dropped, so no tick lands on an unprotected
// pane. A pane that cannot be marked does not wait: it returns nil, as does one
// that was never going to.
func markPendingThenUnsetSkeletonMarker(cfg hydrateConfig, decision resumeDecision) *parkedPane {
	var parked *parkedPane
	if decision.Wait {
		p, err := markResumePending(cfg)
		if err != nil {
			cfg.Logger.Warn("set resume pending marker failed", "pane_key", state.PaneKeyFromFIFOPath(cfg.FIFO), "error", err)
		} else {
			parked = &p
		}
	}
	unsetSkeletonMarkerOrLog(cfg)
	return parked
}

// The pane and the executable are resolved before any write, so a refusal is
// taken while the pane is still an eager one: a marked pane whose chain could
// not be composed would fire its hook and freeze its saved scrollback for life.
// The baked hook key is written as the pane's token first, unconditionally and
// with no read: restore's re-stamp is best-effort, and a waiting pane carrying
// no token leaves its registration stale to the hook sweep, which can delete it
// before Enter runs it. The key is the pane's saved token, so the write never
// changes a durable identity and a later refusal has nothing to lift.
// The alternate-screen pin goes before the marker because a pin left behind
// overrides one display setting on one pane, where a marker left behind freezes
// that pane's scrollback; a refused marker lifts the pin, since the pane never
// waits.
func markResumePending(cfg hydrateConfig) (parkedPane, error) {
	pane, err := requireTmuxPane()
	if err != nil {
		return parkedPane{}, err
	}
	resolveExe := cfg.ResolveExe
	if resolveExe == nil {
		resolveExe = resumeChainExe
	}
	exe, err := resolveExe()
	if err != nil {
		return parkedPane{}, err
	}
	if err := cfg.Client.SetPaneOption(pane, state.PortalPaneIDOption, cfg.HookKey); err != nil {
		return parkedPane{}, err
	}
	if err := cfg.Client.SetPaneOption(pane, altScreenOption, "on"); err != nil {
		return parkedPane{}, err
	}
	if err := state.SetResumePendingMarker(cfg.Client, pane); err != nil {
		if liftErr := cfg.Client.UnsetPaneOption(pane, altScreenOption); liftErr != nil {
			cfg.Logger.Warn("lift alternate-screen pin failed", "pane_key", state.PaneKeyFromFIFOPath(cfg.FIFO), "error", liftErr)
		}
		return parkedPane{}, err
	}
	return parkedPane{Pane: string(pane), Exe: exe}, nil
}

// Failure is non-fatal: the next bootstrap re-skeletons the pane and clears it.
func unsetSkeletonMarkerOrLog(cfg hydrateConfig) {
	if err := state.UnsetSkeletonMarkerForFIFO(cfg.Client, cfg.FIFO); err != nil {
		cfg.Logger.Warn("unset skeleton marker failed", "pane_key", state.PaneKeyFromFIFOPath(cfg.FIFO), "error", err)
	}
}

// syscall.Exec returns only on failure, and that path must end through
// log.Close(1) + osExit(1), or the just-emitted exec marker stands as a phantom
// handoff.
func defaultExecShell(prog string, args []string) {
	err := syscall.Exec(prog, args, os.Environ())
	hydrateLogger.Warn("exec handoff failed", "target", prog, "args", strings.Join(args, " "), "error", err)
	log.Close(1)
	osExit(1)
}

var hydrateRunFunc = runHydrate

// stateHydrateCmd is wired by skeleton restore as each restored pane's initial
// shell command.
var stateHydrateCmd = &cobra.Command{
	Use:    "hydrate",
	Short:  "Hydrate a restored pane from saved scrollback (internal)",
	Args:   cobra.NoArgs,
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		fifo, _ := cmd.Flags().GetString("fifo")
		file, _ := cmd.Flags().GetString("file")
		hookKey, _ := cmd.Flags().GetString("hook-key")

		// A nil store degrades the hook lookup to a bare $SHELL, which beats
		// failing the per-pane helper closed.
		store, _ := loadHookStore()

		cfg := hydrateConfig{
			FIFO:              fifo,
			File:              file,
			HookKey:           hookKey,
			Stdout:            cmd.OutOrStdout(),
			Client:            tmux.DefaultClient(),
			Logger:            hydrateLogger,
			HookStore:         store,
			ExecShell:         defaultExecShell,
			DisableTTYSignals: clearStdinSignals,
			OpenFIFO:          openFIFOWithTimeout,
			HandleFileMissing: handleHydrateFileMissing,
			HandleTimeout:     handleHydrateTimeout,
		}
		return hydrateRunFunc(cfg)
	},
}

func init() {
	stateHydrateCmd.Flags().String("fifo", "", "Absolute path to the per-pane FIFO")
	stateHydrateCmd.Flags().String("file", "", "Absolute path to the saved scrollback file")
	// Optional: a pane with no durable token is armed with no key at all, and an
	// absent flag reads the same as an empty one - "no hook".
	stateHydrateCmd.Flags().String("hook-key", "", "Saved pane token identifying the pane's resume hook")
	_ = stateHydrateCmd.MarkFlagRequired("fifo")
	_ = stateHydrateCmd.MarkFlagRequired("file")

	stateCmd.AddCommand(stateHydrateCmd)
}
