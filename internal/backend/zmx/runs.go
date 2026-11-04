package zmx

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Runs reports whether the Backend's zmx binary runs: zmx version exits 0,
// bound on the Backend's directory, within the ceiling Create's run of the
// binary has. A failure carries what zmx printed. It judges no protocol: zmx's
// version string says nothing of the one its daemons speak.
func (b *Backend) Runs(ctx context.Context) error {
	ctx, cancel := b.bound(ctx, b.limits.create, "")
	defer cancel()
	cmd := exec.CommandContext(ctx, b.binary, "version")
	cmd.Env = b.env(envSession, envPrefix, envTmux, envTmuxPane)
	cmd.WaitDelay = pipeGrace
	out, err := cmd.CombinedOutput()
	switch {
	case err == nil:
		return nil
	case ctx.Err() != nil:
		return fmt.Errorf("zmx version: %w", context.Cause(ctx))
	}
	if msg := strings.TrimSpace(string(out)); msg != "" {
		return fmt.Errorf("zmx version: %s: %w", msg, err)
	}
	return fmt.Errorf("zmx version: %w", err)
}
