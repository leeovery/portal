package zmx

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// KillByCLI ends the session name with zmx's own kill, run in the Backend's
// directory within the kill ceiling. zmx speaks whatever protocol its own
// daemons do, so it ends a session of a zmx Portal can't speak to, and it,
// not Portal, signals the session's processes. It refuses a name Create
// refuses, as zmx kill would read some of those as its flags or as a prefix
// of many names.
func (b *Backend) KillByCLI(ctx context.Context, name string) error {
	if err := b.ValidateName(name); err != nil {
		return fmt.Errorf("zmx kill %q: %w", name, err)
	}
	ctx, cancel := b.bound(ctx, b.limits.kill, "; the kill may still land")
	defer cancel()
	cmd := exec.CommandContext(ctx, b.binary, "kill", name)
	cmd.Env = b.env(envSession, envPrefix)
	cmd.WaitDelay = pipeGrace
	out, err := cmd.CombinedOutput()
	switch {
	case err == nil:
		return b.killed(name, out)
	case ctx.Err() != nil:
		return fmt.Errorf("zmx kill %q: %w", name, context.Cause(ctx))
	}
	if msg := strings.TrimSpace(string(out)); msg != "" {
		return fmt.Errorf("zmx kill %q: %s: %w", name, msg, err)
	}
	return fmt.Errorf("zmx kill %q: %w", name, err)
}

// killed confirms a zmx kill that exited 0 took name's socket with it. zmx
// kill exits 0 though a session's kill fails, printing why, and a kill that
// lands waits for the daemon to hang up, which unlinks the socket first.
func (b *Backend) killed(name string, out []byte) error {
	_, held, err := b.held(name)
	switch {
	case err != nil:
		return fmt.Errorf("zmx kill %q: look for its socket after the kill: %w", name, err)
	case held:
		return fmt.Errorf("zmx kill %q exited 0, its socket still there: %s", name, strings.TrimSpace(string(out)))
	}
	return nil
}
