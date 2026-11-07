//go:build !integration

package spawn

// Every non-integration build runs what an adapter asks: the adapter sandbox
// is the integration build's alone.
func adapterSandbox([]string) error { return nil }
