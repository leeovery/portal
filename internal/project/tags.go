package project

import (
	"errors"
	"slices"
	"strings"

	"github.com/leeovery/portal/internal/fileutil"
)

// ErrProjectNotFound is returned by AddTag/RemoveTag when no project matches the
// given path. Tags can only be mutated on a known project (the projects edit
// modal lists known projects only), so an unknown path is an addressing error,
// not a no-op — and the store performs no Save.
var ErrProjectNotFound = errors.New("project not found")

// NormaliseTag converts a raw tag value into its canonical form: leading and
// trailing whitespace trimmed. Case and internal whitespace are preserved, so a
// tag is stored and displayed exactly as the user typed it (e.g. "Personal"
// stays "Personal" rather than being folded to "personal"). It returns
// ok==false (with an empty string) for input that is empty or whitespace-only,
// which callers treat as a rejected/no-op tag.
//
// This is the sole canonical-form function for tags. Every tag comparison —
// per-project dedup, the cross-project union that defines which tags exist, and
// By-Tag grouping — MUST call it rather than re-implementing the trim, so the
// stored value, the grouping key, and the displayed heading stay identical
// everywhere. Because case is preserved, matching is case-sensitive: "Work" and
// "work" are distinct tags (and distinct By-Tag groups).
func NormaliseTag(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false
	}
	return trimmed, true
}

// findByPath locates the index of the project whose Path exactly matches path.
// It returns the index and true when found, or (-1, false) when no project
// matches. It is the shared exact-path lookup for the AddTag/RemoveTag tag
// mutations, so both routes treat a missing project identically.
func findByPath(projects []Project, path string) (int, bool) {
	idx := slices.IndexFunc(projects, func(p Project) bool {
		return p.Path == path
	})
	return idx, idx >= 0
}

// AddTag adds rawTag to the tag set of the project matched by exact path, in
// canonical (trimmed + lower-cased) form, deduped. It is a no-op (returns nil,
// no Save) when rawTag is blank/whitespace-only or the project already carries
// the canonical tag. It returns ErrProjectNotFound (without a Save) when no
// project matches path. On a real change it persists via Save and emits one
// breadcrumb: INFO "modify" on success, or WARN "modify" with the wrapped error
// and its error_class on a persist failure. via is "cli" — the sole caller is
// the user-facing projects-edit modal.
func (s *Store) AddTag(path, rawTag string) error {
	projects, err := s.Load()
	if err != nil {
		return err
	}

	idx, ok := findByPath(projects, path)
	if !ok {
		return ErrProjectNotFound
	}

	tag, ok := NormaliseTag(rawTag)
	if !ok {
		// Blank/whitespace-only: rejected no-op, no Save, no breadcrumb.
		return nil
	}

	if slices.Contains(projects[idx].Tags, tag) {
		// Already present (canonical dedup): no-op, no Save, no breadcrumb.
		return nil
	}

	projects[idx].Tags = append(projects[idx].Tags, tag)
	return s.saveTagMutation(projects, projects[idx].Name, path, tag)
}

// RemoveTag removes the canonical form of rawTag from the tag set of the project
// matched by exact path. It is a no-op (returns nil, no Save) when rawTag is
// blank/whitespace-only or the canonical tag is absent. It returns
// ErrProjectNotFound (without a Save) when no project matches path. On a real
// change it persists via Save and emits one breadcrumb: INFO "modify" on
// success, or WARN "modify" with the wrapped error and its error_class on a
// persist failure. via is "cli" — the sole caller is the user-facing
// projects-edit modal.
func (s *Store) RemoveTag(path, rawTag string) error {
	projects, err := s.Load()
	if err != nil {
		return err
	}

	idx, ok := findByPath(projects, path)
	if !ok {
		return ErrProjectNotFound
	}

	tag, ok := NormaliseTag(rawTag)
	if !ok {
		// Blank/whitespace-only: rejected no-op, no Save, no breadcrumb.
		return nil
	}

	before := len(projects[idx].Tags)
	projects[idx].Tags = slices.DeleteFunc(projects[idx].Tags, func(existing string) bool {
		return existing == tag
	})
	if len(projects[idx].Tags) == before {
		// Tag absent: nothing changed, no Save, no breadcrumb.
		return nil
	}

	return s.saveTagMutation(projects, projects[idx].Name, path, tag)
}

// saveTagMutation persists a tag-set change and emits the shared "modify"
// breadcrumb under the projects component. value carries the canonical tag and
// via is fixed to "cli" (the projects-edit modal is the sole origin). It mirrors
// the Upsert/Rename success/failure breadcrumb shape.
func (s *Store) saveTagMutation(projects []Project, name, path, tag string) error {
	if err := s.Save(projects); err != nil {
		logger.Warn("modify", "op", "modify", "project", name, "path", path, "value", tag, "via", "cli",
			"error", err, "error_class", fileutil.ClassifyWriteError(err))
		return err
	}
	logger.Info("modify", "op", "modify", "project", name, "path", path, "value", tag, "via", "cli")
	return nil
}
