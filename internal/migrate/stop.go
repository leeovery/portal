package migrate

// StopEnv names, in an integration build's environment, the point a run
// stops at, as a crash there would stop it, so a test can leave a run cut
// short for a re-run to finish. Production builds read nothing
// (stop_sandbox_prod.go).
const StopEnv = "PORTAL_TEST_MIGRATE_STOP"

// BetweenRenames is the point between the commit's two renames: the tmux
// version's state moved to state-tmux/, the staging dir not yet in its place.
const BetweenRenames = "between-renames"
