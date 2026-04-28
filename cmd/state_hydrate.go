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
// by tasks 3-9 and 3-10. HookKey is reserved for Phase 4 hook lookup.
type hydrateConfig struct {
	FIFO              string
	File              string
	HookKey           string
	Stdout            io.Writer
	Client            *tmux.Client
	Logger            *state.Logger
	ExecShell         func(shell string)
	OpenFIFO          func(path string, timeout time.Duration) (*os.File, error)
	HandleFileMissing func(cfg hydrateConfig) error
	HandleTimeout     func(cfg hydrateConfig) error
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
				return cfg.HandleTimeout(cfg)
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

	// 3. Open the saved scrollback file.
	sb, err := os.Open(cfg.File)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			if cfg.HandleFileMissing != nil {
				return cfg.HandleFileMissing(cfg)
			}
			return err
		}
		return fmt.Errorf("open scrollback %s: %w", cfg.File, err)
	}
	defer func() { _ = sb.Close() }()

	// 4. Reset preamble — cursor visible, exit alt-screen, SGR reset.
	_, _ = io.WriteString(cfg.Stdout, hydrateResetPreamble)

	// 5. Stream scrollback to stdout. io.Copy preserves bytes verbatim and
	// streams in 32K blocks; the 5MB-file test verifies this end-to-end.
	if _, err := io.Copy(cfg.Stdout, sb); err != nil {
		if cfg.HandleFileMissing != nil {
			return cfg.HandleFileMissing(cfg)
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

	// 9. Exec the user shell. Phase 4 will add hook chaining here.
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	cfg.ExecShell(shell)
	return nil // unreachable in production (syscall.Exec replaces process)
}

// defaultExecShell is the production ExecShell seam: hand the process off to
// $SHELL via syscall.Exec. syscall.Exec only returns on error; if it does,
// the helper exits 1 so the pane closes rather than dangling without a shell.
func defaultExecShell(shell string) {
	_ = syscall.Exec(shell, []string{shell}, os.Environ())
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

		cfg := hydrateConfig{
			FIFO:      fifo,
			File:      file,
			HookKey:   hookKey,
			Stdout:    cmd.OutOrStdout(),
			Client:    tmux.NewClient(&tmux.RealCommander{}),
			Logger:    nil,
			ExecShell: defaultExecShell,
			OpenFIFO:  openFIFOWithTimeout,
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
