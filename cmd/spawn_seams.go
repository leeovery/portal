package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/xdg"
)

var spawnLogger = log.For("spawn")

type TerminalDetector interface {
	Detect() spawn.Identity
}

// The open burst, the picker and doctor wire these seams into
// differently-shaped structs, so the compiler cannot catch one side gaining or
// re-constructing a seam the others lack — hence one bundle they all read.
type productionSpawnSeams struct {
	Detector *spawn.Detector
	Resolve  spawn.AdapterResolver
	Ack      spawn.AckChannelFull
	Exe      spawn.ExecutableResolver
	Env      spawn.WindowEnv
	Exists   func(string) bool
	Logger   *slog.Logger
}

// buildProductionSpawnSeams detects the terminal through b, runners and, for
// a session no runner leads, b's own search for the session's clients.
func buildProductionSpawnSeams(ctx context.Context, b backend.Backend, runners spawn.RunnerLocator) productionSpawnSeams {
	return productionSpawnSeams{
		Detector: spawn.NewDetector(b, runners, b),
		Resolve:  buildResolver().Resolve,
		Ack:      stateAcks{},
		Exe:      os.Executable,
		Env:      windowEnv(b),
		Exists: func(name string) bool {
			exists, err := b.Exists(ctx, name)
			return err == nil && exists
		},
		Logger: spawnLogger,
	}
}

// windowEnv is what a spawned window's portal resolves by, so that it finds
// the sessions, state, config and log its opener did whatever the window's
// shell sets: this process's PATH, the state directory as resolved here, what
// b needs to reach its sessions, and each of Portal's path and logging
// variables as this process has it — a path made xdg.Absolute, since the
// window starts in another directory, and one this process lacks unset.
func windowEnv(b backend.Backend) spawn.WindowEnv {
	return func() (spawn.WindowEnvironment, error) {
		var env spawn.WindowEnvironment
		stateDir, err := state.Dir()
		if err != nil {
			return env, err
		}
		env.Set = append([]string{"PATH=" + os.Getenv("PATH"), xdg.StateDir.EnvVar + "=" + stateDir}, b.ReachEnv()...)
		for _, name := range xdg.Variables() {
			if name == xdg.StateDir.EnvVar {
				continue
			}
			if err := carry(&env, name, xdg.Absolute); err != nil {
				return env, err
			}
		}
		for _, name := range log.Variables() {
			if err := carry(&env, name, verbatim); err != nil {
				return env, err
			}
		}
		return env, nil
	}
}

// carry sets name in env to this process's value as resolve has it, or unsets
// it when this process doesn't have it set.
func carry(env *spawn.WindowEnvironment, name string, resolve func(string) (string, error)) error {
	value, set := os.LookupEnv(name)
	if !set {
		env.Unset = append(env.Unset, name)
		return nil
	}
	value, err := resolve(value)
	if err != nil {
		return fmt.Errorf("carry %s to a window: %w", name, err)
	}
	env.Set = append(env.Set, name+"="+value)
	return nil
}

func verbatim(value string) (string, error) { return value, nil }

// Fails safe: an unresolvable config path degrades to an empty config —
// native-only resolution — rather than disabling the feature outright.
func buildResolver() *spawn.Resolver {
	cfg := spawn.TerminalsConfig{}
	if path, err := configFilePath(xdg.TerminalsFile); err == nil {
		cfg = spawn.NewTerminalsStore(path).Load()
	}
	return spawn.NewResolver(cfg)
}

// stateAcks is the spawn-ack channel in Portal's state directory, resolved at
// each use, as the runner registry is. A directory that won't resolve fails
// the use: a window's ack goes unwritten, and the burst counts it failed.
type stateAcks struct{}

var (
	_ spawn.AckWriter      = stateAcks{}
	_ spawn.AckChannelFull = stateAcks{}
)

func (stateAcks) Write(batch, token string) error {
	c, err := inStateDir(spawn.NewFileAckChannel)
	if err != nil {
		return err
	}
	return c.Write(batch, token)
}

func (stateAcks) Collect(batch string) (map[string]struct{}, error) {
	c, err := inStateDir(spawn.NewFileAckChannel)
	if err != nil {
		return nil, err
	}
	return c.Collect(batch)
}

func (stateAcks) Clean(batch string) error {
	c, err := inStateDir(spawn.NewFileAckChannel)
	if err != nil {
		return err
	}
	return c.Clean(batch)
}
