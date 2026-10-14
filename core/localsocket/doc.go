// Package localsocket listens on Unix sockets that only the user can reach, and finds out which process is on the
// other end of a connection: its PID, executable and parent processes.
package localsocket
