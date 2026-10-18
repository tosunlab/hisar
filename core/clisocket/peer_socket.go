//go:build !windows

package clisocket

import (
	"errors"
	"net"
	"syscall"
)

func resolvePeer(connection net.Conn) (Peer, error) {
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
