package cmd

import (
	"context"
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
// a session no runner leads, b's own process-table search for its zmx
// clients, which only the zmx driver has.
func buildProductionSpawnSeams(ctx context.Context, b backend.Backend, runners spawn.RunnerLocator) productionSpawnSeams {
	clients, ok := b.(spawn.ClientLister)
	if !ok {
		clients = noClients{}
	}
	return productionSpawnSeams{
		Detector: spawn.NewDetector(b, runners, clients),
		Resolve:  resolveThroughSeam(buildResolver()),
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

// sessionReach is a backend that can say what another process needs to reach
// its sessions; the zmx driver can.
type sessionReach interface {
	ReachEnv() []string
}

// windowEnv is what a spawned window's portal resolves by, so that it finds
// the sessions, state and config its opener did whatever the window's shell
// sets: this process's PATH, the state directory as resolved here, what b
// needs to reach its sessions, and each of Portal's path variables this
// process has set, set empty included.
func windowEnv(b backend.Backend) spawn.WindowEnv {
	return func() ([]string, error) {
		stateDir, err := state.Dir()
		if err != nil {
			return nil, err
		}
		env := []string{"PATH=" + os.Getenv("PATH"), xdg.StateDir.EnvVar + "=" + stateDir}
		if r, ok := b.(sessionReach); ok {
			env = append(env, r.ReachEnv()...)
		}
		for _, name := range xdg.Variables() {
			if value, set := os.LookupEnv(name); set && name != xdg.StateDir.EnvVar {
				env = append(env, name+"="+value)
			}
		}
		return env, nil
	}
}

// noClients finds no zmx client of any session, so detection inside a
// session no runner leads refuses.
type noClients struct{}

func (noClients) AttachClients(string) ([]backend.ClientProcess, error) { return nil, nil }

// spawnAdapter is what a resolved terminal opens its windows through: the
// resolver's own adapter in production. cmd's TestMain poisons it, since
// detection walks a test's ancestry to the developer's own terminal.
var spawnAdapter = func(a spawn.Adapter) spawn.Adapter { return a }

// resolveThroughSeam is r's resolution, its adapter passed through
// spawnAdapter.
func resolveThroughSeam(r *spawn.Resolver) spawn.AdapterResolver {
	return func(id spawn.Identity) (spawn.Adapter, spawn.Resolution) {
		adapter, resolution := r.Resolve(id)
		if adapter == nil {
			return nil, resolution
		}
		return spawnAdapter(adapter), resolution
	}
}

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

func (stateAcks) channel() (*spawn.FileAckChannel, error) {
	dir, err := state.Dir()
	if err != nil {
		return nil, err
	}
	return spawn.NewFileAckChannel(dir), nil
}

func (a stateAcks) Write(batch, token string) error {
	c, err := a.channel()
	if err != nil {
		return err
	}
	return c.Write(batch, token)
}

func (a stateAcks) Collect(batch string) (map[string]struct{}, error) {
	c, err := a.channel()
	if err != nil {
		return nil, err
	}
	return c.Collect(batch)
}

func (a stateAcks) Clean(batch string) error {
	c, err := a.channel()
	if err != nil {
		return err
	}
	return c.Clean(batch)
}
