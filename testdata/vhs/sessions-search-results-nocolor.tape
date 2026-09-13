# vhs tape: the search-opened sessions list under the NO_COLOR carve-out.
#
# SCAFFOLDING, NOT AN ASSET. This tape and the PNG it renders exist for the
# visual gate on the directory column and are CLEARED OUT at the feature's
# sign-off. The Go fixture definition in internal/capture is PERMANENT and is
# not covered by that clearance — the swap-and-diff guard drives it.
#
# 1200x700 at FontSize 16 is 105 columns by 27 rows.
#
# $HOME is pinned on the RUN, not on the build: the fixture data lives under
# /home/user, which AbbreviateHome only folds for a process whose home is that
# directory, and `go run` under a pinned $HOME cannot initialise its build cache
# (GOCACHE/GOPATH resolve from $HOME). That split is why there are two lines.
#
# The colourless row is driven by the environment rather than by a fixture flag:
# a flagged fixture is excluded from the swap-and-diff guard, and the harness
# already honours NO_COLOR at render time.
#
# WHAT TO LOOK AT: the directory is separated from the name by the same single
# space and nothing else — no bracket, glyph or separator standing in for the
# weight colour carried.
Output "testdata/vhs/.gifcache/sessions-search-results-nocolor.gif"
Set FontFamily "JetBrains Mono"
Set FontSize 16
Set Width 1200
Set Height 700
Set Shell "bash"
Type "go build -o /tmp/portal-capturetool ./cmd/capturetool"
Enter
Sleep 8s
Type "HOME=/home/user NO_COLOR=1 /tmp/portal-capturetool --fixture sessions-search-results --theme tokyo-night"
Enter
Sleep 4s
Screenshot "testdata/vhs/sessions-search-results-nocolor.png"
