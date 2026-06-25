#!/bin/sh
# Demo container entrypoint. Builds a realistic, self-contained portal world:
# git project dirs under ~/work, seeded projects/aliases/prefs, and (by default)
# a populated set of stamped tmux sessions so `x` opens onto a full picker.
#
# Modes:
#   default        warm picker — sessions pre-created here, server already up.
#   DEMO_COLD=1    no sessions/server pre-created; the caller drives a cold boot
#                  (and, with a baked state seed, the restore + loading screen).
#
# With no args it drops into an interactive zsh ready for the demo keystrokes;
# with args it execs them (e.g. `docker run ... portal version`).
set -eu

WORK="$HOME/work"
CFG="$HOME/.config/portal"
SEED="$HOME/seed"
mkdir -p "$WORK" "$CFG"

# --- git identity + no pager (so seeded `git log` scrollback never opens less) ---
git config --global user.email demo@example.com
git config --global user.name "Demo"
git config --global init.defaultBranch main
git config --global core.pager cat
git config --global advice.detachedHead false

# --- realistic git project dirs (idempotent) ---
PROJECTS="portal api-gateway web-dashboard payments-service mobile-app infra-terraform auth-service data-pipeline docs-site notifications"
for p in $PROJECTS; do
  d="$WORK/$p"
  [ -d "$d/.git" ] && continue
  mkdir -p "$d"
  git -C "$d" init -q -b main
  printf '# %s\n' "$p" > "$d/README.md"
  git -C "$d" add -A
  git -C "$d" commit -qm "Initial commit"
done

# --- portal config + demo shell ---
cp "$SEED/projects.json" "$CFG/projects.json"
cp "$SEED/aliases"       "$CFG/aliases"
cp "$SEED/prefs.json"    "$CFG/prefs.json"
cp "$SEED/zshrc"         "$HOME/.zshrc"
cp "$SEED/tmux.conf"     "$HOME/.tmux.conf"

STATE="$CFG/state"
mkdir -p "$STATE"

# Each session is stamped with @portal-dir exactly as portal's create path does,
# so grouping by project / tag resolves correctly. A couple of commands seed a
# little scrollback for the Space preview and the restore capture.
mksession() {  # name dir cmd
  tmux new-session -d -s "$1" -c "$2" 2>/dev/null || return 0
  tmux set-option -t "$1" @portal-dir "$2" 2>/dev/null || true
  [ -n "$3" ] && tmux send-keys -t "$1" "$3" Enter 2>/dev/null || true
  return 0
}

if [ "${DEMO_COLD:-0}" = "1" ]; then
  # Cold-boot / restore demo: lay down a captured sessions.json + scrollback so
  # the next `x` runs a real restore (honest loading screen -> repopulated
  # picker). Crucially we do NOT start a tmux server here — the boot stays cold.
  if [ -d "$HOME/seed-state" ]; then
    cp "$HOME/seed-state/sessions.json" "$STATE/sessions.json" 2>/dev/null || true
    [ -d "$HOME/seed-state/scrollback" ] && cp -a "$HOME/seed-state/scrollback" "$STATE/" 2>/dev/null || true
  fi
else
  # Warm demo: a populated, stamped session set so `x` opens onto a full picker.
  mksession portal-a3f9          "$WORK/portal"           "git status -sb; git log --oneline -6; ls"
  mksession portal-release-2c1d  "$WORK/portal"           "git status -sb"
  mksession api-gateway-7c2e     "$WORK/api-gateway"      "git log --oneline -5"
  mksession web-dashboard-4b1d   "$WORK/web-dashboard"    "ls -la"
  mksession payments-9e22        "$WORK/payments-service" "git log --oneline -5"
  mksession payments-hotfix-2c7a "$WORK/payments-service" "git status -sb"
  mksession mobile-app-5f8b      "$WORK/mobile-app"       "ls"
  mksession infra-tf-1a6c        "$WORK/infra-terraform"  "git log --oneline -5"
  mksession auth-service-8d3e    "$WORK/auth-service"     "git status -sb"
  mksession data-pipeline-6b9f   "$WORK/data-pipeline"    "ls -la"
  mksession docs-site-3e4a       "$WORK/docs-site"        "git log --oneline -5"
  mksession notifications-7a1b   "$WORK/notifications"    "git status -sb"
fi

if [ "$#" -gt 0 ]; then exec "$@"; fi
exec zsh -i
