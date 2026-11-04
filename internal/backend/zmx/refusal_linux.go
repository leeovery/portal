package zmx

// refusalProvesNoListener is true on Linux, where a connect to a unix socket
// with a full listen backlog fails with EAGAIN, so ECONNREFUSED means no
// process listens.
const refusalProvesNoListener = true
