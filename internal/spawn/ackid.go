package spawn

import (
	"fmt"
	"strings"
)

// SpawnMarkerPrefix is the tmux server-option name prefix used to key each
// spawned window's token-ack confirmation. It is deliberately distinct from
// internal/state's SkeletonMarkerPrefix ("@portal-skeleton-") so the two
// server-option enumerators are blind to each other's markers.
const SpawnMarkerPrefix = "@portal-spawn-"

// spawnIDAlphabet is the option-name-safe charset for ack ids — identical to
// the session package's nanoid alphabet: no ".", no ":", no space, and
// crucially no "-". The absence of "-" is load-bearing: it makes the
// "<batch>-<token>" marker name split on its single hyphen delimiter
// unambiguous (see SpawnMarkerName / ParseSpawnMarkerName).
const spawnIDAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// NewSpawnID produces an option-name-safe ack id from gen. It exists so batch
// and per-window token ids are drawn from one vocabulary and are never the
// renameable session name (which can carry set-option-invalid characters).
//
// A generator error is wrapped and propagated — it never collapses to an empty
// id. On success the result is defensively verified to be non-empty and wholly
// within spawnIDAlphabet; a non-option-safe value yields an error and an empty
// id rather than a marker name that set-option would reject. Production callers
// pass session.NewNanoIDGenerator(); batch and each per-window token are
// independent NewSpawnID calls (independent → collision-resistant).
func NewSpawnID(gen func() (string, error)) (string, error) {
	id, err := gen()
	if err != nil {
		return "", fmt.Errorf("spawn: generate ack id: %w", err)
	}
	if !isOptionSafeID(id) {
		return "", fmt.Errorf("spawn: generated ack id %q is not option-safe", id)
	}
	return id, nil
}

// isOptionSafeID reports whether s is non-empty and every rune is in
// spawnIDAlphabet.
func isOptionSafeID(s string) bool {
	return s != "" && strings.IndexFunc(s, func(r rune) bool {
		return !strings.ContainsRune(spawnIDAlphabet, r)
	}) < 0
}

// SpawnMarkerName renders the tmux server-option name for a batch's token ack:
// "@portal-spawn-<batch>-<token>". Because batch and token are hyphen-free
// option-safe ids, the single hyphen between them is an unambiguous delimiter.
func SpawnMarkerName(batch, token string) string {
	return SpawnMarkerPrefix + batch + "-" + token
}

// ParseSpawnMarkerName is the inverse of SpawnMarkerName. It returns
// (batch, token, true) for a well-formed name and ("", "", false) for a foreign
// prefix, a missing delimiter, or an empty batch or token. The first hyphen
// after the prefix is the sole delimiter (ids are hyphen-free).
func ParseSpawnMarkerName(name string) (batch, token string, ok bool) {
	rest, found := strings.CutPrefix(name, SpawnMarkerPrefix)
	if !found {
		return "", "", false
	}
	b, t, found := strings.Cut(rest, "-")
	if !found || b == "" || t == "" {
		return "", "", false
	}
	return b, t, true
}

// FormatSpawnAckFlag renders the "<batch>:<token>" value carried by the
// portal attach --spawn-ack flag. The colon is unambiguous because option-safe
// ids are colon-free.
func FormatSpawnAckFlag(batch, token string) string {
	return batch + ":" + token
}

// ParseSpawnAckFlag is the inverse of FormatSpawnAckFlag. It returns
// (batch, token, true) for a well-formed value and ("", "", false) for a
// missing colon or an empty batch or token.
func ParseSpawnAckFlag(value string) (batch, token string, ok bool) {
	b, t, found := strings.Cut(value, ":")
	if !found || b == "" || t == "" {
		return "", "", false
	}
	return b, t, true
}
