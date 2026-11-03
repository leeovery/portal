// Package nanoid holds the vocabulary of Portal's generated ids: the charset,
// the widths, the generators that mint them and the predicates that recognise
// an id and a pane token. It depends on the standard library alone, so packages that
// must not import each other — the hooks store and the session tree among
// them — can each reach the vocabulary without an edge between them.
package nanoid

import (
	"crypto/rand"
	"fmt"
	"strings"
)

// Alphabet is the charset of generated ids, each safe in a file name, a flag
// and an environment value. The absence of "-" and ":" is load-bearing: it
// keeps a spawn ack's "<batch>-<token>" file name and its
// "--ack <batch>:<token>" flag unambiguous.
const Alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// width is the byte length of a general-purpose generated id — a session-name
// suffix or a spawn batch id. Nothing persisted is classified by it, so it is
// free to move: pane tokens carry their own width.
const width = 6

// Generator mints one id per call, erroring only when the system entropy source
// fails.
type Generator func() (string, error)

// NewGenerator returns a Generator minting general-purpose ids at width.
func NewGenerator() Generator {
	return generatorOfWidth(width)
}

// NewPaneTokenGenerator returns a Generator minting pane tokens at
// paneTokenWidth, the width IsTokenShaped recognises.
func NewPaneTokenGenerator() Generator {
	return generatorOfWidth(paneTokenWidth)
}

func generatorOfWidth(n int) Generator {
	return func() (string, error) {
		bytes := make([]byte, n)
		if _, err := rand.Read(bytes); err != nil {
			return "", fmt.Errorf("failed to read random bytes: %w", err)
		}
		for i := range bytes {
			bytes[i] = Alphabet[int(bytes[i])%len(Alphabet)]
		}
		return string(bytes), nil
	}
}

// paneTokenWidth is the byte length of a pane token, read by both the mint and
// IsTokenShaped so generation and recognition cannot drift apart.
//
// It is part of hooks.json's on-disk contract: changing it reclassifies every
// key already persisted there, which the staleness rule then declines to judge
// and retains forever. Moving it is a migration event, not a tuning change.
const paneTokenWidth = 6

// IsTokenShaped reports whether s could have been produced by the pane-token
// mint: an id exactly paneTokenWidth bytes wide.
func IsTokenShaped(s string) bool {
	return len(s) == paneTokenWidth && IsIDShaped(s)
}

// maxIDWidth bounds an id read back from outside Portal: wider than any Portal
// mints.
const maxIDWidth = 32

// IsIDShaped reports whether s could be one of Portal's generated ids, at any
// width it mints: 1 to maxIDWidth bytes, each drawn from Alphabet. An id read
// back from a file name, a flag or another process's environment that passes
// names one path in its directory and no other.
//
// Length and membership are both counted in bytes, so no multi-byte input can
// satisfy the pair.
func IsIDShaped(s string) bool {
	if s == "" || len(s) > maxIDWidth {
		return false
	}
	for i := range len(s) {
		if strings.IndexByte(Alphabet, s[i]) < 0 {
			return false
		}
	}
	return true
}
