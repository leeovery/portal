package sourceguardtest

import (
	"go/ast"
	"go/build/constraint"
	"slices"
)

// IntegrationTag is the build tag the integration lane is selected by, and the
// one every lane question in this module's guards is asked about. It is named
// here so the guards that ask share the spelling with the ones that report it.
const IntegrationTag = "integration"

// BuildConstraint returns the build constraint the file states, and whether it
// states one. The constraint is the first //go:build (or legacy // +build) line
// preceding the package clause; a line anywhere after it constrains nothing and
// is not read.
//
// A line that announces a constraint but cannot be parsed states none: the file
// carrying it does not build either, so there is no lane for a caller to place
// it in, and answering "no constraint" gives every caller the same reading.
func BuildConstraint(file *ast.File) (constraint.Expr, bool) {
	for _, group := range file.Comments {
		if group.Pos() > file.Package {
			break
		}
		for _, comment := range group.List {
			if !constraint.IsGoBuild(comment.Text) && !constraint.IsPlusBuild(comment.Text) {
				continue
			}
			expr, err := constraint.Parse(comment.Text)
			if err != nil {
				return nil, false
			}
			return expr, true
		}
	}
	return nil, false
}

// SatisfiedWith reports whether expr holds when exactly tags are set — the
// question a guard scoped to one lane asks of a file. expr must be one
// BuildConstraint found.
func SatisfiedWith(expr constraint.Expr, tags ...string) bool {
	return expr.Eval(func(tag string) bool { return slices.Contains(tags, tag) })
}

// SatisfiedWithout reports whether expr holds when tags are unset and every
// other tag is set — the question a guard asks of a file about the lane those
// tags select it out of. expr must be one BuildConstraint found.
func SatisfiedWithout(expr constraint.Expr, tags ...string) bool {
	return expr.Eval(func(tag string) bool { return !slices.Contains(tags, tag) })
}
