package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/leeovery/portal/internal/spawn"
	"github.com/spf13/cobra"
)

var openBurstDeps *OpenBurstDeps

// OpenBurstDeps overrides production dependencies; an unset field falls back to
// the production implementation.
type OpenBurstDeps struct {
	Detector   TerminalDetector
	Resolve    spawn.AdapterResolver
	Connector  SessionConnector
	ExePath    spawn.ExecutableResolver
	Env        spawn.WindowEnv
	Ack        spawn.AckChannelFull
	NewBurster func(adapter spawn.Adapter) *spawn.Burster
	Logger     *slog.Logger
	// LocalMint self-connects a mint trigger in the invoking terminal.
	LocalMint func(cmd *cobra.Command, dir string, command []string) error
}

// The shared seam bundle, which binds the backend and loads terminals.json,
// is built only when a seam it fills is left unset, so a fully-injected caller
// does neither.
func buildOpenBurstDeps(cmd *cobra.Command) (*OpenBurstDeps, error) {
	deps := &OpenBurstDeps{}
	if openBurstDeps != nil {
		*deps = *openBurstDeps
	}

	if deps.missesSharedSeam() {
		b, err := sessionBackend()
		if err != nil {
			return nil, err
		}
		fillOpenBurstSeams(deps, buildProductionSpawnSeams(cmd.Context(), b, stateRunners{}))
	}
	if deps.Connector == nil {
		deps.Connector = connectorFunc(func(name string) error {
			return openSessionFunc(cmd, name)
		})
	}
	if deps.NewBurster == nil {
		// Reads the defaulted seams at burst time rather than here.
		deps.NewBurster = func(adapter spawn.Adapter) *spawn.Burster {
			return spawn.NewBurster(adapter, deps.Ack, deps.ExePath, deps.Env)
		}
	}
	if deps.LocalMint == nil {
		deps.LocalMint = func(c *cobra.Command, dir string, command []string) error {
			return openPathFunc(c, dir, command)
		}
	}
	return deps, nil
}

// missesSharedSeam reports whether a seam the shared bundle fills is unset.
func (d *OpenBurstDeps) missesSharedSeam() bool {
	return d.Detector == nil || d.Resolve == nil || d.ExePath == nil || d.Env == nil || d.Ack == nil || d.Logger == nil
}

// fillOpenBurstSeams fills each of deps' bundle-backed seams left unset.
func fillOpenBurstSeams(deps *OpenBurstDeps, seams productionSpawnSeams) {
	if deps.Detector == nil {
		deps.Detector = seams.Detector
	}
	if deps.Resolve == nil {
		deps.Resolve = seams.Resolve
	}
	if deps.ExePath == nil {
		deps.ExePath = seams.Exe
	}
	if deps.Env == nil {
		deps.Env = seams.Env
	}
	if deps.Ack == nil {
		deps.Ack = seams.Ack
	}
	if deps.Logger == nil {
		deps.Logger = seams.Logger
	}
}

// runOpenBurstWithDeps requires len(surfaces) >= 2. The trigger (first in
// command-line order) absorbs the invoking terminal and self-connects LAST,
// after the N−1 external windows: its connector holds the terminal until the
// attach ends, or exec-replaces the Portal process, so connecting first would
// open only one surface. Duplicates in the surface set are honoured, never
// deduped.
func runOpenBurstWithDeps(cmd *cobra.Command, surfaces []spawn.Surface, command []string, deps *OpenBurstDeps) error {
	// A command rides mint windows only, so a set of pure attaches has nowhere to
	// run it. Refused before detect/resolve/spawn.
	if len(command) > 0 && !hasMintSurface(surfaces) {
		return NewUsageError(commandAttachOnlyMessage)
	}

	trigger, external := spawn.SplitTriggerFirst(surfaces)

	id := deps.Detector.Detect()
	adapter, resolution := deps.Resolve(id)

	// Atomic no-op: with no adapter the N−1 external windows cannot open, so the
	// trigger must not half-connect either — a "trigger only" open would violate
	// the all-surfaces intent.
	if resolution == spawn.ResolutionUnsupported {
		spawn.LogUnsupported(deps.Logger, id)
		return errors.New(spawn.UnsupportedNoopMessage(id))
	}

	batch, results, err := deps.NewBurster(adapter).Run(context.Background(), external, command, nil)
	if err != nil {
		return err
	}

	// Cleaned before the trigger handoff, which may never return. Best-effort:
	// an ack left behind goes once stale, with a later burst's Clean.
	_ = deps.Ack.Clean(batch)

	// The trigger's self-connect below is independent of these outcomes: its target
	// is unrelated to the externals', so reporting must not gate it. This diverges
	// from the picker's burst, which skips its self-attach on any failed batch.
	confirmed, failed := spawn.PartitionResults(results)
	total := len(surfaces)
	if perm, ok := spawn.FirstPermission(results); ok {
		// A permission wall stops the burst — every later window hits the identical
		// wall — and takes the no-batch-summary arm, surfacing guidance once.
		spawn.LogWindowResults(deps.Logger, results)
		spawn.LogPermission(deps.Logger, id, resolution, perm.Result.Detail)
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), perm.Result.Guidance)
	} else if len(failed) > 0 {
		// Leave-what-opened: confirmed windows stay, failed surfaces are neither
		// retried nor torn down.
		spawn.LogBatchSummary(deps.Logger, id, resolution, results, total, true, batch)
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), spawn.PartialFailureMessage(failed, len(confirmed) > 0))
	} else {
		spawn.LogBatchSummary(deps.Logger, id, resolution, results, total, true, batch)
	}

	// The summary above optimistically counted the trigger's self-attach: it must
	// be emitted before this connect, which returns only once the attach ends,
	// if ever. The corrective WARN on the failure path stops portal.log claiming
	// an `opened N/N` that never landed; an exit status is an attach that landed
	// and has since ended.
	err = connectTrigger(cmd, trigger, command, deps)
	if _, ended := errors.AsType[*ExitCodeError](err); err != nil && !ended {
		spawn.LogTriggerConnectFailed(deps.Logger, trigger.Value, err.Error())
	}
	return err
}

func hasMintSurface(surfaces []spawn.Surface) bool {
	for _, s := range surfaces {
		if s.Kind == spawn.SurfaceMint {
			return true
		}
	}
	return false
}

// connectTrigger connects the invoking terminal, switching it inside a
// session as any open does; the N−1 external windows always run the spawned
// argv, outside any session.
func connectTrigger(cmd *cobra.Command, trigger spawn.Surface, command []string, deps *OpenBurstDeps) error {
	if trigger.Kind == spawn.SurfaceMint {
		return deps.LocalMint(cmd, trigger.Value, command)
	}
	return deps.Connector.Connect(trigger.Value)
}
