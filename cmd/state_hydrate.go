package cmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/resumemode"
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

	// Decision is what the pane's registration resolved to, held by pointer so
	// a refusal taken inside a by-value handler reaches the exec. A nil one is
	// a call that resolved nothing, which looks its registration up itself.
	Decision *resumeDecision
}

// resumeDecision carries one pane's resolved resume mode from the top of the
// helper to whichever tail it ends on. Exe and Pane are resolved by the mark
// step, which is where a refusal to wait has to be taken.
type resumeDecision struct {
	Wait   bool
	Lookup hooks.OnResume
	Exe    string
	Pane   string
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
	cfg.Decision = resolveResumeDecision(cfg)
	f, err := cfg.OpenFIFO(cfg.FIFO, hydrateTimeout)
	if err != nil {
		if errors.Is(err, ErrHydrateTimeout) {
			if cfg.HandleTimeout != nil {
				if err := cfg.HandleTimeout(cfg); err != nil {
					return err
				}
				time.Sleep(hydrateSettleSleep)
				execShellOrHookAndExit(cfg)
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
			if hErr := cfg.HandleFileMissing(cfg, hydrateFileMissingContext{Cause: err}); hErr != nil {
				return hErr
			}
			execShellOrHookAndExit(cfg)
			return nil
		}
		return fmt.Errorf("open scrollback %s: %w", cfg.File, err)
	}
	defer func() { _ = sb.Close() }()

	start := time.Now()
	n, err := io.Copy(cfg.Stdout, sb)
	took := time.Since(start)
	if err != nil {
		if cfg.HandleFileMissing != nil {
			if hErr := cfg.HandleFileMissing(cfg, hydrateFileMissingContext{Cause: err}); hErr != nil {
				return hErr
			}
			execShellOrHookAndExit(cfg)
			return nil
		}
		return err
	}

	_, _ = io.WriteString(cfg.Stdout, hydrateResetPostamble)

	// Let tmux's PTY parser finish ingesting the dump before the skeleton marker is unset.
	time.Sleep(hydrateSettleSleep)

	markPendingThenUnsetSkeletonMarker(cfg)

	cfg.Logger.Info("scrollback replayed", "bytes", n, "took", took)

	execShellOrHookAndExit(cfg)
	return nil
}

func execShellAndExit(cfg hydrateConfig) {
	shell := resolveShell()
	execHandOff(cfg.Logger, cfg.ExecShell, shell, []string{shell}, false)
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
func resolveResumeDecision(cfg hydrateConfig) *resumeDecision {
	lookup := lookupOnResumeOrLog(cfg)
	wait := lookup.Found && lookup.Command != "" &&
		resumemode.Resolve(lookup.Mode, installResumeMode(cfg)) == resumemode.Lazy
	return &resumeDecision{Wait: wait, Lookup: lookup}
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
	if err != nil || store == nil {
		return resumemode.Default
	}
	// The error is discarded because the mode beside it is already the default.
	mode, _ := store.LoadResumeMode()
	return mode
}

// An absent store, a failed read and an unregistered key are all "no hook", and
// each is recorded as the hydrate catalog already words it.
func lookupOnResumeOrLog(cfg hydrateConfig) hooks.OnResume {
	if cfg.HookStore == nil {
		cfg.Logger.Debug("hook lookup", "hook_key", cfg.HookKey, "result", "miss")
		return hooks.OnResume{}
	}
	onResume, err := cfg.HookStore.LookupOnResume(cfg.HookKey, hooks.ViaHydrate)
	if err != nil {
		cfg.Logger.Debug("hook lookup", "hook_key", cfg.HookKey, "result", "error", "error", err)
		cfg.Logger.Warn("lookup on-resume hook failed", "hook_key", cfg.HookKey, "error", err)
		return hooks.OnResume{}
	}
	if !onResume.Found {
		cfg.Logger.Debug("hook lookup", "hook_key", cfg.HookKey, "result", "miss")
		return hooks.OnResume{}
	}
	cfg.Logger.Debug("hook lookup", "hook_key", cfg.HookKey, "result", "hit")
	return onResume
}

// A lookup failure degrades to a bare shell so the pane stays usable when
// hooks.json is unreadable.
func execShellOrHookAndExit(cfg hydrateConfig) {
	cfg.Logger = hydrateLoggerOrDefault(cfg.Logger)
	var lookup hooks.OnResume
	switch {
	case cfg.Decision == nil:
		lookup = lookupOnResumeOrLog(cfg)
	case cfg.Decision.Wait:
		execResumeChainAndExit(cfg)
		return
	default:
		lookup = cfg.Decision.Lookup
	}
	if !lookup.Found {
		execShellAndExit(cfg)
		return
	}
	prog, args := hookExecArgs(lookup.Command, resolveShell())
	execHandOff(cfg.Logger, cfg.ExecShell, prog, args, true)
}

// execResumeChainAndExit parks the pane on the draw followed by the chain's
// tail, so a waiter that ends without answering leaves a pane its user can
// still type into. Both halves are composed from the decision, which resolves
// nothing further: a resolution failing after the marker is written would leave
// the pane frozen for life.
func execResumeChainAndExit(cfg hydrateConfig) {
	payload := resumeChainPayload{
		Command: cfg.Decision.Lookup.Command,
		HookKey: cfg.HookKey,
		Pane:    cfg.Decision.Pane,
		PaneKey: state.PaneKeyFromFIFOPath(cfg.FIFO),
	}
	chained := shellWords(resumeChainArgv(cfg.Decision.Exe, resumeDrawSubcommand, payload)) + "; " +
		shellWords(resumeChainArgv(cfg.Decision.Exe, resumeRecoverSubcommand, payload))
	args := []string{"sh", "-c", chained}
	execHandOff(cfg.Logger, cfg.ExecShell, "/bin/sh", args, true)
}

// Clearing the skeleton marker is the recovery: the FIFO is already unlinked, so
// leaving it set would offer no retry, only a re-fired ENOENT on the next attach.
func handleHydrateTimeout(cfg hydrateConfig) error {
	cfg.Logger = hydrateLoggerOrDefault(cfg.Logger)
	_, _ = io.WriteString(cfg.Stdout, hydrateResetPreamble)

	// Best-effort: an already-removed FIFO or a permission error must not block
	// the shell exec the helper falls through to.
	_ = os.Remove(cfg.FIFO)

	cfg.Logger.Warn("timeout waiting for hydrate signal", "hook_key", cfg.HookKey, "path", cfg.FIFO)

	cfg.Logger.Info("signal timeout", "took", hydrateTimeout)

	markPendingThenUnsetSkeletonMarker(cfg)
	return nil
}

// The preamble is already on stdout and must not be re-emitted, partial bytes
// already streamed are left in place, and there is no settle sleep because
// nothing was fully dumped.
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

	// Unlike the timeout path: with no scrollback to dump, the save loop should
	// resume capturing this pane on the next tick rather than skip it forever.
	markPendingThenUnsetSkeletonMarker(cfg)
	return nil
}

// markPendingThenUnsetSkeletonMarker holds the ordering the pane's saved
// transcript depends on: a pane that is going to wait carries its own marker
// before the mid-restore one is dropped, so no tick lands on an unprotected
// pane. A pane that cannot be marked does not wait.
func markPendingThenUnsetSkeletonMarker(cfg hydrateConfig) {
	if cfg.Decision != nil && cfg.Decision.Wait {
		if err := markResumePending(cfg); err != nil {
			cfg.Logger.Warn("set resume pending marker failed", "pane_key", state.PaneKeyFromFIFOPath(cfg.FIFO), "error", err)
			cfg.Decision.Wait = false
		}
	}
	unsetSkeletonMarkerOrLog(cfg)
}

// The pane and the executable are resolved before the write, so a refusal is
// taken while the pane is still an eager one: a marked pane whose chain could
// not be composed would fire its hook and freeze its saved scrollback for life.
func markResumePending(cfg hydrateConfig) error {
	pane, err := requireTmuxPane()
	if err != nil {
		return err
	}
	resolveExe := cfg.ResolveExe
	if resolveExe == nil {
		resolveExe = resumeChainExe
	}
	exe, err := resolveExe()
	if err != nil {
		return err
	}
	cfg.Decision.Pane, cfg.Decision.Exe = string(pane), exe
	return state.SetResumePendingMarker(cfg.Client, pane)
}

// Failure is non-fatal: the next bootstrap re-skeletons the pane and clears it.
func unsetSkeletonMarkerOrLog(cfg hydrateConfig) {
	if err := state.UnsetSkeletonMarkerForFIFO(cfg.Client, cfg.FIFO); err != nil {
		cfg.Logger.Warn("unset skeleton marker failed", "pane_key", state.PaneKeyFromFIFOPath(cfg.FIFO), "error", err)
	}
}

// syscall.Exec returns only on failure. That path must still terminate non-zero
// so the pane closes, and must do so via log.Close(1) + osExit(1), or the
// just-emitted exec marker stands as a phantom handoff.
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
