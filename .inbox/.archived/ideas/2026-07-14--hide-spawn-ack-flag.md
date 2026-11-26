# Consider marking the internal `--spawn-ack` flag hidden

The `--spawn-ack <batch>:<token>` flag on `portal attach` is described in its help as "internal:" but is not marked hidden, so it appears in `portal attach --help`. Consider `_ = attachCmd.Flags().MarkHidden("spawn-ack")` in `init` to keep the internal spawn carrier out of user-facing help.

Judgment call: recipe / argv authors composing the spawned command manually may actually want it visible, so this is a decision rather than a mechanical fix.

Location: `cmd/attach.go:92-95`. (Report 3-3.)

Source: review of restore-host-terminal-windows/restore-host-terminal-windows
