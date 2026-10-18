// Package commandline serves the hisar CLI on ~/.hisar/cli.sock and installs the bundled hisar on the PATH.
package clisocket

import (
	"net"

	"github.com/tryhisar/hisar/core/localsocket"
)

type Peer struct {
	UID int
	PID int
}

// TerminalSession identifies where a request comes from; the leader's start time tells a reused session ID apart.
type TerminalSession struct {
	SessionID       int
	TerminalDevice  int64
	LeaderStartTime int64
	TerminalName    string
}

type sessionKey struct {
	sessionID       int
	terminalDevice  int64
	leaderStartTime int64
}

func (session TerminalSession) getKey() sessionKey {
	return sessionKey{sessionID: session.SessionID, terminalDevice: session.TerminalDevice, leaderStartTime: session.LeaderStartTime}
}

// hasTerminal: readProcessSession gives a process without a controlling terminal the terminal device 0.
func (session TerminalSession) hasTerminal() bool {
	return session.TerminalDevice != 0
}

type PeerResolver interface {
	ResolvePeer(connection net.Conn) (Peer, error)
	GetTerminalSession(pid int) (TerminalSession, error)
	GetProcessPath(pid int) (string, error)
	GetParentPID(pid int) (int, error)
}

type SystemPeerResolver struct{}

// ResolvePeer reads the peer's credentials: the socket's on macOS and Linux, the pipe client's on Windows.
func (SystemPeerResolver) ResolvePeer(connection net.Conn) (Peer, error) {
	return resolvePeer(connection)
}

func (SystemPeerResolver) GetTerminalSession(pid int) (TerminalSession, error) {
	session, err := readProcessSession(pid)
	if err != nil {
		return TerminalSession{}, err
	}
	session.TerminalName = findTerminalName(session.TerminalDevice)
	return session, nil
}

func (SystemPeerResolver) GetProcessPath(pid int) (string, error) {
	return localsocket.GetProcessPath(pid)
}

func (SystemPeerResolver) GetParentPID(pid int) (int, error) {
	return localsocket.GetParentPID(pid)
}
