// Package localsocket listens on endpoints that only the user can reach, and finds out which process is on the other
// end of a connection: its PID, executable and parent processes. The endpoints are Unix sockets in ~/.hisar, and on
// Windows named pipes whose DACL admits only the user, with both ends checking that the other runs as the same user.
package localsocket
