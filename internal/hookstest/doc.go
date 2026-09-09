// Package hookstest provides the shared scaffolding for staging and
// interrogating hooks.json in tests — the seed hook-key vocabulary, the store
// seeders and path resolution, the byte-identity assertion a route that must
// write nothing is held to, and the sidecar lock fixture with the two
// breadcrumb assertions its contract is read through: the degraded read's DEBUG
// line, and the WARN a mutation that could not take the sidecar leaves.
//
// It offers two path-based routes to a hooks.json and no others: StageStore,
// which stages the file to a description and hands back a store over it rather
// than leaving the caller to author the file, its sidecar and its permissions;
// and its path-only siblings HooksPath and SidecarPath, which compose a path
// and stage nothing, for a fixture whose subject is the absence of one of
// those files or its creation by the code under test. SeedHooksJSON is the
// separate env-resolved seeder, for a fixture staging the file a subprocess
// will resolve for itself.
//
// It lives outside _test.go so any package's tests can import it; production
// code must not.
package hookstest
