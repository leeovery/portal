package cmd

import (
	"fmt"
	"os"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hooksweep"
	"github.com/leeovery/portal/internal/nanoid"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/xdg"
	"github.com/spf13/cobra"
)

// HookKeyResolver maps a tmux pane ID ("%3") to its hook key — the pane's
// durable token, empty for a pane that has never been stamped.
type HookKeyResolver interface {
	ResolveHookKey(paneID tmux.Target) (string, error)
}

// PaneHookLister is the staleness cycle's live pane enumeration, declared once
// in internal/hooksweep and aliased here for cmd's own seams.
type PaneHookLister = hooksweep.PaneHookLister

// PaneOptionSetter writes one tmux option onto one pane.
type PaneOptionSetter interface {
	SetPaneOption(target tmux.Target, name, value string) error
}

var (
	_ HookKeyResolver  = (*tmux.Client)(nil)
	_ PaneHookLister   = (*tmux.Client)(nil)
	_ PaneOptionSetter = (*tmux.Client)(nil)
)

var hooksDeps *HooksDeps

type HooksDeps struct {
	KeyResolver HookKeyResolver
	PaneLister  PaneHookLister
	PaneStamper PaneOptionSetter
	TokenMinter nanoid.Generator
}

func requireTmuxPane() (tmux.Target, error) {
	paneID := os.Getenv("TMUX_PANE")
	if paneID == "" {
		return "", fmt.Errorf("must be run from inside a tmux pane")
	}
	return tmux.PaneIDTarget(paneID), nil
}

func buildHooksTmuxClient() *tmux.Client {
	return tmux.DefaultClient()
}

// resolveCurrentPaneKey returns the current pane's hook key alongside the pane
// it was read from, so a caller that must stamp acts on that same pane.
//
// A failed read is surfaced as the resolver phrased it: a gone pane is reported
// in tmux's own words behind one Portal-authored clause, and restating that
// clause here would put a second one in front of them.
func resolveCurrentPaneKey() (hookKey string, paneID tmux.Target, err error) {
	paneID, err = requireTmuxPane()
	if err != nil {
		return "", "", err
	}

	hookKey, err = hookSeams().KeyResolver.ResolveHookKey(paneID)
	if err != nil {
		return "", "", err
	}

	return hookKey, paneID, nil
}

// hookSeams returns the seams every hook command runs against: whatever a test
// injected, with the production default filled in for each seam it left unset.
func hookSeams() HooksDeps {
	var seams HooksDeps
	if hooksDeps != nil {
		seams = *hooksDeps
	}

	client := buildHooksTmuxClient()
	if seams.KeyResolver == nil {
		seams.KeyResolver = client
	}
	if seams.PaneLister == nil {
		seams.PaneLister = client
	}
	if seams.PaneStamper == nil {
		seams.PaneStamper = client
	}
	if seams.TokenMinter == nil {
		seams.TokenMinter = nanoid.NewPaneTokenGenerator()
	}

	return seams
}

// stampPaneToken mints a token for an un-stamped pane and writes it, returning
// tmux's own error unaltered so a target naming no pane reads as tmux said it.
func stampPaneToken(paneID tmux.Target) (string, error) {
	seams := hookSeams()

	token, err := seams.TokenMinter()
	if err != nil {
		return "", fmt.Errorf("failed to mint a pane token: %w", err)
	}

	if err := seams.PaneStamper.SetPaneOption(paneID, state.PortalPaneIDOption, token); err != nil {
		return "", err
	}

	return token, nil
}

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

		// Nothing to resolve means no tmux read at all.
		if len(list) == 0 {
			return nil
		}

		locations := paneLocationsByToken(hookSeams().PaneLister)

		for _, h := range list {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\n", h.Key, h.Event, h.Command, locations[h.Key]); err != nil {
				return err
			}
		}

		return nil
	},
}

// paneLocationsByToken maps each live pane's token to where that pane lives.
func paneLocationsByToken(lister PaneHookLister) map[string]string {
	rows, err := lister.ListAllPaneHookKeys()
	if err != nil {
		// `hook` starts no tmux server, so a read against a machine with no server
		// is ordinary rather than a failure: every location renders empty and the
		// listing still succeeds.
		return nil
	}

	locations := make(map[string]string, len(rows))
	for _, row := range rows {
		// An unstamped pane answers to no key: mapping its empty token would lend
		// its location to an entry that names no pane.
		if row.Token == "" {
			continue
		}
		// Two rows can carry one token without anyone hand-stamping it: a
		// window shared by grouped sessions, or linked with link-window,
		// enumerates once per session that shows it, so one pane is listed
		// twice. First row wins and the entry resolves to one location.
		if _, seen := locations[row.Token]; seen {
			continue
		}
		locations[row.Token] = row.Location
	}
	return locations
}

var hooksSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Register a resume hook for the current pane",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		hookKey, paneID, err := resolveCurrentPaneKey()
		if err != nil {
			return err
		}

		command, err := cmd.Flags().GetString("on-resume")
		if err != nil {
			return err
		}

		// The stamp lands before the entry: an entry written first, or after a
		// stamp that failed, is keyed to a token no pane carries.
		if hookKey == "" {
			if hookKey, err = stampPaneToken(paneID); err != nil {
				return err
			}
		}

		store, err := loadHookStore()
		if err != nil {
			return err
		}

		if err := store.Set(hookKey, hooks.EventOnResume, command, hooks.ViaCLI); err != nil {
			return err
		}

		touchSaveRequestedForHook(hookKey)
		return nil
	},
}

// touchSaveRequestedForHook nudges the daemon into capturing the pane's token on
// its next tick rather than waiting for its gap branch, narrowing the window in
// which a crash leaves an entry keyed to a token no saved pane carries.
//
// Best-effort by design: the entry is already durably written, so a failure here
// costs a latency optimisation, not a registration — hence its own op, never set,
// and no effect on the exit status. Both failure modes share the one emission so
// neither can double up.
func touchSaveRequestedForHook(hookKey string) {
	dir, err := state.EnsureDir()
	if err == nil {
		err = state.TouchSaveRequested(dir)
	}
	if err != nil {
		hooksLogger.Warn("touch-save-requested", "op", "touch-save-requested",
			"hook_key", hookKey, "via", hooks.ViaCLI.String(), "error", err)
	}
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
	Short: "Remove a resume hook for the current pane",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		paneKey, err := cmd.Flags().GetString("pane-key")
		if err != nil {
			return err
		}

		var hookKey string
		if paneKey != "" {
			hookKey = paneKey
		} else {
			hookKey, _, err = resolveCurrentPaneKey()
			if err != nil {
				return err
			}
			// A live pane carrying no token, in Portal's own words: the probe in
			// resolveCurrentPaneKey has already separated this from a gone pane.
			// Deciding it here keeps an empty key out of the mutation entirely.
			if hookKey == "" {
				return fmt.Errorf("no resume hook registered for this pane")
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
	hooksSetCmd.Flags().String("on-resume", "", "Command to run when resuming the pane")
	_ = hooksSetCmd.MarkFlagRequired("on-resume")

	hooksRmCmd.Flags().Bool("on-resume", false, "Remove the on-resume hook")
	_ = hooksRmCmd.MarkFlagRequired("on-resume")
	hooksRmCmd.Flags().String("pane-key", "", "Hook key of the entry to remove, taken verbatim — any key, including an old-format one (defaults to the current pane's token)")

	hookCmd.AddCommand(hooksListCmd)
	hookCmd.AddCommand(hooksSetCmd)
	hookCmd.AddCommand(hooksRmCmd)
	rootCmd.AddCommand(hookCmd)
}
