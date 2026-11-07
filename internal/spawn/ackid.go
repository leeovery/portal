package spawn

import (
	"fmt"
	"strings"

	"github.com/leeovery/portal/internal/nanoid"
)

// NewSpawnID errors on a value that is no id (nanoid.IsIDShaped) rather than
// let it reach an ack's path.
func NewSpawnID(gen func() (string, error)) (string, error) {
	id, err := gen()
	if err != nil {
		return "", fmt.Errorf("spawn: generate ack id: %w", err)
	}
	if !nanoid.IsIDShaped(id) {
		return "", fmt.Errorf("spawn: generated ack id %q is not path-safe", id)
	}
	return id, nil
}

// ackFileName joins batch and token with a hyphen: valid ids are
// hyphen-free, so the delimiter is unambiguous.
func ackFileName(batch, token string) (string, error) {
	if !nanoid.IsIDShaped(batch) || !nanoid.IsIDShaped(token) {
		return "", fmt.Errorf("spawn: ack %q:%q is no burst's", batch, token)
	}
	return batch + "-" + token, nil
}

// parseAckFileName is the inverse of ackFileName.
func parseAckFileName(name string) (batch, token string, ok bool) {
	b, t, found := strings.Cut(name, "-")
	if !found || !nanoid.IsIDShaped(b) || !nanoid.IsIDShaped(t) {
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
	if !found || !nanoid.IsIDShaped(b) || !nanoid.IsIDShaped(t) {
		return "", "", false
	}
	return b, t, true
}
