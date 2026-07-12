package cmd

import (
	"errors"
	"fmt"

	"github.com/leeovery/portal/internal/spawn"
	"github.com/spf13/cobra"
)

// TerminalDetector resolves the host terminal's identity for the spawn
// command's --detect dry-run. It is the seam that lets the command body be
// Executed with a fabricated detector — no real tmux, ps, or defaults reads.
type TerminalDetector interface {
	Detect() spawn.Identity
}

// spawnDeps holds injectable dependencies for the spawn command. When nil,
// real implementations are used.
var spawnDeps *SpawnDeps

// SpawnDeps allows injecting dependencies for testing.
type SpawnDeps struct {
	Detector TerminalDetector
}

var spawnCmd = &cobra.Command{
	Use:   "spawn [sessions...]",
	Short: "Detect the host terminal (--detect) or open sessions in host-local windows",
	// SilenceUsage/SilenceErrors keep cobra from printing its own usage/error
	// text; main's classify owns exit codes and stderr. The FlagErrorFunc
	// bridges cobra's flag-parse errors (e.g. an unknown flag) into a
	// *cmd.UsageError so they exit 2 like the empty-invocation usage gate.
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		detect, err := cmd.Flags().GetBool("detect")
		if err != nil {
			return err
		}

		if detect {
			id := buildSpawnDeps(cmd).Detect()
			if id.IsNull() {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "no host-local terminal detected")
				return err
			}
			// "Name · BundleID" echoes the design separator, e.g.
			// "Apple Terminal · com.apple.Terminal".
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s · %s\n", id.Name, id.BundleID)
			return err
		}

		if len(args) == 0 {
			return NewUsageError("spawn: provide one or more sessions, or use --detect")
		}

		// Phase-1 placeholder: the spawn burst (pre-flight -> sequential spawn
		// -> self-attach) arrives in Phase 2, which replaces this branch. A
		// plain (non-usage) error keeps the increment honest without claiming
		// an exit-2 usage failure.
		return errors.New("spawn: opening sessions is not yet available")
	},
}

// buildSpawnDeps returns the terminal detector for the spawn command. When
// spawnDeps is set (testing), uses the injected detector. Otherwise builds the
// production detector against the shared tmux client from cmd.Context().
func buildSpawnDeps(cmd *cobra.Command) TerminalDetector {
	if spawnDeps != nil {
		return spawnDeps.Detector
	}
	return spawn.NewDetector(tmuxClient(cmd))
}

func init() {
	spawnCmd.Flags().Bool("detect", false, "print the detected host terminal identity and exit without opening anything")
	spawnCmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return NewUsageError(err.Error())
	})
	rootCmd.AddCommand(spawnCmd)
}
