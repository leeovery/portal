package zmx

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/leeovery/portal/internal/backend"
)

// envTrack names the variables a zmx client sends its session to track for
// it, comma-separated.
const envTrack = "ZMX_TRACK_ENV"

// defaultTracked is what zmx tracks for a client whose environment has no
// ZMX_TRACK_ENV: cfg.zig's tracked_envs, pinned to the zmx build by a
// contract test.
const defaultTracked = "DISPLAY,SSH_AUTH_SOCK,SSH_AGENT_PID,SSH_CONNECTION,WINDOWID,XAUTHORITY,KITTY_LISTEN_ON,KITTY_PID,KITTY_WINDOW_ID"

// Track adds name to the variables zmx tracks for the client the command
// attaches: those its ZMX_TRACK_ENV lists or, with none, zmx's default list,
// as zmx reads them. A ZMX_TRACK_ENV set empty tracks nothing, so then name
// alone.
func (b *Backend) Track(command backend.Command, name, value string) backend.Command {
	tracked, set := lookupSet(command.Env, envTrack)
	if !set {
		tracked = defaultTracked
	}
	if !slices.Contains(trackedNames(tracked), name) {
		if tracked != "" {
			tracked += ","
		}
		tracked += name
	}
	command.Env = setEnv(setEnv(command.Env, envTrack, tracked), name, value)
	return command
}

// trackedNames splits a ZMX_TRACK_ENV list as zmx does, trimming each name.
func trackedNames(list string) []string {
	var names []string
	for name := range strings.SplitSeq(list, ",") {
		if name = strings.Trim(name, " \t\r\n"); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// setEnv is a copy of env with key set to value alone.
func setEnv(env []string, key, value string) []string {
	env = slices.DeleteFunc(slices.Clone(env), func(kv string) bool {
		k, _, _ := strings.Cut(kv, "=")
		return k == key
	})
	return append(env, key+"="+value)
}

// LeaderEnv sends EnvGet, which the daemon answers with the leading client's
// tracked environment, within the probe ceiling.
func (b *Backend) LeaderEnv(ctx context.Context, name string) (backend.ClientEnv, error) {
	ctx, cancel := b.bound(ctx, b.limits.probe, "")
	defer cancel()
	c, s, err := b.open(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("read the leader of session %q: %w", name, err)
	}
	defer func() { _ = c.Close() }()
	if err := c.send(tagEnvGet, nil); err != nil {
		return nil, fmt.Errorf("read the leader of session %q: %w", name, b.noAnswer(ctx, s, err))
	}
	p, err := c.await(tagEnvData)
	if errors.Is(err, errUnsupported) {
		return nil, fmt.Errorf("read the leader of session %q: %w", name, err)
	}
	if err != nil {
		return nil, fmt.Errorf("read the leader of session %q: %w", name, b.noAnswer(ctx, s, err))
	}
	return decodeEnv(p), nil
}
