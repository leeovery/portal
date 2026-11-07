// Package harnesstest holds the subject-neutral scaffolding a test harness is
// built from.
//
// Two things live here. The stand-in a fatal-on-failure helper is driven by
// when its own failing paths are the subject: TestingT, the subset of
// *testing.T such a helper needs, and Recorder, which records what the helper
// reported instead of failing the test running it. And the waits a test uses to
// let a live system converge: PollUntil for a plain deadline, and AwaitProgress
// for an observation whose verdict must not be decided by wall clock alone.
//
// Both are subject-neutral on purpose. Neither belongs to any one domain, so
// homing them here keeps a package whose subject is bytes, or Go source, or
// zmx from importing an unrelated helper package for them.
//
// It depends on stdlib alone and carries no build tag, so every suite it serves
// runs in the unit lane. Test-only: production code must not import it.
package harnesstest
