package cmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
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
	Logger            *state.Logger
	HookStore         *hooks.Store
	ExecShell         func(prog string, args []string)
	OpenFIFO          func(path string, timeout time.Duration) (*os.File, error)
	HandleFileMissing func(cfg hydrateConfig, ctx hydrateFileMissingContext) error
	HandleTimeout     func(cfg hydrateConfig) error
}

// hydrateFileMissingContext carries the underlying cause of a file-missing
// transition into handleHydrateFileMissing so the handler can log distinct
// prefixes for ENOENT, permission, and generic I/O failures. The preamble has
// already been written by runHydrate by the time the handler runs (see
// runHydrate step ordering); the handler must not re-emit it.
type hydrateFileMissingContext struct {
	Cause error
}

// paneKeyFromFIFOPath strips the "hydrate-" prefix and ".fifo" suffix from the
// FIFO basename to recover the live paneKey used by the @portal-skeleton-<key>
// marker. Example: "/run/portal/hydrate-foo__0.0.fifo" → "foo__0.0".
func paneKeyFromFIFOPath(fifoPath string) string {
	base := filepath.Base(fifoPath)
	base = strings.TrimSuffix(base, ".fifo")
	base = strings.TrimPrefix(base, "hydrate-")
	return base
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
	livePaneKey := paneKeyFromFIFOPath(cfg.FIFO)

	// 1. Block on FIFO until signal arrives or timeout fires.
	f, err := cfg.OpenFIFO(cfg.FIFO, hydrateTimeout)
	if err != nil {
		if errors.Is(err, ErrHydrateTimeout) {
			if cfg.HandleTimeout != nil {
				if err := cfg.HandleTimeout(cfg); err != nil {
					return err
				}
				// Timeout path falls through to a bare-shell exec — pane
				// gets an empty $SHELL prompt; no hook firing on this path.
				execShellAndExit(cfg)
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
	_ = f.Close()
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
	markerName := "@portal-skeleton-" + livePaneKey
	if err := cfg.Client.UnsetServerOption(markerName); err != nil && cfg.Logger != nil {
		cfg.Logger.Warn("hydrate", "unset marker %s: %v", markerName, err)
	}

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

// execShellOrHookAndExit is the post-hydration terminal exec for the
// signal-arrived and file-missing paths (NOT the timeout path — see spec
// "Helper Behavior on Startup", step 3e: "exec $SHELL (bare shell; no hook
// firing on this path)").
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
	if cfg.HookStore == nil {
		execShellAndExit(cfg)
		return
	}
	command, found, err := hooks.LookupOnResume(cfg.HookStore, cfg.HookKey)
	if err != nil {
		if cfg.Logger != nil {
			cfg.Logger.Warn("hydrate", "lookup on-resume hook for %s: %v", cfg.HookKey, err)
		}
		execShellAndExit(cfg)
		return
	}
	if !found {
		execShellAndExit(cfg)
		return
	}
	shell := resolveShell()
	chained := command + "; exec " + shell
	cfg.ExecShell("/bin/sh", []string{"sh", "-c", chained})
}

// handleHydrateTimeout is invoked when openFIFOWithTimeout returns
// ErrHydrateTimeout. Spec ("Helper Behavior on Startup", step 3): emit the
// reset preamble only, unlink the FIFO, log a warning naming the hook-key,
// do NOT unset the @portal-skeleton marker (the marker stays set so the next
// attach re-signals and retries hydration), and do NOT sleep 100ms — nothing
// was dumped, so there is no PTY parser settle to wait on. After this returns
// nil, runHydrate falls through to exec the user's $SHELL.
func handleHydrateTimeout(cfg hydrateConfig) error {
	// 1. Reset preamble only — no scrollback dump, no postamble.
	_, _ = io.WriteString(cfg.Stdout, hydrateResetPreamble)

	// 2. Unlink the FIFO. Defense in depth — the next bootstrap also sweeps
	// stale hydrate-*.fifo files. Errors are tolerated silently because the
	// FIFO may not exist (e.g., already removed) and a permission error here
	// must not block the shell exec the helper falls through to.
	_ = os.Remove(cfg.FIFO)

	// 3. Log a warning naming the hook-key + FIFO so operators can correlate
	// the entry with the affected pane in the saved sessions.json.
	if cfg.Logger != nil {
		cfg.Logger.Warn("hydrate", "timeout waiting for signal on --hook-key=%s --fifo=%s", cfg.HookKey, cfg.FIFO)
	}

	// 4. Deliberately NO UnsetServerOption — marker stays set so the next
	// attach re-signals.
	// 5. Deliberately NO 100ms sleep — nothing was dumped to settle.
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
	// 1. Log a distinct WARN entry per failure cause so operators can tell
	// missing files (likely GC race) apart from permission misconfiguration
	// or transient disk I/O errors.
	if cfg.Logger != nil {
		switch {
		case errors.Is(ctx.Cause, fs.ErrNotExist):
			cfg.Logger.Warn("hydrate", "scrollback file not found for --hook-key=%s --file=%s", cfg.HookKey, cfg.File)
		case errors.Is(ctx.Cause, fs.ErrPermission):
			cfg.Logger.Warn("hydrate", "scrollback file unreadable (permission denied) for --hook-key=%s --file=%s", cfg.HookKey, cfg.File)
		default:
			cfg.Logger.Warn("hydrate", "scrollback file I/O error for --hook-key=%s --file=%s: %v", cfg.HookKey, cfg.File, ctx.Cause)
		}
	}

	// 2. Deliberately NO 100ms sleep — nothing was fully dumped, so there is
	// no PTY parser settle to wait on.

	// 3. Unset the skeleton marker — KEY DIFFERENCE FROM TIMEOUT PATH. With
	// no scrollback to dump, the pane is empty and the save loop should
	// resume capturing it on the next tick rather than skipping it forever.
	livePaneKey := paneKeyFromFIFOPath(cfg.FIFO)
	markerName := "@portal-skeleton-" + livePaneKey
	if err := cfg.Client.UnsetServerOption(markerName); err != nil && cfg.Logger != nil {
		cfg.Logger.Warn("hydrate", "unset marker %s: %v", markerName, err)
	}
	return nil
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
		cfg := hydrateConfig{
			FIFO:              fifo,
			File:              file,
			HookKey:           hookKey,
			Stdout:            cmd.OutOrStdout(),
			Client:            tmux.NewClient(&tmux.RealCommander{}),
			Logger:            nil,
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
