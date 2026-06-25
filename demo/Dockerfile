# Portal demo image — a self-contained Linux box that runs the real portal
# binary, its tmux server, and its save daemon in full isolation from the host.
#
# Nothing here can reach the host: the container has its own PID namespace, so
# the bootstrap orphan-daemon sweep (`pgrep -fx '^portal state daemon'`) only
# ever sees this container's daemon.
FROM debian:bookworm-slim

# tmux: the session server portal drives. git: project-dir / git-root resolution.
# procps: pgrep/ps for the daemon identity checks. zsh: the demo's login shell.
# locales: a UTF-8 locale so the TUI's box-drawing + glyphs render correctly.
RUN apt-get update && apt-get install -y --no-install-recommends \
        tmux git procps zsh ca-certificates locales \
 && sed -i 's/# en_US.UTF-8 UTF-8/en_US.UTF-8 UTF-8/' /etc/locale.gen \
 && locale-gen \
 && rm -rf /var/lib/apt/lists/*

# COLORTERM=truecolor is load-bearing: without it portal/lipgloss downsamples the
# 24-bit Modern Vivid palette to 256-colour and the canvas/accents render wrong.
ENV LANG=en_US.UTF-8 LC_ALL=en_US.UTF-8 TERM=xterm-256color COLORTERM=truecolor

# The real portal binary (cross-compiled by build.sh).
COPY bin/portal /usr/local/bin/portal

# A non-root user so demo paths read naturally (~/work/<project>).
RUN useradd -m -s /usr/bin/zsh demo
USER demo
WORKDIR /home/demo

COPY --chown=demo:demo entrypoint.sh /home/demo/entrypoint.sh
COPY --chown=demo:demo seed /home/demo/seed

ENTRYPOINT ["/home/demo/entrypoint.sh"]
