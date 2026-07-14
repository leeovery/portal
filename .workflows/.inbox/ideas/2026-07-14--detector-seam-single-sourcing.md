# Single-source the Detector spawn seam (the 7th of 7)

The Detector seam is still constructed at two sites across the CLI + picker split: `buildProductionSpawnSeams.Detector` (read by the picker) and `spawnDetector`'s `spawn.NewDetector(tmuxClient(cmd))` (used by the CLI / `--detect`). Both call `spawn.NewDetector(client)`, so they are equivalent today — but a future change to detector construction must touch both. That is the exact drift class Task 10-2 targeted and closed for six of seven seams; the Detector is the remaining one.

Consider having `buildProductionSpawnSeams.Detector` delegate through `spawnDetector` (or a shared `newDetector(client)` helper) so the Detector is single-sourced too. Task 10-2 explicitly left this open ("either delegate ... or source only the other six"), so it is a genuine design choice, not a defect. (Also: on the CLI path `buildProductionSpawnSeams` constructs a Detector that `buildSpawnDeps` discards — negligible cost, a cheap struct build.)

Location: `cmd/spawn.go:292-302` & `:337-339`.

Source: review of restore-host-terminal-windows/restore-host-terminal-windows
