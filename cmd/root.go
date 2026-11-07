package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/state"
	"github.com/spf13/cobra"
)

// Bootstrap must not run for these: it would heal doctor's subject, build what
// uninstall then tears down, run on migrate's version 1 state before migrate
// runs it on the state it makes, run again inside the saver and the helpers
// its `state` subcommands are, and run on every __complete TAB; the rest only
// read or label sessions. A match anywhere in the parent chain exempts
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
	"migrate":    true,
	"rename":     true,
	"state":      true,
	"theme":      true,
	"uninstall":  true,
	"version":    true,
}

// The state check spares version and init, which read no state, so a new
// shell's eval of init's script keeps working before portal migrate; doctor, which reports
// an unusable sessions.json, or a migration cut short, itself, and repairs
// nothing on either; current, which only reads
// it and shows the name alone when it can't, since footers run it on every
// refresh; migrate, whose subject is the version 1 sessions.json the check
// refuses, and which judges each state dir it finds itself; and the hidden
// state commands, the helpers a restored session runs: a refused helper
// exits, and the session it runs in ends with it.
var stateCheckExempt = map[string]bool{
	"current": true,
	"doctor":  true,
	"init":    true,
	"migrate": true,
	"state":   true,
	"version": true,
}

// Exempt from the bootstrap, these still write state: sessions.json's claims,
// a label, hooks.json. Run against another zmx directory than the one the
// state directory belongs to, they would claim entries for sessions the state
// doesn't hold, so they make the check the bootstrap's gate makes, recording
// nothing. Keyed on the command's path below the root, so the silent `hooks`
// alias rides on "hook". Doctor's --fix makes the same check itself, after
// its diagnosis.
var exemptStateWriters = map[string]bool{
	"hook rm":  true,
	"hook set": true,
	"rename":   true,
}

// refuseOtherZmxDir is the fatal of a state directory that belongs to another
// zmx directory than this Portal's, or whose record can't be read. One that
// records none passes, as does one that can't be resolved.
func refuseOtherZmxDir() error {
	dir, err := state.Dir()
	if err != nil {
		return nil
	}
	return zmxDirOwner(dir, zmx.DirFor(os.Environ())).Refusal()
}

// pathBelowRoot is cmd's path without the root's name, "hook set" for
// `portal hook set`.
func pathBelowRoot(cmd *cobra.Command) string {
	return strings.TrimPrefix(cmd.CommandPath(), cmd.Root().Name()+" ")
}

var bootstrapDeps *BootstrapDeps

// BootstrapDeps overrides the bootstrap's gate and the bootstrap it owes;
// production leaves the package-level bootstrapDeps nil.
type BootstrapDeps struct {
	// Owed reports whether a bootstrap is owed. Nil owes one exactly when an
	// Orchestrator is staged, so an empty BootstrapDeps owes none.
	Owed func(ctx context.Context) bool

	// Orchestrator is the bootstrap a command owed one runs.
	Orchestrator bootstrap.Runner

	// ForceMemoise opts back into the production sync.Once gate, which tests
	// bypass by default so subtests can swap mocks without resetting it.
	ForceMemoise bool
}

var (
	bootstrapOnce          sync.Once
	bootstrapWarningsSlice []bootstrap.Warning
	bootstrapErr           error
)

// owedBootstrap is the bootstrap this command owes, nil when it owes none:
// the warm path, which asks the gate alone.
func owedBootstrap(ctx context.Context) (bootstrap.Runner, error) {
	if bootstrapDeps != nil {
		return bootstrapDeps.owed(ctx), nil
	}
	p, err := newProductionBootstrap()
	if err != nil {
		return nil, err
	}
	if !p.gate.Owed(ctx) {
		return nil, nil
	}
	return p.runner()
}

func (d BootstrapDeps) owed(ctx context.Context) bootstrap.Runner {
	if d.Owed != nil && !d.Owed(ctx) {
		return nil
	}
	return d.Orchestrator
}

func runBootstrap(ctx context.Context, runner bootstrap.Runner) ([]bootstrap.Warning, error) {
	if bootstrapDeps != nil && !bootstrapDeps.ForceMemoise {
		return runner.Run(ctx)
	}
	bootstrapOnce.Do(func() {
		bootstrapWarningsSlice, bootstrapErr = runner.Run(ctx)
	})
	return bootstrapWarningsSlice, bootstrapErr
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
		if exemptStateWriters[pathBelowRoot(cmd)] {
			return refuseOtherZmxDir()
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

// bootstrapFor runs the bootstrap the command owes, or defers it to the
// picker's loading page on a picker launch. A command owed none runs
// straight away.
func bootstrapFor(cmd *cobra.Command, args []string) error {
	runner, err := owedBootstrap(cmd.Context())
	if err != nil {
		return err
	}
	if shouldRunConcurrentBootstrap(cmd, args, runner) {
		// openTUI runs the deferred bootstrap in a goroutine behind the
		// loading page.
		cmd.SetContext(context.WithValue(cmd.Context(), deferredBootstrapKey, &deferredBootstrap{runner: runner}))
		return nil
	}
	if runner == nil {
		return nil
	}

	warnings, err := runBootstrap(bootstrap.WithProgressEmitter(cmd.Context(), waitNotice(cmd.ErrOrStderr())), runner)
	if err != nil {
		return err
	}
	// Every picker launch took the deferred route above, so warnings precede
	// the command's own output here.
	for _, w := range warnings {
		bootstrapWarnings.Add(w)
	}
	bootstrapWarnings.EmitTo(cmd.ErrOrStderr())
	return nil
}

// waitNotice is a command's progress off the picker: one line on w should its
// bootstrap wait a while for another Portal's, and nothing of its steps.
func waitNotice(w io.Writer) bootstrap.ProgressEmitter {
	return func(ev bootstrap.StepEvent) {
		if ev.Waiting {
			_, _ = fmt.Fprintln(w, "Waiting for another Portal to finish starting…")
		}
	}
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

// createFatal is the fatal a create's failure is, nil for one that is none:
// the one mapping Execute and the picker's mints share.
func createFatal(err error) error {
	if fatal := bootstrap.UnsupportedCreate(err); fatal != nil {
		return fatal
	}
	return nil
}

// Execute writes a *bootstrap.FatalError's UserMessage to stderr before
// returning it, and never calls os.Exit — the caller maps the returned error to
// the process exit code. A create's refusal of a session made by a zmx Portal
// can't talk to, from any command, is the fatal a bootstrap meeting that zmx
// gives.
func Execute() error {
	err := rootCmd.Execute()
	if err == nil {
		return nil
	}
	if _, ok := errors.AsType[*bootstrap.FatalError](err); !ok {
		if refused := createFatal(err); refused != nil {
			err = refused
		}
	}
	if fatal, ok := errors.AsType[*bootstrap.FatalError](err); ok {
		_, _ = fmt.Fprintln(fatalErrorStderr, fatal.UserMessage)
	}
	return err
}
