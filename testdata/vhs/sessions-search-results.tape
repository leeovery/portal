# vhs tape: the sessions list a search form opened, with the directory column.
#
# SCAFFOLDING, NOT AN ASSET. This tape and the PNG it renders exist for the
# visual gate on the directory column and are CLEARED OUT at the feature's
# sign-off. The Go fixture definition in internal/capture is PERMANENT and is
# not covered by that clearance — the swap-and-diff guard drives it.
#
# 1200x700 at FontSize 16 is 105 columns by 27 rows.
#
# WHAT TO LOOK AT: the directory beside each session name — one space after the
# name, in the same muted rung as the window count (one step brighter on the
# selected row); "~/code/portal" abbreviated, "/opt/portal-tools" left absolute,
# "api-work" present on the strength of its directory alone, "legacy-port-shim"
# showing an empty slot, and the deepest path left-truncated so its tail
# survives.
Output "testdata/vhs/.gifcache/sessions-search-results.gif"
Set FontFamily "JetBrains Mono"
Set FontSize 16
Set Width 1200
Set Height 700
Set Shell "bash"
Type "go run ./cmd/capturetool --fixture sessions-search-results --theme tokyo-night"
Enter
Sleep 12s
Screenshot "testdata/vhs/sessions-search-results.png"
