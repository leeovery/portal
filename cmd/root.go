package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"sync"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/spf13/cobra"
)

// Bootstrap must not run for these: it would heal doctor's subject, build what
// uninstall then tears down, run again inside the saver and the helpers its
// `state` subcommands are, and run on every __complete TAB; the rest only read
// or label sessions. A match anywhere in the parent chain exempts
// the invocation, keyed on cobra's canonical Name() so the silent `hooks` alias
// rides on "hook".
var bootstrapExempt = map[string]bool{
	"__complete": true,
	"alias":      true,
	"current":    true,
	"doctor":     true,
	"help":       true,
	"hook":       true,
	"init":       true,
	"rename":     true,
	"state":      true,
	"theme":      true,
	"uninstall":  true,
	"version":    true,
}

// The state check spares version, which reads no state; doctor, which reports
// an unusable sessions.json itself and writes none; current, which only reads
// it and shows the name alone when it can't, since footers run it on every
// refresh; and the hidden state commands, the helpers a restored session runs:
// a refused helper exits, and the session it runs in ends with it.
var stateCheckExempt = map[string]bool{
	"current": true,
	"doctor":  true,
	"state":   true,
	"version": true,
}

var bootstrapDeps *BootstrapDeps

// BootstrapDeps overrides bootstrap collaborators; production leaves the
// package-level bootstrapDeps nil.
type BootstrapDeps struct {
	// Orchestrator is the bootstrap. Production has none until the zmx
	// bootstrap lands, so no command runs one.
	Orchestrator bootstrap.Runner

	// ForceMemoise opts back into the production sync.Once gate, which tests
	// bypass by default so subtests can swap mocks without resetting it.
	ForceMemoise bool
}

var (
	bootstrapOnce          sync.Once
	bootstrapStarted       bool
	bootstrapWarningsSlice []bootstrap.Warning
	bootstrapErr           error
)

func bootstrapRunner() bootstrap.Runner {
	if bootstrapDeps == nil {
		return nil
	}
	return bootstrapDeps.Orchestrator
}

func runBootstrap(ctx context.Context, runner bootstrap.Runner) (bool, []bootstrap.Warning, error) {
	if bootstrapDeps != nil && !bootstrapDeps.ForceMemoise {
		if runner == nil {
			return false, nil, nil
		}
		return runner.Run(ctx)
	}
	bootstrapOnce.Do(func() {
		if runner == nil {
			return
		}
		bootstrapStarted, bootstrapWarningsSlice, bootstrapErr = runner.Run(ctx)
	})
	return bootstrapStarted, bootstrapWarningsSlice, bootstrapErr
}

var rootCmd = &cobra.Command{
	Use:   "portal",
	Short: "An interactive session picker for zmx",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if !inChain(cmd, stateCheckExempt) {
			if err := refuseUnusableState(); err != nil {
				return err
			}
		}
		if inChain(cmd, bootstrapExempt) {
			return nil
		}
		// The picker creates and attaches too, so it fails here, before it
		// draws, rather than at the pick.
		if cmd.Name() == openCmd.Name() {
			if _, err := attachingBackend(); err != nil {
				return err
			}
		}
		return bootstrapFor(cmd, args)
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

// bootstrapFor runs the injected bootstrap, or defers it to the picker's
// loading page on a picker launch. With none injected it does nothing.
func bootstrapFor(cmd *cobra.Command, args []string) error {
	runner := bootstrapRunner()
	if shouldRunConcurrentBootstrap(cmd, args, runner) {
		// openTUI runs the deferred orchestrator in a goroutine behind the
		// loading page, and reads serverStarted from the progress pipe's
		// terminal event.
		cmd.SetContext(context.WithValue(cmd.Context(), deferredBootstrapKey, &deferredBootstrap{runner: runner}))
		return nil
	}
	if runner == nil {
		return nil
	}

	started, warnings, err := runBootstrap(cmd.Context(), runner)
	if err != nil {
		return err
	}
	// Every picker launch took the deferred route above, so warnings precede
	// the command's own output here.
	for _, w := range warnings {
		bootstrapWarnings.Add(w)
	}
	bootstrapWarnings.EmitTo(cmd.ErrOrStderr())
	cmd.SetContext(context.WithValue(cmd.Context(), serverStartedKey, started))
	return nil
}

// inChain reports whether cmd or any command above it is named in set.
func inChain(cmd *cobra.Command, set map[string]bool) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if set[c.Name()] {
			return true
		}
	}
	return false
}

// isTUIPath reports whether this invocation will launch the picker, and so must
// not have warnings written to stderr — they would corrupt the alt-screen.
//
// A search form is classified on its shape rather than on its outcome: whether
// it ends at the picker depends on how many live sessions its term matches, and
// on a cold start there are none to match until restore has finished.
func isTUIPath(cmd *cobra.Command, args []string) bool {
	if cmd.Name() != "open" || anyOpenDomainPin(cmd) {
		return false
	}
	return len(preDashPositionals(cmd, args)) == 0 || len(searchFormPositionals(cmd, args)) > 0
}

// A domain pin dispatches one resolved target directly, so it is not a picker
// launch. Reads openDomainPinFlags so a new pin cannot be omitted here.
func anyOpenDomainPin(cmd *cobra.Command) bool {
	return slices.ContainsFunc(openDomainPinFlags, cmd.Flags().Changed)
}

// A picker launch with a bootstrap to run shows the loading page while it
// runs. With no runner there is nothing to wait for, so the picker opens
// straight onto its list.
func shouldRunConcurrentBootstrap(cmd *cobra.Command, args []string, runner bootstrap.Runner) bool {
	return runner != nil && isTUIPath(cmd, args)
}

var fatalErrorStderr io.Writer = os.Stderr

// Execute writes a *bootstrap.FatalError's UserMessage to stderr before
// returning it, and never calls os.Exit — the caller maps the returned error to
// the process exit code.
func Execute() error {
	err := rootCmd.Execute()
	if err == nil {
		return nil
	}
	if fatal, ok := errors.AsType[*bootstrap.FatalError](err); ok {
		_, _ = fmt.Fprintln(fatalErrorStderr, fatal.UserMessage)
	}
	return err
}
