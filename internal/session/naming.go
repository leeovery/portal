// Package session provides session management utilities for Portal.
package session

import (
	"crypto/rand"
	"fmt"
	"strings"
)

const (
	maxRetries = 10
	suffixLen  = 6
)

// NanoIDAlphabet is the single shared option-name-safe charset used both for
// session-name nanoid suffixes and for the spawn package's ack ids
// (internal/spawn/ackid.go). It contains lowercase, uppercase, and digits only:
// no ".", ":", or space, and crucially no "-" — the absence of "-" keeps the
// "<batch>-<token>" spawn-marker split unambiguous. Change it here and both
// consumers observe it in lockstep.
const NanoIDAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// IDGenerator produces a random string suitable for use as a session name suffix.
type IDGenerator func() (string, error)

// ExistsFunc reports whether a tmux session with the given name already exists.
type ExistsFunc func(name string) bool

// SanitiseProjectName replaces characters that are invalid in tmux session names
// (periods and colons) with hyphens.
func SanitiseProjectName(name string) string {
	r := strings.NewReplacer(".", "-", ":", "-")
	return r.Replace(name)
}

// GenerateSessionName produces a unique tmux session name in the format {project}-{nanoid}.
// It sanitises the project name, appends a 6-character random suffix from gen,
// and retries up to 10 times if the generated name collides with an existing session.
func GenerateSessionName(projectName string, gen IDGenerator, exists ExistsFunc) (string, error) {
	sanitised := SanitiseProjectName(projectName)

	for range maxRetries {
		suffix, err := gen()
		if err != nil {
			return "", fmt.Errorf("failed to generate session ID: %w", err)
		}

		candidate := sanitised + "-" + suffix
		if !exists(candidate) {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("failed to generate unique session name after %d attempts", maxRetries)
}

// NewNanoIDGenerator returns an IDGenerator that produces 6-character alphanumeric strings
// using crypto/rand.
func NewNanoIDGenerator() IDGenerator {
	return func() (string, error) {
		bytes := make([]byte, suffixLen)
		if _, err := rand.Read(bytes); err != nil {
			return "", fmt.Errorf("failed to read random bytes: %w", err)
		}
		for i := range bytes {
			bytes[i] = NanoIDAlphabet[int(bytes[i])%len(NanoIDAlphabet)]
		}
		return string(bytes), nil
	}
}
