package clisocket

import (
	"errors"
	"net"
	"os"

	"github.com/tryhisar/hisar/core/localsocket"
)

// errNoTerminalSessions keeps the socket closed to every caller until Windows terminal sessions can be told apart.
var errNoTerminalSessions = errors.New("commandline: terminal sessions are not read on Windows yet")

// resolvePeer refuses a pipe client that runs as another user, so the UID the server compares is always this user's.
func resolvePeer(connection net.Conn) (Peer, error) {
	pid, err := localsocket.GetPipeClientPID(connection)
	if err != nil {
		return Peer{}, err
	}
	return Peer{UID: os.Getuid(), PID: pid}, nil
}

func readProcessSession(int) (TerminalSession, error) {
	return TerminalSession{}, errNoTerminalSessions
}

func findTerminalName(int64) string { return "" }
