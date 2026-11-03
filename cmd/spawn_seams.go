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
	Getenv   func(string) string
	Exists   func(string) bool
	Logger   *slog.Logger
}

// buildProductionSpawnSeams detects the terminal through b, the runner
// registry and, for a session no runner leads, b's own process-table search
// for its zmx clients, which only the zmx driver has.
func buildProductionSpawnSeams(ctx context.Context, b backend.Backend) productionSpawnSeams {
	clients, ok := b.(spawn.ClientLister)
	if !ok {
		clients = noClients{}
	}
	return productionSpawnSeams{
		Detector: spawn.NewDetector(b, stateRunners{}, clients),
		Resolve:  buildResolver().Resolve,
		Ack:      stateAcks{},
		Exe:      os.Executable,
		Getenv:   os.Getenv,
		Exists: func(name string) bool {
			exists, err := b.Exists(ctx, name)
			return err == nil && exists
		},
		Logger: spawnLogger,
	}
}

// noClients finds no zmx client of any session, so detection inside a
// session no runner leads refuses.
type noClients struct{}

func (noClients) AttachClients(string) ([]int, error) { return nil, nil }

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
