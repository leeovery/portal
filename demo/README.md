# Portal demo harness

Records the animated clips used in the project README, by driving the **real
released `portal` binary** through a realistic, fully-sandboxed world.

The clips: `portal-cold` (cold-boot resurrection — loading screen, restore,
attach with replayed scrollback) and `portal-tour` (the feature tour — grouping,
fuzzy filter, scrollback preview, attach).

## Why a container (the safety model)

Portal's bootstrap runs an **orphan-daemon sweep**: `pgrep -fx '^portal state
daemon( |$)'` across the whole host, SIGKILL-ing every match that isn't the
current tmux server's `_portal-saver` pane. Run on a separate socket on the
host, that sweep would kill your **real** daemon.

So the entire demo runs inside a Linux container (OrbStack/Docker). The
container has its own **PID namespace**, so the in-container `pgrep` can only
ever see the container's own daemon — the host's tmux server, config, and
daemon are structurally unreachable. VHS records on the host (native fonts); only
the interactive terminal crosses the boundary via `docker run -it`.

## Workflow

```bash
./build.sh                 # cross-compile linux/arm64 portal, build the image,
                           # capture a restore seed, bake the final image
vhs portal-cold.tape       # render the colour-accurate master -> out/portal-cold.mp4
./finalize.sh portal-cold  # grade it -> out/portal-cold-vivid.{mp4,gif}
# repeat for portal-tour; then copy the vivid mp4s into ../art/ and commit
```

`bin/`, `out/`, and `.seed-state/` are build artifacts (gitignored).

## Pieces

| File | Role |
|------|------|
| `build.sh` | Two-stage build: `portal-demo:base` (binary + tmux + seed) → record seed → `portal-demo:latest` (base + baked restore seed). |
| `Dockerfile` / `Dockerfile.cold` | Base image / final image that bakes `.seed-state`. |
| `entrypoint.sh` | Seeds 12 git projects + `projects.json`/aliases/prefs/zsh/tmux. **Warm** (default): pre-creates 12 stamped tmux sessions. **`DEMO_COLD=1`**: lays down the restore seed and starts no server, so `x` runs a real cold-boot restore. |
| `seed/` | `projects.json`, `aliases`, `prefs.json`, `zshrc`, `tmux.conf` (a slim dark status bar matching the Modern Vivid canvas). |
| `record-seed.sh` | Boots the warm image, runs the daemon briefly so it dumps `sessions.json` + per-pane scrollback, snapshots them to `.seed-state/`. |
| `*.tape` | VHS scripts (host-side). |
| `finalize.sh` | Grades a master mp4 with `eq=saturation=1.55:contrast=1.08` and emits the vivid mp4 + gif. |

## Two notes

- **Colour grade.** VHS reproduces portal's exact sRGB palette (verified
  pixel-for-pixel). It reads muted next to a wide-gamut terminal (Ghostty on a
  P3 Mac renders the same hex values more vividly, un-colour-managed), so
  `finalize.sh` bakes a saturation/contrast boost to match that on-screen pop.
- **Cold boot lands on Projects.** After a cold-boot restore the picker opens on
  the Projects page, not the just-restored Sessions, so `portal-cold.tape`
  presses `x` once to reveal them. Logged as a real bug in the inbox — not a
  demo artifact.
