package cmd

import (
	"context"
	"fmt"
	"io"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/migrate"
	"github.com/leeovery/portal/internal/nanoid"
	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/session"
	"github.com/leeovery/portal/internal/state"
	"github.com/spf13/cobra"
)

var migrateDeps *MigrateDeps

// MigrateDeps overrides how portal migrate reaches the tmux version's server,
// the tmux version's daemon and the bootstrap it ends with; production leaves
// migrateDeps nil. A field left nil, or zero, is production's.
type MigrateDeps struct {
	Tmux   migrate.TmuxServer
	Daemon migrate.Daemon
	// Bootstrap runs the bootstrap a run ends with, its progress told on
	// stderr, and returns how its restore ended, with what it restored, and
	// its warnings.
	Bootstrap func(ctx context.Context, stderr io.Writer) (migrate.Restore, []bootstrap.Warning, error)
}

// migrateTmux is the tmux version's server as production reaches it: plain
// tmux, which honours $TMUX. cmd's TestMain makes it panic.
var migrateTmux = func() migrate.TmuxServer { return migrate.Tmux{} }

func currentMigrateDeps() MigrateDeps {
	var deps MigrateDeps
	if migrateDeps != nil {
		deps = *migrateDeps
	}
	if deps.Tmux == nil {
		deps.Tmux = migrateTmux()
	}
	if deps.Bootstrap == nil {
		deps.Bootstrap = migrateBootstrap
	}
	return deps
}

// Migrate is exempt from the state check, as the version 1 sessions.json it
// migrates is what that check refuses, and from the root's bootstrap, which
// it runs itself once the state is this version's.
var migrateCmd = &cobra.Command{
	Use:           "migrate",
	Short:         "Move the tmux version of Portal's saved sessions and resume hooks to this version",
	Args:          cobra.NoArgs,
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runMigrate(cmd.Context(), currentMigrateDeps(), cmd.OutOrStdout(), cmd.ErrOrStderr())
	},
}

func init() {
	rootCmd.AddCommand(migrateCmd)
}

// runMigrate migrates, then, when the state dir is this version's, runs the
// bootstrap and prints the report. A bootstrap's fatal follows the report.
func runMigrate(ctx context.Context, deps MigrateDeps, stdout, stderr io.Writer) error {
	cfg, err := migrateConfig(deps, stderr)
	if err != nil {
		return migrate.Stopped(err)
	}
	outcome, err := migrate.Run(ctx, cfg)
	if err != nil {
		return err
	}
	var restored migrate.Restore
	var bootErr error
	if outcome.Course.Bootstraps() {
		restored, bootErr = bootstrapMigrated(ctx, deps, cfg, stderr)
	}
	shown := func(path string) string { return resolver.AbbreviateHome(resolver.ProcessHome(), path) }
	if _, err := fmt.Fprint(stdout, migrate.Report(outcome, restored, shown)); err != nil {
		return err
	}
	return bootErr
}

// bootstrapMigrated runs the bootstrap on the state dir cfg names, showing
// its warnings, and returns what it restored, whether any session waits on
// its resume panel, read before it, and what it left, read after it from the
// index, with its fatal.
func bootstrapMigrated(ctx context.Context, deps MigrateDeps, cfg migrate.Config, stderr io.Writer) (migrate.Restore, error) {
	waits := migrate.Waits(cfg)
	restored, warnings, err := deps.Bootstrap(ctx, stderr)
	for _, w := range warnings {
		bootstrapWarnings.Add(w)
	}
	bootstrapWarnings.EmitTo(stderr)
	restored.Waits = waits
	restored.Left, restored.LeftUnread = migrate.Left(ctx, cfg)
	return restored, err
}

// migrateConfig is the run over the state dir and hooks.json the environment
// names, its names judged by the session backend's rules.
func migrateConfig(deps MigrateDeps, stderr io.Writer) (migrate.Config, error) {
	dir, err := state.Dir()
	if err != nil {
		return migrate.Config{}, err
	}
	hooksFile, err := hooksFilePath()
	if err != nil {
		return migrate.Config{}, err
	}
	b, err := sessionBackend()
	if err != nil {
		return migrate.Config{}, err
	}
	runner := &resolver.RealCommandRunner{}
	return migrate.Config{
		StateDir:  dir,
		HooksFile: hooksFile,
		Tmux:      deps.Tmux,
		Daemon:    deps.Daemon,
		Live:      func(ctx context.Context) ([]string, error) { return sessionNames(ctx, b) },
		Naming: migrate.Naming{
			Validate:    b.ValidateName,
			Refuses:     zmx.RefusesRune,
			Mint:        nanoid.NewGenerator(),
			GroupingDir: func(cwd string) (string, bool) { return session.GroupingDir(cwd, runner) },
		},
		Waiting: func(pid int) { _, _ = fmt.Fprintln(stderr, migrate.WaitingLine(pid)) },
	}, nil
}

// sessionNames is the name of every session b holds, Portal's own included.
func sessionNames(ctx context.Context, b backend.Backend) ([]string, error) {
	sessions, err := b.List(ctx)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(sessions))
	for _, s := range sessions {
		names = append(names, s.Name)
	}
	return names, nil
}

// migrateBootstrap runs the bootstrap a run ends with, when one is owed, as
// any command runs it off the picker, and returns how its restore ended.
// Like any command's, it runs none when none is owed, whatever the index
// still owes: a re-run forces no restore.
func migrateBootstrap(ctx context.Context, stderr io.Writer) (migrate.Restore, []bootstrap.Warning, error) {
	p, err := newProductionBootstrap()
	if err != nil {
		return migrate.Restore{End: migrate.RestoreStopped}, nil, err
	}
	if !p.gate.Owed(ctx) {
		return migrate.Restore{}, nil, nil
	}
	var restored *restore.Result
	runner, err := p.runnerTelling(func(res restore.Result) { restored = &res })
	if err != nil {
		return migrate.Restore{End: migrate.RestoreStopped}, nil, err
	}
	warnings, err := runner.Run(bootstrap.WithProgressEmitter(ctx, waitNotice(stderr)))
	return restoreOf(restored, err), warnings, err
}

// restoreOf is how the restore of a bootstrap that ended with err ended, res
// its result, nil when it ran none. A fatal, before the restore or during it,
// stops it, after the sessions it made. With none, a bootstrap that ran no
// restore either found none owed under its lock or left the restore
// unfinished before reaching it, for sessions it couldn't mark owed or a
// saver it couldn't judge, which its warning tells of. A restore that
// couldn't read sessions.json or list zmx's sessions made nothing, for the
// reason the bootstrap's warning gives.
func restoreOf(res *restore.Result, err error) migrate.Restore {
	switch {
	case err != nil && res != nil:
		return migrate.Restore{End: migrate.RestoreStopped, Restored: res.Restored}
	case err != nil:
		return migrate.Restore{End: migrate.RestoreStopped}
	case res == nil:
		return migrate.Restore{}
	case res.IndexErr != nil || res.ListErr != nil:
		return migrate.Restore{End: migrate.RestoreUnread}
	}
	return migrate.Restore{End: migrate.RestoreRan, Restored: res.Restored}
}
