package spawn

import (
	"fmt"
	"strings"

	"github.com/leeovery/portal/internal/nanoid"
)

// maxAckID bounds a batch or token, so neither names an unreasonable file.
const maxAckID = 32

// NewSpawnID errors on a value outside nanoid.Alphabet rather than let it
// reach an ack's path.
func NewSpawnID(gen func() (string, error)) (string, error) {
	id, err := gen()
	if err != nil {
		return "", fmt.Errorf("spawn: generate ack id: %w", err)
	}
	if !validAckID(id) {
		return "", fmt.Errorf("spawn: generated ack id %q is not path-safe", id)
	}
	return id, nil
}

// validAckID reports whether s could have been minted by NewSpawnID, and so
// names a path in the ack directory and no other.
func validAckID(s string) bool {
	return s != "" && len(s) <= maxAckID && strings.IndexFunc(s, func(r rune) bool {
		return !strings.ContainsRune(nanoid.Alphabet, r)
	}) < 0
}

// ackFileName joins batch and token with a hyphen: valid ids are
// hyphen-free, so the delimiter is unambiguous.
func ackFileName(batch, token string) (string, error) {
	if !validAckID(batch) || !validAckID(token) {
		return "", fmt.Errorf("spawn: ack %q:%q is no burst's", batch, token)
	}
	return batch + "-" + token, nil
}

// parseAckFileName is the inverse of ackFileName.
func parseAckFileName(name string) (batch, token string, ok bool) {
	b, t, found := strings.Cut(name, "-")
	if !found || !validAckID(b) || !validAckID(t) {
		return "", "", false
	}
	return b, t, true
}

// FormatSpawnAckFlag renders the open command's --ack value: valid ids are
// colon-free, so the colon is an unambiguous delimiter.
func FormatSpawnAckFlag(batch, token string) string {
	return batch + ":" + token
}

// ParseSpawnAckFlag is the inverse of FormatSpawnAckFlag, refusing a batch or
// token no burst could have minted.
func ParseSpawnAckFlag(value string) (batch, token string, ok bool) {
	b, t, found := strings.Cut(value, ":")
	if !found || !validAckID(b) || !validAckID(t) {
		return "", "", false
	}
	return b, t, true
}
