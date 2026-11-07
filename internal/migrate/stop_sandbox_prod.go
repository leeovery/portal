//go:build !integration

package migrate

// A run stops nowhere in any build but the integration build's: the stop
// sandbox is that build's alone.
func stopAt(string) error { return nil }
