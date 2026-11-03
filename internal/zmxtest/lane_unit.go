//go:build !integration

package zmxtest

// integrationLane reports whether this build is the integration lane's, the
// only one the weekly zmx check runs.
const integrationLane = false
