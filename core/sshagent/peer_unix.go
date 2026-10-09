//go:build darwin || linux

package sshagent

import (
	"errors"
	"net"
	"path/filepath"
	"syscall"
)

type SystemPeerResolver struct{}

func GetProcessPath(pid int) (string, error) {
	return getExecutablePath(pid)
}

func (SystemPeerResolver) ResolvePeer(connection net.Conn) (PeerInfo, error) {
	syscallConnection, ok := connection.(syscall.Conn)
	if !ok {
		return PeerInfo{}, errors.New("sshagent: connection has no file descriptor")
	}
	rawConnection, err := syscallConnection.SyscallConn()
	if err != nil {
		return PeerInfo{}, err
	}
	var pid int
	var pidError error
	if err := rawConnection.Control(func(descriptor uintptr) {
		pid, pidError = getPeerPID(int(descriptor))
	}); err != nil {
		return PeerInfo{}, err
	}
	if pidError != nil {
		return PeerInfo{}, pidError
	}
	path, err := getExecutablePath(pid)
	if err != nil {
		return PeerInfo{PID: pid, Name: UnknownPeerName}, nil
	}
	return PeerInfo{PID: pid, Path: path, Name: filepath.Base(path)}, nil
}
