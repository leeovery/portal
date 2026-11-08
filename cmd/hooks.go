package cmd

import (
	"context"
	"fmt"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/resumemode"
	"github.com/leeovery/portal/internal/xdg"
	"github.com/spf13/cobra"
)

const resumeModeFlagName = "resume-mode"

// `hooks` is a permanent back-compat alias for machine-written invocations. It
// must stay a plain Aliases entry — cmd.Deprecated would print a notice.
var hookCmd = &cobra.Command{
	Use:     "hook",
	Aliases: []string{"hooks"},
	Short:   "Manage resume hooks",
}

var hooksListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all registered hooks",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := loadHookStore()
		if err != nil {
			return err
		}

		list, err := store.List(hooks.ViaCLI)
		if err != nil {
			return err
		}

		// Nothing to resolve means no backend read at all.
		if len(list) == 0 {
			return nil
		}

		locations := sessionLocations(cmd.Context())

		for _, h := range list {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\t%s\n", h.Key, h.Event, h.Command, locations[h.Key], h.Resume.String()); err != nil {
				return err
			}
		}

		return nil
	},
}

// sessionLocations maps each live session's name, the key its hooks are
// stored under, to what it shows. A key no live session carries maps to
// nothing.
func sessionLocations(ctx context.Context) map[string]string {
	b, err := sessionBackend()
	if err != nil {
		return nil
	}
	sessions, err := listShownSessions(ctx, b)
	if err != nil {
		// A listing that fails leaves every location empty; the hooks
		// themselves still list.
		return nil
	}
	locations := make(map[string]string, len(sessions))
	for _, s := range sessions {
		locations[s.Name] = s.DisplayName()
	}
	return locations
}

var hooksSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Register a resume hook for the current session",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Resolved before anything touches a file: a refused value must leave
		// no entry written.
		mode, err := resumeModeFlag(cmd)
		if err != nil {
			return err
		}

		command, err := cmd.Flags().GetString("on-resume")
		if err != nil {
			return err
		}

		b, err := sessionBackend()
		if err != nil {
			return err
		}
		hookKey, err := currentSessionIn(b)
		if err != nil {
			return err
		}
		return setResumeHook(cmd.Context(), b, hookKey, hooks.Registration{Command: command, Resume: mode})
	},
}

// registerHook is (*hooks.Store).Set; tests pause it to hold the commit lock
// open.
var registerHook = (*hooks.Store).Set

// setResumeHook claims the session's entry, then registers its resume hook, as
// rename claims before a label: a hook beside an entry left by an earlier
// creation would go with it when a later claim replaced that entry. The hook
// is written under the claim's commit lock, so a kill can't remove the entry
// before it lands and leave it on a dead name, and only once the claim's write
// has landed and what it dropped is forgotten: set any sooner, the forgetting
// of an earlier creation's entry would take it, being keyed by the same name.
func setResumeHook(ctx context.Context, b backend.Backend, name string, registration hooks.Registration) error {
	live, err := liveSession(ctx, b, name, "no hook is set")
	if err != nil {
		return err
	}
	return claimSession("cli", b.ValidateName, live, nil, func() error {
		store, err := loadHookStore()
		if err != nil {
			return err
		}
		return registerHook(store, name, hooks.EventOnResume, registration, hooks.ViaCLI)
	})
}

// resumeModeFlag reads the registration's resume-mode pin: an unpassed flag
// names no mode, and a passed one must spell a mode the vocabulary admits —
// the empty string included, which is a value rather than an absence.
func resumeModeFlag(cmd *cobra.Command) (resumemode.Mode, error) {
	if !cmd.Flags().Changed(resumeModeFlagName) {
		return resumemode.Unset, nil
	}

	value, err := cmd.Flags().GetString(resumeModeFlagName)
	if err != nil {
		return resumemode.Unset, err
	}

	mode, ok := resumemode.Parse(value)
	if !ok {
		return resumemode.Unset, NewUsageError(fmt.Sprintf("--%s must be %q or %q",
			resumeModeFlagName, resumemode.Eager, resumemode.Lazy))
	}

	return mode, nil
}

func loadHookStore() (*hooks.Store, error) {
	path, err := hooksFilePath()
	if err != nil {
		return nil, err
	}

	return hooks.NewStore(path), nil
}

func hooksFilePath() (string, error) {
	return configFilePath(xdg.HooksFile)
}

var hooksRmCmd = &cobra.Command{
	Use:   "rm",
	Short: "Remove a resume hook for the current session",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		paneKey, err := cmd.Flags().GetString("pane-key")
		if err != nil {
			return err
		}

		hookKey := paneKey
		if hookKey == "" {
			if hookKey, err = currentSession(); err != nil {
				return err
			}
		}

		store, err := loadHookStore()
		if err != nil {
			return err
		}

		// The removal itself reports whether an entry went: a read taken before it
		// would answer from a snapshot the mutation never saw.
		removed, err := store.Remove(hookKey, hooks.EventOnResume, hooks.ViaCLI)
		if err != nil {
			return err
		}
		if !removed {
			return fmt.Errorf("no resume hook registered for %s", hookKey)
		}

		return nil
	},
}

func init() {
	hooksSetCmd.Flags().String("on-resume", "", "Command to run when resuming the session")
	hooksSetCmd.Flags().String(resumeModeFlagName, "", "Resume mode this registration is pinned to: eager or lazy (default: the install-wide setting)")
	_ = hooksSetCmd.MarkFlagRequired("on-resume")

	hooksRmCmd.Flags().Bool("on-resume", false, "Remove the on-resume hook")
	_ = hooksRmCmd.MarkFlagRequired("on-resume")
	hooksRmCmd.Flags().String("pane-key", "", "Hook key of the entry to remove, taken verbatim (defaults to the current session's name)")

	hookCmd.AddCommand(hooksListCmd)
	hookCmd.AddCommand(hooksSetCmd)
	hookCmd.AddCommand(hooksRmCmd)
	rootCmd.AddCommand(hookCmd)
}
