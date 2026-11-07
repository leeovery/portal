//go:build integration

package cmd

import "github.com/leeovery/portal/internal/backend"

// DoctorSaver is what doctor reads of the saver.
type DoctorSaver = doctorSaver

// ReadOnlySaverHost is the saver host doctor reads b's saver through.
func ReadOnlySaverHost(b backend.Backend, stateDir string) (DoctorSaver, error) {
	return newReadOnlySaverHost(b, stateDir)
}
