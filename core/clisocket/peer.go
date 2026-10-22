// Package commandline serves the hisar CLI on ~/.hisar/cli.sock and installs the bundled hisar on the PATH.
package clisocket

import (
	"errors"
	"net"
	"syscall"

	"github.com/tryhisar/hisar/core/agentharness"
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
	// GetAgentSession finds the agent harness a process runs inside; an error means its chain is unknown.
	GetAgentSession(pid int) (agentharness.Session, bool, error)
	GetProcessPath(pid int) (string, error)
	GetParentPID(pid int) (int, error)
}

type SystemPeerResolver struct{}

func (SystemPeerResolver) ResolvePeer(connection net.Conn) (Peer, error) {
	syscallConnection, isSyscallConnection := connection.(syscall.Conn)
	if !isSyscallConnection {
		return Peer{}, errors.New("commandline: connection has no file descriptor")
	}
	rawConnection, err := syscallConnection.SyscallConn()
	if err != nil {
		return Peer{}, err
	}
	var peer Peer
	var peerError error
	if err := rawConnection.Control(func(descriptor uintptr) {
		peer, peerError = getPeerCredential(int(descriptor))
	}); err != nil {
		return Peer{}, err
	}
	return peer, peerError
}

// GetTerminalSession has nothing to read for PID 0: a peer in another PID namespace, as the terminal is to a Flatpak
// Hisar. That connection has no terminal, so it is approved on its own (getApprovalScope).
func (SystemPeerResolver) GetTerminalSession(pid int) (TerminalSession, error) {
	if pid == 0 {
		return TerminalSession{}, nil
	}
	session, err := readProcessSession(pid)
	if err != nil {
		return TerminalSession{}, err
	}
	session.TerminalName = findTerminalName(session.TerminalDevice)
	return session, nil
}

// GetAgentSession reads the chain with sysctl or /proc only, so it checks no code signature.
func (SystemPeerResolver) GetAgentSession(pid int) (agentharness.Session, bool, error) {
	return agentharness.Find(agentharness.ReadChain(pid, agentharness.SystemReader()))
}

func (SystemPeerResolver) GetProcessPath(pid int) (string, error) {
	return localsocket.GetProcessPath(pid)
}

func (SystemPeerResolver) GetParentPID(pid int) (int, error) {
	return localsocket.GetParentPID(pid)
}
