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
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// Reset sequences and timing constants for the hydrate helper. The preamble
// makes the cursor visible, exits the alternate screen buffer defensively, and
// resets SGR; the postamble repeats the same sequences and adds CRLF so the
// shell prompt that follows lands on a clean column-0 row. Spec: "Helper
// Behavior on Startup" in built-in-session-resurrection.
const (
	hydrateResetPreamble  = "\x1b[?25h\x1b[?1049l\x1b[0m"
	hydrateResetPostamble = "\x1b[?25h\x1b[?1049l\x1b[0m\r\n"
	hydrateTimeout        = 3 * time.Second
	hydrateSettleSleep    = 100 * time.Millisecond
)

// ErrHydrateTimeout is returned by openFIFOWithTimeout when the blocking open
// did not complete within the supplied timeout. The 3-second default is set
// by hydrateTimeout; tests use shorter values.
var ErrHydrateTimeout = errors.New("fifo open timeout")

// hydrateConfig groups every dependency runHydrate needs. Tests inject stubs
// for ExecShell and OpenFIFO; HandleTimeout / HandleFileMissing are filled in
// by tasks 3-9 and 3-10. HookStore + HookKey drive the Phase 4 hook lookup —
// HookKey is the saved structural identifier (per spec "Helper hook lookup
// under index drift") and is used verbatim, not the FIFO-derived live key.
//
// ExecShell takes (prog, args) so the hook-firing path can hand off to
// `sh -c '<HOOK>; exec $SHELL'` with the user-registered command living in
// its own argv slot — sh's parser handles any embedded quotes.
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
}

// hydrateLoggerOrDefault returns logger when non-nil, else the package's
// component-bound hydrateLogger. The hydrate entry points normalize cfg.Logger
// through this so a caller (or test) that leaves Logger nil never panics on a
// *slog.Logger nil-receiver — preserving the nil-tolerant contract the legacy
// bespoke logger provided.
func hydrateLoggerOrDefault(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return hydrateLogger
	}
	return logger
}

// hydrateFileMissingContext carries the underlying cause of a file-missing
// transition into handleHydrateFileMissing so the handler can log distinct
// prefixes for ENOENT, permission, and generic I/O failures. The preamble has
// already been written by runHydrate by the time the handler runs (see
// runHydrate step ordering); the handler must not re-emit it.
type hydrateFileMissingContext struct {
	Cause error
}

// openFIFOWithTimeout opens path O_RDONLY and abandons the open after timeout.
// The blocking open runs in a goroutine; on timeout, the goroutine remains
// blocked until a writer eventually arrives (and the resulting *os.File is
// leaked) — acceptable here because the helper exec's a shell on the timeout
// path and the process ends shortly after.
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

// runHydrate is the body of "portal state hydrate". It blocks on the per-pane
// FIFO until signal-hydrate writes a byte, then dumps the saved scrollback to
// stdout, sleeps 100ms (PTY parser settle), unsets the skeleton marker, and
// exec's the user shell. Three error paths are handed off to seams:
// FIFO timeout (HandleTimeout), scrollback file missing (HandleFileMissing),
// and io.Copy mid-dump failure (also HandleFileMissing — same recovery).
// In production ExecShell calls syscall.Exec and never returns; the trailing
// `return nil` is reached only in tests.
func runHydrate(cfg hydrateConfig) error {
	cfg.Logger = hydrateLoggerOrDefault(cfg.Logger)
	// 1. Block on FIFO until signal arrives or timeout fires.
	f, err := cfg.OpenFIFO(cfg.FIFO, hydrateTimeout)
	if err != nil {
		if errors.Is(err, ErrHydrateTimeout) {
			if cfg.HandleTimeout != nil {
				if err := cfg.HandleTimeout(cfg); err != nil {
					return err
				}
				// Settle sleep — same posture as the success path (step 7).
				// Spec § Fix 2 → Specific Changes → 4: 100ms preserved before
				// exec so tmux's PTY parser settles after the post-handler
				// reset/marker-unset sequence.
				time.Sleep(hydrateSettleSleep)
				// Timeout path fires on-resume hooks per the timeout-recovery
				// contract (spec § Fix 2 → Specific Changes → 2). The handler
				// has already cleared the marker; lookup happens here, then exec.
				execShellOrHookAndExit(cfg)
				return nil
			}
			return err
		}
		return fmt.Errorf("open fifo %s: %w", cfg.FIFO, err)
	}

	// 2. Read 1 byte (any read counts as the signal). Errors are ignored:
	// even a 0-byte read can mean "writer closed" which is still arrival.
	buf := make([]byte, 1)
	_, _ = f.Read(buf)
	// Close error is irrelevant once the signal byte has been observed — the
	// fd has served its only purpose (blocking until the writer arrived).
	_ = f.Close()
	// FIFO unlink is best-effort cleanup; a residual hydrate-*.fifo is reclaimed
	// by the next bootstrap's orphan-FIFO sweep, so a failure here is harmless.
	_ = os.Remove(cfg.FIFO)

	// 3. Reset preamble — cursor visible, exit alt-screen, SGR reset. Emitted
	// before os.Open so that the file-missing path inherits a written preamble
	// without the handler having to re-emit it.
	_, _ = io.WriteString(cfg.Stdout, hydrateResetPreamble)

	// 4. Open the saved scrollback file. Failure (ENOENT, permission denied,
	// or any other I/O error) routes through HandleFileMissing — preamble is
	// already on stdout, so the pane lands on a clean shell after exec.
	sb, err := os.Open(cfg.File)
	if err != nil {
		if cfg.HandleFileMissing != nil {
			if hErr := cfg.HandleFileMissing(cfg, hydrateFileMissingContext{Cause: err}); hErr != nil {
				return hErr
			}
			// File-missing path fires on-resume hooks per spec step 4e
			// ("Continue to step h (hook/shell exec)"). The handler has
			// already cleared the marker; lookup happens here, then exec.
			execShellOrHookAndExit(cfg)
			return nil
		}
		return fmt.Errorf("open scrollback %s: %w", cfg.File, err)
	}
	defer func() { _ = sb.Close() }()

	// 5. Stream scrollback to stdout. io.Copy preserves bytes verbatim and
	// streams in 32K blocks; the 5MB-file test verifies this end-to-end. A
	// mid-stream Read failure leaves the partial bytes on stdout — handler is
	// invoked to log the cause, unset the marker, and exec the shell.
	if _, err := io.Copy(cfg.Stdout, sb); err != nil {
		if cfg.HandleFileMissing != nil {
			if hErr := cfg.HandleFileMissing(cfg, hydrateFileMissingContext{Cause: err}); hErr != nil {
				return hErr
			}
			// Mid-stream Copy failure shares the file-missing recovery
			// (handler already cleared the marker); fire hooks then exec.
			execShellOrHookAndExit(cfg)
			return nil
		}
		return err
	}

	// 6. Reset postamble + CRLF — give the shell a clean column-0 prompt.
	_, _ = io.WriteString(cfg.Stdout, hydrateResetPostamble)

	// 7. Settle sleep — wait for tmux's PTY parser to finish ingesting the
	// dump before unsetting the marker. See spec section "The 100ms Settle
	// Sleep" for why the helper, not signal-hydrate, owns marker-unset.
	time.Sleep(hydrateSettleSleep)

	// 8. Unset the skeleton marker. Failure is non-fatal: a stale marker
	// only blocks the save loop from re-capturing this pane until next
	// bootstrap, which will re-skeleton the pane and clear it.
	unsetSkeletonMarkerOrLog(cfg)

	// 9. Lookup on-resume hook for cfg.HookKey (saved structural identifier,
	// not live paneKey — preserves hooks across base-index drift) and exec
	// either `sh -c '<HOOK>; exec $SHELL'` or bare $SHELL. Lookup happens
	// AFTER the 100ms settle sleep and AFTER the marker-unset above.
	execShellOrHookAndExit(cfg)
	return nil // unreachable in production (syscall.Exec replaces process)
}

// execShellAndExit resolves $SHELL (defaulting to /bin/sh) and hands the
// process off via cfg.ExecShell. Used by both the signal-arrived path and the
// 3-second-timeout fall-through. In production cfg.ExecShell never returns
// (syscall.Exec replaces the process); in tests it captures the target and
// returns.
func execShellAndExit(cfg hydrateConfig) {
	shell := resolveShell()
	// Terminal hydrate: exec INFO — the helper's last action before the image
	// is replaced. Emitted as the IMMEDIATELY-PRECEDING statement to ExecShell
	// (no statement in between): the unbuffered writer (spec § Defensive
	// invariants → Flush) puts the marker in the kernel before syscall.Exec
	// replaces the process. Structurally parallel to process: exec — target is
	// the exec'd binary, args its space-joined argv. (Mirrors Task 2-14.)
	cfg.Logger.Info("exec", "target", shell, "args", strings.Join([]string{shell}, " "), "hook_present", false)
	cfg.ExecShell(shell, []string{shell})
}

// resolveShell reads $SHELL with /bin/sh fallback. Single resolver shared by
// the bare-shell exec path and the hook-chain exec path so both branches see
// the same shell selection logic.
func resolveShell() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	return shell
}

// execShellOrHookAndExit is the post-hydration terminal exec used by all
// three non-fatal helper paths: signal-arrived (success), file-missing
// recovery, and timeout recovery. The unified exec contract is mandated by
// spec § Fix 2 → Specific Changes → 2 — every recovery path that falls
// through to a shell fires the on-resume hook if one is registered.
//
// On any of: nil HookStore, lookup error, or no hook registered → exec bare
// $SHELL via execShellAndExit. On a registered non-empty on-resume command,
// exec `/bin/sh -c '<cmd>; exec $SHELL'`. The hook command sits in its own
// argv slot (`sh -c <cmd>`) so sh's parser handles any embedded quotes —
// Portal does no string-interpolation of the user-registered command.
//
// Lookup errors degrade silently to bare $SHELL with a single WARN log line
// so the pane lands in a usable shell rather than failing closed when
// hooks.json is unreadable.
func execShellOrHookAndExit(cfg hydrateConfig) {
	cfg.Logger = hydrateLoggerOrDefault(cfg.Logger)
	if cfg.HookStore == nil {
		// A nil store degrades to a bare shell — that is a "miss", NOT an
		// "error" (the lookup never ran; nothing failed). No error attr.
		cfg.Logger.Debug("hook lookup", "hook_key", cfg.HookKey, "result", "miss")
		execShellAndExit(cfg)
		return
	}
	command, found, err := hooks.LookupOnResume(cfg.HookStore, cfg.HookKey)
	if err != nil {
		cfg.Logger.Debug("hook lookup", "hook_key", cfg.HookKey, "result", "error", "error", err)
		cfg.Logger.Warn("lookup on-resume hook failed", "hook_key", cfg.HookKey, "error", err)
		execShellAndExit(cfg)
		return
	}
	if !found {
		// No hook / missing-or-malformed hooks.json → ("", false, nil) → miss.
		cfg.Logger.Debug("hook lookup", "hook_key", cfg.HookKey, "result", "miss")
		execShellAndExit(cfg)
		return
	}
	cfg.Logger.Debug("hook lookup", "hook_key", cfg.HookKey, "result", "hit")
	shell := resolveShell()
	chained := command + "; exec " + shell
	args := []string{"sh", "-c", chained}
	// Terminal hydrate: exec INFO — the IMMEDIATELY-PRECEDING statement to
	// ExecShell (no statement in between): the unbuffered writer (spec §
	// Defensive invariants → Flush) puts the marker in the kernel before
	// syscall.Exec replaces the process. Structurally parallel to process:
	// exec — target is the exec'd binary, args its space-joined argv (verbatim,
	// incl any embedded quotes in the registered hook command). (Mirrors Task
	// 2-14.)
	cfg.Logger.Info("exec", "target", "/bin/sh", "args", strings.Join(args, " "), "hook_present", true)
	cfg.ExecShell("/bin/sh", args)
}

// handleHydrateTimeout is invoked when openFIFOWithTimeout returns
// ErrHydrateTimeout. Per spec § Fix 2 → Specific Changes → 1, the handler
// emits the reset preamble, unlinks the FIFO, logs a warning naming the
// hook-key, and unsets the @portal-skeleton marker via the canonical
// unsetSkeletonMarkerOrLog primitive (mirroring handleHydrateFileMissing).
// runHydrate's timeout branch pays the 100ms settle sleep before exec; this
// handler does not sleep itself.
//
// The marker-unset spec supersession (original line 838 of
// built-in-session-resurrection): leaving the marker set with the FIFO already
// unlinked at the os.Remove above provides no retry — the next attach would
// just re-fire ENOENT. Clearing the marker is the correct recovery contract.
func handleHydrateTimeout(cfg hydrateConfig) error {
	cfg.Logger = hydrateLoggerOrDefault(cfg.Logger)
	// 1. Reset preamble only — no scrollback dump, no postamble.
	_, _ = io.WriteString(cfg.Stdout, hydrateResetPreamble)

	// 2. Unlink the FIFO. Defense in depth — the next bootstrap also sweeps
	// stale hydrate-*.fifo files. Errors are tolerated silently because the
	// FIFO may not exist (e.g., already removed) and a permission error here
	// must not block the shell exec the helper falls through to.
	_ = os.Remove(cfg.FIFO)

	// 3. Log a warning naming the hook-key + FIFO so operators can correlate
	// the entry with the affected pane in the saved sessions.json.
	cfg.Logger.Warn("timeout waiting for hydrate signal", "hook_key", cfg.HookKey, "path", cfg.FIFO)

	unsetSkeletonMarkerOrLog(cfg)
	// Recovery path matches handleHydrateFileMissing: marker unset above; runHydrate's exec fall-through still pays the 100ms settle sleep before exec (preserved per spec — same posture as the success path).
	return nil
}

// handleHydrateFileMissing is invoked when the saved scrollback cannot be
// served — either os.Open fails (ENOENT, permission denied, generic I/O) or
// io.Copy fails mid-stream. Spec ("Helper Behavior on Startup", step 4):
// log a warning, skip the 100ms settle sleep (nothing was fully dumped), unset
// the @portal-skeleton marker inline so the save loop resumes capturing this
// pane, and let runHydrate fall through to exec the user's $SHELL.
//
// The preamble has already been written by runHydrate (step 3) before os.Open
// is attempted, so this handler does NOT emit it again. On a mid-stream
// io.Copy failure, the partial bytes already streamed to stdout are left in
// place — no rollback. The pane lands in a degraded-but-usable shell.
func handleHydrateFileMissing(cfg hydrateConfig, ctx hydrateFileMissingContext) error {
	cfg.Logger = hydrateLoggerOrDefault(cfg.Logger)
	// 1. Log a distinct WARN entry per failure cause so operators can tell
	// missing files (likely GC race) apart from permission misconfiguration
	// or transient disk I/O errors.
	switch {
	case errors.Is(ctx.Cause, fs.ErrNotExist):
		cfg.Logger.Warn("scrollback file not found", "hook_key", cfg.HookKey, "path", cfg.File)
	case errors.Is(ctx.Cause, fs.ErrPermission):
		cfg.Logger.Warn("scrollback file unreadable (permission denied)", "hook_key", cfg.HookKey, "path", cfg.File)
	default:
		cfg.Logger.Warn("scrollback file I/O error", "hook_key", cfg.HookKey, "path", cfg.File, "error", ctx.Cause)
	}

	// 2. Deliberately NO 100ms sleep — nothing was fully dumped, so there is
	// no PTY parser settle to wait on.

	// 3. Unset the skeleton marker — KEY DIFFERENCE FROM TIMEOUT PATH. With
	// no scrollback to dump, the pane is empty and the save loop should
	// resume capturing it on the next tick rather than skipping it forever.
	unsetSkeletonMarkerOrLog(cfg)
	return nil
}

// unsetSkeletonMarkerOrLog clears the @portal-skeleton-<paneKey> server option
// for the pane whose hydration FIFO is cfg.FIFO and logs a single canonical
// WARN line on failure. The FIFO→marker invariant lives entirely inside
// state.UnsetSkeletonMarkerForFIFO; callers in this file hold only the FIFO
// path and never derive the paneKey themselves.
//
// Failure is intentionally non-fatal — both call sites (signal-arrived,
// file-missing recovery) treat a stale marker as recoverable on the next
// bootstrap, which will re-skeleton the pane and clear it.
func unsetSkeletonMarkerOrLog(cfg hydrateConfig) {
	if err := state.UnsetSkeletonMarkerForFIFO(cfg.Client, cfg.FIFO); err != nil {
		cfg.Logger.Warn("unset skeleton marker failed", "pane_key", state.PaneKeyFromFIFOPath(cfg.FIFO), "error", err)
	}
}

// defaultExecShell is the production ExecShell seam: hand the process off via
// syscall.Exec. The (prog, args) shape lets callers pass either a bare-shell
// invocation ([prog]) or a hook-chain invocation (`/bin/sh`, [`sh`, `-c`,
// `<cmd>; exec $SHELL`]). syscall.Exec only returns on error; if it does, the
// helper exits 1 so the pane closes rather than dangling without a shell.
func defaultExecShell(prog string, args []string) {
	_ = syscall.Exec(prog, args, os.Environ())
	os.Exit(1)
}

// hydrateRunFunc is a package-level seam tests use to short-circuit the
// hydrate body for argv-only assertions. Production points it at runHydrate;
// argv-validation tests replace it with a no-op.
var hydrateRunFunc = runHydrate

// stateHydrateCmd is the per-pane initial command at skeleton restore time.
// Hidden from --help; bound flags (fifo, file, hook-key) are required and the
// command takes no positional args. The command is wired by skeleton restore
// as the pane's `tmux new-window`/`split-window` shell-command argument.
var stateHydrateCmd = &cobra.Command{
	Use:    "hydrate",
	Short:  "Hydrate a restored pane from saved scrollback (internal)",
	Args:   cobra.NoArgs,
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		fifo, _ := cmd.Flags().GetString("fifo")
		file, _ := cmd.Flags().GetString("file")
		hookKey, _ := cmd.Flags().GetString("hook-key")

		// loadHookStore() resolves the hooks.json path via configFilePath; a
		// failure means the path itself could not be derived (e.g. no HOME).
		// The hook lookup gracefully degrades to bare $SHELL on a nil store,
		// so swallowing the error here trades a missing hook for an exec'd
		// shell — better than failing closed in the per-pane helper.
		store, _ := loadHookStore()

		// Diagnostics (timeouts, file-missing, marker-unset failures) land in
		// the central log file via the handler configured once by main ->
		// log.Init. Rotation and the append-only writer discipline are now
		// handler-owned (Phase 2), so the helper no longer opens or closes a
		// per-process logger.
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
	stateHydrateCmd.Flags().String("hook-key", "", "Saved structural identifier (<session>:<window>.<pane>)")
	_ = stateHydrateCmd.MarkFlagRequired("fifo")
	_ = stateHydrateCmd.MarkFlagRequired("file")
	_ = stateHydrateCmd.MarkFlagRequired("hook-key")

	stateCmd.AddCommand(stateHydrateCmd)
}
