//go:build integration

package cmd

// ResumeReplyWindow is how long a waiting session's draw's background question
// is owed its reply.
const ResumeReplyWindow = resumeReplyWindow

// ParkedResumeChain is the shell text the hydrate helper parks a waiting
// session under, for the integration lane's sessions to run as restore leaves
// one.
func ParkedResumeChain(exe, flag, command, session string) string {
	return parkedResumeChain(parkedChain{Exe: exe, Flag: flag}, parkedPayload(command, session))
}
